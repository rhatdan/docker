package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-check/check"
)

// See issue docker/docker#8141
func (s *DockerRegistrySuite) TestPullImageWithAliases(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)

	repos := []string{}
	for _, tag := range []string{"recent", "fresh"} {
		repos = append(repos, fmt.Sprintf("%v:%v", repoName, tag))
	}

	// Tag and push the same image multiple times.
	for _, repo := range repos {
		if out, _, err := runCommandWithOutput(exec.Command(dockerBinary, "tag", "busybox", repo)); err != nil {
			c.Fatalf("Failed to tag image %v: error %v, output %q", repos, err, out)
		}
		if out, err := exec.Command(dockerBinary, "push", repo).CombinedOutput(); err != nil {
			c.Fatalf("Failed to push image %v: error %v, output %q", repo, err, string(out))
		}
	}

	// Clear local images store.
	args := append([]string{"rmi"}, repos...)
	if out, err := exec.Command(dockerBinary, args...).CombinedOutput(); err != nil {
		c.Fatalf("Failed to clean images: error %v, output %q", err, string(out))
	}

	// Pull a single tag and verify it doesn't bring down all aliases.
	pullCmd := exec.Command(dockerBinary, "pull", repos[0])
	if out, _, err := runCommandWithOutput(pullCmd); err != nil {
		c.Fatalf("Failed to pull %v: error %v, output %q", repoName, err, out)
	}
	if err := exec.Command(dockerBinary, "inspect", repos[0]).Run(); err != nil {
		c.Fatalf("Image %v was not pulled down", repos[0])
	}
	for _, repo := range repos[1:] {
		if err := exec.Command(dockerBinary, "inspect", repo).Run(); err == nil {
			c.Fatalf("Image %v shouldn't have been pulled down", repo)
		}
	}
}

// pulling library/hello-world should show verified message
func (s *DockerSuite) TestPullVerified(c *check.C) {
	c.Skip("Skipping hub dependent test")

	// Image must be pulled from central repository to get verified message
	// unless keychain is manually updated to contain the daemon's sign key.

	verifiedName := "hello-world"

	// pull it
	expected := "The image you are pulling has been verified"
	pullCmd := exec.Command(dockerBinary, "pull", verifiedName)
	if out, exitCode, err := runCommandWithOutput(pullCmd); err != nil || !strings.Contains(out, expected) {
		if err != nil || exitCode != 0 {
			c.Skip(fmt.Sprintf("pulling the '%s' image from the registry has failed: %v", verifiedName, err))
		}
		c.Fatalf("pulling a verified image failed. expected: %s\ngot: %s, %v", expected, out, err)
	}

	// pull it again
	pullCmd = exec.Command(dockerBinary, "pull", verifiedName)
	if out, exitCode, err := runCommandWithOutput(pullCmd); err != nil || strings.Contains(out, expected) {
		if err != nil || exitCode != 0 {
			c.Skip(fmt.Sprintf("pulling the '%s' image from the registry has failed: %v", verifiedName, err))
		}
		c.Fatalf("pulling a verified image failed. unexpected verify message\ngot: %s, %v", out, err)
	}

}

// pulling an image from the central registry should work
func (s *DockerSuite) TestPullImageFromCentralRegistry(c *check.C) {
	testRequires(c, Network)

	pullCmd := exec.Command(dockerBinary, "pull", "hello-world")
	if out, _, err := runCommandWithOutput(pullCmd); err != nil {
		c.Fatalf("pulling the hello-world image from the registry has failed: %s, %v", out, err)
	}
}

// pulling a non-existing image from the central registry should return a non-zero exit code
func (s *DockerSuite) TestPullNonExistingImage(c *check.C) {
	testRequires(c, Network)

	name := "sadfsadfasdf"
	pullCmd := exec.Command(dockerBinary, "pull", name)
	out, _, err := runCommandWithOutput(pullCmd)

	if err == nil || !strings.Contains(out, fmt.Sprintf("Error: image library/%s:latest not found", name)) {
		c.Fatalf("expected non-zero exit status when pulling non-existing image: %s", out)
	}
}

// pulling an image from the central registry using official names should work
// ensure all pulls result in the same image
func (s *DockerSuite) TestPullImageOfficialNames(c *check.C) {
	testRequires(c, Network)

	names := []string{
		"docker.io/hello-world",
		"index.docker.io/hello-world",
		"library/hello-world",
		"docker.io/library/hello-world",
		"index.docker.io/library/hello-world",
	}
	for _, name := range names {
		pullCmd := exec.Command(dockerBinary, "pull", name)
		out, exitCode, err := runCommandWithOutput(pullCmd)
		if err != nil || exitCode != 0 {
			c.Errorf("pulling the '%s' image from the registry has failed: %s", name, err)
			continue
		}

		// ensure we don't have multiple image names.
		imagesCmd := exec.Command(dockerBinary, "images")
		out, _, err = runCommandWithOutput(imagesCmd)
		if err != nil {
			c.Errorf("listing images failed with errors: %v", err)
		} else if strings.Contains(out, name) {
			if name != "docker.io/hello-world" {
				c.Errorf("images should not have listed '%s'", name)
			}
		}
	}
}

func (s *DockerRegistrySuite) TestPullFromAdditionalRegistry(c *check.C) {
	d := NewDaemon(c)
	if err := d.StartWithBusybox("--add-registry=" + s.reg.url); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing add-registry=%s: %v", s.reg.url, err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// this will pull from docker.io
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from %q: %v", s.reg.url, err)
	}

	helloWorldId := d.getAndTestImageEntry(c, 2, "docker.io/hello-world", "").id
	if helloWorldId == busyboxId {
		c.Fatalf("docker.io/hello-world must have different ID than busybox image")
	}

	// push busybox to additional registry as "library/hello-world" and remove all local images
	if out, err := d.Cmd("tag", "busybox", s.reg.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg.url+"/library/hello-world", err, out)
	}
	toRemove := []string{"library/hello-world", "busybox", "docker.io/hello-world"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// pull the same name again - now the image should be pulled from additional registry
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from %q: %v", s.reg.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg.url+"/library/hello-world", busyboxId)

	// empty images once more
	if out, err := d.Cmd("rmi", s.reg.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to remove image %s: %v, output: %s", s.reg.url+"library/hello-world", err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// now pull with fully qualified name
	if _, err := d.Cmd("pull", "docker.io/library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull docker.io/library/hello-world from %q: %v", s.reg.url, err)
	}
	d.getAndTestImageEntry(c, 1, "docker.io/hello-world", helloWorldId)
}

func (s *DockerRegistriesSuite) TestPullFromAdditionalRegistries(c *check.C) {
	d := NewDaemon(c)
	daemonArgs := []string{"--add-registry=" + s.reg1.url, "--add-registry=" + s.reg2.url}
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// this will pull from docker.io
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from \"docker.io\": %v", err)
	}
	helloWorldId := d.getAndTestImageEntry(c, 2, "docker.io/hello-world", "").id
	if helloWorldId == busyboxId {
		c.Fatalf("docker.io/hello-world must have different ID than busybox image")
	}

	// push:
	//  hello-world to 1st additional registry as "misc/hello-world"
	//  busybox to 2nd additional registry as "library/hello-world"
	if out, err := d.Cmd("tag", "docker.io/hello-world", s.reg1.url+"/misc/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "docker.io/hello-world", err, out)
	}
	if out, err := d.Cmd("tag", "busybox", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "/busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg1.url+"/misc/hello-world"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg1.url+"/misc/hello-world", err, out)
	}
	if out, err := d.Cmd("push", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg2.url+"/library/busybox", err, out)
	}
	// and remove all local images
	toRemove := []string{"misc/hello-world", s.reg2.url + "/library/hello-world", "busybox", "docker.io/hello-world"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// now pull the "library/hello-world" from 2nd additional registry
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from %q: %v", s.reg2.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg2.url+"/library/hello-world", busyboxId)

	// now pull the "misc/hello-world" from 1st additional registry
	if _, err := d.Cmd("pull", "misc/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull misc/hello-world from %q: %v", s.reg2.url, err)
	}
	d.getAndTestImageEntry(c, 2, s.reg1.url+"/misc/hello-world", helloWorldId)

	// tag it as library/hello-world and push it to 1st registry
	if out, err := d.Cmd("tag", s.reg1.url+"/misc/hello-world", s.reg1.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", s.reg1.url+"/misc/hello-world", err, out)
	}
	if out, err := d.Cmd("push", s.reg1.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg1.url+"/library/hello-world", err, out)
	}

	// remove all images
	toRemove = []string{s.reg1.url + "/misc/hello-world", s.reg1.url + "/library/hello-world", s.reg2.url + "/library/hello-world"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// now pull "library/hello-world" from 1st additional registry
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from %q: %v", s.reg1.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg1.url+"/library/hello-world", helloWorldId)

	// now pull fully qualified image from 2nd registry
	if _, err := d.Cmd("pull", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull %s/library/hello-world: %v", s.reg2.url, err)
	}
	d.getAndTestImageEntry(c, 2, s.reg2.url+"/library/hello-world", busyboxId)
}

// Test pulls from blocked public registry and from private registry. This
// shall be called with various daemonArgs containing at least one
// `--block-registry` flag.
func (s *DockerRegistrySuite) doTestPullFromBlockedPublicRegistry(c *check.C, daemonArgs []string) {
	allBlocked := false
	for _, arg := range daemonArgs {
		if arg == "--block-registry=all" {
			allBlocked = true
		}
	}
	d := NewDaemon(c)
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// try to pull from docker.io
	if out, err := d.Cmd("pull", "library/hello-world"); err == nil {
		c.Fatalf("pull from blocked public registry should have failed, output: %s", out)
	}

	// tag busybox as library/hello-world and push it to some private registry
	if out, err := d.Cmd("tag", "busybox", s.reg.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg.url+"/library/hello-world"); !allBlocked && err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg.url+"/library/hello-world", err, out)
	} else if allBlocked && err == nil {
		c.Fatalf("push to private registry should have failed, output: %q", out)
	}

	// remove library/hello-world image
	if out, err := d.Cmd("rmi", s.reg.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", s.reg.url+"/library/hello-world", err, out)
	}
	d.getAndTestImageEntry(c, 1, "busybox", busyboxId)

	// try to pull from private registry
	if out, err := d.Cmd("pull", s.reg.url+"/library/hello-world"); !allBlocked && err != nil {
		c.Fatalf("we should have been able to pull %s/library/hello-world: %v", s.reg.url, err)
	} else if allBlocked && err == nil {
		c.Fatalf("pull from private registry should have failed, output: %q", out)
	} else if !allBlocked {
		d.getAndTestImageEntry(c, 2, s.reg.url+"/library/hello-world", busyboxId)
	}
}

func (s *DockerRegistrySuite) TestPullFromBlockedPublicRegistry(c *check.C) {
	for _, blockedRegistry := range []string{"public", "docker.io"} {
		s.doTestPullFromBlockedPublicRegistry(c, []string{"--block-registry=" + blockedRegistry})
	}
}

func (s *DockerRegistrySuite) TestPullWithAllRegistriesBlocked(c *check.C) {
	s.doTestPullFromBlockedPublicRegistry(c, []string{"--block-registry=all"})
}

// Test pulls from additional registry with public registry blocked. This
// shall be called with various daemonArgs containing at least one
// `--block-registry` flag.
func (s *DockerRegistriesSuite) doTestPullFromPrivateRegistriesWithPublicBlocked(c *check.C, daemonArgs []string) {
	allBlocked := false
	for _, arg := range daemonArgs {
		if arg == "--block-registry=all" {
			allBlocked = true
		}
	}
	d := NewDaemon(c)
	daemonArgs = append(daemonArgs, "--add-registry="+s.reg1.url)
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// try to pull from blocked public registry
	if out, err := d.Cmd("pull", "library/hello-world"); err == nil {
		c.Fatalf("pulling from blocked public registry should have failed, output: %s", out)
	}

	// push busybox to
	//  additional registry as "misc/busybox"
	//  private registry as "library/busybox"
	// and remove all local images
	if out, err := d.Cmd("tag", "busybox", s.reg1.url+"/misc/busybox"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("tag", "busybox", s.reg2.url+"/library/busybox"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg1.url+"/misc/busybox"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg1.url+"/misc/busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg2.url+"/library/busybox"); !allBlocked && err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg2.url+"/library/busybox", err, out)
	} else if allBlocked && err == nil {
		c.Fatalf("push to private registry should have failed, output: %q", out)
	}
	toRemove := []string{"busybox", "misc/busybox", s.reg2.url + "/library/busybox"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// try to pull "library/busybox" from additional registry
	if out, err := d.Cmd("pull", "library/busybox"); err == nil {
		c.Fatalf("pull of library/busybox from additional registry should have failed, output: %q", out)
	}

	// now pull the "misc/busybox" from additional registry
	if _, err := d.Cmd("pull", "misc/busybox"); err != nil {
		c.Fatalf("we should have been able to pull misc/hello-world from %q: %v", s.reg1.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg1.url+"/misc/busybox", busyboxId)

	// try to pull "library/busybox" from private registry
	if out, err := d.Cmd("pull", s.reg2.url+"/library/busybox"); !allBlocked && err != nil {
		c.Fatalf("we should have been able to pull %s/library/busybox: %v", s.reg2.url, err)
	} else if allBlocked && err == nil {
		c.Fatalf("pull from private registry should have failed, output: %q", out)
	} else if !allBlocked {
		d.getAndTestImageEntry(c, 2, s.reg2.url+"/library/busybox", busyboxId)
	}
}

func (s *DockerRegistriesSuite) TestPullFromPrivateRegistriesWithPublicBlocked(c *check.C) {
	for _, blockedRegistry := range []string{"public", "docker.io"} {
		s.doTestPullFromPrivateRegistriesWithPublicBlocked(c, []string{"--block-registry=" + blockedRegistry})
	}
}

func (s *DockerRegistriesSuite) TestPullFromAdditionalRegistryWithAllBlocked(c *check.C) {
	s.doTestPullFromPrivateRegistriesWithPublicBlocked(c, []string{"--block-registry=all"})
}

func (s *DockerRegistriesSuite) TestPullFromBlockedRegistry(c *check.C) {
	d := NewDaemon(c)
	daemonArgs := []string{"--block-registry=" + s.reg1.url, "--add-registry=" + s.reg2.url}
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	busyboxId := d.getAndTestImageEntry(c, 1, "busybox", "").id

	// pull image from docker.io
	if _, err := d.Cmd("pull", "library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from \"docker.io\": %v", err)
	}
	helloWorldId := d.getAndTestImageEntry(c, 2, "docker.io/hello-world", "").id
	if helloWorldId == busyboxId {
		c.Fatalf("docker.io/hello-world must have different ID than busybox image")
	}

	// push "hello-world" to blocked and additional registry and remove all local images
	if out, err := d.Cmd("tag", "busybox", s.reg1.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("tag", "busybox", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to tag image %s: error %v, output %q", "busybox", err, out)
	}
	if out, err := d.Cmd("push", s.reg1.url+"/library/hello-world"); err == nil {
		c.Fatalf("push to blocked registry should have failed, output: %q", out)
	}
	if out, err := d.Cmd("push", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", s.reg2.url+"/library/hello-world", err, out)
	}
	toRemove := []string{"library/hello-world", s.reg1.url + "/library/hello-world", "docker.io/hello-world", "busybox"}
	if out, err := d.Cmd("rmi", toRemove...); err != nil {
		c.Fatalf("failed to remove images %v: %v, output: %s", toRemove, err, out)
	}
	d.getAndTestImageEntry(c, 0, "", "")

	// try to pull "library/hello-world" from blocked registry
	if out, err := d.Cmd("pull", s.reg1.url+"/library/hello-world"); err == nil {
		c.Fatalf("pull of library/hello-world from additional registry should have failed, output: %q", out)
	}

	// now pull the "library/hello-world" from additional registry
	if _, err := d.Cmd("pull", s.reg2.url+"/library/hello-world"); err != nil {
		c.Fatalf("we should have been able to pull library/hello-world from %q: %v", s.reg2.url, err)
	}
	d.getAndTestImageEntry(c, 1, s.reg2.url+"/library/hello-world", busyboxId)
}
