//
// udbflash - Tool to download and flash devices with an Ubuntu Image based
//            system
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
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"launchpad.net/goget-ubuntu-touch/devices"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

func main() {
	if _, err := parser.Parse(); err != nil {
		log.Fatal(err)
	}
	if args.TLSSkipVerify {
		ubuntuimage.TLSSkipVerify()
	}
	script := args.RunScript
	if script != "" {
		if p, err := filepath.Abs(script); err != nil {
			log.Fatal("Run script not found:", err)
		} else {
			script = p
		}

		fi, err := os.Lstat(script)
		if err != nil {
			log.Fatal(err)
		}
		if fi.Mode()&0100 == 0 {
			log.Fatalf("The script %s passed via --run-script is not executable", script)
		}
	}
	channels, err := ubuntuimage.NewChannels(args.Server)
	if err != nil {
		log.Fatal(err)
	}
	if args.ListChannels {
		for k, v := range channels {
			if v.Alias != "" {
				fmt.Printf("%s (alias to %s)\n", k, v.Alias)
			} else {
				fmt.Println(k)
			}
		}
		return
	}
	cacheDir := ubuntuimage.GetCacheDir()
	if args.CleanCache {
		log.Print("Cleaning prevously downloaded content")
		if err := os.RemoveAll(cacheDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	adb, err := devices.NewUbuntuDebugBridge()
	var fastboot devices.Fastboot
	if err != nil {
		log.Fatal(err)
	}
	if args.Serial != "" {
		adb.SetSerial(args.Serial)
		fastboot.SetSerial(args.Serial)
	}
	if args.Device == "" {
		if args.Bootstrap {
			log.Print("Expecting the device to be in the bootloader... waiting")
			args.Device, err = fastboot.GetDevice()
		} else {
			log.Print("Expecting the device to expose an adb interface...")
			// TODO needs to work from recovery as well
			//adb.WaitForDevice()
			args.Device, err = adb.GetDevice()
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("Device is |%s|", args.Device)
	deviceChannel, err := channels.GetDeviceChannel(
		args.Server, args.Channel, args.Device)
	if err != nil {
		log.Fatal(err)
	}
	var image ubuntuimage.Image
	if args.Revision <= 0 {
		image, err = deviceChannel.GetRelativeImage(args.Revision)
	} else {
		image, err = deviceChannel.GetImage(args.Revision)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Flashing version %d from %s channel and server %s to device %s",
		image.Version, args.Channel, args.Server, args.Device)
	if deviceChannel.Alias != "" {
		log.Printf("%s is a channel alias to %s", deviceChannel.Alias, args.Channel)
	}

	// TODO use closures
	signFiles := ubuntuimage.GetGPGFiles()
	totalFiles := len(image.Files) + len(signFiles)
	files := make(chan Files, totalFiles)
	done := make(chan bool, totalFiles)
	for _, file := range image.Files {
		go bitDownloader(file, files, args.Server, cacheDir)
	}
	for _, file := range signFiles {
		go bitDownloader(file, files, args.Server, cacheDir)
	}
	if args.Bootstrap {
		var downloadedFiles []Files
		for i := 0; i < totalFiles; i++ {
			downloadedFiles = append(downloadedFiles, <-files)
		}
		//Find the recovery image
		var recovery string
		for _, file := range downloadedFiles {
			if strings.HasSuffix(file.FilePath, ".xz") {
				fmt.Println(file.FilePath)
				recovery, err = tryExtractRecovery(file.FilePath)
				if err == nil {
					break
				}
			}
		}
		if recovery == "" {
			log.Fatal("Recovery image not found, cannot continue with bootstrap")
		}
		err = fastboot.Flash("recovery", recovery)
		if err != nil {
			log.Fatal("Can't flash recovery image")
		}
		err = fastboot.Format("cache")
		if err != nil {
			log.Print("Cache formatting was not successful, flashing may fail, " +
				"check your partitions on device")
		}
		err = fastboot.BootImage(recovery)
		if err != nil {
			log.Fatal("Can't boot recovery image")
		}
		os.Remove(recovery)
		adb.WaitForRecovery()
		// Resend all the files
		for _, file := range downloadedFiles {
			files <- file
		}
		args.Wipe = true
	}
	go bitPusher(adb, files, done)
	for i := 0; i < totalFiles; i++ {
		<-done
	}
	ubuntuCommands, err := ubuntuimage.GetUbuntuCommands(image.Files, cacheDir, args.Wipe)
	if err != nil {
		log.Fatal("Cannot create commands file")
	}
	log.Printf("Created ubuntu_command: %s", ubuntuCommands)

	adb.Push(ubuntuCommands, "/cache/recovery/ubuntu_command")
	defer func() {
		if err == nil {
			err = os.Remove(ubuntuCommands)
			if err != nil {
				log.Fatal(err)
			}
		}
	}()

	// either customize the flashing process by running a user provided script
	// or reboot into recovery to let the standard upgrade script to run
	if script != "" {
		log.Printf("Preparing to run %s to finish the flashing process\n", script)
		cmd := exec.Command(script)
		cmd.Stdout = os.Stdout
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}

	} else {
		log.Print("Rebooting into recovery to flash")
		adb.RebootRecovery()
		err = adb.WaitForRecovery()
		if err != nil {
			log.Fatal(err)
		}
	}
}

type Files struct{ FilePath, SigPath string }

// bitDownloader downloads
func bitDownloader(file ubuntuimage.File, files chan<- Files, server, downloadDir string) {
	err := file.MakeRelativeToServer(server)
	if err != nil {
		log.Fatal(err)
	}
	err = file.Download(downloadDir)
	if err != nil {
		log.Fatal(err)
	}
	files <- Files{FilePath: filepath.Join(downloadDir, file.Path),
		SigPath: filepath.Join(downloadDir, file.Signature)}
}

// bitPusher
func bitPusher(adb devices.UbuntuDebugBridge, files <-chan Files, done chan<- bool) {
	if _, err := adb.Shell("rm -rf /cache/recovery/*.xz /cache/recovery/*.xz.asc"); err != nil {
		log.Fatal("Cannot cleanup /cache/recovery/ to ensure clean deployment", err)
	}
	freeSpace := "unknown"
	dfCacheCmd := "df -h | grep /dev/disk/by-partlabel/cache"
	if free, err := adb.Shell(dfCacheCmd); err != nil {
		log.Fatal("Unable to retrieve free space on target")
	} else {
		//Filesystem Size Used Avail Use% Mounted on
		free := strings.Fields(free)
		if len(free) > 3 {
			freeSpace = free[3]
		}
	}
	errMsg := "Cannot push %s to device: free space on /cache/recovery is %s"
	for {
		file := <-files
		go func() {
			log.Printf("Start pushing %s to device", file.FilePath)
			err := adb.Push(file.FilePath, "/cache/recovery/")
			if err != nil {
				log.Fatalf(errMsg, file.SigPath, freeSpace)
			}
			err = adb.Push(file.SigPath, "/cache/recovery/")
			if err != nil {
				log.Fatalf(errMsg, file.SigPath, freeSpace)
			}
			log.Printf("Done pushing %s to device", file.FilePath)
			done <- true
		}()
	}
}

func tryExtractRecovery(tarxz string) (recovery string, err error) {
	f, err := os.Open(tarxz)
	if err != nil {
		log.Fatalf("Can't open %s in search for recovery", tarxz)
	}
	defer f.Close()
	r := xzReader(f)
	tempfile, err := ioutil.TempFile(os.TempDir(), "recoverytar")
	if err != nil {
		log.Fatal("Can't create tempfile to search for for recovery")
	}
	defer func() {
		tempfile.Close()
		os.Remove(tempfile.Name())
	}()
	n, err := io.Copy(tempfile, r)
	if err != nil {
		log.Fatalf("copied %d bytes with err: %v", n, err)
	}
	_, err = tempfile.Seek(0, 0)
	if err != nil {
		log.Fatal("Failed to rewind")
	}
	tr := tar.NewReader(tempfile)
	var recoveryFile *os.File
	//Going to return inside this loop, ugly, yeah
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			log.Fatalln(err)
		}
		if strings.Contains(hdr.Name, "recovery.img") {
			recoveryFile, err = ioutil.TempFile(os.TempDir(), "recovery")
			defer recoveryFile.Close()
			if err != nil {
				log.Fatal(err)
			}
			if _, err := io.Copy(recoveryFile, tr); err != nil {
				log.Fatal(err)
			}
			return recoveryFile.Name(), nil
		}
	}
	return "", errors.New("Recovery Partition not found")
}

func xzReader(r io.Reader) io.ReadCloser {
	rpipe, wpipe := io.Pipe()

	cmd := exec.Command("xz", "--decompress", "--stdout")
	cmd.Stdin = r
	cmd.Stdout = wpipe

	go func() {
		err := cmd.Run()
		wpipe.CloseWithError(err)
	}()

	return rpipe
}
