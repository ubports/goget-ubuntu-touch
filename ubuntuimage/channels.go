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
	_ "crypto/sha512"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	channelsPath = "/channels.json"
	indexName    = "index.json"
	FULL_IMAGE   = "full"
)

var client = &http.Client{}

// TLSSkipVerify turns off validation of server TLS certificates. It allows connecting
// to HTTPS servers that use self-signed certificates.
func TLSSkipVerify() {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client = &http.Client{Transport: tr}
}

func NewChannels(server string) (channels Channels, err error) {
	resp, err := client.Get(server + channelsPath)
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
	resp, err := client.Get(channelUri)
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

	deviceChannel.Url = channelUri
	return deviceChannel, err
}

func (deviceChannel *DeviceChannel) GetImage(revision int) (image Image, err error) {
	for _, image := range deviceChannel.Images {
		if image.Type == FULL_IMAGE && image.Version == revision {
			return image, nil
		}
	}
	//If we reached this point, that means we haven't found the image we were looking for.
	return image, fmt.Errorf("Failed to locate image %d", revision)
}

func (deviceChannel *DeviceChannel) ListImageVersions() (err error) {

	jsonData := map[string]interface{}{}

	resp, err := client.Get(deviceChannel.Url)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		statusErr := errors.New(fmt.Sprintf("Invalid HTTP response: %d", resp.StatusCode))
		return statusErr
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		return err
	}

	images := jsonData["images"].([]interface{})

	for i := range images {
		entry := images[i].(map[string]interface{})

		imageType := entry["type"]

		if imageType != FULL_IMAGE {
			// ignore delta images as they cannot be used to
			// perform an initial device flash
			continue
		}

		fmt.Printf("%d: description='%s'\n",
			int(entry["version"].(float64)),
			entry["description"])
	}

	return nil
}

func (deviceChannel *DeviceChannel) GetRelativeImage(revision int) (image Image, err error) {
	var steps int
	if revision < 0 {
		revision = -revision
	}
	for _, image := range deviceChannel.Images {
		if image.Type != FULL_IMAGE {
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
