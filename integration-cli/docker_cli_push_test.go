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
