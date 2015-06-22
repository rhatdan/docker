package main

import (
	"testing"

	"github.com/go-check/check"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func init() {
	check.Suite(&DockerSuite{})
}

type DockerSuite struct {
}

func (s *DockerSuite) TearDownTest(c *check.C) {
	deleteAllContainers()
	deleteAllImages()
}

func init() {
	check.Suite(&DockerRegistrySuite{
		ds: &DockerSuite{},
	})
}

type DockerRegistrySuite struct {
	ds  *DockerSuite
	reg *testRegistryV2
}

func (s *DockerRegistrySuite) SetUpTest(c *check.C) {
	s.reg = setupRegistry(c)
}

func (s *DockerRegistrySuite) TearDownTest(c *check.C) {
	s.reg.Close()
	s.ds.TearDownTest(c)
}

func init() {
	check.Suite(&DockerDaemonSuite{
		ds: &DockerSuite{},
	})
}

type DockerDaemonSuite struct {
	ds *DockerSuite
	d  *Daemon
}

func (s *DockerDaemonSuite) SetUpTest(c *check.C) {
	s.d = NewDaemon(c)
}

func (s *DockerDaemonSuite) TearDownTest(c *check.C) {
	s.d.Stop()
	s.ds.TearDownTest(c)
}

type DockerRegistriesSuite struct {
	ds   *DockerSuite
	reg1 *testRegistryV2
	reg2 *testRegistryV2
}

func init() {
	check.Suite(&DockerRegistriesSuite{
		ds: &DockerSuite{},
	})
}

func (s *DockerRegistriesSuite) SetUpTest(c *check.C) {
	s.reg1 = setupRegistryAt(c, privateRegistryURLs[0])
	s.reg2 = setupRegistryAt(c, privateRegistryURLs[1])
}

func (s *DockerRegistriesSuite) TearDownTest(c *check.C) {
	s.reg2.Close()
	s.reg1.Close()
	s.ds.TearDownTest(c)
}
