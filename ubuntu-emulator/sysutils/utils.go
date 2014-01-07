//
// ubuntu-emu - Tool to download and run Ubuntu Touch emulator instances
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

func CreateEmptyFile(path string, size int64) (err error) {
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
	size = size * 1024 * 1024 * 1024
	if err := file.Truncate(size); err != nil {
		return errors.New(fmt.Sprintf("Error creating %s of size %d to stage image onto", path, size))
	}
	return nil
}

func DropPrivs() error {
	uid, gid := GetSudoEnvInt()
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

func GetSudoEnv() (uid, gid string) {
	uid = os.Getenv("SUDO_UID")
	gid = os.Getenv("SUDO_GID")
	if uid == "" {
		uid = "0"
	}

	if gid == "" {
		gid = "0"
	}
	return uid, gid
}

func GetSudoEnvInt() (uid, gid int) {
	uidString, gidString := GetSudoEnv()
	uid, _ = strconv.Atoi(uidString)
	gid, _ = strconv.Atoi(gidString)
	return uid, gid
}
