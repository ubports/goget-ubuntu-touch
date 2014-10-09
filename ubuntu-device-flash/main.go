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
	"fmt"
	"os"

	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

var cacheDir = ubuntuimage.GetCacheDir()

func main() {
	args := os.Args

	parser.SubcommandsOptional = true
	if _, err := parser.ParseArgs(args); err != nil {
		os.Exit(1)
	}

	if parser.Active == nil {
		fmt.Println("DEPRECATED: Implicit 'touch' subcommand assumed")
		touchCmd.Channel = globalArgs.Channel
		if err := touchCmd.Execute(args); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}
