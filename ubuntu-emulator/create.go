//
// ubuntu-emu - Tool to download and run Ubuntu Touch emulator instances
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

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

import (
	"errors"
	"fmt"
	"launchpad.net/goget-ubuntu-touch/ubuntu-emulator/diskimage"
	"launchpad.net/goget-ubuntu-touch/ubuntu-emulator/sysutils"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type CreateCmd struct {
	Channel  string `long:"channel" description:"Select device channel"`
	Server   string `long:"server" description:"Select image server"`
	Revision int    `long:"revision" description:"Select revision"`
	RawDisk  bool   `long:"use-raw-disk" description:"Use raw disks instead of qcow2"`
}

var createCmd CreateCmd

const (
	device         = "generic"
	defaultChannel = "devel"
	defaultServer  = "https://system-image.ubuntu.com"
)

func init() {
	createCmd.Channel = defaultChannel
	createCmd.Server = defaultServer
	parser.AddCommand("create",
		"Create new emulator instance named 'name'",
		"Creates a new emulator instance name 'name' by downloading the necessary components "+
			"from the image server",
		&createCmd)
}

func (createCmd *CreateCmd) Execute(args []string) error {
	if len(args) != 1 {
		return errors.New("Instance name 'name' is required")
	}
	instanceName := args[0]

	if syscall.Getuid() != 0 {
		return errors.New("Creation requires sudo (root)")
	}

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}

	channels, err := ubuntuimage.NewChannels(createCmd.Server)
	if err != nil {
		return err
	}
	deviceChannel, err := channels.GetDeviceChannel(
		createCmd.Server, createCmd.Channel, device)
	if err != nil {
		return err
	}
	var image ubuntuimage.Image
	if createCmd.Revision <= 0 {
		image, err = deviceChannel.GetRelativeImage(createCmd.Revision)
	} else {
		image, err = deviceChannel.GetImage(createCmd.Revision)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Creating \"%s\" from %s revision %d\n", instanceName, createCmd.Channel, image.Version)
	fmt.Println("Downloading...")
	files, _ := download(image)
	dataDir := getInstanceDataDir(instanceName)
	if os.MkdirAll(dataDir, 0700) != nil {
		return err
	}

	fmt.Println("Setting up...")
	//This image will later be copied into sdcard.img as system.img and will hold the Ubuntu rootfs
	ubuntuImage := diskimage.New(filepath.Join(dataDir, "ubuntu-system.img"), "UBUNTU", 3)
	//This image represents userdata, it will be marked with .writable_image and hold the
	//Ubuntu rootfs.
	sdcardImage := diskimage.New(filepath.Join(dataDir, "sdcard.img"), "USERDATA", 4)
	systemImage := diskimage.NewExisting(filepath.Join(dataDir, "system.img"))

	if err := createSystem(ubuntuImage, sdcardImage, files); err != nil {
		return err
	}

	var deviceTar string
	if deviceTar, err = getDeviceTar(files); err != nil {
		return err
	}
	if err = flatExtractImages(deviceTar, dataDir); err != nil {
		return err
	}

	// boot.img must be in dataDir
	if err = extractBoot(dataDir); err != nil {
		return err
	}
	if createCmd.RawDisk != true {
		fmt.Println("Creating snapshots for disks...")
		for _, img := range []*diskimage.DiskImage{systemImage, sdcardImage} {
			if err := img.ConvertQcow2(); err != nil {
				return err
			}
		}
	}

	if err = sysutils.WriteStamp(dataDir, image); err != nil {
		return err
	}

	fmt.Printf("Succesfully created emulator instance %s in %s\n", instanceName, dataDir)
	return nil
}

func createSystem(ubuntuImage, sdcardImage *diskimage.DiskImage, files []string) (err error) {
	for _, img := range []*diskimage.DiskImage{ubuntuImage, sdcardImage} {
		if err := img.Create(); err != nil {
			return err
		}
	}

	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.EscalatePrivs(); err != nil {
		return err
	}
	defer func() (err error) {
		return sysutils.DropPrivs()
	}()

	if err := ubuntuImage.Mount(); err != nil {
		return err
	}
	if err := ubuntuImage.Provision(files); err != nil {
		if err := ubuntuImage.Unmount(); err != nil {
			fmt.Println("Unmounting error when errors:", err)
		}
		return err
	}
	if err := ubuntuImage.Unmount(); err != nil {
		return err
	}
	if err := sdcardImage.Mount(); err != nil {
		return err
	}
	defer sdcardImage.Unmount()
	if err = sdcardImage.Writable(); err != nil {
		return err
	}
	if err := ubuntuImage.Move(filepath.Join(sdcardImage.Mountpoint, "system.img")); err != nil {
		return err
	}
	return nil
}

func download(image ubuntuimage.Image) (files []string, err error) {
	cacheDir := ubuntuimage.GetCacheDir()
	totalFiles := len(image.Files)
	done := make(chan string, totalFiles)
	for _, file := range image.Files {
		go bitDownloader(file, done, createCmd.Server, cacheDir)
	}
	for i := 0; i < totalFiles; i++ {
		files = append(files, <-done)
	}
	return files, nil
}

// bitDownloader downloads
func bitDownloader(file ubuntuimage.File, done chan<- string, server, downloadDir string) {
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err := file.Download(server, downloadDir)
	if err != nil {
		fmt.Print(fmt.Sprintf("Cannot download %s", file))
		os.Exit(1)
	}
	filePath := filepath.Join(downloadDir, file.Path)
	done <- filePath
}
