//
// diskimage - handles ubuntu disk images
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
	"testing"

	. "launchpad.net/gocheck"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type SnappyTestSuite struct{}

var _ = Suite(&SnappyTestSuite{})

func (s *SnappyTestSuite) TestLegacy(c *C) {
	c.Check(isLegacy("rolling", "edge", 1), Equals, false)

	c.Check(isLegacy("15.04", "edge", 1), Equals, true)
	c.Check(isLegacy("15.04", "edge", 149), Equals, true)
	c.Check(isLegacy("15.04", "edge", 150), Equals, false)

	c.Check(isLegacy("15.04", "alpha", 1), Equals, true)
	c.Check(isLegacy("15.04", "alpha", 9), Equals, true)
	c.Check(isLegacy("15.04", "alpha", 10), Equals, false)

	c.Check(isLegacy("15.04", "stable", 1), Equals, true)
	c.Check(isLegacy("15.04", "stable", 4), Equals, true)
	c.Check(isLegacy("15.04", "stable", 5), Equals, false)
}
