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
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

type RunCmd struct {
	Skin      string `long:"skin" description:"Select skin/emulator type"`
	KernelCmd string `long:"kernel-cmdline" description:"Replace kernel cmdline"`
	Memory    string `long:"memory" description:"Set the device memory"`
	Scale     string `long:"scale" description:"Scale the emulator size"`
	Recovery  bool   `long:"recovery" description:"Boot into recovery"`
}

var runCmd RunCmd

const (
	defaultMemory      = "512"
	defaultScale       = "1.0"
	defaultSkin        = "EDGE"
	installPath        = "/usr/share/android/emulator"
	subpathEmulatorCmd = "out/host/linux-x86/bin/emulator"
)

var skinDirs = []string{
	"skins",
	"/usr/share/ubuntu-emulator/skins",
	"/usr/share/android/emulator/development/tools/emulator/skins",
}

var extendedRunHelp string = "Runs a new emulator instance name 'name' which " +
	"was previously created. If the ANDROID_BUILD_TOP envionment variable is " +
	"found, used during Android side development, the emulator runtime will " +
	"be executed from there if possible. ANDROID_BUILT_TOP is set after an " +
	"android 'lunch' target is selected."

func init() {
	runCmd.Skin = defaultSkin
	runCmd.Scale = defaultScale
	runCmd.Memory = defaultMemory
	parser.AddCommand("run",
		"Run emulator instance named 'name'",
		extendedRunHelp,
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

	device, err := readDeviceStamp(dataDir)
	if err != nil {
		return err
	}
	var deviceInfo map[string]string
	if d, ok := devices[device]; ok {
		deviceInfo = d
	} else {
		return errors.New("Cannot run specified emulator environment")
	}

	ramdisk := bootRamdisk
	if runCmd.Recovery {
		ramdisk = recoveryRamdisk
	}
	cmdOpts := []string{
		"-memory", runCmd.Memory,
		"-skindir", skinDir, "-skin", runCmd.Skin,
		"-sysdir", dataDir,
		"-kernel", filepath.Join(dataDir, kernelName),
		"-ramdisk", filepath.Join(dataDir, ramdisk),
		"-data", filepath.Join(dataDir, dataImage),
		"-system", filepath.Join(dataDir, systemImage),
		"-sdcard", filepath.Join(dataDir, sdcardImage),
		"-cache", filepath.Join(dataDir, cacheImage),
		"-force-32bit", "-no-snapstorage",
		"-gpu", "on",
		"-scale", runCmd.Scale,
		"-no-jni", "-show-kernel", "-verbose",
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

	emulatorCmd := getEmulatorCmd()

	cmd := exec.Command(emulatorCmd, cmdOpts...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func getEmulatorCmd() string {
	androidTree := os.Getenv("ANDROID_BUILD_TOP")
	cmd := path.Join(androidTree, subpathEmulatorCmd)
	if fInfo, err := os.Stat(cmd); err == nil && fInfo.Mode()&0111 != 0 {
		fmt.Println("Using", cmd, "for the emulator runtime")
		return cmd
	}
	return path.Join(installPath, subpathEmulatorCmd)
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
