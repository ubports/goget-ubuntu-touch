//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2013-2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ubports/goget-ubuntu-touch/sysutils"
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
	BaseImage

	legacyGrub bool
}

func NewCoreGrubImage(location string, size int64, rootSize int, hw HardwareDescription, oem OemDescription, updateGrub bool, label string) *CoreGrubImage {
	return &CoreGrubImage{
		BaseImage: BaseImage{
			location:  location,
			size:      size,
			rootSize:  rootSize,
			hardware:  hw,
			oem:       oem,
			partCount: 5,
			label:     label,
		},
		legacyGrub: updateGrub,
	}
}

const grubCfgContent = `# console only, no graphics/vga
GRUB_CMDLINE_LINUX_DEFAULT="console=tty1 console=ttyS0 panic=-1"
GRUB_TERMINAL=console
# LP: #1035279
GRUB_RECORDFAIL_TIMEOUT=0
`

const grubStubContent = `set prefix=($root)'/EFI/ubuntu/grub'
configfile $prefix/grub.cfg
`

//Partition creates a partitioned image from an img
func (img *CoreGrubImage) Partition() error {
	if err := sysutils.CreateEmptyFile(img.location, img.size, sysutils.GB); err != nil {
		return err
	}

	parted, err := newParted(mkLabelGpt)
	if err != nil {
		return err
	}

	parted.addPart(grubLabel, "", fsNone, 4)
	parted.addPart(bootLabel, bootDir, fsFat32, 128)
	parted.addPart(systemALabel, systemADir, fsExt4, img.rootSize)
	parted.addPart(systemBLabel, systemBDir, fsExt4, img.rootSize)
	parted.addPart(writableLabel, writableDir, fsExt4, -1)

	parted.setBoot(2)
	parted.setBiosGrub(1)

	img.parts = parted.parts

	return parted.create(img.location)
}

func (img *CoreGrubImage) SetupBoot() error {
	if !img.legacyGrub {
		// destinations
		bootPath := filepath.Join(img.baseMount, string(bootDir), "EFI", "ubuntu", "grub")
		if err := img.GenericBootSetup(bootPath); err != nil {
			return err
		}
	}

	return img.setupGrub()
}

func (img *CoreGrubImage) setupGrub() error {
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

	efiDir := filepath.Join(img.System(), "boot", "efi")
	if err := os.MkdirAll(efiDir, 0755); err != nil {
		return fmt.Errorf("unable to create %s dir: %s", efiDir, err)
	}

	if err := bindMount(img.Boot(), efiDir); err != nil {
		return err
	}
	defer unmount(efiDir)

	// create efi layout
	efiGrubDir := filepath.Join(img.System(), "boot", "efi", "EFI", "ubuntu", "grub")
	if err := os.MkdirAll(efiGrubDir, 0755); err != nil {
		return fmt.Errorf("unable to create %s dir: %s", efiGrubDir, err)
	}

	// create fw update directory
	fwDir := filepath.Join(img.System(), "boot", "efi", "EFI", "ubuntu", "fw")
	if err := os.MkdirAll(fwDir, 0755); err != nil {
		return fmt.Errorf("unable to create %s dir: %s", fwDir, err)
	}

	bootGrubDir := filepath.Join(img.System(), "boot", "grub")
	if err := bindMount(efiGrubDir, bootGrubDir); err != nil {
		return err
	}
	defer unmount(bootGrubDir)

	var grubTarget string

	arch := img.oem.Architecture()

	switch arch {
	case "armhf":
		grubTarget = "arm-efi"
	case "amd64":
		grubTarget = "x86_64-efi"
	case "i386":
		grubTarget = "i386-efi"
	default:
		return fmt.Errorf("unsupported architecture for GRUB on EFI: %s", arch)
	}

	if arch == "amd64" || arch == "i386" {
		// install grub BIOS support
		if out, err := exec.Command("chroot", img.System(), "grub-install", "/root_dev").CombinedOutput(); err != nil {
			return fmt.Errorf("unable to install grub (BIOS): %s", out)
		}
	}

	// install grub EFI
	if out, err := exec.Command("chroot", img.System(), "grub-install", fmt.Sprint("--target="+grubTarget), "--no-nvram", "--removable", "--efi-directory=/boot/efi").CombinedOutput(); err != nil {
		return fmt.Errorf("unable to install grub (EFI): %s", out)
	}
	// tell our EFI grub where to find its full config
	efiBootDir := filepath.Join(img.System(), "boot", "efi", "EFI", "BOOT")
	grubStub, err := os.Create(filepath.Join(efiBootDir, "grub.cfg"))
	if err != nil {
		return fmt.Errorf("unable to create %s file: %s", grubStub.Name(), err)
	}
	defer grubStub.Close()
	if _, err := io.WriteString(grubStub, grubStubContent); err != nil {
		return err
	}

	if img.legacyGrub {
		if err := img.updateGrub(); err != nil {
			return err
		}
	}

	return nil
}

func (img *CoreGrubImage) updateGrub() error {
	// ensure we run not into recordfail issue
	grubDir := filepath.Join(img.System(), "etc", "default", "grub.d")
	if err := os.MkdirAll(grubDir, 0755); err != nil {
		return fmt.Errorf("unable to create %s dir: %s", grubDir, err)
	}
	grubFile, err := os.Create(filepath.Join(grubDir, "50-system-image.cfg"))
	if err != nil {
		return fmt.Errorf("unable to create %s file: %s", grubFile.Name(), err)
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
