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
	"path/filepath"
)

type RunCmd struct {
	Skin      string `long:"skin" description:"Select skin/emulator type"`
	KernelCmd string `long:"kernel-cmdline" description:"Replace kernel cmdline"`
	Memory    string `long:"memory" description:"Set the device memory"`
}

var runCmd RunCmd

const (
	defaultMemory    = "512"
	defaultSkin      = "WVGA800"
	skinDir          = "/usr/share/android/emulator/development/tools/emulator/skins"
	emulatorCmd      = "/usr/share/android/emulator/out/host/linux-x86/bin/emulator"
)

func init() {
	runCmd.Skin = defaultSkin
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

	cmd := exec.Command(emulatorCmd,
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
		"-shell", "-no-jni", "-show-kernel", "-verbose",
		"-qemu",
		"-cpu", cpu,
		"-append", runCmd.KernelCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
