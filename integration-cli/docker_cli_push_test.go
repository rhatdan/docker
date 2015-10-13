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

	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

const (
	confirmText  = "want to push to public registry? [y/n]"
	farewellText = "nothing pushed."
	loginText    = "login prior to push:"
)

// Pushing an image to a private registry.
func (s *DockerRegistrySuite) TestPushBusyboxImage(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
	// tag the image to upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)
	// push the image to the registry
	dockerCmd(c, "push", repoName)
}

// pushing an image without a prefix should throw an error
func (s *DockerSuite) TestPushUnprefixedRepo(c *check.C) {
	out, _, err := dockerCmdWithError("push", "busybox")
	c.Assert(err, check.NotNil, check.Commentf("pushing an unprefixed repo didn't result in a non-zero exit status: %s", out))
}

func (s *DockerRegistrySuite) TestPushUntagged(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
	expected := "Repository does not exist"

	out, _, err := dockerCmdWithError("push", repoName)
	c.Assert(err, check.NotNil, check.Commentf("pushing the image to the private registry should have failed: output %q", out))
	c.Assert(out, checker.Contains, expected, check.Commentf("pushing the image failed"))
}

func (s *DockerRegistrySuite) TestPushBadTag(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox:latest", privateRegistryURL)
	expected := "does not exist"

	out, _, err := dockerCmdWithError("push", repoName)
	c.Assert(err, check.NotNil, check.Commentf("pushing the image to the private registry should have failed: output %q", out))
	c.Assert(out, checker.Contains, expected, check.Commentf("pushing the image failed"))
}

func (s *DockerRegistrySuite) TestPushMultipleTags(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURL)
	repoTag1 := fmt.Sprintf("%v/dockercli/busybox:t1", privateRegistryURL)
	repoTag2 := fmt.Sprintf("%v/dockercli/busybox:t2", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoTag1)

	dockerCmd(c, "tag", "busybox", repoTag2)

	dockerCmd(c, "push", repoName)

	// Ensure layer list is equivalent for repoTag1 and repoTag2
	out1, _ := dockerCmd(c, "pull", repoTag1)

	imageAlreadyExists := ": Image already exists"
	var out1Lines []string
	for _, outputLine := range strings.Split(out1, "\n") {
		if strings.Contains(outputLine, imageAlreadyExists) {
			out1Lines = append(out1Lines, outputLine)
		}
	}

	out2, _ := dockerCmd(c, "pull", repoTag2)

	var out2Lines []string
	for _, outputLine := range strings.Split(out2, "\n") {
		if strings.Contains(outputLine, imageAlreadyExists) {
			out1Lines = append(out1Lines, outputLine)
		}
	}
	c.Assert(out2Lines, checker.HasLen, len(out1Lines))

	for i := range out1Lines {
		c.Assert(out1Lines[i], checker.Equals, out2Lines[i])
	}
}

func (s *DockerRegistrySuite) TestPushEmptyLayer(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/emptylayer", privateRegistryURL)
	emptyTarball, err := ioutil.TempFile("", "empty_tarball")
	c.Assert(err, check.IsNil, check.Commentf("Unable to create test file"))

	tw := tar.NewWriter(emptyTarball)
	err = tw.Close()
	c.Assert(err, check.IsNil, check.Commentf("Error creating empty tarball"))

	freader, err := os.Open(emptyTarball.Name())
	c.Assert(err, check.IsNil, check.Commentf("Could not open test tarball"))

	importCmd := exec.Command(dockerBinary, "import", "-", repoName)
	importCmd.Stdin = freader
	out, _, err := runCommandWithOutput(importCmd)
	c.Assert(err, check.IsNil, check.Commentf("import failed: %q", out))

	// Now verify we can push it
	out, _, err = dockerCmdWithError("push", repoName)
	c.Assert(err, check.IsNil, check.Commentf("pushing the image to the private registry has failed: %s", out))
}

func (s *DockerTrustSuite) TestTrustedPush(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("Error running trusted push: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))
}

func (s *DockerTrustSuite) TestTrustedPushWithEnvPasswords(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmdWithPassphrases(pushCmd, "12345678", "12345678")
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("Error running trusted push: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))
}

// This test ensures backwards compatibility with old ENV variables. Should be
// deprecated by 1.10
func (s *DockerTrustSuite) TestTrustedPushWithDeprecatedEnvPasswords(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusteddeprecated:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmdWithDeprecatedEnvPassphrases(pushCmd, "12345678", "12345678")
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("Error running trusted push: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))
}

func (s *DockerTrustSuite) TestTrustedPushWithFaillingServer(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmdWithServer(pushCmd, "https://example.com:81/")
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.NotNil, check.Commentf("Missing error while running trusted push w/ no server"))
	c.Assert(out, checker.Contains, "error contacting notary server", check.Commentf("Missing expected output on trusted push"))
}

func (s *DockerTrustSuite) TestTrustedPushWithoutServerAndUntrusted(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	pushCmd := exec.Command(dockerBinary, "push", "--disable-content-trust", repoName)
	s.trustedCmdWithServer(pushCmd, "https://example.com/")
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push with no server and --disable-content-trust failed: %s\n%s", err, out))
	c.Assert(out, check.Not(checker.Contains), "Error establishing connection to notary repository", check.Commentf("Missing expected output on trusted push with --disable-content-trust:"))
}

func (s *DockerTrustSuite) TestTrustedPushWithExistingTag(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)
	dockerCmd(c, "push", repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push with existing tag"))
}

func (s *DockerTrustSuite) TestTrustedPushWithExistingSignedTag(c *check.C) {
	repoName := fmt.Sprintf("%v/dockerclipushpush/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	// Do a trusted push
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push with existing tag"))

	// Do another trusted push
	pushCmd = exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err = runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push with existing tag"))

	dockerCmd(c, "rmi", repoName)

	// Try pull to ensure the double push did not break our ability to pull
	pullCmd := exec.Command(dockerBinary, "pull", repoName)
	s.trustedCmd(pullCmd)
	out, _, err = runCommandWithOutput(pullCmd)
	c.Assert(err, check.IsNil, check.Commentf("Error running trusted pull: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Status: Downloaded", check.Commentf("Missing expected output on trusted pull with --disable-content-trust"))

}

func (s *DockerTrustSuite) TestTrustedPushWithIncorrectPassphraseForNonRoot(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercliincorretpwd/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	// Push with default passphrases
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push:\n%s", out))

	// Push with wrong passphrases
	pushCmd = exec.Command(dockerBinary, "push", repoName)
	s.trustedCmdWithPassphrases(pushCmd, "12345678", "87654321")
	out, _, err = runCommandWithOutput(pushCmd)
	c.Assert(err, check.NotNil, check.Commentf("Error missing from trusted push with short targets passphrase: \n%s", out))
	c.Assert(out, checker.Contains, "password invalid, operation has failed", check.Commentf("Missing expected output on trusted push with short targets/snapsnot passphrase"))
}

// This test ensures backwards compatibility with old ENV variables. Should be
// deprecated by 1.10
func (s *DockerTrustSuite) TestTrustedPushWithIncorrectDeprecatedPassphraseForNonRoot(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercliincorretdeprecatedpwd/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	// Push with default passphrases
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))

	// Push with wrong passphrases
	pushCmd = exec.Command(dockerBinary, "push", repoName)
	s.trustedCmdWithDeprecatedEnvPassphrases(pushCmd, "12345678", "87654321")
	out, _, err = runCommandWithOutput(pushCmd)
	c.Assert(err, check.NotNil, check.Commentf("Error missing from trusted push with short targets passphrase: \n%s", out))
	c.Assert(out, checker.Contains, "password invalid, operation has failed", check.Commentf("Missing expected output on trusted push with short targets/snapsnot passphrase"))
}

func (s *DockerTrustSuite) TestTrustedPushWithExpiredSnapshot(c *check.C) {
	c.Skip("Currently changes system time, causing instability")
	repoName := fmt.Sprintf("%v/dockercliexpiredsnapshot/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	// Push with default passphrases
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))

	// Snapshots last for three years. This should be expired
	fourYearsLater := time.Now().Add(time.Hour * 24 * 365 * 4)

	runAtDifferentDate(fourYearsLater, func() {
		// Push with wrong passphrases
		pushCmd = exec.Command(dockerBinary, "push", repoName)
		s.trustedCmd(pushCmd)
		out, _, err = runCommandWithOutput(pushCmd)
		c.Assert(err, check.NotNil, check.Commentf("Error missing from trusted push with expired snapshot: \n%s", out))
		c.Assert(out, checker.Contains, "repository out-of-date", check.Commentf("Missing expected output on trusted push with expired snapshot"))
	})
}

func (s *DockerTrustSuite) TestTrustedPushWithExpiredTimestamp(c *check.C) {
	c.Skip("Currently changes system time, causing instability")
	repoName := fmt.Sprintf("%v/dockercliexpiredtimestamppush/trusted:latest", privateRegistryURL)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)

	// Push with default passphrases
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	s.trustedCmd(pushCmd)
	out, _, err := runCommandWithOutput(pushCmd)
	c.Assert(err, check.IsNil, check.Commentf("trusted push failed: %s\n%s", err, out))
	c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push"))

	// The timestamps expire in two weeks. Lets check three
	threeWeeksLater := time.Now().Add(time.Hour * 24 * 21)

	// Should succeed because the server transparently re-signs one
	runAtDifferentDate(threeWeeksLater, func() {
		pushCmd := exec.Command(dockerBinary, "push", repoName)
		s.trustedCmd(pushCmd)
		out, _, err := runCommandWithOutput(pushCmd)
		c.Assert(err, check.IsNil, check.Commentf("Error running trusted push: %s\n%s", err, out))
		c.Assert(out, checker.Contains, "Signing and pushing trust metadata", check.Commentf("Missing expected output on trusted push with expired timestamp"))
	})
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
					if err == io.EOF {
						errChan <- nil
					} else {
						errChan <- fmt.Errorf("Failed to read line: %v", err)
					}
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

	busyboxID := d.getAndTestImageEntry(c, 1, "busybox", "").id

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
	d.getAndTestImageEntry(c, 1, s.reg.url+"/library/busybox", busyboxID)
}

func (s *DockerRegistrySuite) TestPushCustomTagToAdditionalRegistry(c *check.C) {
	d := NewDaemon(c)
	if err := d.StartWithBusybox("--add-registry=" + s.reg.url); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing add-registry=%s: %v", s.reg.url, err)
	}
	defer d.Stop()

	busyboxID := d.getAndTestImageEntry(c, 1, "busybox", "").id

	if out, err := d.Cmd("tag", "busybox", "user/busybox:1.2.3"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("tag", "busybox", s.reg.url+"/user/busybox:latest"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", "user/busybox:1.2.3"); err != nil {
		c.Fatalf("failed to push image user/busybox: error %v, output %q", err, out)
	}
	d.getAndTestImageEntry(c, 3, "user/busybox", busyboxID)
	toRemove := []string{"user/busybox:1.2.3"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 2, s.reg.url+"/user/busybox", busyboxID)
}
