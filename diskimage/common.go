//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

var debugPrint bool

func init() {
	if debug := os.Getenv("DEBUG_DISK"); debug != "" {
		debugPrint = true
	}
}

type Image interface {
	Mount() error
	Unmount() error
	Format() error
	Partition() error
	Map() error
	Unmap() error
	BaseMount() string
}

type SystemImage interface {
	System() string
	Writable() string
}

type CoreImage interface {
	Image
	SystemImage
	SetupBoot(oemRootPath string) error
	FlashExtra(oemRootPath, devicePart string) error
}

type HardwareDescription struct {
	Kernel          string `yaml:"kernel"`
	Dtbs            string `yaml:"dtbs"`
	Initrd          string `yaml:"initrd"`
	PartitionLayout string `yaml:"partition-layout,omitempty"`
	Bootloader      string `yaml:"bootloader"`
}

type BootAssetRawFiles struct {
	Path   string `yaml:"path"`
	Offset string `yaml:"offset"`
}

type BootAssetFiles struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target,omitempty"`
}

type BootAssets struct {
	Files    []BootAssetFiles    `yaml:"files,omitempty"`
	RawFiles []BootAssetRawFiles `yaml:"raw-files,omitempty"`
}

type OemDescription struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`

	OEM *struct {
		Hardware *struct {
			Bootloader      string      `yaml:"bootloader"`
			PartitionLayout string      `yaml:"partition-layout"`
			Dtb             string      `yaml:"dtb,omitempty"`
			Platform        string      `yaml:"platform"`
			Architecture    string      `yaml:"architecture"`
			BootAssets      *BootAssets `yaml:"boot-assets,omitempty"`
		} `yaml:"hardware,omitempty"`

		Store *struct {
			ID string `yaml:"id,omitempty"`
		}
	} `yaml:"oem,omitempty"`

	Config map[string]interface{} `yaml:"config,omitempty"`
}

func (o OemDescription) InstallPath() string {
	return filepath.Join("/oem", o.Name, o.Version)
}

func (o OemDescription) Architecture() string {
	return o.OEM.Hardware.Architecture
}

func (o OemDescription) PartitionLayout() string {
	return o.OEM.Hardware.PartitionLayout
}

func (o OemDescription) Platform() string {
	return o.OEM.Hardware.Platform
}

func (o *OemDescription) SetPlatform(platform string) {
	o.OEM.Hardware.Platform = platform
}

func sectorSize(dev string) (string, error) {
	out, err := exec.Command("blockdev", "--getss", dev).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("unable to determine block size: %s", out)
	}

	return strings.TrimSpace(string(out)), err
}

func mount(partitions []partition) (baseMount string, err error) {
	baseMount, err = ioutil.TempDir(os.TempDir(), "diskimage")
	if err != nil {
		return "", err
	}

	// We change the mode so snappy can unpack as non root
	if err := os.Chmod(baseMount, 0777); err != nil {
		return "", err
	}

	//Remove Mountpoint if we fail along the way
	defer func() {
		if err != nil {
			os.Remove(baseMount)
		}
	}()

	for _, part := range partitions {
		if part.fs == fsNone {
			continue
		}

		mountpoint := filepath.Join(baseMount, string(part.dir))
		if err := os.MkdirAll(mountpoint, 0777); err != nil {
			return "", err
		}
		if out, err := exec.Command("mount", filepath.Join("/dev/mapper", part.loop), "-o", "user", mountpoint).CombinedOutput(); err != nil {
			return "", fmt.Errorf("unable to mount dir to create system image: %s", out)
		}
	}

	return baseMount, nil

}

func printOut(args ...interface{}) {
	if debugPrint {
		fmt.Println(args...)
	}
}
