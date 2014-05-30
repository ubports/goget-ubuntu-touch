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
	"fmt"

	"log"

	"launchpad.net/goget-ubuntu-touch/devices"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

type FactoryResetCmd struct {
	DeveloperMode bool   `long:"developer-mode" description:"Enables developer mode after the factory reset"`
	Serial        string `long:"serial" description:"Serial of the device to operate"`
}

var factoryResetCmd FactoryResetCmd

func init() {
	parser.AddCommand("factory-reset",
		"Resets to stock install",
		"Resets a device to its stock install with the possibility to enable developer mode by default",
		&factoryResetCmd)
}

func (factoryResetCmd *FactoryResetCmd) Execute(args []string) error {
	var enableList []string
	if factoryResetCmd.DeveloperMode {
		enableList = append(enableList, "developer_mode")
	}
	//files: nil, files location: "", wipe: true, enable: enableList
	ubuntuCommands, err := ubuntuimage.GetUbuntuCommands(nil, "", true, enableList)
	if err != nil {
		return fmt.Errorf("cannot create commands file: %s", err)
	}

	adb, err := devices.NewUbuntuDebugBridge()
	if err != nil {
		log.Fatal(err)
	}
	if factoryResetCmd.Serial != "" {
		adb.SetSerial(factoryResetCmd.Serial)
	}
	if err := adb.Push(ubuntuCommands, "/cache/recovery/ubuntu_command"); err != nil {
		return err
	}
	fmt.Println("Rebooting to finish factory reset")
	adb.RebootRecovery()
	return nil
}
