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
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
)

type CommonTestSuite struct {
	tmpdir      string
	oem         OemDescription
	packageInst string
}

var _ = Suite(&CommonTestSuite{})

func (s *CommonTestSuite) SetUpTest(c *C) {
	s.tmpdir = c.MkDir()
	s.oem = OemDescription{Name: "packagename", Version: "42"}
	s.packageInst = fmt.Sprintf("%s.%s", s.oem.Name, "sideload")
}

func (s *CommonTestSuite) TestOemInstallPath(c *C) {
	err := os.MkdirAll(filepath.Join(s.tmpdir, "oem", s.packageInst, s.oem.Version), 0755)
	c.Assert(err, IsNil)

	installPath, err := s.oem.InstallPath(s.tmpdir)

	c.Assert(err, IsNil)
	c.Assert(installPath, Equals, filepath.Join(s.tmpdir, "/oem/packagename.sideload/42"))
}

func (s *CommonTestSuite) TestOemInstallPathNoOem(c *C) {
	err := os.MkdirAll(filepath.Join(s.tmpdir, "oem", s.packageInst), 0755)
	c.Assert(err, IsNil)

	installPath, err := s.oem.InstallPath(s.tmpdir)

	c.Assert(err, NotNil)
	c.Assert(installPath, Equals, "")
}
