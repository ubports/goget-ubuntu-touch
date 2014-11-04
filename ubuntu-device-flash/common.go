package main

import (
	"log"
	"path/filepath"

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
