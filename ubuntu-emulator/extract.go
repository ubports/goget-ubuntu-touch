//
// ubuntu-emu - Tool to download and run Ubuntu Touch emulator instances
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

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
	"archive/tar"
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

func flatExtractImages(tarxz, dataDir string) error {
	f, err := os.Open(tarxz)
	if err != nil {
		return errors.New(fmt.Sprintf("Can't open %s for extraction", tarxz))
	}
	r := xzReader(f)
	tempfile, err := ioutil.TempFile(os.TempDir(), "image")
	if err != nil {
		return errors.New("Can't create tempfile to create emulator instance")
	}
	defer func() {
		tempfile.Close()
		os.Remove(tempfile.Name())
	}()
	n, err := io.Copy(tempfile, r)
	if err != nil {
		return errors.New(fmt.Sprintf("copied %d bytes with err: %v", n, err))
	}
	_, err = tempfile.Seek(0, 0)
	if err != nil {
		return errors.New("Failed to rewind")
	}
	tr := tar.NewReader(tempfile)
	var imageFile *os.File
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return err
		}
		imageFileName := path.Base(hdr.Name)
		if path.Ext(imageFileName) != ".img" {
			fmt.Println("Non image file found:", imageFileName, hdr.Name)
			continue
		}
		if imageFile, err = os.Create(filepath.Join(dataDir, imageFileName)); err != nil {
			return err
		}
		defer imageFile.Close()
		writer := bufio.NewWriter(imageFile)
		reader := bufio.NewReader(tr)
		defer func() {
			if err == nil {
				err = writer.Flush()
			}
		}()

		if _, err := io.Copy(writer, reader); err != nil {
			return err
		}
	}
	return nil
}

func xzReader(r io.Reader) io.ReadCloser {
	rpipe, wpipe := io.Pipe()

	cmd := exec.Command("xz", "--decompress", "--stdout")
	cmd.Stdin = r
	cmd.Stdout = wpipe

	go func() {
		err := cmd.Run()
		wpipe.CloseWithError(err)
	}()

	return rpipe
}
