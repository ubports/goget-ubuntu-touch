package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"launchpad.net/goget-ubuntu-touch/sysutils"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

func getImage(deviceChannel ubuntuimage.DeviceChannel) (image ubuntuimage.Image, err error) {
	if globalArgs.Revision <= 0 {
		image, err = deviceChannel.GetRelativeImage(globalArgs.Revision)
	} else {
		image, err = deviceChannel.GetImage(globalArgs.Revision)
	}
	return image, err
}

type Files struct{ FilePath, SigPath string }

// bitDownloader downloads
func bitDownloader(file ubuntuimage.File, files chan<- Files, server, downloadDir string) {
	// hack to circumvent https://code.google.com/p/go/issues/detail?id=1435
	if syscall.Getuid() == 0 {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()

		if err := sysutils.DropPrivs(); err != nil {
			log.Fatal(err)
		}
	}

	err := file.MakeRelativeToServer(server)
	if err != nil {
		log.Fatal(err)
	}
	err = file.Download(downloadDir)
	if err != nil {
		log.Fatal(err)
	}
	files <- Files{FilePath: filepath.Join(downloadDir, file.Path),
		SigPath: filepath.Join(downloadDir, file.Signature)}
}

// ensureExists touches a file. It can be used to create a dummy .asc file if none exists
func ensureExists(path string) error {
	f, err := os.OpenFile(path, syscall.O_WRONLY|syscall.O_CREAT, 0666)
	if err != nil {
		return fmt.Errorf("Cannot touch %s : %s", path, err)
	}
	f.Close()

	return nil
}

// expandFile checks for file existence, correct permissions and returns the absolute path.
func expandFile(path string) (abspath string, err error) {
	if p, err := filepath.Abs(path); err != nil {
		return "", err
	} else {
		abspath = p
	}

	fi, err := os.Lstat(abspath)
	if err != nil {
		return "", err
	}

	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a valid file", abspath)
	}

	return abspath, err
}

// isDevicePart checks if the file corresponds to the device part.
func isDevicePart(path string) bool {
	return strings.Contains(path, "device")
}

func copyFile(src, dst string) error {
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	reader := bufio.NewReader(srcFile)
	writer := bufio.NewWriter(dstFile)
	defer func() {
		if err != nil {
			writer.Flush()
		}
	}()
	if _, err = io.Copy(writer, reader); err != nil {
		return err
	}
	writer.Flush()
	return nil
}
