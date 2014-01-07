// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package devices

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

var adbCommand string

func init() {
	// adbCommand = "adb.exe"
	adbCommand = "adb"
}

// Returns an AndroidDebugBridge while ensuring the server is started
func NewAndroidDebugBridge() (adb AndroidDebugBridge, err error) {
	err = exec.Command(adbCommand, "start-server").Run()
	return adb, err
}

// Returns an AndroidDebugBridge while ensuring the server is started
func NewUbuntuDebugBridge() (adb UbuntuDebugBridge, err error) {
	err = exec.Command(adbCommand, "start-server").Run()
	return adb, err
}

// Sets the serial device to connect to for future calls to *adb
func (adb *AndroidDebugBridge) SetSerial(serial string) {
	adb.serial = serial
	adb.params = append(adb.params, []string{"-s", serial}...)
}

// GetDevice parses the android property system to determine the device being used
func (adb *AndroidDebugBridge) GetDevice() (deviceName string, err error) {
	cmd := append(adb.params, []string{"shell", "getprop", "ro.product.device"}...)
	out, err := exec.Command(adbCommand, cmd...).Output()
	if err != nil {
		return deviceName, err
	}
	// This will fail if a device name ever leaves ASCII
	adb.deviceName = strings.TrimSpace(string(out))
	return adb.deviceName, err
}

// Push copies a file from src to dst over the adb server
func (adb AndroidDebugBridge) Push(src, dst string) (err error) {
	// TODO add file path verification
	cmd := append(adb.params, []string{"push", src, dst}...)
	err = exec.Command(adbCommand, cmd...).Run()
	return err
}

// Pull copies a file from src to dst over the adb server
func (adb AndroidDebugBridge) Pull(src, dst string) (err error) {
	// TODO add file path verification
	cmd := append(adb.params, []string{"pull", src, dst}...)
	err = exec.Command(adbCommand, cmd...).Run()
	return err
}

// RebootBooloader restarts the system into the bootloader
func (adb AndroidDebugBridge) RebootBootloader() (err error) {
	return adb.reboot("bootloader")
}

// RebootRecovery restarts the system into recovery
func (adb AndroidDebugBridge) RebootRecovery() (err error) {
	return adb.reboot("recovery")
}

// reboot restarts into the desired target
func (adb AndroidDebugBridge) reboot(mode string) (err error) {
	cmd := append(adb.params, []string{"reboot", mode}...)
	err = exec.Command(adbCommand, cmd...).Run()
	return err
}

// Reboot restarts the system from the shell. This is different than
// calling adb reboot directly.
func (adb UbuntuDebugBridge) Reboot() (err error) {
	cmd := append(adb.params, []string{"shell", "reboot"}...)
	err = exec.Command(adbCommand, cmd...).Run()
	return err
}

// WaitForRecovery idles until the image has booted into recovery
// for recovery
func (adb UbuntuDebugBridge) WaitForRecovery() error {
	// Recovery takes some time to get into, so we wait a bit
	time.Sleep(10 * time.Second)
	done := make(chan bool)
	cmd := append(adb.params, []string{"shell", "ls"}...)
	go func() {
		for {
			err := exec.Command(adbCommand, cmd...).Run()
			if err == nil {
				done <- true
			}
			time.Sleep(5 * time.Second)
		}
	}()
	for {
		select {
		case <-done:
			return nil
		case <-time.After(60 * time.Second):
			return errors.New(fmt.Sprint("Failed to enter Recovery"))
		}
	}
}
