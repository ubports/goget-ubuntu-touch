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
	"io/ioutil"
	"log"
	"os"

	"launchpad.net/goget-ubuntu-touch/bootimg"
)

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	imgBytes, err := ioutil.ReadFile(os.Args[1])
	checkError(err)

	boot, err := bootimg.New(imgBytes)
	checkError(err)

	err = boot.WriteRamdisk("ramdisk1.gz")
	checkError(err)
	err = boot.WriteKernel("kernel1")
	checkError(err)
}
