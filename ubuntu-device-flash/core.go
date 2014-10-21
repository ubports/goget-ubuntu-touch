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
	"io/ioutil"
	"os"
	"os/exec"
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
	Channel  string `long:"channel" description:"Specify the channel to use" default:"ubuntu-core/utopic"`
	Device   string `long:"device" description:"Specify the device to use" default:"generic_amd64"`
	Keyboard string `long:"keyboard-layout" description:"Specify the keyboard layout" default:"us"`
	Output   string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Size     int64  `long:"size" short:"s" description:"Size of image file to create in GB (min 6)" default:"20"`
}

var coreCmd CoreCmd

func (coreCmd *CoreCmd) Execute(args []string) error {
	if syscall.Getuid() != 0 {
		return errors.New("command requires sudo/pkexec (root)")
	}

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}

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

	for _, f := range sigFiles {
		go bitDownloader(f, sigFilesChan, globalArgs.Server, cacheDir)
	}

	for _, f := range image.Files {
		go bitDownloader(f, filesChan, globalArgs.Server, cacheDir)
	}

	filePathChan := make(chan string)

	go func() {
		for i := 0; i < len(image.Files); i++ {
			f := <-filesChan
			fmt.Println("Download finished for", f.FilePath)
			filePathChan <- f.FilePath
		}
		close(filePathChan)
	}()

	img := diskimage.New(coreCmd.Output, "", coreCmd.Size)
	if err := img.Partition(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(coreCmd.Output)
		}
	}()

	{
		if err := sysutils.EscalatePrivs(); err != nil {
			return err
		}
		defer sysutils.DropPrivs()

		if err := coreCmd.partition(img); err != nil {
			return err
		}

		if err := coreCmd.setup(img, filePathChan); err != nil {
			return err
		}
	}

	fmt.Println("New image complete, launch by running: kvm", coreCmd.Output)

	return nil
}

func (coreCmd *CoreCmd) partition(img *diskimage.DiskImage) error {
	if err := img.MapPartitions(); err != nil {
		return err
	}
	defer img.UnMapPartitions()

	return img.CreateExt4()
}

func (coreCmd *CoreCmd) setup(img *diskimage.DiskImage, filePathChan <-chan string) error {
	if err := img.MapPartitions(); err != nil {
		return err
	}
	defer img.UnMapPartitions()

	if err := img.Mount(); err != nil {
		return err
	}
	defer img.Unmount()

	for f := range filePathChan {
		if out, err := exec.Command("tar", "--numeric-owner", "-axvf", f, "-C", img.Mountpoint).CombinedOutput(); err != nil {
			return fmt.Errorf("issues while extracting: %s", out)
		}
	}

	{
		systemPath, err := img.System()
		if err != nil {
			return err
		}

		if err := coreCmd.setupBootloader(systemPath); err != nil {
			return err
		}

		if err := coreCmd.setupKeyboardLayout(systemPath); err != nil {
			return err
		}
	}

	{
		userPath, err := img.User()
		if err != nil {
			return err
		}

		for _, dir := range []string{"system-data", "user-data", "cache"} {
			dirPath := filepath.Join(userPath, dir)
			if err := os.Mkdir(dirPath, 0755); err != nil {
				return err
			}
		}
	}

	return nil
}

func (coreCmd *CoreCmd) setupKeyboardLayout(systemPath string) error {
	kbFilePath := filepath.Join(systemPath, "etc", "default", "keyboard")

	kbFileContents, err := ioutil.ReadFile(kbFilePath)
	if err != nil {
		return err
	}

	r := strings.NewReplacer("XKBLAYOUT=\"us\"", fmt.Sprintf("XKBLAYOUT=\"%s\"", coreCmd.Keyboard))
	kbFileContents = []byte(r.Replace(string(kbFileContents)))

	return ioutil.WriteFile(kbFilePath, kbFileContents, 0644)
}

func (coreCmd *CoreCmd) setupBootloader(systemPath string) error {
	for _, dev := range []string{"dev", "proc", "sys"} {
		src := filepath.Join("/", dev)
		dst := filepath.Join(systemPath, dev)
		if err := bindMount(src, dst); err != nil {
			return err
		}
		defer unmount(dst)
	}

	firmwarePath := filepath.Join(systemPath, "sys", "firmware")
	if err := bindMount(filepath.Join(systemPath, "mnt"), firmwarePath); err != nil {
		return err
	}
	defer unmount(firmwarePath)

	outputPath, err := filepath.Abs(coreCmd.Output)
	if err != nil {
		return errors.New("cannot determined absolute path for output image")
	}

	rootDevPath := filepath.Join(systemPath, "root_dev")

	if f, err := os.Create(rootDevPath); err != nil {
		return err
	} else {
		f.Close()
		defer os.Remove(rootDevPath)
	}

	if err := bindMount(outputPath, rootDevPath); err != nil {
		return err
	}
	defer unmount(rootDevPath)

	if out, err := exec.Command("chroot", systemPath, "grub-install", "/root_dev").CombinedOutput(); err != nil {
		return fmt.Errorf("unable to install grub: %s", out)
	} else {
		fmt.Println(string(out))
	}

	// I don't know why this is needed, I just picked it up from the original implementation
	time.Sleep(3 * time.Second)

	if out, err := exec.Command("chroot", systemPath, "update-grub").CombinedOutput(); err != nil {
		return fmt.Errorf("unable to update grub: %s", out)
	} else {
		fmt.Println(string(out))
	}

	return nil
}

func bindMount(src, dst string) error {
	if out, err := exec.Command("mount", "--bind", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while bind mounting: %s", out)
	}

	return nil
}

func unmount(dst string) error {
	if out, err := exec.Command("umount", dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while unmounting: %s", out)
	}

	return nil
}
