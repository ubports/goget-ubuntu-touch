//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import "fmt"

// ErrMount represents a mount error
type ErrMount struct {
	dev        string
	mountpoint string
	fs         fsType
	out        []byte
}

func (e ErrMount) Error() string {
	return fmt.Sprintf("cannot mount %s(%s) on %s: %s", e.dev, e.fs, e.mountpoint, e.out)
}
