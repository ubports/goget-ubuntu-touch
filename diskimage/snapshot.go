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
	"os"
	"os/exec"
)

const BaseSnapshot = "pristine"

//ConvertQcow2 converts an image from raw to qcow2
func (img *DiskImage) ConvertQcow2() error {
	var cmd *exec.Cmd
	// convert
	cmd = exec.Command("qemu-img", "convert", "-f", "raw", img.path,
		"-O", "qcow2", "-o", "compat=0.10", img.path+".qcow2")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("Error while converting %s: %s", img.path, out))
	}

	// check
	cmd = exec.Command("qemu-img", "check", img.path+".qcow2")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("Error while converting %s: %s", img.path, out))
	}
	os.Rename(img.path+".qcow2", img.path)
	return img.Snapshot(BaseSnapshot)

}

//Snapshot creates a DiskImage's snapshot with the specified snapshot in label
func (img *DiskImage) Snapshot(label string) error {
	// snap
	cmd := exec.Command("qemu-img", "snapshot", "-c", label, img.path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("Error while converting %s: %s", img.path, out))
	}
	return nil
}

//RevertSnapshot reverts a DiskImage's snapshot to the specified snapshot in label
func (img *DiskImage) RevertSnapshot(label string) error {
	// unsnap
	cmd := exec.Command("qemu-img", "snapshot", "-c", label, img.path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("Error while converting %s: %s", img.path, out))
	}
	return nil
}
