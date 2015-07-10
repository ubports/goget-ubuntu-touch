//
// diskimage - handles ubuntu disk images
//
// Copyright (c) 2015 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package diskimage

import (
	"fmt"
	"strings"
)

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

// ErrMapCount represents an error on the expected amount of partitions
type ErrMapCount struct {
	foundParts    int
	expectedParts int
}

func (e ErrMapCount) Error() string {
	return fmt.Sprintf("expected %d partitons but found %d", e.expectedParts, e.foundParts)
}

// ErrExec is an error from an external command
type ErrExec struct {
	output  []byte
	command []string
}

func (e ErrExec) Error() string {
	return fmt.Sprintf("error while executing external command %s: %s", strings.Join(e.command, " "), e.output)
}
