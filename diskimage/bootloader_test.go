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
	"io/ioutil"
	"path/filepath"
	"testing"

	. "launchpad.net/gocheck"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type BootAssetFilesTestSuite struct {
	bootPathDir    string
	oemRootPathDir string
	files          []BootAssetFiles
}

var _ = Suite(&BootAssetFilesTestSuite{})

const fileName = "files-%d"

func (s *BootAssetFilesTestSuite) createTestFiles(c *C) {
	for i := 0; i < 3; i++ {
		f := fmt.Sprintf(fileName, i)
		fAbsolutePath := filepath.Join(s.oemRootPathDir, f)
		c.Assert(ioutil.WriteFile(fAbsolutePath, []byte(f), 0644), IsNil)
		s.files = append(s.files, BootAssetFiles{Path: f})
	}
}

func (s *BootAssetFilesTestSuite) verifyTestFiles(c *C) {
	for i := range s.files {
		f := fmt.Sprintf(fileName, i)

		fAbsolutePath := filepath.Join(s.bootPathDir, f)
		if s.files[i].Target != "" {
			fAbsolutePath = filepath.Join(s.bootPathDir, s.files[i].Target)
		}

		content, err := ioutil.ReadFile(fAbsolutePath)
		c.Assert(err, IsNil)
		c.Assert(string(content), Equals, fmt.Sprintf("files-%d", i))
	}
}

func (s *BootAssetFilesTestSuite) SetUpTest(c *C) {
	s.bootPathDir = c.MkDir()
	s.oemRootPathDir = c.MkDir()

	s.createTestFiles(c)
}

func (s *BootAssetFilesTestSuite) TearDownTest(c *C) {
	s.files = nil
}

func (s *BootAssetFilesTestSuite) TestCopyFiles(c *C) {
	c.Assert(setupBootAssetFiles(s.bootPathDir, s.oemRootPathDir, s.files), IsNil)

	s.verifyTestFiles(c)
}

func (s *BootAssetFilesTestSuite) TestCopyFilesWithTarget(c *C) {
	f := fmt.Sprintf(fileName, len(s.files))
	fAbsolutePath := filepath.Join(s.oemRootPathDir, f)
	c.Assert(ioutil.WriteFile(fAbsolutePath, []byte(f), 0644), IsNil)
	s.files = append(s.files, BootAssetFiles{Path: f, Target: "otherpath"})

	c.Assert(setupBootAssetFiles(s.bootPathDir, s.oemRootPathDir, s.files), IsNil)

	s.verifyTestFiles(c)
}

func (s *BootAssetFilesTestSuite) TestCopyFilesWithTargetInDir(c *C) {
	f := fmt.Sprintf(fileName, len(s.files))
	fAbsolutePath := filepath.Join(s.oemRootPathDir, f)
	c.Assert(ioutil.WriteFile(fAbsolutePath, []byte(f), 0644), IsNil)
	s.files = append(s.files, BootAssetFiles{Path: f, Target: filepath.Join("subpath", "otherpath")})

	c.Assert(setupBootAssetFiles(s.bootPathDir, s.oemRootPathDir, s.files), IsNil)

	s.verifyTestFiles(c)
}
