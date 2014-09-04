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
	"syscall"

	"launchpad.net/goget-ubuntu-touch/devices"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

func main() {
	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
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
		if fi.Mode()&0100 == 0 || !fi.Mode().IsRegular() {
			log.Fatalf("The script %s passed via --run-script is not executable", script)
		}
	}

	if args.Bootstrap {
		args.Wipe = true
	}

	if args.Password != "" && !args.Wipe && !args.DeveloperMode {
		log.Fatal("Default password setup requires --developer-mode, and --wipe or --bootstrap")
	}

	tarballPath := args.DeviceTarball
	if tarballPath != "" {
		if p, err := filepath.Abs(tarballPath); err != nil {
			log.Fatal("Device tarball not found", err)
		} else {
			tarballPath = p
		}
		fi, err := os.Lstat(tarballPath)
		if err != nil {
			log.Fatal(err)
		}
		if !fi.Mode().IsRegular() {
			log.Fatalf("The file %s passed via --device-tarball is not a regular file\n", tarballPath)
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
	if args.ShowImage {
		fmt.Printf("Description: %s\nVersion: %d\nChannel: %s\n", image.Description, image.Version, args.Channel)
		for _, f := range image.Files {
			f.MakeRelativeToServer(args.Server)
			fmt.Printf("%d %s%s %d %s\n", f.Order, f.Server, f.Path, f.Size, f.Checksum)
		}
		return
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
	for i, file := range image.Files {
		if tarballPath != "" && strings.HasPrefix(file.Path, "/pool/device") {
			//change the file paths so they are correctly picked up by bitPusher later on
			image.Files[i].Path = tarballPath
			image.Files[i].Signature = tarballPath + ".asc"
			useLocalTarball(image.Files[i], files)
		} else {
			go bitDownloader(file, files, args.Server, cacheDir)
		}
	}
	for _, file := range signFiles {
		go bitDownloader(file, files, args.Server, cacheDir)
	}
	if args.DownloadOnly {
		for i := 0; i < totalFiles; i++ {
			<-files
		}
		log.Printf("Downloaded files for version %d, channel %s, exiting without flashing as requested.\n", image.Version, args.Channel)
		os.Exit(0)
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
	}
	go bitPusher(adb, files, done)
	for i := 0; i < totalFiles; i++ {
		<-done
	}

	var enableList []string
	if args.DeveloperMode {
		enableList = append(enableList, "developer_mode")
	}
	if args.Password != "" {
		enableList = append(enableList, "default_password "+args.Password)
	}

	ubuntuCommands, err := ubuntuimage.GetUbuntuCommands(image.Files, cacheDir, args.Wipe, enableList)
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

// ensureExists touches a file. It can be used to create a dummy .asc file if none exists
func ensureExists(path string) {
	f, err := os.OpenFile(path, syscall.O_WRONLY|syscall.O_CREAT, 0666)
	if err != nil {
		log.Fatal("Cannot touch %s : %s", path, err)
	}
	f.Close()
}

type Files struct{ FilePath, SigPath string }

// useLocalTarball adds a local file to the ones to be pushed
func useLocalTarball(file ubuntuimage.File, files chan<- Files) {
	ensureExists(file.Signature)
	files <- Files{FilePath: file.Path, SigPath: file.Signature}
}

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
	dfCacheCmd := "df -h | grep /android/cache"
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
