package sysutils

import (
	"os"
	"path/filepath"
	"testing"

	. "launchpad.net/gocheck"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type UtilsTestSuite struct {
	tmpdir string
}

var _ = Suite(&UtilsTestSuite{})

func (s *UtilsTestSuite) SetUpTest(c *C) {
	s.tmpdir = c.MkDir()
}

func (s *UtilsTestSuite) TestCreateEmptyFileGiB(c *C) {
	f := filepath.Join(s.tmpdir, "gib")
	c.Assert(CreateEmptyFile(f, 1, GiB), IsNil)

	fStat, err := os.Stat(f)
	c.Assert(err, IsNil)

	c.Assert(fStat.Size(), Equals, int64(1024*1024*1024))
}

func (s *UtilsTestSuite) TestCreateEmptyFileGB(c *C) {
	f1 := filepath.Join(s.tmpdir, "gb")
	c.Assert(CreateEmptyFile(f1, 1, GB), IsNil)

	fStat1, err := os.Stat(f1)
	c.Assert(err, IsNil)

	c.Assert(fStat1.Size(), Equals, int64(974999552))

	f2 := filepath.Join(s.tmpdir, "gb")
	c.Assert(CreateEmptyFile(f2, 2, GB), IsNil)

	fStat2, err := os.Stat(f2)
	c.Assert(err, IsNil)

	c.Assert(fStat2.Size(), Equals, int64(1949999616))
}
