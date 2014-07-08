/*
 * Copyright 2014 Canonical Ltd.
 *
 * Authors:
 * Sergio Schvezov: sergio.schvezov@canonical.com
 *
 * This file is part of ubuntu-emulator.
 *
 * ubuntu-emulator is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; version 3.
 *
 * ubuntu-emulator is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"os"
	"path/filepath"
	"testing"
	. "launchpad.net/gocheck"
)

var _ = Suite(&EmulatorRunTestSuite{})

type EmulatorRunTestSuite struct {
	tmpDir string
}

func (s *EmulatorRunTestSuite) SetUpTest(c *C) {
	s.tmpDir = c.MkDir()
}

func Test(t *testing.T) { TestingT(t) }

func (s *EmulatorRunTestSuite) TestRunPathInstalled(c *C) {
	c.Assert(getEmulatorCmd(), Equals, filepath.Join(installPath, subpathEmulatorCmd))
}

func (s *EmulatorRunTestSuite) TestRunPathAndroidTree(c *C) {
	emuCmd := filepath.Join(s.tmpDir, subpathEmulatorCmd)
	os.MkdirAll(filepath.Dir(emuCmd), 0777)
	cmdFile, err := os.Create(emuCmd)
	c.Assert(err, IsNil)
	cmdFile.Chmod(0755)
	cmdFile.Close()
	c.Assert(os.Setenv("ANDROID_BUILD_TOP", s.tmpDir), IsNil)
	c.Assert(getEmulatorCmd(), Equals, emuCmd)
	c.Assert(os.Setenv("ANDROID_BUILD_TOP", ""), IsNil)
}
