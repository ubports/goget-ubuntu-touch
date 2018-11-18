//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

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

type CoreUBootImage struct {
	BaseImage
}

const snappySystemTemplate = `# This is a snappy variables and boot logic file and is entirely generated and
# managed by Snappy. Modifications may break boot
######
# functions to load kernel, initrd and fdt from various env values
loadfiles=run loadkernel; run loadinitrd; run loadfdt
loadkernel=load mmc ${mmcdev}:${mmcpart} ${loadaddr} ${snappy_ab}/${kernel_file}
loadinitrd=load mmc ${mmcdev}:${mmcpart} ${initrd_addr} ${snappy_ab}/${initrd_file}; setenv initrd_size ${filesize}
loadfdt=load mmc ${mmcdev}:${mmcpart} ${fdtaddr} ${snappy_ab}/dtbs/${fdtfile}

# standard kernel and initrd file names; NB: fdtfile is set early from bootcmd
kernel_file={{ .Kernel }}
initrd_file={{ .Initrd }}
{{ .Fdt }}

# extra kernel cmdline args, set via mmcroot
snappy_cmdline=init=/lib/systemd/systemd ro panic=-1 fixrtc

# boot logic
# either "a" or "b"; target partition we want to boot
snappy_ab=a
# stamp file indicating a new version is being tried; removed by s-i after boot
snappy_stamp=snappy-stamp.txt
# either "regular" (normal boot) or "try" when trying a new version
snappy_mode=regular
# if we're trying a new version, check if stamp file is already there to revert
# to other version
snappy_boot=if test "${snappy_mode}" = "try"; then if test -e mmc ${bootpart} ${snappy_stamp}; then if test "${snappy_ab}" = "a"; then setenv snappy_ab "b"; else setenv snappy_ab "a"; fi; else fatwrite mmc ${mmcdev}:${mmcpart} 0x0 ${snappy_stamp} 0; fi; fi; run loadfiles; setenv mmcroot /dev/disk/by-label/system-${snappy_ab} ${snappy_cmdline}; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} ${fdtaddr}
`

type FlashInstructions struct {
	Bootloader []string `yaml:"bootloader"`
}

func NewCoreUBootImage(location string, size int64, rootSize int, hw HardwareDescription, oem OemDescription, label string) *CoreUBootImage {
	return &CoreUBootImage{
		BaseImage{
			hardware:  hw,
			oem:       oem,
			location:  location,
			size:      size,
			rootSize:  rootSize,
			partCount: 4,
			label:     label,
		},
	}
}

//Partition creates a partitioned image from an img
func (img *CoreUBootImage) Partition() error {
	if err := sysutils.CreateEmptyFile(img.location, img.size, sysutils.GB); err != nil {
		return err
	}
	table := mkLabelMsdos

	if img.label == "gpt" {
		table = mkLabelGpt
	}
	parted, err := newParted(table)
	if err != nil {
		return err
	}

	parted.addPart(bootLabel, bootDir, fsFat32, 128)
	parted.addPart(systemALabel, systemADir, fsExt4, 1024)
	parted.addPart(systemBLabel, systemBDir, fsExt4, 1024)
	parted.addPart(writableLabel, writableDir, fsExt4, -1)

	parted.setBoot(1)

	img.parts = parted.parts

	return parted.create(img.location)
}

func (img CoreUBootImage) SetupBoot() error {
	// destinations
	bootPath := filepath.Join(img.baseMount, string(bootDir))
	bootSnappySystemPath := filepath.Join(bootPath, "snappy-system.txt")

	if err := img.GenericBootSetup(bootPath); err != nil {
		return err
	}

	// populate both A/B
	for _, part := range img.oem.SystemParts() {
		bootDtbPath := filepath.Join(bootPath, part, "dtbs")
		if err := img.provisionDtbs(bootDtbPath); err != nil {
			return err
		}
	}

	snappySystemFile, err := os.Create(bootSnappySystemPath)
	if err != nil {
		return err
	}
	defer snappySystemFile.Close()

	var fdtfile string
	if platform := img.oem.Platform(); platform != "" {
		fdtfile = fmt.Sprintf("fdtfile=%s.dtb", platform)
	}

	templateData := struct{ Fdt, Kernel, Initrd string }{
		Fdt: fdtfile, Kernel: kernelFileName, Initrd: initrdFileName,
	}

	t := template.Must(template.New("snappy-system").Parse(snappySystemTemplate))
	t.Execute(snappySystemFile, templateData)

	return nil
}

func (img CoreUBootImage) provisionDtbs(bootDtbPath string) error {
	dtbsPath := filepath.Join(img.baseMount, img.hardware.Dtbs)

	if _, err := os.Stat(dtbsPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	dtbFis, err := ioutil.ReadDir(dtbsPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(bootDtbPath, 0755); err != nil {
		return err
	}

	dtb := filepath.Join(dtbsPath, fmt.Sprintf("%s.dtb", img.oem.Platform()))

	// if there is a specific dtb for the platform, copy it.
	// First look in oem and then in device.
	if oemDtb := img.oem.OEM.Hardware.Dtb; oemDtb != "" && img.oem.Platform() != "" {
		oemRoot, err := img.oem.InstallPath()
		if err != nil {
			return err
		}

		oemDtb := filepath.Join(oemRoot, oemDtb)
		dst := filepath.Join(bootDtbPath, filepath.Base(dtb))
		if err := sysutils.CopyFile(oemDtb, dst); err != nil {
			return err
		}
	} else if _, err := os.Stat(dtb); err == nil {
		dst := filepath.Join(bootDtbPath, filepath.Base(dtb))
		if err := sysutils.CopyFile(dtb, dst); err != nil {
			return err
		}
	} else {
		for _, dtbFi := range dtbFis {
			src := filepath.Join(dtbsPath, dtbFi.Name())
			dst := filepath.Join(bootDtbPath, dtbFi.Name())
			if err := sysutils.CopyFile(src, dst); err != nil {
				return err
			}
		}
	}

	return nil
}
