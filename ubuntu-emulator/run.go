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
	"launchpad.net/goget-ubuntu-touch/ubuntu-emulator/sysutils"
	"os"
	"os/exec"
	"path/filepath"
)

type RunCmd struct {
	Skin      string `long:"skin" description:"Select skin/emulator type"`
	KernelCmd string `long:"kernel-cmdline" description:"Replace kernel cmdline"`
	Memory    string `long:"memory" description:"Set the device memory"`
	Scale	  string `long:"scale" description:"Scale the emulator size"`
}

var runCmd RunCmd

const (
	defaultMemory    = "512"
	defaultScale     = "1.0"
	defaultSkin      = "EDGE"
	emulatorCmd      = "/usr/share/android/emulator/out/host/linux-x86/bin/emulator"
)

var	skinDirs = []string {
	"skins",
	"/usr/share/ubuntu-emulator/skins",
	"/usr/share/android/emulator/development/tools/emulator/skins",
}

func init() {
	runCmd.Skin = defaultSkin
	runCmd.Scale = defaultScale
	runCmd.Memory = defaultMemory
	parser.AddCommand("run",
		"Run emulator instance named 'name'",
		"Runs a new emulator instance name 'name' which was previously created",
		&runCmd)
}

func (runCmd *RunCmd) Execute(args []string) error {
	if len(args) != 1 {
		return errors.New("Instance name 'name' is required")
	}
	instanceName := args[0]
	dataDir := getInstanceDataDir(instanceName)
	if instanceExists(dataDir) != true {
		return errors.New(fmt.Sprintf("This instance does not exist, use 'create %s' to create it", instanceName))
	}

	skinDir, err := getSkinDir(runCmd.Skin)
	if err != nil {
		return err
	}

	device, err := sysutils.ReadDeviceStamp(dataDir)
	if err != nil {
		return err
	}
	var deviceInfo map[string]string
	if d, ok := devices[device]; ok {
		deviceInfo = d
	} else {
		return errors.New("Cannot run specified emulator environment")
	}

	cmdOpts := []string{
		"-memory", runCmd.Memory,
		"-skindir", skinDir, "-skin", runCmd.Skin,
		"-sysdir", dataDir,
		"-kernel", filepath.Join(dataDir, kernelName),
		"-data", filepath.Join(dataDir, dataImage),
		"-system", filepath.Join(dataDir, systemImage),
		"-sdcard", filepath.Join(dataDir, sdcardImage),
		"-cache", filepath.Join(dataDir, cacheImage),
		"-force-32bit", "-no-snapstorage",
		"-gpu", "on",
		"-scale", runCmd.Scale,
		"-shell", "-no-jni", "-show-kernel", "-verbose",
		"-qemu",
	}
	
	if cpu, ok := deviceInfo["cpu"]; ok {
		cmdOpts = append(cmdOpts, []string{"-cpu", cpu}...)
	}
	if runCmd.KernelCmd != "" {
		cmdOpts = append(cmdOpts, []string{"-append", runCmd.KernelCmd}...)
	}

	//we need to export ANDROID_PRODUCT_OUT so the emulator command can create the
	//correct hardware-qemu.ini
	if err := os.Setenv("ANDROID_PRODUCT_OUT", dataDir); err != nil {
		return err
	}

	cmd := exec.Command(emulatorCmd, cmdOpts...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func getSkinDir(skin string) (string, error) {
	for _, skinDir := range skinDirs {
		if dir, err := os.Stat(skinDir); err != nil || !dir.IsDir() {
			continue
		}
		skinPath := filepath.Join(skinDir, skin)
		if dir, err := os.Stat(skinPath); err != nil || !dir.IsDir() {
			continue
		}
		return skinDir, nil
	}
	return "", errors.New(fmt.Sprintf("Cannot find skin %s in any directory from path %s", skin, skinDirs))
}

