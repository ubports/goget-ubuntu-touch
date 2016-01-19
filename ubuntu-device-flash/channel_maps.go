//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>

package main

import "path"

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

func systemImageChannel(flavor, release, channel string) string {
	return path.Join(flavor, release, channel)
}

const (
	archArmhf = "armhf"
	archArm64 = "arm64"
	archAmd64 = "amd64"
	archi386  = "i386"
)

const (
	deviceArmhf = "generic_armhf"
	deviceArm64 = "generic_arm64"
	deviceAmd64 = "generic_amd64"
	devicei386  = "generic_i386"
)

func systemImageDeviceChannel(arch string) string {
	switch arch {
	case archArmhf:
		return deviceArmhf
	case archArm64:
		return deviceArm64
	case archAmd64:
		return deviceAmd64
	case archi386:
		return devicei386
	}

	return arch
}
