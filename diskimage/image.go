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
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"launchpad.net/goget-ubuntu-touch/sysutils"
)

type imageLabel string
type directory string

const (
	systemALabel  imageLabel = "system-a"
	systemBLabel  imageLabel = "system-b"
	writableLabel imageLabel = "writable"
)

const (
	systemADir  directory = "system"
	systemBDir  directory = "system-b"
	writableDir directory = "writable"
)

type partition struct {
	label imageLabel
	dir   directory
	loop  string
}

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

// User returns the user path
func (img *DiskImage) User() (string, error) {
	if img.parts == nil {
		return "", errors.New("img is not setup with partitions")
	}

	if img.Mountpoint == "" {
		return "", errors.New("img not mounted")
	}

	return filepath.Join(img.Mountpoint, string(writableDir)), nil
}

//System returns the system path
func (img *DiskImage) System() (string, error) {
	if img.parts == nil {
		return "", errors.New("img is not setup with partitions")
	}

	if img.Mountpoint == "" {
		return "", errors.New("img not mounted")
	}

	return filepath.Join(img.Mountpoint, string(systemADir)), nil
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
	if img.parts == nil {
		return img.emulatorMount()
	}

	return img.coreMount()
}

func (img *DiskImage) coreMount() (err error) {
	for _, part := range img.parts {
		mountpoint := filepath.Join(img.Mountpoint, string(part.dir))
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return err
		}
		if out, err := exec.Command("mount", filepath.Join("/dev/mapper", part.loop), mountpoint).CombinedOutput(); err != nil {
			return fmt.Errorf("unable to mount dir to create system image: %s", out)
		}
	}
	return nil
}

func (img *DiskImage) emulatorMount() (err error) {
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

	if img.parts == nil {
		if err := img.emulatorUnmount(); err != nil {
			return err
		}
	} else {
		if err := img.coreUnmount(); err != nil {
			return err
		}
	}

	img.Mountpoint = ""

	return nil
}

func (img *DiskImage) coreUnmount() (err error) {
	for _, part := range img.parts {
		mountpoint := filepath.Join(img.Mountpoint, string(part.dir))
		if out, err := exec.Command("umount", mountpoint).CombinedOutput(); err != nil {
			return fmt.Errorf("unable to unmount dir for image: %s", out)
		}
		if err := os.Remove(mountpoint); err != nil {
			return err
		}
	}
	return nil
}

func (img *DiskImage) emulatorUnmount() (err error) {
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

//Partition creates a partitioned image from an img
func (img *DiskImage) Partition(dual bool) error {
	if err := sysutils.CreateEmptyFile(img.path, img.size); err != nil {
		return err
	}

	partedCmd := exec.Command("parted", img.path)
	stdin, err := partedCmd.StdinPipe()
	if err != nil {
		return err
	}

	stdin.Write([]byte("mklabel msdos\n"))
	stdin.Write([]byte("mkpart primary ext4 2048s 3905535s\n"))
	if dual {
		stdin.Write([]byte("mkpart primary ext4 3905536s 7809023s\n"))
		stdin.Write([]byte("mkpart primary ext4 7809024s -1s\n"))
	} else {
		stdin.Write([]byte("mkpart primary ext4 3905536s -1s\n"))
	}
	stdin.Write([]byte("set 1 boot on\n"))
	stdin.Write([]byte("unit s print\n"))
	stdin.Write([]byte("quit\n"))

	return partedCmd.Run()
}

//MapPartitions creates loop devices for the partitions
func (img *DiskImage) MapPartitions(dual bool) error {
	kpartxCmd := exec.Command("kpartx", "-avs", img.path)
	stdout, err := kpartxCmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := kpartxCmd.Start(); err != nil {
		return err
	}

	loops := make([]string, 0, 3)
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

	expectedLoops := 2
	if dual {
		expectedLoops = 3
	}

	if len(loops) != expectedLoops {
		return errors.New("more partitions then expected while creating loop mapping")
	}

	if dual {
		img.parts = []partition{
			partition{label: systemALabel, dir: systemADir, loop: loops[0]},
			partition{label: systemBLabel, dir: systemBDir, loop: loops[1]},
			partition{label: writableLabel, dir: writableDir, loop: loops[2]},
		}
	} else {
		img.parts = []partition{
			partition{label: systemALabel, dir: systemADir, loop: loops[0]},
			partition{label: writableLabel, dir: writableDir, loop: loops[1]},
		}
	}

	if err := kpartxCmd.Wait(); err != nil {
		return err
	}

	return nil
}

//UnMapPartitions destroys loop devices for the partitions
func (img *DiskImage) UnMapPartitions() error {
	for _, part := range img.parts {
		if err := exec.Command("dmsetup", "clear", part.loop).Run(); err != nil {
			return err
		}
	}
	return exec.Command("kpartx", "-d", img.path).Run()
}

//CreateExt4 returns a ext4 partition for a given file
func (img DiskImage) CreateExt4() error {
	if img.parts == nil {
		if err := sysutils.CreateEmptyFile(img.path, img.size); err != nil {
			return err
		}
		return exec.Command("mkfs.ext4", "-F", "-L", img.label, img.path).Run()
	}

	for _, part := range img.parts {
		dev := filepath.Join("/dev/mapper", part.loop)
		if out, err := exec.Command("mkfs.ext4", "-F", "-L", string(part.label), dev).CombinedOutput(); err != nil {
			return fmt.Errorf("unable to create filesystem: %s", out)
		}
	}

	return nil
}

//CreateVFat returns a vfat partition for a given file
func (img DiskImage) CreateVFat() error {
	if err := sysutils.CreateEmptyFile(img.path, img.size); err != nil {
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
