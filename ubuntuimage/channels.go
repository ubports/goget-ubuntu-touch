//
// Helpers to work with an Ubuntu image based Upgrade implementation
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package ubuntuimage

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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const channelsPath = "/channels.json"

func NewChannels(server string) (channels Channels, err error) {
	resp, err := http.Get(server + channelsPath)
	if err != nil {
		return channels, err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&channels); err != nil {
		return channels, fmt.Errorf("Unable to parse channel information from %s", server)
	}
	return channels, nil
}

func (channels Channels) GetDeviceChannel(server, channel, device string) (deviceChannel DeviceChannel, err error) {
	if _, found := channels[channel]; !found {
		return deviceChannel, fmt.Errorf("Channel %s not found on server %s", channel, server)
	} else if _, found := channels[channel].Devices[device]; !found {
		return deviceChannel, fmt.Errorf("Device %s not found on server %s channel %s",
			device, server, channel)
	}
	channelUri := server + channels[channel].Devices[device].Index
	resp, err := http.Get(channelUri)
	if err != nil {
		return deviceChannel, err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&deviceChannel)
	if err != nil {
		return deviceChannel, fmt.Errorf("Cannot parse channel information for device on %s", channelUri)
	}
	deviceChannel.Alias = channels[channel].Alias
	order := func(i1, i2 *Image) bool {
		return i1.Version > i2.Version
	}
	ImageBy(order).ImageSort(deviceChannel.Images)
	return deviceChannel, err
}

func (deviceChannel *DeviceChannel) GetImage(revision int) (image Image, err error) {
	for _, image := range deviceChannel.Images {
		if image.Type == "full" && image.Version == revision {
			return image, nil
		}
	}
	//If we reached this point, that means we haven't found the image we were looking for.
	return image, fmt.Errorf("Failed to locate image %d", revision)
}

func (deviceChannel *DeviceChannel) GetRelativeImage(revision int) (image Image, err error) {
	var steps int
	if revision < 0 {
		revision = -revision
	}
	for _, image := range deviceChannel.Images {
		if image.Type != "full" {
			continue
		}
		if steps == revision {
			return image, nil
		}
		steps++
	}
	//If we reached this point, that means we haven't found the image we were looking for.
	if revision == 0 {
		err = errors.New("Failed to locate latest image information")
	} else {
		err = fmt.Errorf("Failed to locate relative image to latest - %d", revision)
	}
	return Image{}, err
}
