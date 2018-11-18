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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubports/goget-ubuntu-touch/bootimg"
)

func getDeviceTar(files []string) (string, error) {
	for _, file := range files {
		if strings.Contains(file, "device") {
			return file, nil
		}
	}
	return "", errors.New("Could not find device specific tar")
}

func extractBoot(dataDir string, bootName string, ramdiskName string) error {
	bootPath := filepath.Join(dataDir, bootName)
	imgBytes, err := ioutil.ReadFile(bootPath)
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot read %s", bootPath))
	}
	boot, err := bootimg.New(imgBytes)
	if err != nil {
		return err
	}
	ramdiskPath := filepath.Join(dataDir, ramdiskName)
	if err := boot.WriteRamdisk(ramdiskPath); err != nil {
		return err
	}
	if ramdiskName == bootRamdisk {
		kernelPath := filepath.Join(dataDir, kernelName)
		if boot.WriteKernel(kernelPath); err != nil {
			return err
		}
	}
	return nil
}

func instanceExists(path string) bool {
	f, err := os.Stat(path)
	if err == nil {
		if f.IsDir() {
			return true
		}
	}
	return false
}

func getDataDir() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = "$HOME/.local/share"
	}
	return filepath.Join(os.ExpandEnv(dataDir), filepath.Base(os.Args[0]))
}

func getInstanceDataDir(instanceName string) (dataDir string) {
	return filepath.Join(getDataDir(), instanceName)
}
