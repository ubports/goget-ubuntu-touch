//
// ubuntu-device-flash - Tool to download and flash devices with an Ubuntu Image
//                       based system
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package main

import (
	"fmt"

	"launchpad.net/goget-ubuntu-touch/diskimage"
	"launchpad.net/goget-ubuntu-touch/ubuntuimage"
)

type imageFlavor string

const (
	minSizePersonal = 10
	minSizeCore     = 4
)

const (
	rootSizePersonal = 4096
	rootSizeCore     = 1024
)

const (
	flavorPersonal imageFlavor = "personal"
	flavorCore     imageFlavor = "core"
)

func (f imageFlavor) Channel() string {
	return fmt.Sprintf("ubuntu-%s", f)
}

func (f imageFlavor) minSize() int64 {
	switch f {
	case flavorPersonal:
		return minSizePersonal
	case flavorCore:
		return minSizeCore
	default:
		panic("invalid flavor")
	}
}

func (f imageFlavor) rootSize() int {
	switch f {
	case flavorPersonal:
		return rootSizePersonal
	case flavorCore:
		return rootSizeCore
	default:
		panic("invalid flavor")
	}
}

type Snapper struct {
	Channel string `long:"channel" description:"Specify the channel to use" default:"stable"`
	Output  string `long:"output" short:"o" description:"Name of the image file to create" required:"true"`
	Oem     string `long:"oem" description:"The snappy oem package to base the image out of" default:"generic-amd64"`
	StoreID string `long:"store" description:"Set an alternate store id."`

	Development struct {
		Install       []string `long:"install" description:"Install additional packages (can be called multiple times)"`
		DevicePart    string   `long:"device-part" description:"Specify a local device part to override the one from the server"`
		DeveloperMode bool     `long:"developer-mode" description:"Finds the latest public key in your ~/.ssh and sets it up using cloud-init"`
	} `group:"Development"`

	Positional struct {
		Release string `positional-arg-name:"release" description:"The release to base the image out of (15.04 or rolling)" required:"true"`
	} `positional-args:"yes" required:"yes"`

	img      diskimage.CoreImage
	hardware diskimage.HardwareDescription
	oem      diskimage.OemDescription

	size int64

	flavor            imageFlavor
	device            string
	customizationFunc []func() error
}

func (s *Snapper) systemImage() (*ubuntuimage.Image, error) {
	return nil, nil
}

func (s *Snapper) install(systemPath string) error {
	return nil
}

// deploy orchestrates the priviledged part of the setup
func (s *Snapper) deploy(systemImage *ubuntuimage.Image, filePathChan <-chan string) error {
	return nil
}

func (s *Snapper) create() error {
	return fmt.Errorf(`Building core images is currently not supported.

Images for ubuntu-core 15.04 can be build with the ppa:snappy-dev/tools.
Building images for ubuntu-core 16.04 will be supported by this tool soon.
`)
}
