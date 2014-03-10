//
// Helpers to work with an Ubuntu image based Upgrade implementation
//
// Copyright (c) 2014 Canonical Ltd.
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
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"testing"
)

const develChannelMako = `{
    "global": {
        "generated_at": "Thu Feb 20 10:10:47 UTC 2014"
    },
    "images": [
        {
            "description": "ubuntu=20140206,device=20140115.1,version=166",
            "files": [
                {
                    "checksum": "36deae060a01dea39bb6a42b4be963c19b88e1c600b405061d74cca7d241e34a",
                    "order": 0,
                    "path": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.tar.xz",
                    "signature": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.tar.xz.asc",
                    "size": 329233668
                },
                {
                    "checksum": "cd5e1da151e815f37539c0900f0b5635cb97d7632acfc793bb29c5b490f6d735",
                    "order": 1,
                    "path": "/pool/device-9d396bed22b036d92241420d70b4656ea3ef895a3b43274f936d764081b0d1e2.tar.xz",
                    "signature": "/pool/device-9d396bed22b036d92241420d70b4656ea3ef895a3b43274f936d764081b0d1e2.tar.xz.asc",
                    "size": 37135880
                },
                {
                    "checksum": "9ac8890b6b64ee8c0af24723976cabef16c57c89db262a559ca79af77091e720",
                    "order": 2,
                    "path": "/devel/mako/version-166.tar.xz",
                    "signature": "/devel/mako/version-166.tar.xz.asc",
                    "size": 344
                }
            ],
            "type": "full",
            "version": 150
        },
        {
            "description": "ubuntu=20140206,device=20140115.1,version=166",
            "files": [
                {
                    "checksum": "36deae060a01dea39bb6a42b4be963c19b88e1c600b405061d74cca7d241e34a",
                    "order": 0,
                    "path": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.tar.xz",
                    "signature": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.tar.xz.asc",
                    "size": 329233668
                },
                {
                    "checksum": "cd5e1da151e815f37539c0900f0b5635cb97d7632acfc793bb29c5b490f6d735",
                    "order": 1,
                    "path": "/pool/device-9d396bed22b036d92241420d70b4656ea3ef895a3b43274f936d764081b0d1e2.tar.xz",
                    "signature": "/pool/device-9d396bed22b036d92241420d70b4656ea3ef895a3b43274f936d764081b0d1e2.tar.xz.asc",
                    "size": 37135880
                },
                {
                    "checksum": "9ac8890b6b64ee8c0af24723976cabef16c57c89db262a559ca79af77091e720",
                    "order": 2,
                    "path": "/devel/mako/version-166.tar.xz",
                    "signature": "/devel/mako/version-166.tar.xz.asc",
                    "size": 344
                }
            ],
            "type": "full",
            "version": 166
        },
        {
            "base": 161,
            "description": "ubuntu=20140206,device=20140115.1,version=166",
            "files": [
                {
                    "checksum": "621b201190c3a5ee0b2a4adc7f9823dbca6168374d03646d774e2044f43410f9",
                    "order": 0,
                    "path": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.delta-ubuntu-d204c93d737253aa07088ffd7e5d060ad28e205c32d008379b4d70ec5f7c3ca4.tar.xz",
                    "signature": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.delta-ubuntu-d204c93d737253aa07088ffd7e5d060ad28e205c32d008379b4d70ec5f7c3ca4.tar.xz.asc",
                    "size": 23963072
                },
                {
                    "checksum": "9ac8890b6b64ee8c0af24723976cabef16c57c89db262a559ca79af77091e720",
                    "order": 1,
                    "path": "/devel/mako/version-166.tar.xz",
                    "signature": "/devel/mako/version-166.tar.xz.asc",
                    "size": 344
                }
            ],
            "type": "delta",
            "version": 166
        }
	]
}
`

const develChannelMakoOnlyDelta = `{
    "global": {
        "generated_at": "Thu Feb 20 10:10:47 UTC 2014"
    },
    "images": [
        {
            "base": 161,
            "description": "ubuntu=20140206,device=20140115.1,version=166",
            "files": [
                {
                    "checksum": "621b201190c3a5ee0b2a4adc7f9823dbca6168374d03646d774e2044f43410f9",
                    "order": 0,
                    "path": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.delta-ubuntu-d204c93d737253aa07088ffd7e5d060ad28e205c32d008379b4d70ec5f7c3ca4.tar.xz",
                    "signature": "/pool/ubuntu-7b59094905d81305ee88d22f6468fd1ce74b515a3be8098848a90c8062a0e192.delta-ubuntu-d204c93d737253aa07088ffd7e5d060ad28e205c32d008379b4d70ec5f7c3ca4.tar.xz.asc",
                    "size": 23963072
                },
                {
                    "checksum": "9ac8890b6b64ee8c0af24723976cabef16c57c89db262a559ca79af77091e720",
                    "order": 1,
                    "path": "/devel/mako/version-166.tar.xz",
                    "signature": "/devel/mako/version-166.tar.xz.asc",
                    "size": 344
                }
            ],
            "type": "delta",
            "version": 166
        }
	]
}
`
const channels = `{
    "devel": {
        "alias": "trusty",
        "devices": {
            "flo": {
                "index": "/devel/flo/index.json"
            },
            "generic": {
                "index": "/devel/generic/index.json"
            },
            "goldfish": {
                "index": "/devel/goldfish/index.json"
            },
            "grouper": {
                "index": "/devel/grouper/index.json"
            },
            "maguro": {
                "index": "/devel/maguro/index.json"
            },
            "mako": {
                "index": "/devel/mako/index.json"
            },
            "manta": {
                "index": "/devel/manta/index.json"
            }
        }
    },
    "devel-customized": {
        "alias": "trusty-customized",
        "devices": {
            "flo": {
                "index": "/devel-customized/flo/index.json"
            },
            "generic": {
                "index": "/devel-customized/generic/index.json"
            },
            "goldfish": {
                "index": "/devel-customized/goldfish/index.json"
            },
            "grouper": {
                "index": "/devel-customized/grouper/index.json"
            },
            "maguro": {
                "index": "/devel-customized/maguro/index.json"
            },
            "mako": {
                "index": "/devel-customized/mako/index.json"
            },
            "manta": {
                "index": "/devel-customized/manta/index.json"
            }
        }
    }
}`

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type DeviceChannelsSuite struct {
	channels Channels
	devices  map[string]Device
	ts       *httptest.Server
}

var _ = Suite(&DeviceChannelsSuite{})

func (s *DeviceChannelsSuite) SetUpTest(c *C) {
	s.devices = make(map[string]Device)
	s.channels = make(map[string]Channel)
	s.channels["trusty"] = Channel{
		Devices: map[string]Device{"mako": Device{"/" + "trusty/mako/index.json"}}}
	s.channels["touch/trusty"] = Channel{
		Devices: map[string]Device{"mako": Device{"/" + "touch/trusty/mako/index.json"}}}
	s.channels["touch/devel"] = Channel{
		Devices: map[string]Device{"mako": Device{"/" + "touch/devel/mako/index.json"}},
		Alias:   "touch/trusty"}
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, develChannelMako)
	}))
}

func (s *DeviceChannelsSuite) TestChannelNotFoundWithNoChannels(c *C) {
	s.channels = make(map[string]Channel)
	channel := "trusty"
	device := "mako"
	expectedErr := fmt.Errorf("Channel %s not found on server %s", channel, s.ts.URL)
	_, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, DeepEquals, expectedErr)
}

func (s *DeviceChannelsSuite) TestChannelDeviceNotFoundInChannel(c *C) {
	device := "hammerhead"
	channel := "trusty"
	expectedErr := fmt.Errorf("Device %s not found on server %s channel %s",
		device, s.ts.URL, channel)
	_, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, DeepEquals, expectedErr)
}

func (s *DeviceChannelsSuite) TestChannelDeviceNotFoundInChannelWithSlash(c *C) {
	device := "hammerhead"
	channel := "touch/trusty"
	expectedErr := fmt.Errorf("Device %s not found on server %s channel %s",
		device, s.ts.URL, channel)
	_, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, DeepEquals, expectedErr)
}

func (s *DeviceChannelsSuite) TestChannelInvalidDataForDevice(c *C) {
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Invalid data")
	}))
	device := "mako"
	channel := "touch/trusty"
	expectedErr := fmt.Errorf("Cannot parse channel information for device on %s",
		s.ts.URL+"/"+channel+"/"+device+"/index.json")
	_, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, DeepEquals, expectedErr)
}

func (s *DeviceChannelsSuite) TestChannelValidDataForDevice(c *C) {
	device := "mako"
	channel := "touch/trusty"
	images, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(images, NotNil)
}

func (s *DeviceChannelsSuite) TestChannelValidDataForDeviceFromAlias(c *C) {
	device := "mako"
	channel := "touch/devel"
	channelData, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(channelData, NotNil)
	c.Assert(channelData.Alias, Equals, "touch/trusty")
	c.Assert(channelData.Images, NotNil)
}

func (s *DeviceChannelsSuite) TestGetLatestImageForChannel(c *C) {
	device := "mako"
	channel := "touch/devel"
	channelData, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(channelData, NotNil)
	c.Assert(channelData.Alias, Equals, "touch/trusty")
	c.Assert(channelData.Images, NotNil)
	image, err := channelData.GetLatestImage()
	c.Assert(err, IsNil)
	c.Check(image.Version, Equals, 166)
	c.Check(image.Type, Equals, "full")
	c.Check(len(image.Files), Equals, 3)
}

func (s *DeviceChannelsSuite) TestFailsGetLatestImageForChannelWithOnlyDeltas(c *C) {
	device := "mako"
	channel := "touch/devel"
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, develChannelMakoOnlyDelta)
	}))
	channelData, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(channelData, NotNil)
	c.Assert(channelData.Alias, Equals, "touch/trusty")
	c.Assert(channelData.Images, NotNil)
	expectedErr := errors.New("Failed to locate latest image information")
	_, err = channelData.GetLatestImage()
	c.Assert(err, DeepEquals, expectedErr)
}

func (s *DeviceChannelsSuite) TestGetSpecificImageForChannel(c *C) {
	device := "mako"
	channel := "touch/devel"
	channelData, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(channelData, NotNil)
	c.Assert(channelData.Alias, Equals, "touch/trusty")
	c.Assert(channelData.Images, NotNil)
	image, err := channelData.GetImage(166)
	c.Assert(err, IsNil)
	c.Check(image.Version, Equals, 166)
	c.Check(image.Type, Equals, "full")
	c.Check(len(image.Files), Equals, 3)
}

func (s *DeviceChannelsSuite) TestFailGetSpecificImageForChannel(c *C) {
	device := "mako"
	channel := "touch/devel"
	channelData, err := s.channels.GetDeviceChannel(s.ts.URL, channel, device)
	c.Assert(err, IsNil)
	c.Assert(channelData, NotNil)
	c.Assert(channelData.Alias, Equals, "touch/trusty")
	c.Assert(channelData.Images, NotNil)
	rev := 162
	expectedErr := fmt.Errorf("Failed to locate image %d", rev)
	_, err = channelData.GetImage(rev)
	c.Assert(err, DeepEquals, expectedErr)
}

type ChannelsSuite struct {
	ts *httptest.Server
}

var _ = Suite(&ChannelsSuite{})

func (s *ChannelsSuite) SetUpTest(c *C) {
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, channels)
	}))
}

func (s *ChannelsSuite) TestGetChannelsFromServer(c *C) {
	channels, err := NewChannels(s.ts.URL)
	c.Assert(err, IsNil)
	c.Assert(channels, NotNil)
	devel, develOk := channels["devel"]
	c.Assert(develOk, Equals, true)
	c.Check(devel.Alias, Equals, "trusty")
	c.Check(len(devel.Devices), Equals, 7)
	device, deviceOk := devel.Devices["flo"]
	c.Assert(deviceOk, Equals, true)
	c.Check(device.Index, Equals, "/devel/flo/index.json")
}

func (s *ChannelsSuite) TestInvalidDataWhenGetChannelsFromServer(c *C) {
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Invalid data")
	}))
	expectedErr := fmt.Errorf("Unable to parse channel information from %s", s.ts.URL)
	_, err := NewChannels(s.ts.URL)
	c.Assert(err, DeepEquals, expectedErr)
}
