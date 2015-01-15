//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2013 Canonical Ltd.
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

type imageLabel string
type directory string
type mklabelType string
type fsType string

const (
	grubLabel     imageLabel = "grub"
	bootLabel     imageLabel = "system-boot"
	systemALabel  imageLabel = "system-a"
	systemBLabel  imageLabel = "system-b"
	writableLabel imageLabel = "writable"
)

const (
	grubDir     directory = "system/boot/grub-gpt"
	bootDir     directory = "boot"
	systemADir  directory = "system"
	systemBDir  directory = "system-b"
	writableDir directory = "writable"
)

const (
	mkLabelGpt   mklabelType = "gpt"
	mkLabelMsdos mklabelType = "msdos"
)

const (
	fsFat32 fsType = "fat32"
	fsExt4  fsType = "ext4"
	fsNone  fsType = ""
)

var errUnsupportedPartitioning = errors.New("unsupported partitioning")

type parted struct {
	mklabel       mklabelType
	parts         []partition
	bootPartition int
	biosGrub      int
}

type partition struct {
	begin int
	end   int
	fs    fsType
	label imageLabel
	dir   directory
	loop  string
}

func newParted(mklabel mklabelType) (*parted, error) {
	if mklabel != mkLabelGpt && mklabel != mkLabelMsdos {
		return nil, errUnsupportedPartitioning
	}

	return &parted{
		mklabel: mklabel,
	}, nil
}

// size in MiB
func (p *parted) addPart(label imageLabel, mountDir directory, fs fsType, size int) {
	var begin int

	if len(p.parts) == 0 {
		begin = 8192
	} else {
		begin = p.parts[len(p.parts)-1].end + 1
	}

	end := size
	if size != -1 {
		end = mib2Blocks(size) + begin - 1
	}

	part := partition{
		begin: begin,
		end:   end,
		label: label,
		fs:    fs,
		dir:   mountDir,
	}

	p.parts = append(p.parts, part)
}

func (p *parted) create(target string) error {
	partedCmd := exec.Command("parted", target)
	stdin, err := partedCmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := partedCmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := partedCmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := partedCmd.Start(); err != nil {
		return err
	}

	stdin.Write([]byte(fmt.Sprintf("mklabel %s\n", p.mklabel)))

	if p.mklabel == mkLabelGpt {
		for _, parts := range p.parts {
			if parts.end != -1 {
				stdin.Write([]byte(fmt.Sprintf("mkpart %s %s %ds %ds\n", parts.label, parts.fs, parts.begin, parts.end)))
			} else {
				stdin.Write([]byte(fmt.Sprintf("mkpart %s %s %ds %dM\n", parts.label, parts.fs, parts.begin, parts.end)))
			}
		}
	} else if p.mklabel == mkLabelMsdos {
		if len(p.parts) > 4 {
			panic("invalid amount of partitions for msdos")
		}

		for _, parts := range p.parts {
			if parts.end != -1 {
				stdin.Write([]byte(fmt.Sprintf("mkpart primary %s %ds %ds\n", parts.fs, parts.begin, parts.end)))
			} else {
				stdin.Write([]byte(fmt.Sprintf("mkpart primary %s %ds %dM\n", parts.fs, parts.begin, parts.end)))
			}
		}
	} else {
		panic("unsupported mklabel for partitioning")
	}

	if p.bootPartition != 0 {
		stdin.Write([]byte(fmt.Sprintf("set %d boot on\n", p.bootPartition)))
	}

	if p.biosGrub != 0 {
		stdin.Write([]byte(fmt.Sprintf("set %d bios_grub on\n", p.biosGrub)))
	}

	stdin.Write([]byte("quit\n"))

	if debugPrint {
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
	}

	if err := partedCmd.Wait(); err != nil {
		return errors.New("issues while partitioning")
	}

	return nil
}

func (p *parted) setBiosGrub(partNumber int) {
	if partNumber-1 > len(p.parts) {
		panic("cannot set part number to inexistent partition")
	}

	p.biosGrub = partNumber
}

func (p *parted) setBoot(partNumber int) {
	if partNumber-1 > len(p.parts) {
		panic("cannot set part number to inexistent partition")
	}

	p.bootPartition = partNumber
}

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	if s%4 != 0 {
		panic(fmt.Sprintf("invalid partition size: %d", s))
	}

	return s
}

func isMapped(parts []partition) bool {
	for i := range parts {
		if parts[i].loop != "" {
			return true
		}
	}

	return false
}

func mapPartitions(parts []partition, loops []string) {
	for i := range parts {
		parts[i].loop = loops[i]
	}
}

func unmapPartitions(parts []partition) {
	for i := range parts {
		parts[i].loop = ""
	}
}
