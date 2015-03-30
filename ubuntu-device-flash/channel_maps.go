//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>

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

const (
	channelCoreEdge  = "edge"
	channelCoreAlpha = "alpha"
	channelCoreBeta  = "beta"
)

const (
	channelCoreDevel         = "ubuntu-core/devel"
	channelCoreDevelProposed = "ubuntu-core/devel-proposed"
)

func systemImageChannel(channel string) string {
	switch channel {
	case channelCoreEdge:
		return channelCoreDevelProposed
	case channelCoreAlpha:
		return channelCoreDevel
	}

	return channel
}

const (
	archArmhf = "armhf"
	archAmd64 = "amd64"
	archi386  = "i386"
)

const (
	deviceArmhf = "generic_armhf"
	deviceAmd64 = "generic_amd64"
	devicei386  = "generic_i386"
)

func systemImageDeviceChannel(arch string) string {
	switch arch {
	case archArmhf:
		return deviceArmhf
	case archAmd64:
		return deviceAmd64
	case archi386:
		return devicei386
	}

	return arch
}
