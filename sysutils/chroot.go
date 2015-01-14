//
// sysutils - Tool to download and run Ubuntu Touch emulator instances
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package sysutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

const (
	binQemuArmStatic  = "/usr/bin/qemu-arm-static"
	pkgQemuUserStatic = "qemu-user-static"
)

var mountpoints = []string{"dev", "proc", "sys", filepath.Join("sys", "firmware")}

func AddQemuStatic(chroot string) error {
	dst := filepath.Join(chroot, binQemuArmStatic)
	if out, err := exec.Command("cp", binQemuArmStatic, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("issues while setting up password: %s", out)
	}

	return nil
}

func RemoveQemuStatic(chroot string) error {
	dst := filepath.Join(chroot, binQemuArmStatic)

	return os.Remove(dst)
}

func VerifyDependencies(arch string) error {
	switch arch {
	case "armhf":
		if _, err := os.Stat(binQemuArmStatic); err != nil {
			return fmt.Errorf("missing dependency %s (apt install %s)", binQemuArmStatic, pkgQemuUserStatic)
		}
	}

	return nil
}

func ChrootBindMount(chroot string) error {
	for _, mnt := range mountpoints {
		src := filepath.Join("/", mnt)
		dst := filepath.Join(chroot, mnt)
		if err := bindMount(src, dst); err != nil {
			return err
		}
	}

	return nil
}

func ChrootBindUnmount(chroot string) error {
	for i := len(mountpoints) - 1; i >= 0; i-- {
		if err := unmount(filepath.Join(chroot, mountpoints[i])); err != nil {
			return err
		}
	}

	return nil
}

func ChrootRun(cmd ...string) error {
	if out, err := exec.Command("chroot", cmd...).CombinedOutput(); err != nil {
		return fmt.Errorf("unable to run chroot command: %s", out)
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
