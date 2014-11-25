//
// ubuntu-emu - Tool to download and run Ubuntu Touch emulator instances
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const networkConfig string = `# interfaces(5) file used by ifup(8) and ifdown(8)
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 10.0.2.15
    netmask 255.255.255.0
    gateway 10.0.2.2
    dns-nameservers 10.0.2.3

iface eth1 inet manual
iface eth2 inet manual
iface eth3 inet manual
iface eth4 inet manual
iface eth5 inet manual
`

type setupFile struct{ path, content string }

var setupFiles = []setupFile{
	{"custom/custom.prop", "custom.location.fake=true"},
	{"etc/network/interfaces", networkConfig},
	{"etc/profile.d/hud-service.sh", "export HUD_DISABLE_VOICE=1"},
}

//setupFile writes a setup to a target file
func (img DiskImage) writeFile(file setupFile) error {
	profilePath := filepath.Join(img.Mountpoint, file.path)
	dirPath := filepath.Dir(profilePath)
	if fi, err := os.Stat(dirPath); err != nil {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			fmt.Println(err)
		}
	} else if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory, customization failed", profilePath)
	}
	if err := ioutil.WriteFile(profilePath, []byte(file.content), 0644); err != nil {
		return err
	}
	return nil
}

//Writable allows writes on the created running image
func (img DiskImage) Writable() error {
	writeFlag := filepath.Join(img.Mountpoint, ".writable_image")
	if err := ioutil.WriteFile(writeFlag, []byte(""), 0644); err != nil {
		return err
	}
	return nil
}

//OverrideAdbInhibit will inhibit abd from shutting down when the screen is locked
func (img DiskImage) OverrideAdbInhibit() error {
	writeFlag := filepath.Join(img.Mountpoint, ".adb_onlock")
	return ioutil.WriteFile(writeFlag, []byte(""), 0644)
}
