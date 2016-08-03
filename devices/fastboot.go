//
// Helpers to talk to devices that support ADB or Fastboot
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package devices

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
	"os/exec"
	"strings"
)

var fastbootCommand string

func init() {
	fastbootCommand = "fastboot"
}

// Sets the serial device to connect to for future calls to *fastboot
func (fastboot *Fastboot) SetSerial(serial string) {
	fastboot.serial = serial
	fastboot.params = append(fastboot.params, []string{"-s", serial}...)
}

// Reboot restarts the system from the Android bootloader
func (fastboot Fastboot) Reboot() (err error) {
	cmd := append(fastboot.params, []string{"reboot"}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}

// Boot boots the system from the Android bootloader from the boot partition
func (fastboot Fastboot) Boot() (err error) {
	cmd := append(fastboot.params, []string{"boot"}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}

// BootImage boots the system from the Android specified boot image
func (fastboot Fastboot) BootImage(image string) (err error) {
	cmd := append(fastboot.params, []string{"boot", image}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}

// Flash flashes the specified image to partition on the device
func (fastboot Fastboot) Flash(partition, image string) (err error) {
	cmd := append(fastboot.params, []string{"flash", partition, image}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}

// Format formats the specified partition on the device
func (fastboot Fastboot) Format(partition string) (err error) {
	cmd := append(fastboot.params, []string{"format", partition}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}

// GetDevice obtains the device name from fastboot
func (fastboot Fastboot) GetDevice() (device string, err error) {
	cmd := append(fastboot.params, []string{"getvar", "product"}...)
	deviceOutput, err := exec.Command(fastbootCommand, cmd...).CombinedOutput()
	lines := strings.Split(string(deviceOutput), "\n")
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if strings.Contains(fields[0], "product") {
			device = strings.TrimSpace(fields[1])
			break
		}
	}
	return device, err
}

// Send OEM specific commands to bootloader
func (fastboot Fastboot) SendOemCommand(command string) (err error) {
	cmd := append(fastboot.params, []string{"oem", command}...)
	err = exec.Command(fastbootCommand, cmd...).Run()
	return err
}
