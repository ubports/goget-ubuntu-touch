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
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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

	Snapper
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
	coreCmd.flavor = flavorCore

	if err := coreCmd.sanityCheck(); err != nil {
		return err
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

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}

	fmt.Println("Fetching information from server...")
	systemImage, err := coreCmd.systemImage(coreCmd.Deprecated.Device)
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

	loader := coreCmd.oem.OEM.Hardware.Bootloader
	switch loader {
	case "grub":
		coreCmd.img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	case "u-boot":
		coreCmd.img = diskimage.NewCoreUBootImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	default:
		fmt.Printf("Bootloader set to '%s' in oem hardware description, assuming grub as a fallback\n", loader)
		coreCmd.img = diskimage.NewCoreGrubImage(coreCmd.Output, coreCmd.Size, coreCmd.hardware, coreCmd.oem)
	}

	printOut("Partitioning...")
	if err := coreCmd.img.Partition(); err != nil {
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

	coreCmd.hardware = <-hwChan

	// Execute the following code with escalated privs and drop them when done
	if err := coreCmd.deploy(systemImage, filePathChan); err != nil {
		return err
	}

	if err := coreCmd.img.FlashExtra(); err != nil {
		return err
	}

	coreCmd.printSummary()

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
