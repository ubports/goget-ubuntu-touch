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

import (
	"errors"
	"fmt"

	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

func init() {
	parser.AddCommand("query",
		"Run queries against the image server",
		"Choose from the list of query options to retrieve information from the server",
		&queryCmd)

	//queryCmd.AddCommand("list-channels", "List available channels", "", &channelCmd)
}

type QueryCmd struct {
	ListChannels bool   `long:"list-channels" description:"List available channels"`
	ListImages   bool   `long:"list-images" description:"List available images for a channel"`
	ShowImage    bool   `long:"show-image" description:"Show information for an image in the given channel"`
	Channel      string `long:"channel" description:"Specify an alternate channel"`
	Device       string `long:"device" description:"Specify the device to use as a base for querying" required:"true"`
}

var queryCmd QueryCmd

func (queryCmd *QueryCmd) Execute(args []string) error {
	if globalArgs.TLSSkipVerify {
		ubuntuimage.TLSSkipVerify()
	}

	if queryCmd.ListChannels {
		return queryCmd.printChannelList()
	}

	if queryCmd.ShowImage {
		return queryCmd.printImageInformation()
	}

	if queryCmd.ListImages {
		return queryCmd.printImageList()
	}

	return errors.New("A query option is requrired")
}

func (queryCmd *QueryCmd) printImageList() error {
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	deviceChannel, err := channels.GetDeviceChannel(
		globalArgs.Server, queryCmd.Channel, queryCmd.Device)
	if err != nil {
		return err
	}

	return deviceChannel.ListImageVersions()
}

func (queryCmd *QueryCmd) printChannelList() error {
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	for k, v := range channels {
		if _, ok := v.Devices[queryCmd.Device]; !ok {
			continue
		}

		if !v.Hidden {
			if v.Alias != "" {
				fmt.Printf("%s (alias to %s)\n", k, v.Alias)
			} else {
				fmt.Println(k)
			}
		}
	}

	return nil
}

func (queryCmd *QueryCmd) printImageInformation() error {
	channels, err := ubuntuimage.NewChannels(globalArgs.Server)
	if err != nil {
		return err
	}

	if queryCmd.Channel == "" {
		return errors.New("channel required")
	}

	deviceChannel, err := channels.GetDeviceChannel(globalArgs.Server, queryCmd.Channel, queryCmd.Device)
	if err != nil {
		return err
	}

	image, err := getImage(deviceChannel)
	if err != nil {
		return err
	}

	fmt.Printf("Device: %s\nDescription: %s\nVersion: %d\nChannel: %s\nFiles:\n", queryCmd.Device, image.Description, image.Version, queryCmd.Channel)
	for _, f := range image.Files {
		f.MakeRelativeToServer(globalArgs.Server)
		fmt.Printf(" %d %s%s %d %s\n", f.Order, f.Server, f.Path, f.Size, f.Checksum)
	}

	return nil
}
