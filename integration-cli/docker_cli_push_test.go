package main

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-check/check"
)

// pulling an image from the central registry should work
func (s *DockerRegistrySuite) TestPushBusyboxImage(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
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
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)

	expected := "Repository does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		c.Fatalf("pushing the image to the private registry should have failed: output %q", out)
	} else if !strings.Contains(out, expected) {
		c.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
}

func (s *DockerRegistrySuite) TestPushBadTag(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox:latest", privateRegistryURL)

	expected := "does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		c.Fatalf("pushing the image to the private registry should have failed: output %q", out)
	} else if !strings.Contains(out, expected) {
		c.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
}

func (s *DockerRegistrySuite) TestPushMultipleTags(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
	repoTag1 := fmt.Sprintf("%v/dockercli/busybox:t1", privateRegistryURL)
	repoTag2 := fmt.Sprintf("%v/dockercli/busybox:t2", privateRegistryURL)
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
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
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
	repoName := fmt.Sprintf("%v/dockercli/emptylayer", privateRegistryURL)
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

func (s *DockerSuite) TestPushToPublicRegistry(c *check.C) {
	const (
		confirmText  = "want to push to public registry? [y/n]"
		farewellText = "nothing pushed."
	)
	repoName := "docker.io/dockercli/busybox"
	// tag the image to upload it to the private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoName)

	// `sayNo` says whether to terminate communication with negative answer or
	// by closing input stream
	runTest := func(pushCmd *exec.Cmd, sayNo bool) {
		stdin, err := pushCmd.StdinPipe()
		if err != nil {
			c.Fatalf("Failed to get stdin pipe for process: %v", err)
		}
		stdout, err := pushCmd.StdoutPipe()
		if err != nil {
			c.Fatalf("Failed to get stdout pipe for process: %v", err)
		}
		stderr, err := pushCmd.StderrPipe()
		if err != nil {
			c.Fatalf("Failed to get stderr pipe for process: %v", err)
		}
		if err := pushCmd.Start(); err != nil {
			c.Fatalf("Failed to start pushing to private registry: %v", err)
		}

		outReader := bufio.NewReader(stdout)

		readConfirmText := func(out *bufio.Reader) {
			line, err := out.ReadBytes(']')
			if err != nil {
				c.Fatalf("Failed to read a confirmation text for a push: %v", err)
			}
			if !strings.HasSuffix(strings.ToLower(string(line)), confirmText) {
				c.Fatalf("Expected confirmation text %q, not: %q", confirmText, line)
			}
			buf := make([]byte, 4)
			n, err := out.Read(buf)
			if err != nil {
				c.Fatalf("Failed to read confirmation text for a push: %v", err)
			}
			if n > 2 || n < 1 || buf[0] != ':' {
				c.Fatalf("Got unexpected line ending: %q", string(buf))
			}
		}
		readConfirmText(outReader)

		stdin.Write([]byte("\n"))
		readConfirmText(outReader)
		stdin.Write([]byte("  \n"))
		readConfirmText(outReader)
		stdin.Write([]byte("foo\n"))
		readConfirmText(outReader)
		stdin.Write([]byte("no\n"))
		readConfirmText(outReader)
		if sayNo {
			stdin.Write([]byte(" n \n"))
		} else {
			stdin.Close()
		}

		line, isPrefix, err := outReader.ReadLine()
		if err != nil {
			c.Fatalf("Failed to read farewell: %v", err)
		}
		if isPrefix {
			c.Errorf("Got unexpectedly long output.")
		}
		lowered := strings.ToLower(string(line))
		if sayNo {
			if !strings.HasSuffix(lowered, farewellText) {
				c.Errorf("Expected farewell %q, not: %q", farewellText, string(line))
			}
			if strings.Contains(lowered, confirmText) {
				c.Errorf("God unexpected confirmation text: %q", string(line))
			}
		} else {
			if lowered != "eof" {
				c.Errorf("Expected \"EOF\" not: %q", string(line))
			}
			if line, _, err = outReader.ReadLine(); err != io.EOF {
				c.Errorf("Expected EOF, not: %q", line)
			}
		}
		if line, _, err = outReader.ReadLine(); err != io.EOF {
			c.Errorf("Expected EOF, not: %q", line)
		}
		errReader := bufio.NewReader(stderr)
		for ; err != io.EOF; line, _, err = errReader.ReadLine() {
			c.Errorf("Expected no message on stderr, got: %q", string(line))
		}

		// Wait for command to finish with short timeout.
		finish := make(chan struct{})
		go func() {
			if err := pushCmd.Wait(); err != nil && sayNo {
				c.Error(err)
			} else if err == nil && !sayNo {
				c.Errorf("Process should have failed after closing input stream.")
			}
			close(finish)
		}()
		select {
		case <-finish:
		case <-time.After(500 * time.Millisecond):
			cause := "standard input close"
			if sayNo {
				cause = "negative answer"
			}
			c.Fatalf("Docker push failed to exit on %s.", cause)
		}
	}
	runTest(exec.Command(dockerBinary, "push", repoName), false)
	runTest(exec.Command(dockerBinary, "push", repoName), true)
}
