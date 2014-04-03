//
// Helpers to work with an Ubuntu image based Upgrade implementation
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package ubuntuimage

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
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cheggaaa/pb"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
)

func hashMatches(filePath, hash string) bool {
	h := sha256.New()
	var err error
	fileBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false
	}
	h.Write(fileBytes)
	hashString := hex.EncodeToString(h.Sum(nil))
	if hashString == hash {
		return true
	}
	return false
}

const commandsStart = `format system
load_keyring image-master.tar.xz image-master.tar.xz.asc
load_keyring image-signing.tar.xz image-signing.tar.xz.asc
mount system
`

func GetUbuntuCommands(files []File, downloadDir string, wipe bool) (commandsFile string, err error) {
	tmpFile, err := ioutil.TempFile(downloadDir, "ubuntu_commands")
	if err != nil {
		return commandsFile, err
	}
	writer := bufio.NewWriter(tmpFile)
	defer func() {
		if err == nil {
			writer.Flush()
		}
	}()
	if wipe {
		writer.WriteString("format data\n")
	}
	writer.WriteString(commandsStart)
	order := func(f1, f2 *File) bool {
		return f1.Order < f2.Order
	}
	By(order).Sort(files)
	for _, file := range files {
		writer.WriteString(
			fmt.Sprintf("update %s %s\n", filepath.Base(file.Path), filepath.Base(file.Signature)))
	}
	writer.WriteString("unmount system\n")
	return tmpFile.Name(), err
}

func GetGPGFiles() []File {
	return []File{{Path: "/gpg/image-master.tar.xz", Signature: "/gpg/image-master.tar.xz.asc"},
		{Path: "/gpg/image-signing.tar.xz", Signature: "/gpg/image-signing.tar.xz.asc"}}
}

func getLockFd(file string) (fileLock *os.File, err error) {
	err = os.MkdirAll(filepath.Dir(file), 0700)
	if err != nil {
		return nil, err
	}
	fileLock, err = os.Create(file + "_lock")
	if err != nil {
		return nil, err
	}
	return fileLock, nil
}

func (file File) Download(downloadDir string) (err error) {
	//TODO Verify downloaded gpg agains image
	path := filepath.Join(downloadDir, file.Path)
	// Create file lock to avoid multiple processes downloading the same file
	lock, err := getLockFd(path)
	if err != nil {
		return err
	}
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer func() {
		os.Remove(lock.Name())
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	}()

	if hashMatches(path, file.Checksum) {
		return nil
	}
	for _, f := range []string{file.Signature, file.Path} {
		uri := file.Server + f
		path := filepath.Join(downloadDir, f)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		if err != nil {
			return err
		}
		target, err := os.Create(path + "_")
		if err != nil {
			return err
		}
		defer func() {
			target.Close()
			if err == nil {
				os.Rename(path+"_", path)
			}
		}()
		err = download(uri, target)
		if err != nil {
			return err
		}
	}
	return err
}

func download(uri string, writer io.Writer) (err error) {
	resp, err := http.Get(uri)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("Got status code %d for %s", resp.StatusCode, uri))
	}
	defer resp.Body.Close()

	// Only display a progress bar for files > 2K in size
	// nb, ContentLength is -1 for *.asc files
	if (resp.ContentLength > 2048) {
		pbar := pb.New(int(resp.ContentLength))
		pbar.ShowSpeed = true
		pbar.Units = pb.U_BYTES
		pbar.Start()

		mw := io.MultiWriter(writer, pbar)
		_, err = io.Copy(mw, resp.Body)
		pbar.Finish()
	} else {
		bufReader := bufio.NewReader(resp.Body)
		bufWriter := bufio.NewWriter(writer)
		defer func() {
			if err == nil {
				err = bufWriter.Flush()
			}
		}()

		_, err = bufReader.WriteTo(bufWriter)
	}

	return err
}


func GetCacheDir() (cacheDir string) {
	cacheDir = os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		cacheDir = "$HOME/.cache/"
	}
	cacheDir = os.ExpandEnv(cacheDir)
	return filepath.Join(cacheDir, "ubuntuimages")
}
