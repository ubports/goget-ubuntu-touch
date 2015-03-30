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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"launchpad.net/goget-ubuntu-touch/sysutils"
	"launchpad.net/goyaml"
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
	CoreImage
	SystemImage
	hardware  HardwareDescription
	oem       OemDescription
	location  string
	size      int64
	baseMount string
	parts     []partition
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
kernel_file=vmlinuz
initrd_file=initrd.img
{{ . }}

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

func NewCoreUBootImage(location string, size int64, hw HardwareDescription, oem OemDescription) *CoreUBootImage {
	return &CoreUBootImage{
		hardware: hw,
		oem:      oem,
		location: location,
		size:     size,
	}
}

func (img *CoreUBootImage) Mount() (err error) {
	img.baseMount, err = ioutil.TempDir(os.TempDir(), "core-uboot-disk")
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

func (img *CoreUBootImage) Unmount() (err error) {
	if img.baseMount == "" {
		panic("No base mountpoint set")
	}
	defer func() {
		os.Remove(img.baseMount)
		img.baseMount = ""
	}()

	if out, err := exec.Command("sync").CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to sync filesystems before unmounting: %s", out)
	}

	for _, part := range img.parts {
		mountpoint := filepath.Join(img.baseMount, string(part.dir))
		if out, err := exec.Command("umount", mountpoint).CombinedOutput(); err != nil {
			panic(fmt.Sprintf("unable to unmount dir for image: %s", out))
		}
	}

	return nil
}

//Partition creates a partitioned image from an img
func (img *CoreUBootImage) Partition() error {
	if err := sysutils.CreateEmptyFile(img.location, img.size, sysutils.GB); err != nil {
		return err
	}

	parted, err := newParted(mkLabelMsdos)
	if err != nil {
		return err
	}

	parted.addPart(bootLabel, bootDir, fsFat32, 64)
	parted.addPart(systemALabel, systemADir, fsExt4, 1024)
	parted.addPart(systemBLabel, systemBDir, fsExt4, 1024)
	parted.addPart(writableLabel, writableDir, fsExt4, -1)

	parted.setBoot(1)

	img.parts = parted.parts

	return parted.create(img.location)
}

//Map creates loop devices for the partitions
func (img *CoreUBootImage) Map() error {
	if isMapped(img.parts) {
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
	if len(loops) != 4 {
		return errors.New("more partitions then expected while creating loop mapping")
	}

	mapPartitions(img.parts, loops)

	if err := kpartxCmd.Wait(); err != nil {
		return err
	}

	return nil
}

//Unmap destroys loop devices for the partitions
func (img *CoreUBootImage) Unmap() error {
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

	unmapPartitions(img.parts)

	return nil
}

func (img CoreUBootImage) Format() error {
	for _, part := range img.parts {
		dev := filepath.Join("/dev/mapper", part.loop)

		if part.fs == fsFat32 {
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
		} else {
			if out, err := exec.Command("mkfs.ext4", "-F", "-L", string(part.label), dev).CombinedOutput(); err != nil {
				return fmt.Errorf("unable to create filesystem: %s", out)
			}
		}
	}

	return nil
}

// User returns the writable path
func (img *CoreUBootImage) Writable() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(writableDir))
}

//System returns the system path
func (img *CoreUBootImage) System() string {
	if img.parts == nil {
		panic("img is not setup with partitions")
	}

	if img.baseMount == "" {
		panic("img not mounted")
	}

	return filepath.Join(img.baseMount, string(systemADir))
}

func (img CoreUBootImage) BaseMount() string {
	if img.baseMount == "" {
		panic("image needs to be mounted")
	}

	return img.baseMount
}

func (img CoreUBootImage) SetupBoot() error {
	// destinations
	bootPath := filepath.Join(img.baseMount, string(bootDir))
	bootAPath := filepath.Join(bootPath, "a")
	bootBPath := filepath.Join(bootPath, "b")
	bootuEnvPath := filepath.Join(bootPath, "uEnv.txt")
	bootSnappySystemPath := filepath.Join(bootPath, "snappy-system.txt")

	// origins
	hardwareYamlPath := filepath.Join(img.baseMount, "hardware.yaml")
	kernelPath := filepath.Join(img.baseMount, img.hardware.Kernel)
	initrdPath := filepath.Join(img.baseMount, img.hardware.Initrd)

	if err := os.MkdirAll(bootBPath, 0755); err != nil {
		return err
	}

	// populate both A/B
	for _, path := range []string{bootAPath, bootBPath} {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}

		if err := sysutils.CopyFile(hardwareYamlPath, filepath.Join(path, "hardware.yaml")); err != nil {
			return err
		}

		if err := sysutils.CopyFile(kernelPath, filepath.Join(path, filepath.Base(kernelPath))); err != nil {
			return err
		}

		if err := sysutils.CopyFile(initrdPath, filepath.Join(path, filepath.Base(initrdPath))); err != nil {
			return err
		}

		// create layout
		bootDtbPath := filepath.Join(path, "dtbs")
		if err := os.MkdirAll(bootDtbPath, 0755); err != nil {
			return err
		}

		if err := img.provisionDtbs(bootDtbPath); err != nil {
			return err
		}
	}

	if err := img.provisionUenv(bootuEnvPath); err != nil {
		return err
	}

	// create /boot/uboot
	if err := os.MkdirAll(filepath.Join(img.System(), "boot", "uboot"), 755); err != nil {
		return err
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

	t := template.Must(template.New("snappy-system").Parse(snappySystemTemplate))
	t.Execute(snappySystemFile, fdtfile)

	return nil
}

func (img CoreUBootImage) provisionUenv(bootuEnvPath string) error {
	platform := img.oem.Platform()

	if platform != "" {
		printOut("No platform select, not searching for uEnv.txt")
		return nil
	}

	flashAssetsPath := filepath.Join(img.baseMount, "flashtool-assets", platform)
	uEnvPath := filepath.Join(flashAssetsPath, "uEnv.txt")

	if _, err := os.Stat(flashAssetsPath); os.IsNotExist(err) {
		printOut("No flash assets path available")
		return nil
	} else if err != nil {
		return err
	}

	// if a uEnv.txt is provided in the flashtool-assets, use it
	if _, err := os.Stat(uEnvPath); err == nil {
		printOut("Adding uEnv.txt to", bootuEnvPath)
		if err := sysutils.CopyFile(uEnvPath, bootuEnvPath); err != nil {
			return err
		}
	} else {
		printOut("Can't copy", uEnvPath, "to", bootuEnvPath, "due to:", err)
	}

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

	dtb := filepath.Join(dtbsPath, fmt.Sprintf("%s.dtb", img.oem.Platform()))

	// if there is a specific dtb for the platform, copy it.
	// First look in oem and then in device.
	if oemDtb := img.oem.OEM.Hardware.Dtb; oemDtb != "" && img.oem.Platform() != "" {
		oemDtb := filepath.Join(img.System(), img.oem.InstallPath(), oemDtb)
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

func (img *CoreUBootImage) FlashExtra(devicePart string) error {
	if img.oem.Platform() == "" {
		return nil
	}

	tmpdir, err := ioutil.TempDir("", "device")
	if err != nil {
		return errors.New("cannot create tempdir to extract hardware.yaml from device part")
	}
	defer os.RemoveAll(tmpdir)

	if out, err := exec.Command("tar", "xf", devicePart, "-C", tmpdir, "flashtool-assets").CombinedOutput(); err != nil {
		fmt.Println("No flashtool-assets found, skipping...")
		if debugPrint {
			fmt.Println("device part:", devicePart)
			fmt.Println("command output:", string(out))
		}

		return nil
	}

	flashAssetsPath := filepath.Join(tmpdir, "flashtool-assets", img.oem.Platform())
	flashPath := filepath.Join(flashAssetsPath, "flash.yaml")

	if _, err := os.Stat(flashPath); err != nil && os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(flashPath)
	if err != nil {
		return err
	}

	var flash FlashInstructions
	if err := goyaml.Unmarshal([]byte(data), &flash); err != nil {
		return err
	}

	fmt.Println("Running flashtool-asset commands")
	for _, cmd := range flash.Bootloader {
		cmd = fmt.Sprintf(cmd, flashAssetsPath, img.location)
		cmdFields := strings.Fields(cmd)

		if out, err := exec.Command(cmdFields[0], cmdFields[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to run flash command: %s : %s", cmd, out)
		} else {
			fmt.Println(cmd)
			fmt.Println(string(out))
		}
	}

	return nil
}
