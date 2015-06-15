//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import (
	"bufio"
	"errors"
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
	Kernel string `yaml:"kernel"`
	Dtbs   string `yaml:"dtbs"`
	Initrd string `yaml:"initrd"`
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

	OEM struct {
		Hardware struct {
			Bootloader      string      `yaml:"bootloader"`
			PartitionLayout string      `yaml:"partition-layout"`
			Dtb             string      `yaml:"dtb,omitempty"`
			Platform        string      `yaml:"platform"`
			Architecture    string      `yaml:"architecture"`
			BootAssets      *BootAssets `yaml:"boot-assets,omitempty"`
		} `yaml:"hardware,omitempty"`

		Software struct {
			BuiltIn      []string `yaml:"built-in,omitempty"`
			Preinstalled []string `yaml:"preinstalled,omitempty"`
		} `yaml:"software,omitempty"`

		Store *struct {
			ID string `yaml:"id,omitempty"`
		}
	} `yaml:"oem,omitempty"`

	Config map[string]interface{} `yaml:"config,omitempty"`
}

func (o OemDescription) InstallPath(rootPath string) (string, error) {
	glob, err := filepath.Glob(fmt.Sprintf("%s/oem/%s/%s", rootPath, o.Name, o.Version))
	if err != nil {
		return "", err
	}

	if len(glob) != 1 {
		return "", errors.New("oem package not installed")
	}

	return glob[0], nil
}

func (o OemDescription) Architecture() string {
	return o.OEM.Hardware.Architecture
}

func (o *OemDescription) SetArchitecture(architecture string) {
	o.OEM.Hardware.Architecture = architecture
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

// BaseImage implements the basic primitives to manage images.
type BaseImage struct {
	baseMount string
	hardware  HardwareDescription
	location  string
	oem       OemDescription
	parts     []partition
	partCount int
	size      int64
}

// Mount mounts the image. This also maps the loop device.
func (img *BaseImage) Mount() error {
	if err := img.Map(); err != nil {
		return err
	}

	baseMount, err := ioutil.TempDir(os.TempDir(), "diskimage")
	if err != nil {
		return err
	}

	// We change the mode so snappy can unpack as non root
	if err := os.Chmod(baseMount, 0755); err != nil {
		return err
	}

	//Remove Mountpoint if we fail along the way
	defer func() {
		if err != nil {
			if err := os.Remove(baseMount); err != nil {
				fmt.Println("WARNING: cannot remove", baseMount, "due to", err)
			}
		}
	}()

	for _, part := range img.parts {
		if part.fs == fsNone {
			continue
		}

		mountpoint := filepath.Join(baseMount, string(part.dir))
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return err
		}

		dev := filepath.Join("/dev/mapper", part.loop)
		printOut("Mounting", dev, part.fs, "to", mountpoint)
		if out, errMount := exec.Command("mount", filepath.Join("/dev/mapper", part.loop), mountpoint).CombinedOutput(); errMount != nil {
			return ErrMount{dev: dev, mountpoint: mountpoint, fs: part.fs, out: out}
		}
		// this is cleanup in case one of the mounts fail
		defer func() {
			if err != nil {
				if err := exec.Command("umount", mountpoint).Run(); err != nil {
					fmt.Println("WARNING:", mountpoint, "could not be unmounted")
					return
				}

				if err := os.Remove(mountpoint); err != nil {
					fmt.Println("WARNING: could not remove ", mountpoint)
				}
			}
		}()
	}
	img.baseMount = baseMount

	return nil

}

// Unmount unmounts the image. This also unmaps the loop device.
func (img *BaseImage) Unmount() error {
	if img.baseMount == "" {
		panic("No base mountpoint set")
	}

	if out, err := exec.Command("sync").CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to sync filesystems before unmounting: %s", out)
	}

	for _, part := range img.parts {
		if part.fs == fsNone {
			continue
		}

		mountpoint := filepath.Join(img.baseMount, string(part.dir))
		if out, err := exec.Command("umount", mountpoint).CombinedOutput(); err != nil {
			lsof, _ := exec.Command("lsof", "-w", mountpoint).CombinedOutput()
			printOut(string(lsof))
			return fmt.Errorf("unable to unmount dir for image: %s", out)
		}
	}

	if err := os.RemoveAll(img.baseMount); err != nil {
		return err
	}
	img.baseMount = ""

	return img.Unmap()
}

// Map maps the image to loop devices
func (img *BaseImage) Map() error {
	if isMapped(img.parts) {
		panic("cannot double map partitions")
	}

	kpartxCmd := exec.Command("kpartx", "-avs", img.location)
	stdout, err := kpartxCmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := kpartxCmd.Start(); err != nil {
		return err
	}

	loops := make([]string, 0, img.partCount)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		if len(fields) > 2 {
			loops = append(loops, fields[2])
		} else {
			return errors.New("issues while determining drive mappings")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(loops) != img.partCount {
		return errors.New("more partitions then expected while creating loop mapping")
	}

	mapPartitions(img.parts, loops)

	if err := kpartxCmd.Wait(); err != nil {
		return err
	}

	return nil
}

//Unmap destroys loop devices for the partitions
func (img *BaseImage) Unmap() error {
	if img.baseMount != "" {
		panic("cannot unmap mounted partitions")
	}

	for _, part := range img.parts {
		if err := exec.Command("dmsetup", "clear", part.loop).Run(); err != nil {
			return err
		}
	}

	if err := exec.Command("kpartx", "-ds", img.location).Run(); err != nil {
		return err
	}

	unmapPartitions(img.parts)

	return nil
}

// Format formats the image following the partition types and labels them
// accordingly.
func (img BaseImage) Format() error {
	for _, part := range img.parts {
		dev := filepath.Join("/dev/mapper", part.loop)

		if part.fs == fsFat32 {
			cmd := []string{"-F", "32", "-n", string(part.label)}

			size, err := sectorSize(dev)
			if err != nil {
				return err
			}

			if size != "512" {
				cmd = append(cmd, "-s", "1")
			}

			cmd = append(cmd, "-S", size, dev)

			if out, err := exec.Command("mkfs.vfat", cmd...).CombinedOutput(); err != nil {
				return fmt.Errorf("unable to create filesystem: %s", out)
			}
		} else {
			if out, err := exec.Command("mkfs.ext4", "-F", "-L", string(part.label), dev).CombinedOutput(); err != nil {
				return fmt.Errorf("unable to create filesystem: %s", out)
			}
		}
	}

	return nil
}

// User returns the writable path
func (img BaseImage) Writable() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(writableDir))
}

//System returns the system path
func (img BaseImage) System() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(systemADir))
}

// Boot returns the system-boot path
func (img BaseImage) Boot() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(bootDir))
}

// BaseMount returns the base directory used to mount the image partitions.
func (img BaseImage) BaseMount() string {
	if img.baseMount == "" {
		panic("image needs to be mounted")
	}

	return img.baseMount
}

func printOut(args ...interface{}) {
	if debugPrint {
		fmt.Println(args...)
	}
}
