package main

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/go-check/check"
)

const (
	confirmText  = "want to push to public registry? [y/n]"
	farewellText = "nothing pushed."
	loginText    = "login prior to push:"
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

func readConfirmText(c *check.C, out *bufio.Reader) {
	done := make(chan struct{})
	go func() {
		line, err := out.ReadBytes(']')
		if err != nil {
			c.Fatalf("Failed to read a confirmation text for a push: %v, out: %s", err, line)
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
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		c.Fatalf("Timeout while waiting on confirmation text.")
	}
}

func (s *DockerSuite) TestPushToPublicRegistry(c *check.C) {
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

		readConfirmText(c, outReader)

		stdin.Write([]byte("\n"))
		readConfirmText(c, outReader)
		stdin.Write([]byte("  \n"))
		readConfirmText(c, outReader)
		stdin.Write([]byte("foo\n"))
		readConfirmText(c, outReader)
		stdin.Write([]byte("no\n"))
		readConfirmText(c, outReader)
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

func (s *DockerSuite) TestPushToPublicRegistryNoConfirm(c *check.C) {
	d := NewDaemon(c)
	daemonArgs := []string{"--confirm-def-push=false"}
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	repoName := "docker.io/user/hello-world"
	if out, err := d.Cmd("tag", "busybox", repoName); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}

	runTest := func(name string, arg ...string) {
		args := []string{"--host", d.sock(), name}
		args = append(args, arg...)
		c.Logf("Running %s %s %s", dockerBinary, name, strings.Join(args, " "))
		cmd := exec.Command(dockerBinary, args...)

		stdin, err := cmd.StdinPipe()
		if err != nil {
			c.Fatalf("Failed to get stdin pipe for process: %v", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			c.Fatalf("Failed to get stdout pipe for process: %v", err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			c.Fatalf("Failed to get stderr pipe for process: %v", err)
		}
		if err := cmd.Start(); err != nil {
			c.Fatalf("Failed to start pushing to private registry: %v", err)
		}
		outReader := bufio.NewReader(stdout)

		go io.Copy(os.Stderr, stderr)

		errChan := make(chan error)
		go func() {
			for {
				line, err := outReader.ReadBytes('\n')
				if err != nil {
					errChan <- fmt.Errorf("Failed to read line: %v", err)
					break
				}
				c.Logf("output of push command: %q", line)
				trimmed := strings.ToLower(strings.TrimSpace(string(line)))
				if strings.HasSuffix(trimmed, confirmText) {
					errChan <- fmt.Errorf("Got unexpected confirmation text: %q", line)
					break
				}
				if strings.HasSuffix(trimmed, loginText) {
					errChan <- nil
					break
				}
			}
		}()
		select {
		case err := <-errChan:
			if err != nil {
				c.Fatal(err.Error())
			}
		case <-time.After(10 * time.Second):
			c.Fatal("Push command timeouted!")
		}
		stdin.Close()

		// Wait for command to finish with short timeout.
		finish := make(chan struct{})
		go func() {
			if err := cmd.Wait(); err == nil {
				c.Errorf("Process should have failed after closing input stream.")
			}
			close(finish)
		}()
		select {
		case <-finish:
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("Docker push failed to exit!")
		}
	}

	runTest("push", repoName)
	runTest("push", "-f", repoName)
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.Fatalf("Failed to get stdout pipe for process: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		c.Fatalf("Failed to get stderr pipe for process: %v", err)
	}
	if err := cmd.Start(); err != nil {
		c.Fatalf("Failed to start pushing to public registry: %v", err)
	}
	outReader := bufio.NewReader(stdout)
	errReader := bufio.NewReader(stderr)
	line, isPrefix, err := errReader.ReadLine()
	if err != nil {
		c.Fatalf("Failed to read farewell: %v", err)
	}
	if isPrefix {
		c.Errorf("Got unexpectedly long output.")
	}
	if !reErr.Match(line) {
		c.Errorf("Got unexpected output %q", line)
	}
	if line, _, err = outReader.ReadLine(); err != io.EOF {
		c.Errorf("Expected EOF, not: %q", line)
	}
	for ; err != io.EOF; line, _, err = errReader.ReadLine() {
		c.Errorf("Expected no message on stderr, got: %q", string(line))
	}

	// Wait for command to finish with short timeout.
	finish := make(chan struct{})
	go func() {
		if err := cmd.Wait(); err == nil {
			c.Error("Push command should have failed.")
		}
		close(finish)
	}()
	select {
	case <-finish:
	case <-time.After(1 * time.Second):
		c.Fatalf("Docker push failed to exit.")
	}
}
