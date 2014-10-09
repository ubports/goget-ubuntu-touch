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

import flags "github.com/jessevdk/go-flags"

type arguments struct {
	Revision      int    `long:"revision" description:"revision to use, absolute or relative allowed"`
	Channel       string `long:"channel" description:"(DEPRECATED) Specify the channel to use" default:"ubuntu-touch/stable"`
	DownloadOnly  bool   `long:"download-only" description:"Only download."`
	Server        string `long:"server" description:"Use a different image server" default:"https://system-image.ubuntu.com"`
	CleanCache    bool   `long:"clean-cache" description:"Cleans up cache with all downloaded bits"`
	TLSSkipVerify bool   `long:"tls-skip-verify" description:"Skip TLS certificate validation"`
}

var globalArgs arguments
var parser = flags.NewParser(&globalArgs, flags.Default)
