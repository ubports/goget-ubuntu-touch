//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
	"launchpad.net/goget-ubuntu-touch/diskimage"
	"launchpad.net/goget-ubuntu-touch/sysutils"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
	"launchpad.net/snappy/helpers"
	"launchpad.net/snappy/progress"
	"launchpad.net/snappy/provisioning"
	"launchpad.net/snappy/release"
	"launchpad.net/snappy/snappy"
)

type imageFlavor string

const (
	minSizePersonal = 8
	minSizeCore     = 4
)

const (
	flavorPersonal imageFlavor = "personal"
	flavorCore     imageFlavor = "core"
)

func (f imageFlavor) Channel() string {
	return fmt.Sprintf("ubuntu-%s", f)
}

func (f imageFlavor) minSize() int64 {
	switch f {
	case flavorPersonal:
		return minSizePersonal
	case flavorCore:
		return minSizeCore
	default:
		panic("invalid flavor")
	}
}

type Snapper struct {
	Channel string `long:"channel" description:"Specify the channel to use" default:"stable"`
	Output  string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Size    int64  `long:"size" short:"s" description:"Size of image file to create in GB (min 4)" default:"4"`
	Oem     string `long:"oem" description:"The snappy oem package to base the image out of" default:"generic-amd64"`

	Development struct {
		Install       []string `long:"install" description:"Install additional packages (can be called multiple times)"`
		DevicePart    string   `long:"device-part" description:"Specify a local device part to override the one from the server"`
		DeveloperMode bool     `long:"developer-mode" description:"Finds the latest public key in your ~/.ssh and sets it up using cloud-init"`
	} `group:"Development"`

	Positional struct {
		Release string `positional-arg-name:"release" description:"The release to base the image out of (15.04 or rolling)" required:"true"`
	} `positional-args:"yes" required:"yes"`

	img             diskimage.CoreImage
	hardware        diskimage.HardwareDescription
	oem             diskimage.OemDescription
	stagingRootPath string

	flavor imageFlavor
	device string

	customizationFunc []func() error
}

func (s Snapper) sanityCheck() error {
	// we don't want to overwrite the output, people might get angry :-)
	if helpers.FileExists(s.Output) {
		return fmt.Errorf("Giving up, the desired target output file %#v already exists", s.Output)
	}

	if s.Size < s.flavor.minSize() {
		return fmt.Errorf("minimum size for %s is %d", s.flavor, s.flavor.minSize())
	}

	if syscall.Getuid() != 0 {
		return errors.New("command requires sudo/pkexec (root)")
	}

	return nil
}

func (s *Snapper) systemImage() (*ubuntuimage.Image, error) {
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return nil, err
	}

	channel := systemImageChannel(s.flavor.Channel(), s.Positional.Release, s.Channel)
	// TODO: remove once azure channel is gone
	if s.device == "" {
		s.device = systemImageDeviceChannel(s.oem.Architecture())
	}

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, channel, s.device)
	if err != nil {
		return nil, err
	}

	systemImage, err := getImage(deviceChannel)
	if err != nil {
		return nil, err
	}

	// avoid passing more args to setup()
	globalArgs.Revision = systemImage.Version

	return &systemImage, nil
}

func (s *Snapper) installFlags() snappy.InstallFlags {
	flags := snappy.InhibitHooks | snappy.AllowOEM

	if s.Development.DeveloperMode {
		flags |= snappy.AllowUnauthenticated
	}

	return flags
}

func (s *Snapper) install(systemPath string) error {
	snappy.SetRootDir(systemPath)
	defer snappy.SetRootDir("/")

	flags := s.installFlags()
	oemSoftware := s.oem.OEM.Software
	packageCount := len(s.Development.Install) + len(oemSoftware.BuiltIn) + len(oemSoftware.Preinstalled)
	if s.Oem != "" {
		packageCount += 1
	}
	packageQueue := make([]string, 0, packageCount)

	if s.Oem != "" {
		packageQueue = append(packageQueue, s.Oem)
	}
	packageQueue = append(packageQueue, oemSoftware.BuiltIn...)
	packageQueue = append(packageQueue, oemSoftware.Preinstalled...)
	packageQueue = append(packageQueue, s.Development.Install...)

	for _, snap := range packageQueue {
		fmt.Println("Installing", snap)

		pb := progress.NewTextProgress()
		if _, err := snappy.Install(snap, flags, pb); err != nil {
			return err
		}
	}

	return nil
}

func (s *Snapper) extractOem(oemPackage string) error {
	if oemPackage == "" {
		return nil
	}

	tempDir, err := ioutil.TempDir("", "oem")
	if err != nil {
		return err
	}

	// we need to fix the permissions for tempdir to  be seteuid friendly
	if err := os.Chmod(tempDir, 0755); err != nil {
		return err
	}

	s.stagingRootPath = tempDir

	snappy.SetRootDir(tempDir)
	defer snappy.SetRootDir("/")
	release.Override(release.Release{
		Flavor:  string(s.flavor),
		Series:  s.Positional.Release,
		Channel: s.Channel,
	})

	flags := s.installFlags()
	pb := progress.NewTextProgress()
	if _, err := snappy.Install(oemPackage, flags, pb); err != nil {
		return err
	}

	if err := s.loadOem(tempDir); err != nil {
		return err
	}

	return nil
}

func (s *Snapper) loadOem(systemPath string) error {
	pkgs, err := filepath.Glob(filepath.Join(systemPath, "/oem/*/*/meta/package.yaml"))
	if err != nil {
		return err
	}

	// checking for len(pkgs) > 2 due to the 'current' symlink
	if len(pkgs) == 0 {
		return errors.New("no oem package found")
	} else if len(pkgs) > 2 || err != nil {
		return errors.New("too many oem packages installed")
	}

	f, err := ioutil.ReadFile(pkgs[0])
	if err != nil {
		return errors.New("failed to read oem yaml")
	}

	var oem diskimage.OemDescription
	if err := yaml.Unmarshal([]byte(f), &oem); err != nil {
		return errors.New("cannot decode oem yaml")
	}
	s.oem = oem
	s.oem.SetRoot(systemPath)

	return nil
}

// Creates a YAML file inside the image that contains metadata relating
// to the installation.
func (s Snapper) writeInstallYaml(bootMountpoint string) error {
	selfPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}

	bootDir := ""

	switch s.oem.OEM.Hardware.Bootloader {
	// Running systems use a bindmount for /boot/grub, but
	// since the system isn't booted, create the file in the
	// real location.
	case "grub":
		bootDir = "/EFI/ubuntu/grub"
	}

	installYamlFilePath := filepath.Join(bootMountpoint, bootDir, provisioning.InstallYamlFile)

	i := provisioning.InstallYaml{
		InstallMeta: provisioning.InstallMeta{
			Timestamp:         time.Now(),
			InitialVersion:    fmt.Sprintf("%d", globalArgs.Revision),
			SystemImageServer: globalArgs.Server,
		},
		InstallTool: provisioning.InstallTool{
			Name: filepath.Base(selfPath),
			Path: selfPath,
			// FIXME: we don't know our own version yet :)
			// Version: "???",
		},
		InstallOptions: provisioning.InstallOptions{
			Size:          s.Size,
			SizeUnit:      "GB",
			Output:        s.Output,
			Channel:       s.Channel,
			DevicePart:    s.Development.DevicePart,
			Oem:           s.Oem,
			DeveloperMode: s.Development.DeveloperMode,
		},
	}

	data, err := yaml.Marshal(&i)
	if err != nil {
		return err
	}

	// the file isn't supposed to be modified, hence r/o.
	return ioutil.WriteFile(installYamlFilePath, data, 0444)
}

func extractHWDescription(path string) (hw diskimage.HardwareDescription, err error) {
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	if syscall.Getuid() == 0 {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()

		if err := sysutils.DropPrivs(); err != nil {
			return hw, err
		}
	}

	printOut("Searching for hardware.yaml in device part")
	tmpdir, err := ioutil.TempDir("", "hardware")
	if err != nil {
		return hw, errors.New("cannot create tempdir to extract hardware.yaml from device part")
	}
	defer os.RemoveAll(tmpdir)

	if out, err := exec.Command("tar", "xf", path, "-C", tmpdir, "hardware.yaml").CombinedOutput(); err != nil {
		return hw, fmt.Errorf("failed to extract a hardware.yaml from the device part: %s", out)
	}

	data, err := ioutil.ReadFile(filepath.Join(tmpdir, "hardware.yaml"))
	if err != nil {
		return hw, err
	}

	err = yaml.Unmarshal([]byte(data), &hw)

	return hw, err
}

func (s *Snapper) setup(filePathChan <-chan string, fileCount int) error {
	printOut("Mounting...")
	if err := s.img.Mount(); err != nil {
		return err
	}
	defer func() {
		printOut("Unmounting...")
		if err := s.img.Unmount(); err != nil {
			fmt.Println("WARNING: unexpected issue:", err)
		}
	}()

	printOut("Provisioning...")
	for i := 0; i < fileCount; i++ {
		f := <-filePathChan
		if out, err := exec.Command("fakeroot", "tar", "--numeric-owner", "-axvf", f, "-C", s.img.BaseMount()).CombinedOutput(); err != nil {
			printOut(string(out))
			return fmt.Errorf("issues while extracting: %s", out)
		}
	}

	systemPath := s.img.System()

	if err := s.install(systemPath); err != nil {
		return err
	}

	if err := s.img.SetupBoot(); err != nil {
		return err
	}

	for i := range s.customizationFunc {
		if err := s.customizationFunc[i](); err != nil {
			return err
		}
	}

	// if the device is armhf, we can't to make this copy here since it's faster
	// than on the device.
	if s.oem.Architecture() == archArmhf && s.oem.PartitionLayout() == "system-AB" {
		printOut("Replicating system-a into system-b")

		src := fmt.Sprintf("%s/.", systemPath)
		dst := fmt.Sprintf("%s/system-b", s.img.BaseMount())

		cmd := exec.Command("cp", "-r", "--preserve=all", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to replicate image contents: %s", out)
		}
	}

	return s.writeInstallYaml(s.img.Boot())
}

// deploy orchestrates the priviledged part of the setup
func (s *Snapper) deploy(systemImage *ubuntuimage.Image, filePathChan <-chan string) error {
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.EscalatePrivs(); err != nil {
		return err
	}
	defer sysutils.DropPrivs()

	if err := format(s.img); err != nil {
		return err
	}

	if err := s.setup(filePathChan, len(systemImage.Files)); err != nil {
		return err
	}

	return nil
}

func format(img diskimage.Image) error {
	printOut("Mapping...")
	if err := img.Map(); err != nil {
		return fmt.Errorf("issue while mapping partitions: %s", err)
	}
	defer img.Unmap()

	printOut("Formatting...")
	return img.Format()
}

func (s Snapper) printSummary() {
	fmt.Println("New image complete")
	fmt.Println("Summary:")
	fmt.Println(" Output:", s.Output)
	fmt.Println(" Architecture:", s.oem.Architecture())
	fmt.Println(" Channel:", s.Channel)
	fmt.Println(" Version:", globalArgs.Revision)
}

func (s *Snapper) create() error {
	if err := s.sanityCheck(); err != nil {
		return err
	}

	fmt.Println("Determining oem configuration")
	if err := s.extractOem(s.Oem); err != nil {
		return err
	}
	defer os.RemoveAll(s.stagingRootPath)

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}

	var devicePart string
	if s.Development.DevicePart != "" {
		p, err := expandFile(s.Development.DevicePart)
		if err != nil {
			return err
		}

		devicePart = p
	}

	fmt.Println("Fetching information from server...")
	systemImage, err := s.systemImage()
	if err != nil {
		return err
	}

	filesChan := make(chan Files, len(systemImage.Files))
	defer close(filesChan)

	sigFiles := ubuntuimage.GetGPGFiles()
	sigFilesChan := make(chan Files, len(sigFiles))
	defer close(sigFilesChan)

	fmt.Println("Downloading and setting up...")

	for _, f := range sigFiles {
		go bitDownloader(f, sigFilesChan, globalArgs.Server, cacheDir)
	}

	filePathChan := make(chan string, len(systemImage.Files))
	hwChan := make(chan diskimage.HardwareDescription)

	go func() {
		for i := 0; i < len(systemImage.Files); i++ {
			f := <-filesChan

			if isDevicePart(f.FilePath) {
				devicePart = f.FilePath

				if hardware, err := extractHWDescription(f.FilePath); err != nil {
					fmt.Println("Failed to read harware.yaml from device part, provisioning may fail:", err)
				} else {
					hwChan <- hardware
				}
			}

			printOut("Download finished for", f.FilePath)
			filePathChan <- f.FilePath
		}
		close(hwChan)
	}()

	for _, f := range systemImage.Files {
		if devicePart != "" && isDevicePart(f.Path) {
			printOut("Using a custom device tarball")
			filesChan <- Files{FilePath: devicePart}
		} else {
			go bitDownloader(f, filesChan, globalArgs.Server, cacheDir)
		}
	}

	loader := s.oem.OEM.Hardware.Bootloader
	switch loader {
	case "grub":
		s.img = diskimage.NewCoreGrubImage(s.Output, s.Size, s.hardware, s.oem)
	case "u-boot":
		s.img = diskimage.NewCoreUBootImage(s.Output, s.Size, s.hardware, s.oem)
	default:
		return errors.New("no hardware description in OEM snap")
	}

	printOut("Partitioning...")
	if err := s.img.Partition(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(s.Output)
		}
	}()

	// Handle SIGINT and SIGTERM.
	go func() {
		ch := make(chan os.Signal)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

		for sig := range ch {
			printOut("Received", sig, "... ignoring")
		}
	}()

	s.hardware = <-hwChan

	// Execute the following code with escalated privs and drop them when done
	if err := s.deploy(systemImage, filePathChan); err != nil {
		return err
	}

	if err := s.img.FlashExtra(); err != nil {
		return err
	}

	s.printSummary()

	return nil
}
