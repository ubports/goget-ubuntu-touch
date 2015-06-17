//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
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

func init() {
	parser.AddCommand("personal",
		"Creates ubuntu personal images",
		"",
		&personalCmd)
}

type PersonalCmd struct {
	Size int64 `long:"size" short:"s" description:"Size of image file to create in GB (min 10)" default:"10"`

	Snapper
}

var personalCmd PersonalCmd

func (personalCmd *PersonalCmd) Execute(args []string) error {
	personalCmd.flavor = flavorPersonal
	personalCmd.size = personalCmd.Size

	return personalCmd.create()
}
