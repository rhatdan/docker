package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	confirmText  = "want to push to public registry? [y/n]"
	farewellText = "nothing pushed."
)

// pulling an image from the central registry should work
func TestPushBusyboxImage(t *testing.T) {
	defer setupRegistry(t)()

	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURLs[0])
	// tag the image to upload it to he private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		t.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		t.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
	logDone("push - busybox to private registry")
}

// pushing an image without a prefix should throw an error
func TestPushUnprefixedRepo(t *testing.T) {
	pushCmd := exec.Command(dockerBinary, "push", "busybox")
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		t.Fatalf("pushing an unprefixed repo didn't result in a non-zero exit status: %s", out)
	}
	logDone("push - unprefixed busybox repo must not pass")
}

func TestPushUntagged(t *testing.T) {
	defer setupRegistry(t)()

	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURLs[0])

	expected := "Repository does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		t.Fatalf("pushing the image to the private registry should have failed: outuput %q", out)
	} else if !strings.Contains(out, expected) {
		t.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
	logDone("push - untagged image")
}

func TestPushBadTag(t *testing.T) {
	defer setupRegistry(t)()

	repoName := fmt.Sprintf("%v/dockercli/busybox:latest", privateRegistryURLs[0])

	expected := "does not exist"
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err == nil {
		t.Fatalf("pushing the image to the private registry should have failed: outuput %q", out)
	} else if !strings.Contains(out, expected) {
		t.Fatalf("pushing the image failed with an unexpected message: expected %q, got %q", expected, out)
	}
	logDone("push - image with bad tag")
}

func TestPushMultipleTags(t *testing.T) {
	defer setupRegistry(t)()

	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURLs[0])
	repoTag1 := fmt.Sprintf("%v/dockercli/busybox:t1", privateRegistryURLs[0])
	repoTag2 := fmt.Sprintf("%v/dockercli/busybox:t2", privateRegistryURLs[0])
	// tag the image to upload it tot he private registry
	tagCmd1 := exec.Command(dockerBinary, "tag", "busybox", repoTag1)
	if out, _, err := runCommandWithOutput(tagCmd1); err != nil {
		t.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoTag1)
	tagCmd2 := exec.Command(dockerBinary, "tag", "busybox", repoTag2)
	if out, _, err := runCommandWithOutput(tagCmd2); err != nil {
		t.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoTag2)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		t.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
	logDone("push - multiple tags to private registry")
}

func TestPushInterrupt(t *testing.T) {
	defer setupRegistry(t)()

	repoName := fmt.Sprintf("%v/dockercli/busybox", privateRegistryURLs[0])
	// tag the image to upload it tot he private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		t.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoName)

	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if err := pushCmd.Start(); err != nil {
		t.Fatalf("Failed to start pushing to private registry: %v", err)
	}

	// Interrupt push (yes, we have no idea at what point it will get killed).
	time.Sleep(200 * time.Millisecond)
	if err := pushCmd.Process.Kill(); err != nil {
		t.Fatalf("Failed to kill push process: %v", err)
	}
	// Try agin
	pushCmd = exec.Command(dockerBinary, "push", repoName)
	if err := pushCmd.Start(); err != nil {
		t.Fatalf("Failed to start pushing to private registry: %v", err)
	}

	logDone("push - interrupted")
}

func TestPushEmptyLayer(t *testing.T) {
	defer setupRegistry(t)()
	repoName := fmt.Sprintf("%v/dockercli/emptylayer", privateRegistryURLs[0])
	emptyTarball, err := ioutil.TempFile("", "empty_tarball")
	if err != nil {
		t.Fatalf("Unable to create test file: %v", err)
	}
	tw := tar.NewWriter(emptyTarball)
	err = tw.Close()
	if err != nil {
		t.Fatalf("Error creating empty tarball: %v", err)
	}
	freader, err := os.Open(emptyTarball.Name())
	if err != nil {
		t.Fatalf("Could not open test tarball: %v", err)
	}

	importCmd := exec.Command(dockerBinary, "import", "-", repoName)
	importCmd.Stdin = freader
	out, _, err := runCommandWithOutput(importCmd)
	if err != nil {
		t.Errorf("import failed with errors: %v, output: %q", err, out)
	}

	// Now verify we can push it
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		t.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
	logDone("push - empty layer config to private registry")
}

func readConfirmText(t *testing.T, out *bufio.Reader) {
	done := make(chan struct{})
	go func() {
		line, err := out.ReadBytes(']')
		if err != nil {
			t.Fatalf("Failed to read a confirmation text for a push: %v", err)
		}
		if !strings.HasSuffix(strings.ToLower(string(line)), confirmText) {
			t.Fatalf("Expected confirmation text %q, not: %q", confirmText, line)
		}
		buf := make([]byte, 4)
		n, err := out.Read(buf)
		if err != nil {
			t.Fatalf("Failed to read confirmation text for a push: %v", err)
		}
		if n > 2 || n < 1 || buf[0] != ':' {
			t.Fatalf("Got unexpected line ending: %q", string(buf))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatalf("Timeout while waiting on confirmation text.")
	}
}

func TestPushToPublicRegistry(t *testing.T) {
	repoName := "docker.io/dockercli/busybox"
	// tag the image to upload it to the private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		t.Fatalf("image tagging failed: %s, %v", out, err)
	}
	defer deleteImages(repoName)

	// `sayNo` says whether to terminate communication with negative answer or
	// by closing input stream
	runTest := func(pushCmd *exec.Cmd, sayNo bool) {
		stdin, err := pushCmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to get stdin pipe for process: %v", err)
		}
		stdout, err := pushCmd.StdoutPipe()
		if err != nil {
			t.Fatalf("Failed to get stdout pipe for process: %v", err)
		}
		stderr, err := pushCmd.StderrPipe()
		if err != nil {
			t.Fatalf("Failed to get stderr pipe for process: %v", err)
		}
		if err := pushCmd.Start(); err != nil {
			t.Fatalf("Failed to start pushing to private registry: %v", err)
		}

		outReader := bufio.NewReader(stdout)

		readConfirmText(t, outReader)

		stdin.Write([]byte("\n"))
		readConfirmText(t, outReader)
		stdin.Write([]byte("  \n"))
		readConfirmText(t, outReader)
		stdin.Write([]byte("foo\n"))
		readConfirmText(t, outReader)
		stdin.Write([]byte("no\n"))
		readConfirmText(t, outReader)
		if sayNo {
			stdin.Write([]byte(" n \n"))
		} else {
			stdin.Close()
		}

		line, isPrefix, err := outReader.ReadLine()
		if err != nil {
			t.Fatalf("Failed to read farewell: %v", err)
		}
		if isPrefix {
			t.Errorf("Got unexpectedly long output.")
		}
		lowered := strings.ToLower(string(line))
		if sayNo {
			if !strings.HasSuffix(lowered, farewellText) {
				t.Errorf("Expected farewell %q, not: %q", farewellText, string(line))
			}
			if strings.Contains(lowered, confirmText) {
				t.Errorf("God unexpected confirmation text: %q", string(line))
			}
		} else {
			if lowered != "eof" {
				t.Errorf("Expected \"EOF\" not: %q", string(line))
			}
			if line, _, err = outReader.ReadLine(); err != io.EOF {
				t.Errorf("Expected EOF, not: %q", line)
			}
		}
		if line, _, err = outReader.ReadLine(); err != io.EOF {
			t.Errorf("Expected EOF, not: %q", line)
		}
		errReader := bufio.NewReader(stderr)
		for ; err != io.EOF; line, _, err = errReader.ReadLine() {
			t.Errorf("Expected no message on stderr, got: %q", string(line))
		}

		// Wait for command to finish with short timeout.
		finish := make(chan struct{})
		go func() {
			if err := pushCmd.Wait(); err != nil && sayNo {
				t.Error(err)
			} else if err == nil && !sayNo {
				t.Errorf("Process should have failed after closing input stream.")
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
			t.Fatalf("Docker push failed to exit on %s.", cause)
		}
	}
	runTest(exec.Command(dockerBinary, "push", repoName), false)
	runTest(exec.Command(dockerBinary, "push", repoName), true)

	logDone("push - to public registry")
}

func TestPushToAdditionalRegistry(t *testing.T) {
	reg := setupAndGetRegistryAt(t, privateRegistryURLs[0])
	defer reg.Close()
	d := NewDaemon(t)
	if err := d.StartWithBusybox("--add-registry=" + reg.url); err != nil {
		t.Fatalf("We should have been able to start the daemon with passing add-registry=%s: %v", reg.url, err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(t, 1, "busybox", "").id

	// push busybox to additional registry as "library/busybox" and remove all local images
	if out, err := d.Cmd("tag", "busybox", "library/busybox"); err != nil {
		t.Fatalf("Failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", "library/busybox"); err != nil {
		t.Fatalf("Failed to push image library/busybox: error %v, output %q", err, out)
	}
	toRemove := []string{"busybox", "library/busybox"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		t.Fatalf("Failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(t, 0, "", "")

	// pull it from additional registry
	if _, err := d.Cmd("pull", "library/busybox"); err != nil {
		t.Fatalf("We should have been able to pull library/busybox from %q: %v", reg.url, err)
	}
	d.getAndTestImageEntry(t, 1, reg.url+"/library/busybox", busyboxId)

	logDone("push - to additional registry")
}

func TestPushOfficialImage(t *testing.T) {
	var reErr = regexp.MustCompile(`rename your repository to[^:]*:\s*<user>/busybox\b`)

	// push busybox to public registry as "library/busybox"
	cmd := exec.Command(dockerBinary, "push", "library/busybox")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe for process: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe for process: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe for process: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start pushing to public registry: %v", err)
	}
	outReader := bufio.NewReader(stdout)
	readConfirmText(t, outReader)
	stdin.Write([]byte{'Y', '\n'})

	errReader := bufio.NewReader(stderr)
	line, isPrefix, err := errReader.ReadLine()
	if err != nil {
		t.Fatalf("Failed to read farewell: %v", err)
	}
	if isPrefix {
		t.Errorf("Got unexpectedly long output.")
	}
	if !reErr.Match(line) {
		t.Errorf("Got unexpected output %q", line)
	}
	if line, _, err = outReader.ReadLine(); err != io.EOF {
		t.Errorf("Expected EOF, not: %q", line)
	}
	for ; err != io.EOF; line, _, err = errReader.ReadLine() {
		t.Errorf("Expected no message on stderr, got: %q", string(line))
	}

	// Wait for command to finish with short timeout.
	finish := make(chan struct{})
	go func() {
		if err := cmd.Wait(); err == nil {
			t.Error("Push command should have failed.")
		}
		close(finish)
	}()
	select {
	case <-finish:
	case <-time.After(1 * time.Second):
		t.Fatalf("Docker push failed to exit.")
	}

	logDone("push - official image")
}
