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
	"io/ioutil"
	"path/filepath"
)

type ListCmd struct {
	// TODO Verbose bool   `long:"verbose" description:"Shows additional information from instances listed"`
}

var listCmd ListCmd

func init() {
	parser.AddCommand("list",
		"Lists emulator instances",
		"List emulator instances with information on what was used to create them",
		&listCmd)
}

func (listCmd *ListCmd) Execute(args []string) error {
	dataDir := getDataDir()
	instanceList, err := ioutil.ReadDir(dataDir)
	if err != nil {
		return err
	}
	for _, entry := range instanceList {
		if !entry.IsDir() {
			continue
		}
		if image, err := readStamp(filepath.Join(dataDir, entry.Name())); err == nil {
			fmt.Printf("%s\t%s\n", entry.Name(), image.Description)
		} else {
			fmt.Println(entry.Name())
		}

	}
	return nil
}
