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
	"flag"
	"log"
)

type arguments struct {
	revision                        *int
	bootstrap, listChannels, wipe   *bool
	channel, device, serial, server *string
}

var args arguments

func init() {
	args.bootstrap = flag.Bool("bootstrap", false, "Bootstrap the system, do this from fastboot")
	args.wipe = flag.Bool("wipe", false, "Clear all data after flashing")
	args.revision = flag.Int("revision", 0, "Revision to flash, 0 is current, "+
		"use explicit version number or negative relative ones to current")
	args.channel = flag.String("channel", "stable", "Select channel to flash")
	args.device = flag.String("device", "", "Select device to flash")
	args.serial = flag.String("serial", "", "Serial of the device to operate")
	args.server = flag.String("server", "https://system-image.ubuntu.com",
		"Select image server")
	args.listChannels = flag.Bool("list-channels", false, "List available channels")
	flag.Parse()
	if *args.bootstrap && *args.wipe {
		log.Fatal("Cannot bootstrap and wipe at the same time")
	}
}
