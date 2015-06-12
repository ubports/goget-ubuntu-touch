//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
	"launchpad.net/snappy/helpers"
	"launchpad.net/snappy/progress"
	"launchpad.net/snappy/release"
	"launchpad.net/snappy/snappy"

	"launchpad.net/goget-ubuntu-touch/diskimage"
	"launchpad.net/goget-ubuntu-touch/sysutils"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License version 3, as published
// by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranties of
// MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
// PURPOSE.  See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program.  If not, see <http://www.gnu.org/licenses/>.

func init() {
	parser.AddCommand("core",
		"Creates ubuntu core images",
		"",
		&coreCmd)
}

type CoreCmd struct {
	Channel string `long:"channel" description:"Specify the channel to use" default:"stable"`
	Output  string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Size    int64  `long:"size" short:"s" description:"Size of image file to create in GB (min 4)" default:"4"`
	Oem     string `long:"oem" description:"The snappy oem package to base the image out of" default:"generic-amd64"`

	Development struct {
		DevicePart    string `long:"device-part" description:"Specify a local device part to override the one from the server"`
		DeveloperMode bool   `long:"developer-mode" description:"Finds the latest public key in your ~/.ssh and sets it up using cloud-init"`
		EnableSsh     bool   `long:"enable-ssh" description:"Enable ssh on the image through cloud-init(not needed with developer mode)"`
	} `group:"Development"`

	Deprecated struct {
		Install []string `long:"install" description:"Install additional packages (can be called multiple times)"`
		Cloud   bool     `long:"cloud" description:"Generate a pure cloud image without setting up cloud-init"`
		Device  string   `long:"device" description:"Specify the device to use"`
	} `group:"Deprecated"`

	Positional struct {
		Release string `positional-arg-name:"release" description:"The release to base the image out of (15.04 or rolling)" required:"true"`
	} `positional-args:"yes" required:"yes"`

	hardware        diskimage.HardwareDescription
	oem             diskimage.OemDescription
	stagingRootPath string
}

var coreCmd CoreCmd

const cloudInitMetaData = `instance-id: nocloud-static
`

const cloudInitUserData = `#cloud-config
password: ubuntu
chpasswd: { expire: False }
ssh_pwauth: True
ssh_genkeytypes: ['rsa', 'dsa', 'ecdsa', 'ed25519']
`

func (coreCmd *CoreCmd) Execute(args []string) error {
	// we don't want to overwrite the output, people might get angry :-)
	if helpers.FileExists(coreCmd.Output) {
		return fmt.Errorf("Giving up, the desired target output file %#v already exists", coreCmd.Output)
	}

	if coreCmd.Development.EnableSsh && coreCmd.Deprecated.Cloud {
		return errors.New("--cloud and --enable-ssh cannot be used together")
	}

	var devicePart string
	if coreCmd.Development.DevicePart != "" {
		p, err := expandFile(coreCmd.Development.DevicePart)
		if err != nil {
			return err
		}

		devicePart = p
	}

	fmt.Println("Determining oem configuration")
	if err := coreCmd.extractOem(coreCmd.Oem); err != nil {
		return err
	}
	defer os.RemoveAll(coreCmd.stagingRootPath)

	if !globalArgs.DownloadOnly {
		if syscall.Getuid() != 0 {
			return errors.New("command requires sudo/pkexec (root)")
		}

		// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		if err := sysutils.DropPrivs(); err != nil {
			return err
		}
	}

	fmt.Println("Fetching information from server...")
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	channel := systemImageChannel("ubuntu-core", coreCmd.Positional.Release, coreCmd.Channel)
	// TODO: remove once azure channel is gone
	var device string
	if coreCmd.Deprecated.Device != "" {
		fmt.Println("WARNING: this option should only be used to build azure images")
		device = coreCmd.Deprecated.Device
	} else {
		device = systemImageDeviceChannel(coreCmd.oem.Architecture())
	}

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, channel, device)
	if err != nil {
		return err
	}

	image, err := getImage(deviceChannel)
	if err != nil {
		return err
	}

	filesChan := make(chan Files, len(image.Files))
	defer close(filesChan)

	sigFiles := ubuntuimage.GetGPGFiles()
	sigFilesChan := make(chan Files, len(sigFiles))
	defer close(sigFilesChan)

	fmt.Println("Downloading and setting up...")

	for _, f := range sigFiles {
		go bitDownloader(f, sigFilesChan, globalArgs.Server, cacheDir)
	}

	filePathChan := make(chan string, len(image.Files))
	hwChan := make(chan diskimage.HardwareDescription)

	go func() {
		for i := 0; i < len(image.Files); i++ {
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

	for _, f := range image.Files {
		if devicePart != "" && isDevicePart(f.Path) {
			printOut("Using a custom device tarball")
			filesChan <- Files{FilePath: devicePart}
		} else {
			go bitDownloader(f, filesChan, globalArgs.Server, cacheDir)
		}
	}

	coreCmd.hardware = <-hwChan

	var img diskimage.CoreImage

	if globalArgs.DownloadOnly {
		workDir, err := os.Getwd()
		if err != nil {
			return err
		}

		downloadedFiles := make([]string, 0, len(image.Files))

		for i := 0; i < len(image.Files); i++ {
			f := <-filePathChan
			baseFile := filepath.Base(f)

			if err := sysutils.CopyFile(f, filepath.Join(workDir, baseFile)); err != nil {
				return err
			}
			downloadedFiles = append(downloadedFiles, baseFile)
		}

		fmt.Println("Files downloaded to current directory: ")
		for _, f := range downloadedFiles {
			fmt.Println(" -", f)
		}
		fmt.Println()

		return nil
	}

	loader := coreCmd.oem.OEM.Hardware.Bootloader
	switch loader {
	case "grub":
		img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	case "u-boot":
		img = diskimage.NewCoreUBootImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	default:
		fmt.Printf("Bootloader set to '%s' in oem hardware description, assuming grub as a fallback\n", loader)
		img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	}

	printOut("Partitioning...")
	if err := img.Partition(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			//os.Remove(coreCmd.Output)
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

	// Execute the following code with escalated privs and drop them when done
	err = func() error {
		// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		if err := sysutils.EscalatePrivs(); err != nil {
			return err
		}
		defer sysutils.DropPrivs()

		if err := format(img); err != nil {
			return err
		}

		if err := coreCmd.setup(img, filePathChan, len(image.Files)); err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return err
	}

	oemRootPath, err := coreCmd.oem.InstallPath(coreCmd.stagingRootPath)
	if coreCmd.Oem != "" && err != nil {
		return err
	}

	if err := img.FlashExtra(oemRootPath, devicePart); err != nil {
		return err
	}

	fmt.Println("New image complete")
	fmt.Println("Summary:")
	fmt.Println(" Output:", coreCmd.Output)
	fmt.Println(" Architecture:", coreCmd.oem.Architecture())
	fmt.Println(" Channel:", coreCmd.Channel)
	fmt.Println(" Version:", image.Version)

	if coreCmd.oem.Architecture() != "armhf" {
		fmt.Println("Launch by running: kvm -m 768", coreCmd.Output)
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

func (coreCmd *CoreCmd) setup(img diskimage.CoreImage, filePathChan <-chan string, fileCount int) error {
	printOut("Mapping...")
	if err := img.Map(); err != nil {
		return err
	}
	var unmountErr error
	defer func() {
		printOut("Unmapping...")
		if unmountErr == nil {
			if err := img.Unmap(); err != nil {
				fmt.Println("WARNING: unexpected issue while unmounting:", err)
			}
		} else {
			fmt.Println("WARNING: Unmounting failed, leaving partitions mapped.")
		}
	}()

	printOut("Mounting...")
	if err := img.Mount(); err != nil {
		return err
	}
	defer func() {
		printOut("Unmounting...")
		if unmountErr = img.Unmount(); unmountErr != nil {
			fmt.Println("WARNING: unexpected issue:", unmountErr)
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

	oemRootPath, err := coreCmd.oem.InstallPath(coreCmd.stagingRootPath)
	if coreCmd.Oem != "" && err != nil {
		return err
	}

	if err := img.SetupBoot(oemRootPath); err != nil {
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

	return nil
}

func (coreCmd *CoreCmd) install(systemPath string) error {
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

func (coreCmd *CoreCmd) setupCloudInit(cloudBaseDir, systemData string) error {
	// create a basic cloud-init seed
	cloudDir := filepath.Join(systemData, cloudBaseDir, "seed", "nocloud-net")
	if err := os.MkdirAll(cloudDir, 0755); err != nil {
		return err
	}

	metaDataFile, err := os.Create(filepath.Join(cloudDir, "meta-data"))
	if err != nil {
		return err
	}
	defer metaDataFile.Close()

	if _, err := io.WriteString(metaDataFile, cloudInitMetaData); err != nil {
		return err
	}

	if coreCmd.Development.DeveloperMode {
		fmt.Println("Enabling developer mode...")

		authorizedKey, err := getAuthorizedSshKey()
		if err != nil {
			return fmt.Errorf("failed to obtain a public key for developer mode: %s", err)
		}

		if _, err := io.WriteString(metaDataFile, "public-keys:\n"); err != nil {
			return err
		}

		if _, err := io.WriteString(metaDataFile, fmt.Sprintf("  - %s\n", authorizedKey)); err != nil {
			return err
		}

	}

	userDataFile, err := os.Create(filepath.Join(cloudDir, "user-data"))
	if err != nil {
		return err
	}
	defer userDataFile.Close()

	if _, err := io.WriteString(userDataFile, cloudInitUserData); err != nil {
		return err
	}

	if coreCmd.Development.DeveloperMode || coreCmd.Development.EnableSsh {
		if _, err := io.WriteString(userDataFile, "snappy:\n    ssh_enabled: True\n"); err != nil {
			return err
		}
	}

	return nil
}

func getAuthorizedSshKey() (string, error) {
	sshDir := os.ExpandEnv("$HOME/.ssh")

	fis, err := ioutil.ReadDir(sshDir)
	if err != nil {
		return "", fmt.Errorf("%s: no pub ssh key found, run ssh-keygen first", err)
	}

	var preferredPubKey string
	var latestModTime time.Time
	for i := range fis {
		file := fis[i].Name()
		if strings.HasSuffix(file, ".pub") && !strings.HasSuffix(file, "cert.pub") {
			fileMod := fis[i].ModTime()

			if fileMod.After(latestModTime) {
				latestModTime = fileMod
				preferredPubKey = file
			}
		}
	}

	if preferredPubKey == "" {
		return "", errors.New("no pub ssh key found, run ssh-keygen first")
	}

	pubKey, err := ioutil.ReadFile(filepath.Join(sshDir, preferredPubKey))

	return string(pubKey), err
}

func (coreCmd *CoreCmd) installFlags() snappy.InstallFlags {
	flags := snappy.InhibitHooks | snappy.AllowOEM

	if coreCmd.Development.DeveloperMode {
		flags |= snappy.AllowUnauthenticated
	}

	return flags
}

func (coreCmd *CoreCmd) extractOem(oemPackage string) error {
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
		Flavor:  "core",
		Series:  coreCmd.Positional.Release,
		Channel: coreCmd.Channel,
	})

	flags := coreCmd.installFlags()
	pb := progress.NewTextProgress()
	if _, err := snappy.Install(oemPackage, flags, pb); err != nil {
		return err
	}

	oem, err := coreCmd.loadOem(tempDir)
	if err != nil {
		return err
	}
	coreCmd.oem = oem

	return nil
}

func (coreCmd CoreCmd) loadOem(systemPath string) (oem diskimage.OemDescription, err error) {
	pkgs, err := filepath.Glob(filepath.Join(systemPath, "/oem/*/*/meta/package.yaml"))
	if err != nil {
		return oem, err
	}

	// checking for len(pkgs) > 2 due to the 'current' symlink
	if len(pkgs) == 0 {
		return oem, nil
	} else if len(pkgs) > 2 || err != nil {
		return oem, errors.New("too many oem packages installed")
	}

	f, err := ioutil.ReadFile(pkgs[0])
	if err != nil {
		return oem, errors.New("failed to read oem yaml")
	}

	if err := yaml.Unmarshal([]byte(f), &oem); err != nil {
		return oem, errors.New("cannot decode oem yaml")
	}

	return oem, nil
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
