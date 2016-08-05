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
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"text/template"

	"launchpad.net/goget-ubuntu-touch/diskimage"
	"launchpad.net/goget-ubuntu-touch/sysutils"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

type CreateCmd struct {
	Channel  string `long:"channel" description:"Select device channel"`
	Server   string `long:"server" description:"Select image server"`
	Revision int    `long:"revision" description:"Select revision"`
	RawDisk  bool   `long:"use-raw-disk" description:"Use raw disks instead of qcow2"`
	SDCard   bool   `long:"with-sdcard" description:"Create an external vfat sdcard"`
	Arch     string `long:"arch" description:"Device architecture to use (i386 or armhf)"`
	Password string `long:"password" description:"This sets up the default password for the phablet user" default:"0000"`
	Locale   string `long:"locale" description:"Use a different locale than the default one (e.g.; --locale es_AR.utf8)"`
}

var createCmd CreateCmd

const (
	defaultChannel = "ubuntu-touch/stable"
	defaultServer  = "https://system-image.ubuntu.com"
	defaultArch    = "i386"
)

const (
	binQemuArmStatic  = "/usr/bin/qemu-arm-static"
	pkgQemuUserStatic = "qemu-user-static"
)

const localeTemplate string = `description "Set wizard language"
author "ubuntu-emulator"

start on starting ubuntu-system-settings-wizard

task

script
    setenv() {
        initctl set-env --global $1=$2
        gdbus call --session --dest org.freedesktop.DBus --object-path /org/freedesktop/DBus --method org.freedesktop.DBus.UpdateActivationEnvironment "@a{ss} {'$1': '$2'}"
    }

    uid=$(getent passwd $USER|cut -d: -f3)
    if [ -z $uid ];then
        exit 1
    fi
    if [ ! -e $HOME/.cache/.first-lang-set ]; then
    setenv LANGUAGE {{ . }}
    setenv LC_ALL {{ . }}
    setenv LANG {{ . }}
    dbus-send --print-reply --system --dest=org.freedesktop.Accounts /org/freedesktop/Accounts/User$uid org.freedesktop.Accounts.User.SetFormatsLocale string:{{ . }}
    dbus-send --print-reply --system --dest=org.freedesktop.Accounts /org/freedesktop/Accounts/User$uid org.freedesktop.Accounts.User.SetLanguage string:{{ . }}
    touch $HOME/.cache/.first-lang-set
    fi
end script

# vim:syntax=upstart
`

func init() {
	createCmd.Arch = defaultArch
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

	if err := createCmd.verifyDependencies(); err != nil {
		return err
	}

	var device string
	if d, ok := devices[createCmd.Arch]; ok {
		device = d["name"]
	} else {
		return errors.New("Selected device not supported on this channel")
	}

	if syscall.Getuid() != 0 {
		return errors.New("Creation requires sudo/pkexec (root)")
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

	if err := createCmd.createSystem(ubuntuImage, sdcardImage, files); err != nil {
		return err
	}

	var deviceTar string
	if deviceTar, err = getDeviceTar(files); err != nil {
		return err
	}
	if err = flatExtractImages(deviceTar, dataDir); err != nil {
		return err
	}

	// boot.img must be in dataDir (Normal Boot Ramdisk)
	if err = extractBoot(dataDir, bootImage, bootRamdisk); err != nil {
		return err
	}

	// recovery.img must be in dataDir (Recovery Ramdisk)
	if err = extractBoot(dataDir, recoveryImage, recoveryRamdisk); err != nil {
		return err
	}

	if err := extractBuildProperties(systemImage, dataDir); err != nil {
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

	if createCmd.SDCard {
		fmt.Println("Creating vfat sdcard...")
		sdcard := diskimage.New(filepath.Join(dataDir, "sdcardprime.img"), "SDCARD", 2)
		if err := sdcard.CreateVFat(); err != nil {
			return err
		}
	}

	if err = writeStamp(dataDir, image); err != nil {
		return err
	}
	if err = writeDeviceStamp(dataDir, createCmd.Arch); err != nil {
		return err
	}

	fmt.Printf("Succesfully created emulator instance %s in %s\n", instanceName, dataDir)
	return nil
}

func extractBuildProperties(systemImage *diskimage.DiskImage, dataDir string) error {
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	return systemImage.ExtractFile("build.prop", filepath.Join(dataDir, "system"))
}

func (createCmd *CreateCmd) verifyDependencies() error {
	switch createCmd.Arch {
	case "armhf":
		if _, err := os.Stat(binQemuArmStatic); err != nil {
			return fmt.Errorf("missing dependency %s (apt install %s)", binQemuArmStatic, pkgQemuUserStatic)
		}
	}

	return nil
}

func (createCmd *CreateCmd) createSystem(ubuntuImage, sdcardImage *diskimage.DiskImage, files []string) (err error) {
	for _, img := range []*diskimage.DiskImage{ubuntuImage, sdcardImage} {
		if err := img.CreateExt4(); err != nil {
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
			fmt.Println("Unmount error:", err)
		}
		return err
	}

	fmt.Printf("Setting up a default password for phablet to: '%s'\n", createCmd.Password)
	if err := createCmd.setPassword(ubuntuImage.Mountpoint); err != nil {
		if err := ubuntuImage.Unmount(); err != nil {
			fmt.Println("Unmount error :", err)
		}
		return err
	}

	if err := createCmd.setLocale(ubuntuImage.Mountpoint); err != nil {
		if err := ubuntuImage.Unmount(); err != nil {
			fmt.Println("Unmount error :", err)
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
	if err = sdcardImage.OverrideAdbInhibit(); err != nil {
		return err
	}
	if err := ubuntuImage.Move(filepath.Join(sdcardImage.Mountpoint, "system.img")); err != nil {
		return err
	}
	return nil
}

// setLocale sets the locale to the one specified in locale
func (createCmd *CreateCmd) setLocale(chroot string) error {
	if createCmd.Locale == "" {
		return nil
	}

	if createCmd.Arch == "armhf" {
		if err := addQemuStatic(chroot); err != nil {
			return err
		}

		defer removeQemuStatic(chroot)
	}

	cmd := exec.Command("chroot", chroot, "/bin/sh", "-c", "locale -a")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Verify that the locale is actually part of the emulator
	var localeInstalled bool

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		locale := scanner.Text()
		if createCmd.Locale == locale {
			localeInstalled = true
			break
		}
	}

	if !localeInstalled {
		return errors.New("the selected locale is not available on the image")
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	// Setup the locale
	localeFile, err := os.Create(filepath.Join(chroot, "/usr/share/upstart/sessions/emulator-language.conf"))
	if err != nil {
		return err
	}
	defer localeFile.Close()

	t := template.Must(template.New("locale").Parse(localeTemplate))

	return t.Execute(localeFile, createCmd.Locale)
}

// setPassword is an ugly hack to set the password
func (createCmd *CreateCmd) setPassword(chroot string) error {
	if createCmd.Arch == "armhf" {
		if err := addQemuStatic(chroot); err != nil {
			return err
		}

		defer removeQemuStatic(chroot)
	}

	// Run something that would look like this
	// PATH=$path chroot "$SYSTEM_MOUNTPOINT" /bin/sh -c "echo -n "$user:$password" | chpasswd"
	chrootCmd := fmt.Sprintf("echo -n '%s:%s' | chpasswd", "phablet", createCmd.Password)
	if out, err := exec.Command("chroot", chroot, "/bin/sh", "-c", chrootCmd).CombinedOutput(); err != nil {
		return errors.New(string(out))
	}

	return nil
}

func addQemuStatic(chroot string) error {
	dst := filepath.Join(chroot, binQemuArmStatic)
	if out, err := exec.Command("cp", binQemuArmStatic, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while setting up password: %s", out)
	}

	return nil
}

func removeQemuStatic(chroot string) error {
	dst := filepath.Join(chroot, binQemuArmStatic)

	return os.Remove(dst)
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
	err := file.MakeRelativeToServer(server)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := sysutils.DropPrivs(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = file.Download(downloadDir)
	if err != nil {
		fmt.Printf("Cannot download %s%s: %s\n", file.Server, file.Path, err)
		os.Exit(1)
	}
	filePath := filepath.Join(downloadDir, file.Path)
	done <- filePath
}
