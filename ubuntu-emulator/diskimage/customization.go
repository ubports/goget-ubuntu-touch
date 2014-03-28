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
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

var overrides = []string{"etc/init/ofono.conf",
	"etc/init/ubuntu-location-service.conf",
	"etc/init/whoopsie.conf",
	"usr/share/upstart/sessions/ofono-setup.conf",
	"usr/share/upstart/sessions/mediascanner.conf",
	"etc/init/bluetooth.conf",
	"etc/init/network-manager.conf",
}

const networkConfig string = `# interfaces(5) file used by ifup(8) and ifdown(8)
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 10.0.2.15
    netmask 255.255.255.0
    gateway 10.0.2.2
    dns-nameservers 10.0.2.3
`

const qemuConsole string = `# ttyS2 - getty

start on stopped rc RUNLEVEL=[2345] and not-container
stop on runlevel [!2345]

respawn
exec /sbin/getty -8 38400 ttyS2
`

type setupFile struct{ path, content string }

var setupFiles = []setupFile{{"etc/init/ttyS2.conf", qemuConsole},
	{"etc/network/interfaces", networkConfig},
	{"etc/profile.d/hud-service.sh", "export HUD_DISABLE_VOICE=1"},
}

//overrideJob creates all the hacks to make the emulator work
//these should eventually be dropped or made optional
func (img DiskImage) overrideJob(jobPath string) error {
	if !strings.HasSuffix(jobPath, ".conf") {
		return errors.New(fmt.Sprintf("%s is not an upstart job", jobPath))
	} else {
	}

	jobPath = jobPath[:strings.LastIndex(jobPath, "conf")] + "override"
	if err := ioutil.WriteFile(filepath.Join(img.Mountpoint, jobPath), []byte("manual\n"), 0644); err != nil {
		return err
	}
	return nil
}

//setupFile writes a setup to a target file
func (img DiskImage) writeFile(file setupFile) error {
	profilePath := filepath.Join(img.Mountpoint, file.path)
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
