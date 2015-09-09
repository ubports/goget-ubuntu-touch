//
// sysutils - Tool to download and run Ubuntu Touch emulator instances
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package sysutils

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
	"strconv"
	"syscall"
)

type unit int64

const (
	GiB unit = 1024
	GB  unit = 1000
)

func CreateEmptyFile(path string, size int64, u unit) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(path)
		}
	}()

	switch u {
	case GiB:
		size = size * 1024 * 1024 * 1024
	case GB:
		// The image size will be reduced to fit commercial drives that are
		// smaller than what they claim, 975 comes from 97.5% of the total size
		// but we want to be a multiple of 512 (and size is an int) we divide by
		// 512 and multiply it again
		size = size * 1000 * 1000 * 975 / 512 * 512
	default:
		panic("improper sizing unit used")
	}

	if err := file.Truncate(size); err != nil {
		return errors.New(fmt.Sprintf("Error creating %s of size %d to stage image onto", path, size))
	}
	return nil
}

func DropPrivs() error {
	uid, gid := GetUserEnvInt()
	if err := syscall.Setregid(-1, gid); err != nil {
		return errors.New(fmt.Sprintf("Can't drop gid: %s", err))
	}
	if err := syscall.Setreuid(-1, uid); err != nil {
		return errors.New(fmt.Sprintf("Can't drop uid: %s", err))
	}
	return nil
}

func EscalatePrivs() error {
	err := syscall.Setreuid(-1, 0)
	return err
}

// GetUserEnv checks if the process can drop priviledges by checking if either
// SUDO_UID and SUDO_GID or PKEXEC_UID are set, it returns the corresponding
// uid and gid set in these or 0 otherwise.
func GetUserEnv() (uid, gid string) {
	if v := os.Getenv("SUDO_UID"); v != "" {
		uid = v
	} else if v := os.Getenv("PKEXEC_UID"); v != "" {
		uid = v
	} else {
		uid = "0"
	}

	if v := os.Getenv("SUDO_GID"); v != "" {
		gid = v
	} else {
		gid = "0"
	}
	return uid, gid
}

func GetUserEnvInt() (uid, gid int) {
	uidString, gidString := GetUserEnv()
	uid, _ = strconv.Atoi(uidString)
	gid, _ = strconv.Atoi(gidString)
	return uid, gid
}
