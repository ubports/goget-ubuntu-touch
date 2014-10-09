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
	"path/filepath"

	"launchpad.net/goget-ubuntu-touch/diskimage"
)

type SnapshotCmd struct {
	Revert   string `long:"revert" description:"Reverts to the selected snapshot"`
	Create   string `long:"create" description:"Creates the selected snapshot"`
	Pristine bool   `long:"revert-pristine" description:"Reverts to the originally created snapshot"`
}

var snapshotCmd SnapshotCmd

func init() {
	parser.AddCommand("snapshot",
		"Manipulates snapshots for emulator instance named 'name'",
		"Create and revert to snapshots of the emulator instance name 'name' which was previously created",
		&snapshotCmd)
}

func (snapshotCmd *SnapshotCmd) Execute(args []string) error {
	if len(args) != 1 {
		return errors.New("Instance name 'name' is required")
	}
	if snapshotCmd.Pristine && snapshotCmd.Create != "" && snapshotCmd.Revert != "" {
		return errors.New("Only one option can be selected")
	}
	instanceName := args[0]
	dataDir := getInstanceDataDir(instanceName)

	if instanceExists(dataDir) != true {
		return errors.New(fmt.Sprintf("This instance does not exist, use 'create %s' to create it", instanceName))
	}

	if snapshotCmd.Pristine {
		snapshotCmd.Revert = diskimage.BaseSnapshot
	}

	sdcardImage := diskimage.NewExisting(filepath.Join(dataDir, "sdcard.img"))
	systemImage := diskimage.NewExisting(filepath.Join(dataDir, "system.img"))
	images := []*diskimage.DiskImage{systemImage, sdcardImage}

	if snapshotCmd.Revert != "" {
		for _, img := range images {
			if err := img.RevertSnapshot(snapshotCmd.Revert); err != nil {
				return err
			}
		}
	} else if snapshotCmd.Create != "" {
		for _, img := range images {
			if err := img.Snapshot(snapshotCmd.Create); err != nil {
				return err
			}
		}
	} else {
		errors.New("Command not implemented or supported")
	}

	return nil
}
