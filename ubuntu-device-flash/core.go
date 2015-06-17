//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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

func init() {
	parser.AddCommand("core",
		"Creates ubuntu core images",
		"",
		&coreCmd)
}

type CoreCmd struct {
	EnableSsh bool  `long:"enable-ssh" description:"Enable ssh on the image through cloud-init(not needed with developer mode)"`
	Size      int64 `long:"size" short:"s" description:"Size of image file to create in GB (min 4)" default:"4"`

	Deprecated struct {
		Cloud  bool   `long:"cloud" description:"Generate a pure cloud image without setting up cloud-init"`
		Device string `long:"device" description:"Specify the device to use"`
	} `group:"Deprecated"`

	Snapper
}

var coreCmd CoreCmd

const cloudInitMetaData = `instance-id: nocloud-static
`

const cloudInitUserData = `#cloud-config
password: ubuntu
chpasswd: { expire: False }
ssh_pwauth: True
ssh_genkeytypes: ['rsa', 'dsa', 'ecdsa', 'ed25519']
`

func (coreCmd *CoreCmd) Execute(args []string) error {
	coreCmd.flavor = flavorCore
	coreCmd.size = coreCmd.Size

	if coreCmd.EnableSsh && coreCmd.Deprecated.Cloud {
		return errors.New("--cloud and --enable-ssh cannot be used together")
	}

	if coreCmd.Deprecated.Device != "" {
		fmt.Println("WARNING: this option should only be used to build azure images")
		coreCmd.device = coreCmd.Deprecated.Device
	}

	if !coreCmd.Deprecated.Cloud {
		coreCmd.customizationFunc = append(coreCmd.customizationFunc, coreCmd.setupCloudInit)
	}

	return coreCmd.create()
}

// this function is a hack and should be part of first boot.
func (coreCmd *CoreCmd) setupCloudInit() error {
	systemPath := coreCmd.img.System()
	writablePath := coreCmd.img.Writable()

	cloudBaseDir := filepath.Join("var", "lib", "cloud")

	if err := os.MkdirAll(filepath.Join(systemPath, cloudBaseDir), 0755); err != nil {
		return err
	}

	// create a basic cloud-init seed
	cloudDir := filepath.Join(writablePath, "system-data", cloudBaseDir, "seed", "nocloud-net")
	if err := os.MkdirAll(cloudDir, 0755); err != nil {
		return err
	}

	metaDataFile, err := os.Create(filepath.Join(cloudDir, "meta-data"))
	if err != nil {
		return err
	}
	defer metaDataFile.Close()

	if _, err := io.WriteString(metaDataFile, cloudInitMetaData); err != nil {
		return err
	}

	if coreCmd.Development.DeveloperMode {
		fmt.Println("Enabling developer mode...")

		authorizedKey, err := getAuthorizedSshKey()
		if err != nil {
			return fmt.Errorf("failed to obtain a public key for developer mode: %s", err)
		}

		if _, err := io.WriteString(metaDataFile, "public-keys:\n"); err != nil {
			return err
		}

		if _, err := io.WriteString(metaDataFile, fmt.Sprintf("  - %s\n", authorizedKey)); err != nil {
			return err
		}

	}

	userDataFile, err := os.Create(filepath.Join(cloudDir, "user-data"))
	if err != nil {
		return err
	}
	defer userDataFile.Close()

	if _, err := io.WriteString(userDataFile, cloudInitUserData); err != nil {
		return err
	}

	if coreCmd.Development.DeveloperMode || coreCmd.EnableSsh {
		if _, err := io.WriteString(userDataFile, "snappy:\n    ssh_enabled: True\n"); err != nil {
			return err
		}
	}

	return nil
}

func getAuthorizedSshKey() (string, error) {
	sshDir := os.ExpandEnv("$HOME/.ssh")

	fis, err := ioutil.ReadDir(sshDir)
	if err != nil {
		return "", fmt.Errorf("%s: no pub ssh key found, run ssh-keygen first", err)
	}

	var preferredPubKey string
	var latestModTime time.Time
	for i := range fis {
		file := fis[i].Name()
		if strings.HasSuffix(file, ".pub") && !strings.HasSuffix(file, "cert.pub") {
			fileMod := fis[i].ModTime()

			if fileMod.After(latestModTime) {
				latestModTime = fileMod
				preferredPubKey = file
			}
		}
	}

	if preferredPubKey == "" {
		return "", errors.New("no pub ssh key found, run ssh-keygen first")
	}

	pubKey, err := ioutil.ReadFile(filepath.Join(sshDir, preferredPubKey))

	return string(pubKey), err
}
