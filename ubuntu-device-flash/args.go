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
	flags "github.com/jessevdk/go-flags"
)

type arguments struct {
	Revision      int    `long:"revision" description:"revision to flash, absolute or relative allowed"`
	Bootstrap     bool   `long:"bootstrap" description:"bootstrap the system, do this from the bootloader"`
	ListChannels  bool   `long:"list-channels" description:"List available channels"`
	Wipe          bool   `long:"wipe" description:"Clear all data after flashing"`
	Channel       string `long:"channel" description:"Specify an alternate channel"`
	Device        string `long:"device" description:"Specify the device to flash"`
	DeviceTarball string `long:"device-tarball" description:"Specify a local device tarball to override the one from the server (using official Ubuntu images with custom device tarballs)"`
	DownloadOnly  bool   `long:"download-only" description:"Only download tarballs, do not push to the device."`
	Serial        string `long:"serial" description:"Serial of the device to operate"`
	Server        string `long:"server" description:"Use a different image server"`
	CleanCache    bool   `long:"clean-cache" description:"Cleans up cache with all downloaded bits"`
	TLSSkipVerify bool   `long:"tls-skip-verify" description:"Skip TLS certificate validation"`
	DeveloperMode bool   `long:"developer-mode" description:"Enables developer mode after the factory reset"`
	RunScript     string `long:"run-script" description:"Run a script given by path to finish the flashing process, instead of rebooting to recovery (mostly used during development to work around quirky or incomplete recovery images)"`
	Password      string `long:"password" description:"This sets up the default password for the phablet user. This option is meant for CI and not general use"`
}

var args arguments
var parser = flags.NewParser(&args, flags.Default)

const (
	defaultChannel = "ubuntu-touch/stable"
	defaultServer  = "https://system-image.ubuntu.com"
)

func init() {
	args.Channel = defaultChannel
	args.Server = defaultServer
}
