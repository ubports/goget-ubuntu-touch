//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"launchpad.net/goget-ubuntu-touch/sysutils"
)

func setupBootAssetFiles(bootMount, bootPath, oemRootPath string, files []BootAssetFiles) error {
	printOut("Setting up boot asset files from", oemRootPath, "...")
	for _, file := range files {
		dst := filepath.Join(bootPath, filepath.Base(file.Path))
		if file.Dst != "" {
			dst = filepath.Join(bootMount, file.Dst)
		} else if file.Target != "" {
			dst = filepath.Join(bootPath, file.Target)
		}
		dstDir := filepath.Dir(dst)
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				return err
			}
		}

		src := filepath.Join(oemRootPath, file.Path)
		printOut("Copying", src, "to", dst)
		if err := sysutils.CopyFile(src, dst); err != nil {
			return err
		}
	}

	return nil
}

func setupBootAssetRawFiles(imagePath, oemRootPath string, rawFiles []BootAssetRawFiles) error {
	printOut("Setting up raw boot assets from", oemRootPath, "...")
	img, err := os.OpenFile(imagePath, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer img.Close()

	for _, asset := range rawFiles {
		offsetBytes, err := offsetBytes(asset.Offset)
		if err != nil {
			return err
		}

		src := filepath.Join(oemRootPath, asset.Path)
		printOut("Writing", src)
		assetFile, err := os.Open(src)
		if err != nil {
			return err
		}

		if err := rawwrite(img, assetFile, offsetBytes); err != nil {
			return err
		}
	}

	return nil
}

func setupBootAssetRawPartitions(imagePath string, partCount int, rawPartitions []BootAssetRawPartitions) error {
	printOut("Setting up raw boot asset partitions for", imagePath, "...")
	var part int = partCount

	for _, asset := range rawPartitions {
		part += 1

		size, err := strconv.Atoi(asset.Size)
		if err != nil {
			return err
		}
		size = size * 2

		printOut("creating partition:", asset.Name)

		opts := fmt.Sprintf("0:0:+%d", size)
		if err := exec.Command("sgdisk", "-a", "1", "-n", opts, imagePath).Run(); err != nil {
			return err
		}

		opts = fmt.Sprintf("%d:%s", part, asset.Name)
		if err := exec.Command("sgdisk", "-c", opts, imagePath).Run(); err != nil {
			return err
		}

		opts = fmt.Sprintf("%d:%s", part, asset.Type)
		if err := exec.Command("sgdisk", "-t", opts, imagePath).Run(); err != nil {
			return err
		}

	}

	printOut("sorting partitions")
	if err := exec.Command("sgdisk", "-s", imagePath).Run(); err != nil {
		return err
	}

	return nil
}

func offsetBytes(offset string) (int64, error) {
	// TODO add support for units
	return strconv.ParseInt(offset, 10, 64)
}

func rawwrite(img *os.File, asset io.Reader, offset int64) error {
	if _, err := img.Seek(offset, 0); err != nil {
		return err
	}

	if _, err := io.Copy(img, asset); err != nil {
		return err
	}

	return nil
}
