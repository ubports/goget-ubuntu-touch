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

//Global constansts
const (
	kernelName  = "ubuntu-kernel"
	dataImage   = "userdata.img"
	sdcardImage = "sdcard.img"
	systemImage = "system.img"
	cacheImage  = "cache.img"
)

var devices map[string]map[string]string

func init() {
	devices = map[string]map[string]string{
		"i386": {
			"name": "generic_x86",
			"memory": "2048",
		},
		"armhf": {
			"name": "generic",
			"memory": "512",
			"cpu": "cortex-a9",
		},
	}
}

