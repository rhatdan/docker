package main

import (
	"archive/tar"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/go-check/check"
)

// pulling an image from the central registry should work
func (s *DockerRegistrySuite) TestPushBusyboxImage(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)
	// tag the image to upload it to the private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		c.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
}

// pushing an image without a prefix should throw an error
func (s *DockerSuite) TestPushUnprefixedRepo(c *check.C) {
	pushCmd := exec.Command(dockerBinary, "push", "busybox")
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		c.Fatalf("pushing an unprefixed repo didn't result in a non-zero exit status: %s", out)
	}
}

func (s *DockerRegistrySuite) TestPushUntagged(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)

	expected := "Repository does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		c.Fatalf("pushing the image to the private registry should have failed: output %q", out)
	} else if !strings.Contains(out, expected) {
		c.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
}

func (s *DockerRegistrySuite) TestPushBadTag(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox:latest", s.reg.url)

	expected := "does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		c.Fatalf("pushing the image to the private registry should have failed: output %q", out)
	} else if !strings.Contains(out, expected) {
		c.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
}

func (s *DockerRegistrySuite) TestPushMultipleTags(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)
	repoTag1 := fmt.Sprintf("%v/dockercli/busybox:t1", s.reg.url)
	repoTag2 := fmt.Sprintf("%v/dockercli/busybox:t2", s.reg.url)
	// tag the image and upload it to the private registry
	tagCmd1 := exec.Command(dockerBinary, "tag", "busybox", repoTag1)
	if out, _, err := runCommandWithOutput(tagCmd1); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}
	tagCmd2 := exec.Command(dockerBinary, "tag", "busybox", repoTag2)
	if out, _, err := runCommandWithOutput(tagCmd2); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		c.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
}

func (s *DockerRegistrySuite) TestPushInterrupt(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)
	// tag the image and upload it to the private registry
	if out, _, err := runCommandWithOutput(exec.Command(dockerBinary, "tag", "busybox", repoName)); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if err := pushCmd.Start(); err != nil {
		c.Fatalf("Failed to start pushing to private registry: %v", err)
	}

	// Interrupt push (yes, we have no idea at what point it will get killed).
	time.Sleep(200 * time.Millisecond)
	if err := pushCmd.Process.Kill(); err != nil {
		c.Fatalf("Failed to kill push process: %v", err)
	}
	if out, _, err := runCommandWithOutput(exec.Command(dockerBinary, "push", repoName)); err == nil {
		str := string(out)
		if !strings.Contains(str, "already in progress") {
			c.Fatalf("Push should be continued on daemon side, but seems ok: %v, %s", err, out)
		}
	}
	// now wait until all this pushes will complete
	// if it failed with timeout - there would be some error,
	// so no logic about it here
	for exec.Command(dockerBinary, "push", repoName).Run() != nil {
	}
}

func (s *DockerRegistrySuite) TestPushEmptyLayer(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/emptylayer", s.reg.url)
	emptyTarball, err := ioutil.TempFile("", "empty_tarball")
	if err != nil {
		c.Fatalf("Unable to create test file: %v", err)
	}
	tw := tar.NewWriter(emptyTarball)
	err = tw.Close()
	if err != nil {
		c.Fatalf("Error creating empty tarball: %v", err)
	}
	freader, err := os.Open(emptyTarball.Name())
	if err != nil {
		c.Fatalf("Could not open test tarball: %v", err)
	}

	importCmd := exec.Command(dockerBinary, "import", "-", repoName)
	importCmd.Stdin = freader
	out, _, err := runCommandWithOutput(importCmd)
	if err != nil {
		c.Errorf("import failed with errors: %v, output: %q", err, out)
	}

	// Now verify we can push it
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		c.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
}

func (s *DockerRegistrySuite) TestPushToAdditionalRegistry(c *check.C) {
	d := NewDaemon(c)
	if err := d.StartWithBusybox("--add-registry=" + s.reg.url); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing add-registry=%s: %v", s.reg.url, err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// push busybox to additional registry as "library/busybox" and remove all local images
	if out, err := d.Cmd("tag", "busybox", "library/busybox"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", "library/busybox"); err != nil {
		c.Fatalf("failed to push image library/busybox: error %v, output %q", err, out)
	}
	toRemove := []string{"busybox", "library/busybox"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// pull it from additional registry
	if _, err := d.Cmd("pull", "library/busybox"); err != nil {
		c.Fatalf("we should have been able to pull library/busybox from %q: %v", s.reg.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg.url+"/library/busybox", busyboxId)
}

func (s *DockerSuite) TestPushOfficialImage(c *check.C) {
	var reErr = regexp.MustCompile(`rename your repository to[^:]*:\s*<user>/busybox\b`)

	// push busybox to public registry as "library/busybox"
	cmd := exec.Command(dockerBinary, "push", "library/busybox")
	if out, _, err := runCommandWithOutput(cmd); err == nil {
		c.Fatalf("Push to public registry should have failed: %s", out)
	} else if !reErr.MatchString(out) {
		c.Fatalf("Got unexpected text from push command: %q; error: %v", out, err)
	}
}
