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
	"os"
	"path/filepath"
	"testing"

	. "launchpad.net/gocheck"
	"launchpad.net/goget-ubuntu-touch/sysutils"
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

type BootAssetRawFilesTestSuite struct {
	oemRootPathDir string
	imagePath      string
	files          []BootAssetRawFiles
}

var _ = Suite(&BootAssetRawFilesTestSuite{})

const baseOffset = 60

func (s *BootAssetRawFilesTestSuite) createTestFiles(c *C) {
	for i := 0; i < 2; i++ {
		f := fmt.Sprintf(fileName, i)
		fAbsolutePath := filepath.Join(s.oemRootPathDir, f)
		c.Assert(ioutil.WriteFile(fAbsolutePath, []byte(f), 0644), IsNil)
		offset := fmt.Sprintf("%d", baseOffset+i*20)
		s.files = append(s.files, BootAssetRawFiles{Path: f, Offset: offset})
	}
}

func (s *BootAssetRawFilesTestSuite) verifyTestFiles(c *C) {
	img, err := os.Open(s.imagePath)
	c.Assert(err, IsNil)

	for i := range s.files {
		content := fmt.Sprintf(fileName, i)
		readContent := make([]byte, len(content))

		n, err := img.ReadAt(readContent, int64(baseOffset+i*20))
		c.Assert(err, IsNil)
		c.Assert(n, Equals, len(content))
		c.Assert(string(readContent), Equals, content)

		// check for zeros before and after
		zero := make([]byte, 1)
		_, err = img.ReadAt(zero, int64(baseOffset+i*20-1))
		c.Assert(err, IsNil)
		c.Assert(zero[0], Equals, uint8(0x0))

		_, err = img.ReadAt(zero, int64(baseOffset+i*20+len(content)))
		c.Assert(err, IsNil)
		c.Assert(zero[0], Equals, uint8(0x0))
	}
}

func (s *BootAssetRawFilesTestSuite) SetUpTest(c *C) {
	s.oemRootPathDir = c.MkDir()
	s.imagePath = filepath.Join(c.MkDir(), "image.img")

	c.Assert(sysutils.CreateEmptyFile(s.imagePath, 1, sysutils.GB), IsNil)

	s.createTestFiles(c)
}

func (s *BootAssetRawFilesTestSuite) TearDownTest(c *C) {
	s.files = nil
}

func (s *BootAssetRawFilesTestSuite) TestRawWrite(c *C) {
	c.Assert(setupBootAssetRawFiles(s.imagePath, s.oemRootPathDir, s.files), IsNil)

	s.verifyTestFiles(c)
}

func (s *BootAssetRawFilesTestSuite) TestRawWriteNoValidOffset(c *C) {
	f := fileName
	fAbsolutePath := filepath.Join(s.oemRootPathDir, f)
	c.Assert(ioutil.WriteFile(fAbsolutePath, []byte(f), 0644), IsNil)
	offset := "NaN"
	s.files = []BootAssetRawFiles{BootAssetRawFiles{Path: f, Offset: offset}}

	c.Assert(setupBootAssetRawFiles(s.imagePath, s.oemRootPathDir, s.files), NotNil)
}

func (s *BootAssetRawFilesTestSuite) TestRawWriteNoValidFile(c *C) {
	f := fileName
	offset := "10"
	s.files = []BootAssetRawFiles{BootAssetRawFiles{Path: f, Offset: offset}}

	c.Assert(setupBootAssetRawFiles(s.imagePath, s.oemRootPathDir, s.files), NotNil)
}

func (s *BootAssetRawFilesTestSuite) TestRawWriteNoValidImage(c *C) {
	c.Assert(setupBootAssetRawFiles("where_does_this_ref", s.oemRootPathDir, s.files), NotNil)
}
