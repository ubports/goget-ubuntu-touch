//
// diskimage - handles ubuntu disk images
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
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"launchpad.net/goget-ubuntu-touch/sysutils"
)

type DiskImage struct {
	Mountpoint string
	label      string
	path       string
	size       int64
	parts      []partition
}

//New creates a new DiskImage
func New(path, label string, size int64) *DiskImage {
	var img DiskImage
	img.path = path
	img.label = label
	img.size = size
	return &img
}

//New creates a new DiskImage
func NewExisting(path string) *DiskImage {
	var img DiskImage
	img.path = path
	return &img
}

func (img *DiskImage) Move(dst string) error {
	if err := img.Copy(dst); err != nil {
		return err
	}
	if err := os.Remove(img.path); err != nil {
		return errors.New(fmt.Sprintf("Unable to remove %s when moving to %s", img.path, dst))
	}
	img.path = dst
	return nil
}

func (img *DiskImage) Copy(dst string) (err error) {
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := os.Open(img.path)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	reader := bufio.NewReader(srcFile)
	writer := bufio.NewWriter(dstFile)
	defer func() {
		if err != nil {
			writer.Flush()
		}
	}()
	if _, err = io.Copy(writer, reader); err != nil {
		return err
	}
	writer.Flush()
	return nil
}

//Mount the DiskImage
func (img *DiskImage) Mount() (err error) {
	img.Mountpoint, err = ioutil.TempDir(os.TempDir(), "ubuntu-system")
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to create temp dir to create system image: %s", err))
	}
	//Remove Mountpoint if we fail along the way
	defer func() {
		if err != nil {
			os.Remove(img.Mountpoint)
		}
	}()

	if out, err := exec.Command("mount", img.path, img.Mountpoint).CombinedOutput(); err != nil {
		return fmt.Errorf("unable to mount temp dir to create system image: %s", out)
	}
	return nil
}

//Unmount the DiskImage
func (img *DiskImage) Unmount() error {
	if img.Mountpoint == "" {
		return nil
	}
	defer os.Remove(img.Mountpoint)
	if out, err := exec.Command("sync").CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("Failed to sync filesystems before unmounting: %s", out))
	}

	if err := exec.Command("umount", img.Mountpoint).Run(); err != nil {
		return errors.New("Failed to unmount temp dir where system image was created")
	}
	return nil
}

//Provision unpacks the tarList into the given DiskImage
func (img *DiskImage) Provision(tarList []string) error {
	//TODO use archive/tar
	for _, tar := range tarList {
		if out, err := exec.Command("tar", "--numeric-owner", "--exclude", "partitions*",
			"-xf", tar, "-C", img.Mountpoint).CombinedOutput(); err != nil {
			return errors.New(fmt.Sprintf("Unable to extract rootfs %s to %s: %s", tar, img.Mountpoint, out))
		}
	}
	if err := img.unpackSystem(); err != nil {
		return err
	}
	for _, file := range setupFiles {
		if err := img.writeFile(file); err != nil {
			return err
		}
	}
	return nil
}

//CreateExt4 returns a ext4 partition for a given file
func (img DiskImage) CreateExt4() error {
	if img.parts != nil {
		panic("creating ext4 on images with multiple parts not supported")
	}

	if err := sysutils.CreateEmptyFile(img.path, img.size, sysutils.GiB); err != nil {
		return err
	}

	return exec.Command("mkfs.ext4", "-F", "-L", img.label, img.path).Run()
}

//CreateVFat returns a vfat partition for a given file
func (img DiskImage) CreateVFat() error {
	if err := sysutils.CreateEmptyFile(img.path, img.size, sysutils.GiB); err != nil {
		return err
	}
	return exec.Command("mkfs.vfat", "-n", img.label, img.path).Run()
}

//unpackSystem moves the system partition up one level
func (img DiskImage) unpackSystem() error {
	os.Rename(filepath.Join(img.Mountpoint, "system"),
		filepath.Join(img.Mountpoint, "system-unpack"))
	defer os.Remove(filepath.Join(img.Mountpoint, "system-unpack"))
	dir, err := os.Open(filepath.Join(img.Mountpoint, "system-unpack"))
	if err != nil {
		return err
	}
	defer dir.Close()
	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		name := fileInfo.Name()
		err := os.Rename(filepath.Join(img.Mountpoint, "system-unpack", name),
			filepath.Join(img.Mountpoint, name))
		if err != nil {
			return err
		}
	}
	return nil
}

//ExtractFile extracts a filePath relative to it's mountpoint and copies it to dir.
//This function takes care of mounting and unmounting the img
func (img DiskImage) ExtractFile(filePath string, dir string) error {
	if err := sysutils.EscalatePrivs(); err != nil {
		return err
	}
	if err := img.Mount(); err != nil {
		return err
	}
	if err := sysutils.DropPrivs(); err != nil {
		return err
	}
	defer func() (err error) {
		if err := sysutils.EscalatePrivs(); err != nil {
			return err
		}
		if err := img.Unmount(); err != nil {
			return err
		}
		return sysutils.DropPrivs()
	}()
	if fi, err := os.Stat(dir); err != nil {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if !fi.IsDir() {
		return fmt.Errorf("extract dir %s is not a directory")
	}
	dstFile, err := os.Create(filepath.Join(dir, filePath))
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := os.Open(filepath.Join(img.Mountpoint, filePath))
	if err != nil {
		return err
	}
	defer srcFile.Close()

	reader := bufio.NewReader(srcFile)
	writer := bufio.NewWriter(dstFile)
	defer func() {
		if err != nil {
			writer.Flush()
		}
	}()
	if _, err = io.Copy(writer, reader); err != nil {
		return err
	}
	writer.Flush()
	return nil
}
