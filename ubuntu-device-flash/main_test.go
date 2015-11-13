//
// ubuntu-device-flash - handles ubuntu disk images
//
// Copyright (c) 2015 Canonical Ltd.
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
	"os"
	"os/exec"

	. "launchpad.net/gocheck"
)

type ParserTestSuite struct{}

var _ = Suite(&ParserTestSuite{})

func (s *ParserTestSuite) TestExitStatusOnBadArgs(c *C) {
	if os.Getenv("BE_CRASHER") == "1" {
		execute([]string{"./ubuntu-device-flash", "--verbose", "core", "--output=/tmp/snappy.img", "--developer-mode"})
		return
	}

	cmd := exec.Command(os.Args[0], "-gocheck.f", "ParserTestSuite.TestExitStatusOnBadArgs")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	out, err := cmd.CombinedOutput()
	e, ok := err.(*exec.ExitError)
	c.Assert(ok, Equals, true)
	c.Assert(e.Success(), Equals, false)
	c.Assert(string(out), Equals, "the required argument `release` was not provided\n")
}
