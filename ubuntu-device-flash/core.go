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

	"launchpad.net/goyaml"

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
	Channel       string `long:"channel" description:"Specify the channel to use" default:"ubuntu-core/devel"`
	Device        string `long:"device" description:"Specify the device to use" default:"generic_amd64"`
	Keyboard      string `long:"keyboard-layout" description:"Specify the keyboard layout" default:"us"`
	Output        string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Size          int64  `long:"size" short:"s" description:"Size of image file to create in GB (min 4)" default:"20"`
	DeveloperMode bool   `long:"developer-mode" description:"Finds the latest public key in your ~/.ssh and sets it up using cloud-init"`
	EnableSsh     bool   `long:"enable-ssh" description:"Enable ssh on the image through cloud-init(not need with developer mode)"`
	Cloud         bool   `long:"cloud" description:"Generate a pure cloud image without setting up cloud-init"`

	Development struct {
		DevicePart string `long:"device-part" description:"Specify a local device part to override the one from the server"`
	} `group:"Development"`

	hardware diskimage.HardwareDescription
}

var coreCmd CoreCmd

const cloudInitMetaData = `instance-id: nocloud-static
`

const cloudInitUserData = `#cloud-config
password: passw0rd
chpasswd: { expire: False }
ssh_pwauth: True
`

func (coreCmd *CoreCmd) Execute(args []string) error {
	if coreCmd.EnableSsh && coreCmd.Cloud {
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

	if syscall.Getuid() != 0 {
		return errors.New("command requires sudo/pkexec (root)")
	}

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}

	fmt.Println("Fetching information from server...")

	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, coreCmd.Channel, coreCmd.Device)
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

	switch coreCmd.hardware.Bootloader {
	case "grub":
		img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size)
	case "u-boot":
		img = diskimage.NewCoreUBootImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware)
	default:
		fmt.Printf("Bootloader set to '%s' in hardware description, assuming grub as a fallback\n", coreCmd.hardware.Bootloader)
		img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size)
	}

	printOut("Partitioning...")
	if err := img.Partition(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(coreCmd.Output)
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

	fmt.Println("New image complete, launch by running: kvm -m 768", coreCmd.Output)

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
	defer func() {
		printOut("Unmapping...")
		defer img.Unmap()
	}()

	printOut("Mounting...")
	if err := img.Mount(); err != nil {
		fmt.Println(err)
		return err
	}
	defer func() {
		printOut("Unmounting...")
		if err := img.Unmount(); err != nil {
			fmt.Println(err)
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

	if err := img.SetupBoot(); err != nil {
		return err
	}

	if err := coreCmd.setupKeyboardLayout(systemPath); err != nil {
		return err
	}

	if !coreCmd.Cloud {
		cloudBaseDir := filepath.Join("var", "lib", "cloud")

		if err := os.MkdirAll(filepath.Join(systemPath, cloudBaseDir), 0755); err != nil {
			return err
		}

		if err := coreCmd.setupCloudInit(cloudBaseDir, filepath.Join(writablePath, "system-data")); err != nil {
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

	if coreCmd.DeveloperMode {
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

	if coreCmd.DeveloperMode || coreCmd.EnableSsh {
		if _, err := io.WriteString(userDataFile, "snappy:\n    ssh_enabled: True\n"); err != nil {
			return err
		}
	}

	return nil
}

func (coreCmd *CoreCmd) setupKeyboardLayout(systemPath string) error {
	kbFilePath := filepath.Join(systemPath, "etc", "default", "keyboard")

	// do not error if the image has no keyboard
	if _, err := os.Stat(kbFilePath); err != nil && os.IsNotExist(err) {
		return nil
	}

	kbFileContents, err := ioutil.ReadFile(kbFilePath)
	if err != nil {
		return err
	}

	r := strings.NewReplacer("XKBLAYOUT=\"us\"", fmt.Sprintf("XKBLAYOUT=\"%s\"", coreCmd.Keyboard))
	kbFileContents = []byte(r.Replace(string(kbFileContents)))

	return ioutil.WriteFile(kbFilePath, kbFileContents, 0644)
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

func extractHWDescription(path string) (hw diskimage.HardwareDescription, err error) {
	printOut("Searching for hardware.yaml in device part")
	tmpdir, err := ioutil.TempDir("", "hardware")
	if err != nil {
		return hw, errors.New("cannot create tempdir to extract hardware.yaml from device part")
	}
	defer os.RemoveAll(tmpdir)

	if err := exec.Command("tar", "xf", path, "-C", tmpdir, "hardware.yaml").Run(); err != nil {
		return hw, errors.New("device part does not contain a hardware.yaml")
	}

	data, err := ioutil.ReadFile(filepath.Join(tmpdir, "hardware.yaml"))
	if err != nil {
		return hw, err
	}

	err = goyaml.Unmarshal([]byte(data), &hw)

	return hw, err
}
