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
)

type DestroyCmd struct {
	Yes bool `long:"yes" description:"Assume yes"`
}

var destroyCmd DestroyCmd

func init() {
	parser.AddCommand("destroy",
		"Destroys an emulator instance named 'name'",
		"Destroys an emulator instance name 'name' which was previously created",
		&destroyCmd)
}

func (destroyCmd *DestroyCmd) Execute(args []string) error {
	if len(args) != 1 {
		return errors.New("Instance name 'name' is required")
	}
	instanceName := args[0]
	dataDir := getInstanceDataDir(instanceName)

	if instanceExists(dataDir) != true {
		return errors.New(fmt.Sprintf("This instance does not exist, use 'create %s' to create it", instanceName))
	}

	if destroyCmd.Yes == false {
		dialog := fmt.Sprintf("Are you sure you want to remove instance '%s' located in %s?\n[y/n] ", instanceName, dataDir)
		action, err := confirmDestroy(dialog)
		if err != nil {
			return err
		}
		if action != true {
			return nil
		}
	}
	return os.RemoveAll(dataDir)
}

func confirmDestroy(dialog string) (bool, error) {
	yes := []string{"y", "Y"}
	no := []string{"n", "N"}
	var resp string
	fmt.Print(dialog)
	_, err := fmt.Scanln(&resp)
	if err != nil {
		return false, err
	}
	if containsString(resp, yes) {
		return true, nil
	} else if containsString(resp, no) {
		return false, nil
	} else {
		return confirmDestroy(dialog)
	}
}

func containsString(resp string, list []string) bool {
	for _, i := range list {
		if i == resp {
			return true
		}
	}
	return false
}
