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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"launchpad.net/goget-ubuntu-touch/sysutils"
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

type CoreGrubImage struct {
	CoreImage
	location  string
	size      int64
	baseMount string
	parts     []partition
}

func NewCoreGrubImage(location string, size int64) *CoreGrubImage {
	return &CoreGrubImage{
		location: location,
		size:     size,
	}
}

const grubCfgContent = `# console only, no graphics/vga
GRUB_CMDLINE_LINUX_DEFAULT="console=tty1 console=ttyS0"
GRUB_TERMINAL=console
# LP: #1035279
GRUB_RECORDFAIL_TIMEOUT=0
`

func (img *CoreGrubImage) Mount() (err error) {
	img.baseMount, err = ioutil.TempDir(os.TempDir(), "core-grub-disk")
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to create temp dir to create system image: %s", err))
	}
	//Remove Mountpoint if we fail along the way
	defer func() {
		if err != nil {
			os.Remove(img.baseMount)
		}
	}()

	for _, part := range img.parts {
		if part.label == grubLabel {
			continue
		}

		mountpoint := filepath.Join(img.baseMount, string(part.dir))
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return err
		}
		if out, err := exec.Command("mount", filepath.Join("/dev/mapper", part.loop), mountpoint).CombinedOutput(); err != nil {
			return fmt.Errorf("unable to mount dir to create system image: %s", out)
		}
	}

	return nil
}

func (img *CoreGrubImage) Unmount() (err error) {
	if img.baseMount == "" {
		panic("No base mountpoint set")
	}
	defer os.Remove(img.baseMount)

	if out, err := exec.Command("sync").CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to sync filesystems before unmounting: %s", out)
	}

	for i := len(img.parts) - 1; i >= 0; i-- {
		if img.parts[i].label == grubLabel {
			continue
		}

		mountpoint := filepath.Join(img.baseMount, string(img.parts[i].dir))
		if out, err := exec.Command("umount", mountpoint).CombinedOutput(); err != nil {
			return fmt.Errorf("unable to unmount dir for image: %s", out)
		}
	}

	img.baseMount = ""

	return nil
}

//Partition creates a partitioned image from an img
func (img *CoreGrubImage) Partition() error {
	if err := sysutils.CreateEmptyFile(img.location, img.size); err != nil {
		return err
	}

	partedCmd := exec.Command("parted", img.location)
	stdin, err := partedCmd.StdinPipe()
	if err != nil {
		return err
	}

	stdin.Write([]byte("mklabel gpt\n"))

	stdin.Write([]byte("mkpart non-fs ext4 8192s 16383s\n"))
	stdin.Write([]byte("mkpart boot fat32 16384s 1064959s\n"))
	stdin.Write([]byte("mkpart system-a ext4 1064960s 3162111s\n"))
	stdin.Write([]byte("mkpart system-b ext4 3162112s 5259263s\n"))
	stdin.Write([]byte("mkpart writable ext4 5259264s -1M\n"))

	stdin.Write([]byte("set 1 boot on\n"))
	stdin.Write([]byte("set 2 esp on\n"))
	stdin.Write([]byte("set 1 bios_grub on\n"))
	stdin.Write([]byte("unit s print\n"))
	stdin.Write([]byte("quit\n"))

	return partedCmd.Run()
}

//Map creates loop devices for the partitions
func (img *CoreGrubImage) Map() error {
	if img.parts != nil {
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

	loops := make([]string, 0, 4)
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

	// there are 5 partitions, so there should be five loop mounts
	if len(loops) != 5 {
		return errors.New("more partitions then expected while creating loop mapping")
	}

	img.parts = []partition{
		partition{label: bootLabel, dir: bootDir, loop: loops[1]},
		partition{label: systemALabel, dir: systemADir, loop: loops[2]},
		partition{label: systemBLabel, dir: systemBDir, loop: loops[3]},
		partition{label: writableLabel, dir: writableDir, loop: loops[4]},
		partition{label: grubLabel, dir: grubDir, loop: loops[0]},
	}

	if err := kpartxCmd.Wait(); err != nil {
		return err
	}

	return nil
}

//Unmap destroys loop devices for the partitions
func (img *CoreGrubImage) Unmap() error {
	if img.baseMount != "" {
		panic("cannot unmap mounted partitions")
	}

	for _, part := range img.parts {
		if err := exec.Command("dmsetup", "clear", part.loop).Run(); err != nil {
			return err
		}
	}

	if err := exec.Command("kpartx", "-d", img.location).Run(); err != nil {
		return err
	}

	img.parts = nil

	return nil
}

func (img CoreGrubImage) Format() error {
	for _, part := range img.parts {
		dev := filepath.Join("/dev/mapper", part.loop)

		if part.label == bootLabel {
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
		} else if part.label != grubLabel {
			if out, err := exec.Command("mkfs.ext4", "-F", "-L", string(part.label), dev).CombinedOutput(); err != nil {
				return fmt.Errorf("unable to create filesystem: %s", out)
			}
		}
	}

	return nil
}

// User returns the writable path
func (img CoreGrubImage) Writable() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(writableDir))
}

//System returns the system path
func (img CoreGrubImage) System() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(systemADir))
}

func (img CoreGrubImage) BaseMount() string {
	if img.baseMount == "" {
		panic("image needs to be mounted")
	}

	return img.baseMount
}

func (img *CoreGrubImage) SetupBoot() error {
	for _, dev := range []string{"dev", "proc", "sys"} {
		src := filepath.Join("/", dev)
		dst := filepath.Join(img.System(), dev)
		if err := bindMount(src, dst); err != nil {
			return err
		}
		defer unmount(dst)
	}

	firmwarePath := filepath.Join(img.System(), "sys", "firmware")
	if err := bindMount(filepath.Join(img.System(), "mnt"), firmwarePath); err != nil {
		return err
	}
	defer unmount(firmwarePath)

	outputPath, err := filepath.Abs(img.location)
	if err != nil {
		return errors.New("cannot determined absolute path for output image")
	}

	rootDevPath := filepath.Join(img.System(), "root_dev")

	if f, err := os.Create(rootDevPath); err != nil {
		return err
	} else {
		f.Close()
		defer os.Remove(rootDevPath)
	}

	if err := bindMount(outputPath, rootDevPath); err != nil {
		return err
	}
	defer unmount(rootDevPath)

	if out, err := exec.Command("chroot", img.System(), "grub-install", "/root_dev").CombinedOutput(); err != nil {
		return fmt.Errorf("unable to install grub: %s", out)
	}

	// ensure we run not into recordfail issue
	grubDir := filepath.Join(img.System(), "etc", "default", "grub.d")
	if err := os.MkdirAll(grubDir, 0755); err != nil {
		return fmt.Errorf("unable to create %s dir: %s", grubDir, err)
	}
	grubFile, err := os.Create(filepath.Join(grubDir, "50-system-image.cfg"))
	if err != nil {
		return fmt.Errorf("unable to create %s file: %s", grubFile, err)
	}
	defer grubFile.Close()
	if _, err := io.WriteString(grubFile, grubCfgContent); err != nil {
		return err
	}

	// I don't know why this is needed, I just picked it up from the original implementation
	time.Sleep(3 * time.Second)

	if out, err := exec.Command("chroot", img.System(), "update-grub").CombinedOutput(); err != nil {
		return fmt.Errorf("unable to update grub: %s", out)
	}

	return nil
}

func (img *CoreGrubImage) FlashExtra(devicePart string) error {
	return nil
}

func bindMount(src, dst string) error {
	if out, err := exec.Command("mount", "--bind", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while bind mounting: %s", out)
	}

	return nil
}

func unmount(dst string) error {
	if out, err := exec.Command("umount", dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while unmounting: %s", out)
	}

	return nil
}
