package sysutils

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
)

type FileOpsTestSuite struct {
	tmpdir  string
	srcPath string
	dstPath string
}

var _ = Suite(&FileOpsTestSuite{})

func (s *FileOpsTestSuite) SetUpTest(c *C) {
	s.tmpdir = c.MkDir()
	s.srcPath = filepath.Join(s.tmpdir, "src")
	s.dstPath = filepath.Join(s.tmpdir, "dst")
}

func (s *FileOpsTestSuite) TestCopyModes(c *C) {
	c.Assert(ioutil.WriteFile(s.srcPath, []byte("src"), 0755), IsNil)
	c.Assert(CopyFile(s.srcPath, s.dstPath), IsNil)

	stat, err := os.Stat(s.dstPath)
	c.Assert(err, IsNil)

	mode := stat.Mode()
	c.Assert(mode.IsRegular(), Equals, true)
	c.Assert(mode.String(), Equals, "-rwxr-xr-x")

	contents, err := ioutil.ReadFile(s.dstPath)
	c.Assert(err, IsNil)
	c.Assert(string(contents), Equals, "src")
}

func (s *FileOpsTestSuite) TestCopyNoSourceFails(c *C) {
	err := CopyFile(s.srcPath, s.dstPath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *FileOpsTestSuite) TestCopyDirFails(c *C) {
	c.Assert(os.MkdirAll(s.srcPath, 0755), IsNil)
	err := CopyFile(s.srcPath, s.dstPath)
	c.Assert(err, NotNil)
}
