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
	flavorPersonal imageFlavor = "personal"
	flavorCore     imageFlavor = "core"
)

func (f imageFlavor) Channel() string {
	return fmt.Sprintf("ubuntu-%s", f)
}

type Snapper struct {
	Channel string `long:"channel" description:"Specify the channel to use" default:"stable"`
	Output  string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Size    int64  `long:"size" short:"s" description:"Size of image file to create in GB (min 4)" default:"4"`
	Oem     string `long:"oem" description:"The snappy oem package to base the image out of" default:"generic-amd64"`

	Positional struct {
		Release string `positional-arg-name:"release" description:"The release to base the image out of (15.04 or rolling)" required:"true"`
	} `positional-args:"yes" required:"yes"`

	img             diskimage.CoreImage
	hardware        diskimage.HardwareDescription
	oem             diskimage.OemDescription
	stagingRootPath string

	flavor imageFlavor
}

func (s Snapper) sanityCheck() error {
	// we don't want to overwrite the output, people might get angry :-)
	if helpers.FileExists(s.Output) {
		return fmt.Errorf("Giving up, the desired target output file %#v already exists", s.Output)
	}

	if syscall.Getuid() != 0 {
		return errors.New("command requires sudo/pkexec (root)")
	}

	return nil
}

func (s *Snapper) systemImage(deviceOverride string) (*ubuntuimage.Image, error) {
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return nil, err
	}

	channel := systemImageChannel(s.flavor.Channel(), s.Positional.Release, s.Channel)
	// TODO: remove once azure channel is gone
	var device string
	if deviceOverride != "" {
		fmt.Println("WARNING: this option should only be used to build azure images")
		device = coreCmd.Deprecated.Device
	} else {
		device = systemImageDeviceChannel(coreCmd.oem.Architecture())
	}

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, channel, device)
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

	if coreCmd.Development.DeveloperMode {
		flags |= snappy.AllowUnauthenticated
	}

	return flags
}

func (s *Snapper) install(systemPath string) error {
	snappy.SetRootDir(systemPath)
	defer snappy.SetRootDir("/")

	flags := coreCmd.installFlags()
	oemSoftware := coreCmd.oem.OEM.Software
	packageCount := len(coreCmd.Deprecated.Install) + len(oemSoftware.BuiltIn) + len(oemSoftware.Preinstalled)
	if coreCmd.Oem != "" {
		packageCount += 1
	}
	packageQueue := make([]string, 0, packageCount)

	if coreCmd.Oem != "" {
		packageQueue = append(packageQueue, coreCmd.Oem)
	}
	packageQueue = append(packageQueue, oemSoftware.BuiltIn...)
	packageQueue = append(packageQueue, oemSoftware.Preinstalled...)
	packageQueue = append(packageQueue, coreCmd.Deprecated.Install...)

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

	coreCmd.stagingRootPath = tempDir

	snappy.SetRootDir(tempDir)
	defer snappy.SetRootDir("/")
	release.Override(release.Release{
		Flavor:  string(s.flavor),
		Series:  coreCmd.Positional.Release,
		Channel: coreCmd.Channel,
	})

	flags := coreCmd.installFlags()
	pb := progress.NewTextProgress()
	if _, err := snappy.Install(oemPackage, flags, pb); err != nil {
		return err
	}

	if err := coreCmd.loadOem(tempDir); err != nil {
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

	switch coreCmd.oem.OEM.Hardware.Bootloader {
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
			Size:          coreCmd.Size,
			SizeUnit:      "GB",
			Output:        coreCmd.Output,
			Channel:       coreCmd.Channel,
			DevicePart:    coreCmd.Development.DevicePart,
			Oem:           coreCmd.Oem,
			DeveloperMode: coreCmd.Development.DeveloperMode,
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

func (s *Snapper) setup(img diskimage.CoreImage, filePathChan <-chan string, fileCount int) error {
	printOut("Mounting...")
	if err := img.Mount(); err != nil {
		return err
	}
	defer func() {
		printOut("Unmounting...")
		if err := img.Unmount(); err != nil {
			fmt.Println("WARNING: unexpected issue:", err)
		}
	}()

	printOut("Provisioning...")
	for i := 0; i < fileCount; i++ {
		f := <-filePathChan
		if out, err := exec.Command("fakeroot", "tar", "--numeric-owner", "-axvf", f, "-C", img.BaseMount()).CombinedOutput(); err != nil {
			fmt.Println(string(out))
			return fmt.Errorf("issues while extracting: %s", out)
		}
	}

	writablePath := img.Writable()

	for _, dir := range []string{"system-data", "cache"} {
		dirPath := filepath.Join(writablePath, dir)
		if err := os.Mkdir(dirPath, 0755); err != nil {
			return err
		}
	}

	systemPath := img.System()

	if err := coreCmd.install(systemPath); err != nil {
		return err
	}

	if err := img.SetupBoot(); err != nil {
		return err
	}

	if !coreCmd.Deprecated.Cloud {
		cloudBaseDir := filepath.Join("var", "lib", "cloud")

		if err := os.MkdirAll(filepath.Join(systemPath, cloudBaseDir), 0755); err != nil {
			return err
		}

		if err := coreCmd.setupCloudInit(cloudBaseDir, filepath.Join(writablePath, "system-data")); err != nil {
			return err
		}
	}

	// if the device is armhf, we can't to make this copy here since it's faster
	// than on the device.
	if coreCmd.oem.Architecture() == archArmhf && coreCmd.oem.PartitionLayout() == "system-AB" {
		printOut("Replicating system-a into system-b")

		src := fmt.Sprintf("%s/.", systemPath)
		dst := fmt.Sprintf("%s/system-b", img.BaseMount())

		cmd := exec.Command("cp", "-r", "--preserve=all", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to replicate image contents: %s", out)
		}
	}

	return coreCmd.writeInstallYaml(img.Boot())
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

	if err := s.setup(s.img, filePathChan, len(systemImage.Files)); err != nil {
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
