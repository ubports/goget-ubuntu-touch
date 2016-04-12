//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2013-2014 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

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

func init() {
	parser.AddCommand("touch",
		"Flashes ubuntu touch images",
		"",
		&touchCmd)
}

type TouchCmd struct {
	Bootstrap     bool   `long:"bootstrap" description:"bootstrap the system, do this from the bootloader"`
	Wipe          bool   `long:"wipe" description:"Clear all data after flashing"`
	Serial        string `long:"serial" description:"Serial of the device to operate"`
	DeveloperMode bool   `long:"developer-mode" description:"Enables developer mode after the factory reset, this is meant for automation and makes the device insecure by default (requires --password)"`
	AdbKeys       string `long:"adb-keys" description:"Specify a local adb keys files, instead of using default ~/.android/adbkey.pub (requires --developer-mode)"`
	DeviceTarball string `long:"device-tarball" description:"Specify a local device tarball to override the one from the server (using official Ubuntu images with different device tarballs)"`
	CustomTarball string `long:"custom-tarball" description:"Specify a local custom tarball to override the one from the server (using official Ubuntu images with different custom tarballs)"`
	RunScript     string `long:"run-script" description:"Run a script given by path to finish the flashing process, instead of rebooting to recovery (mostly used during development to work around quirky or incomplete recovery images)"`
	Password      string `long:"password" description:"This sets up the default password for the phablet user. This option is meant for CI and not general use"`
	Channel       string `long:"channel" description:"Specify the channel to use" default:"ubuntu-touch/stable"`
	Device        string `long:"device" description:"Specify the device to flash"`
	RecoveryImage string `long:"recovery-image" description:"Specify the recovery image file to use when flashing, overriding the one from the device tarball (useful if the latter has no adb enabled)"`
	fastboot      devices.Fastboot
	adb           devices.UbuntuDebugBridge
}

var touchCmd TouchCmd

func (touchCmd *TouchCmd) Execute(args []string) error {
	if globalArgs.TLSSkipVerify {
		ubuntuimage.TLSSkipVerify()
	}

	script := touchCmd.RunScript
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

	if touchCmd.Bootstrap {
		touchCmd.Wipe = true
	}

	if touchCmd.DeveloperMode && touchCmd.Password == "" {
		log.Fatal("Developer mode requires --password to be set (and --wipe or --bootstrap)")
	}

	if touchCmd.AdbKeys != "" && !touchCmd.DeveloperMode {
		log.Fatal("Adb keys requires --developer-mode to be set")
	}

	var adbKeyPath string
	if touchCmd.DeveloperMode {
		if touchCmd.AdbKeys != "" {
			p, err := expandFile(touchCmd.AdbKeys)
			if err != nil {
				log.Fatalln("Issues with custom adb keys file", err)
			}
			adbKeyPath = p
		} else {
			home := os.Getenv("HOME")
			p, err := expandFile(filepath.Join(home, "/.android/adbkey.pub"))
			if err != nil {
				fmt.Println("WARNING: missing ~/.android/adbkey.pub, your device will not be preauthorised")
			} else {
				fmt.Println("no --adb-keys defined, using default ~/.android/adbkey.pub")
				adbKeyPath = p
			}
		}
	}

	if touchCmd.Password != "" && !touchCmd.Wipe {
		log.Fatal("Default password setup requires --wipe or --bootstrap")
	}

	if touchCmd.Password != "" || touchCmd.DeveloperMode {
		fmt.Println("WARNING --developer-mode and --password are dangerous as they remove security features from your device")
	}

	if touchCmd.AdbKeys != "" && touchCmd.DeveloperMode {
		fmt.Println("WARNING: --adb-keys is dangerous, potentially authorising multiple cliets to connect to your device")
	}

	var deviceTarballPath string
	if touchCmd.DeviceTarball != "" {
		p, err := expandFile(touchCmd.DeviceTarball)
		if err != nil {
			log.Fatalln("Issues while replacing the device tarball:", err)
		}

		deviceTarballPath = p
	}

	var customTarballPath string
	if touchCmd.CustomTarball != "" {
		p, err := expandFile(touchCmd.CustomTarball)
		if err != nil {
			log.Fatalln("Issues while replacing the custom tarball:", err)
		}

		customTarballPath = p
	}

	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	if globalArgs.CleanCache {
		log.Print("Cleaning prevously downloaded content")
		return os.RemoveAll(cacheDir)
	}

	if err := touchCmd.setupDevice(); err != nil {
		return err
	}
	log.Printf("Device is |%s|", touchCmd.Device)

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, touchCmd.Channel, touchCmd.Device)
	if err != nil {
		return err
	}

	image, err := getImage(deviceChannel)
	if err != nil {
		return err
	}

	log.Printf("Flashing version %d from %s channel and server %s to device %s",
		image.Version, touchCmd.Channel, globalArgs.Server, touchCmd.Device)

	// TODO use closures
	signFiles := ubuntuimage.GetGPGFiles()
	totalFiles := len(image.Files) + len(signFiles)
	files := make(chan Files, totalFiles)
	done := make(chan bool, totalFiles)

	for i, file := range image.Files {
		if deviceTarballPath != "" && isDevicePart(file.Path) {
			//change the file paths so they are correctly picked up by bitPusher later on
			image.Files[i].Path = deviceTarballPath
			image.Files[i].Signature = deviceTarballPath + ".asc"
			useLocalTarball(image.Files[i], files)
		} else if customTarballPath != "" && isCustomPart(file.Path) {
			//change the file paths so they are correctly picked up by bitPusher later on
			image.Files[i].Path = customTarballPath
			image.Files[i].Signature = customTarballPath + ".asc"
			useLocalTarball(image.Files[i], files)
		} else {
			go bitDownloader(file, files, globalArgs.Server, cacheDir)
		}
	}

	for _, file := range signFiles {
		go bitDownloader(file, files, globalArgs.Server, cacheDir)
	}

	if globalArgs.DownloadOnly {
		for i := 0; i < totalFiles; i++ {
			<-files
		}
		log.Printf("Downloaded files for version %d, channel %s, exiting without flashing as requested.\n", image.Version, touchCmd.Channel)
		return nil
	}

	if touchCmd.Bootstrap {
		// Unless --recovery-image is passed use the recovery image from the device tarball
		recovery := touchCmd.RecoveryImage

		if recovery == "" {
			var downloadedFiles []Files
			for i := 0; i < totalFiles; i++ {
				downloadedFiles = append(downloadedFiles, <-files)
			}
			//Find the recovery image
			for _, file := range downloadedFiles {
				if strings.HasSuffix(file.FilePath, ".xz") {
					recovery, err = tryExtractRecovery(file.FilePath)
					if err == nil {
						defer os.Remove(recovery)
						break
					}
				}
			}
			if recovery == "" {
				return errors.New("recovery image not found, cannot continue with bootstrap")
			}
			// Resend all the files
			for _, file := range downloadedFiles {
				files <- file
			}
		}

		log.Print("recovery ", recovery);

		if touchCmd.Device != "turbo" {
			if err := touchCmd.fastboot.Flash("recovery", recovery); err != nil {
				return errors.New("can't flash recovery image")
			}

			if err := touchCmd.fastboot.Format("cache"); err != nil {
				log.Print("Cache formatting was not successful, flashing may fail, " +
					"check your partitions on device")
			}

			if err := touchCmd.fastboot.BootImage(recovery); err != nil {
				return errors.New("Can't boot recovery image")
			}
		} else {
			if err := touchCmd.fastboot.Flash("recovery", recovery); err != nil {
				return errors.New("can't flash recovery image")
			}

			// Turbo bootloader doesn't support the BootImage command so we have
			// to flash the recovery first and then reboot through a OEM specific
			// command the bootloader offers.
			if err := touchCmd.fastboot.SendOemCommand("reboot recovery"); err != nil {
				return errors.New("Can't reboot device");
			}
		}


		if err := touchCmd.adb.WaitForRecovery(); err != nil {
			return err
		}
	}
	go bitPusher(touchCmd.adb, files, done)
	for i := 0; i < totalFiles; i++ {
		<-done
	}

	var enableList []string
	if touchCmd.DeveloperMode {
		enableList = append(enableList, "developer_mode")
		// provision target device with adbkeys if available
		if adbKeyPath != "" {
			err := touchCmd.adb.Push(adbKeyPath, "/cache/recovery/adbkey.pub")
			if err == nil {
				enableList = append(enableList, "adb_keys adbkey.pub")
			}
		}
	}
	if touchCmd.Password != "" {
		enableList = append(enableList, "default_password "+touchCmd.Password)
	}

	ubuntuCommands, err := ubuntuimage.GetUbuntuCommands(image.Files, cacheDir, touchCmd.Wipe, enableList)
	if err != nil {
		return errors.New("cannot create commands file")
	}
	log.Printf("Created ubuntu_command: %s", ubuntuCommands)

	touchCmd.adb.Push(ubuntuCommands, "/cache/recovery/ubuntu_command")
	defer os.Remove(ubuntuCommands)

	// either customize the flashing process by running a user provided script
	// or reboot into recovery to let the standard upgrade script to run
	if script != "" {
		log.Printf("Preparing to run %s to finish the flashing process\n", script)
		cmd := exec.Command(script)
		cmd.Stdout = os.Stdout
		err = cmd.Run()
		if err != nil {
			return err
		}

	} else {
		log.Print("Rebooting into recovery to flash")
		touchCmd.adb.RebootRecovery()
		err = touchCmd.adb.WaitForRecovery()
		if err != nil {
			return err
		}
	}
	return nil
}

func (touchCmd *TouchCmd) setupDevice() (err error) {
	if adb, err := devices.NewUbuntuDebugBridge(); err == nil {
		touchCmd.adb = adb
	} else {
		return err
	}

	if touchCmd.Serial != "" {
		touchCmd.adb.SetSerial(touchCmd.Serial)
		touchCmd.fastboot.SetSerial(touchCmd.Serial)
	}

	if touchCmd.Device == "" {
		if touchCmd.Bootstrap {
			log.Print("Expecting the device to be in the bootloader... waiting")
			touchCmd.Device, err = touchCmd.fastboot.GetDevice()
			return err
		} else {
			log.Print("Expecting the device to expose an adb interface...")
			// TODO needs to work from recovery as well
			//adb.WaitForDevice()
			touchCmd.Device, err = touchCmd.adb.GetDevice()
			if err != nil {
				return errors.New("device cannot be detected over adb")
			}
		}
	}

	return nil
}

// useLocalTarball adds a local file to the ones to be pushed
func useLocalTarball(file ubuntuimage.File, files chan<- Files) {
	if err := ensureExists(file.Signature); err != nil {
		log.Fatal(err)
	}
	files <- Files{FilePath: file.Path, SigPath: file.Signature}
}

// bitPusher
func bitPusher(adb devices.UbuntuDebugBridge, files <-chan Files, done chan<- bool) {
	if err := adb.Ping(); err != nil {
		log.Fatal("Target device cannot be reached over adb")
	}
	if _, err := adb.Shell("rm -rf /cache/recovery/*.xz /cache/recovery/*.xz.asc"); err != nil {
		log.Fatal("Cannot cleanup /cache/recovery/ to ensure clean deployment", err)
	}
	for {
		file := <-files
		go func() {
			log.Printf("Start pushing %s to device", file.FilePath)
			err := adb.Push(file.FilePath, "/cache/recovery/")
			if err != nil {
				log.Fatal(err)
			}
			err = adb.Push(file.SigPath, "/cache/recovery/")
			if err != nil {
				log.Fatal(err)
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
