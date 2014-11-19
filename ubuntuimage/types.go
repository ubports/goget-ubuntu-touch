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

type Device struct {
	Index string
}

type Channel struct {
	Devices map[string]Device `json:"devices"`
	Alias   string            `json:"alias"`
	Hidden  bool              `json:"hidden,omitempty"`
}

type Channels map[string]Channel

type ImageVersion struct {
	Description string
}

type ImageVersions map[int]ImageVersion

type File struct {
	Server                    string
	Checksum, Path, Signature string
	Size, Order               int
}

type Image struct {
	Description, Type string
	Version           int
	Files             []File
}

type DeviceChannel struct {
	Url    string
	Alias  string
	Images []Image
}
