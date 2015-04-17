//
// bootimg - Tool to assemble/dissassemble Android boot.img s
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package bootimg

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
	"bytes"
	"encoding/binary"
	"errors"
	"io/ioutil"
)

type bootHeader map[uint]uint32

const BOOT_MAGIC = "ANDROID!"
const (
	kernelSize = iota
	kernelAddr
	ramdiskSize
	ramdiskAddr
	secondSize
	secondAddr
	tagsAddr
	pageSize
)

type AndroidBootImg struct {
	kernelOffset, ramdiskOffset, secondOffset uint32
	hdr                                       bootHeader
	img                                       []byte
}

// readChunk reads a chunk o b []bytes returns it's Little Endian
// unsigned size(4) value
func readChunk(b []byte) (value uint32, err error) {
	if len(b) != 4 {
		return value, errors.New("Reading an incorrect unsigned chunk of bytes")
	}
	buf := bytes.NewReader(b)
	err = binary.Read(buf, binary.LittleEndian, &value)
	return value, err
}

//New reads a sequence of []byte corresponding to an android boot
//image returning an AndroidBootImg which holds most the parsed headers
//which are relevant to retrieve the contained images
func New(img []byte) (boot AndroidBootImg, err error) {
	boot.img = img
	magic := img[:len(BOOT_MAGIC)]
	if BOOT_MAGIC != string(magic) {
		return boot, errors.New("This is not on an android bootimg")
	}

	boot.hdr = make(bootHeader)
	for i, start := kernelSize, len(BOOT_MAGIC); i <= pageSize; i++ {
		if boot.hdr[uint(i)], err = readChunk(img[start : start+4]); err != nil {
			return boot, err
		}
		// sizeof(unsigned)
		start += 4
	}
	n := (boot.hdr[kernelSize] + boot.hdr[pageSize] - 1) / boot.hdr[pageSize]
	m := (boot.hdr[ramdiskSize] + boot.hdr[pageSize] - 1) / boot.hdr[pageSize]
	//o := (boot.hdr[secondSize] + boot.hdr[pageSize] - 1) / boot.hdr[pageSize]
	boot.kernelOffset = boot.hdr[pageSize]
	boot.ramdiskOffset = boot.kernelOffset + (n * boot.hdr[pageSize])
	boot.secondOffset = boot.ramdiskOffset + (m * boot.hdr[pageSize])
	return boot, nil
}

//WriteRamdisk writes the ramdisk contained in AndroidBootImg to filepath
func (boot *AndroidBootImg) WriteRamdisk(filePath string) error {
	begin := boot.ramdiskOffset
	end := boot.ramdiskOffset + boot.hdr[ramdiskSize]
	err := ioutil.WriteFile(filePath, boot.img[begin:end], 0644)
	return err
}

//WriteKernel writes the kernel contained in AndroidBootImg to filepath
func (boot *AndroidBootImg) WriteKernel(filePath string) error {
	begin := boot.kernelOffset
	end := boot.kernelOffset + boot.hdr[kernelSize]
	err := ioutil.WriteFile(filePath, boot.img[begin:end], 0644)
	return err
}

//WriteSecond writes the second image contained in AndroidBootImg to filepath,
//as this image is not mandatory it returns error if not found.
func (boot *AndroidBootImg) WriteSecond(filePath string) error {
	if boot.hdr[secondSize] == 0 {
		return errors.New("Second size does not exist in this boot image")
	}
	begin := boot.secondOffset
	end := boot.secondOffset + boot.hdr[secondSize]
	err := ioutil.WriteFile(filePath, boot.img[begin:end], 0644)
	return err
}
