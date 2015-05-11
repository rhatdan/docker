package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-check/check"
)

func (s *DockerSuite) TestInspectImage(c *check.C) {
	imageTest := "emptyfs"
	imageTestID := "511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158"
	id, err := inspectField(imageTest, "Id")
	c.Assert(err, check.IsNil)

	if id != imageTestID {
		c.Fatalf("Expected id: %s for image: %s but received id: %s", imageTestID, imageTest, id)
	}

}

func (s *DockerSuite) TestInspectInt64(c *check.C) {
	runCmd := exec.Command(dockerBinary, "run", "-d", "-m=300M", "busybox", "true")
	out, _, _, err := runCommandWithStdoutStderr(runCmd)
	if err != nil {
		c.Fatalf("failed to run container: %v, output: %q", err, out)
	}

	out = strings.TrimSpace(out)

	inspectOut, err := inspectField(out, "HostConfig.Memory")
	c.Assert(err, check.IsNil)

	if inspectOut != "314572800" {
		c.Fatalf("inspect got wrong value, got: %q, expected: 314572800", inspectOut)
	}
}

func (s *DockerSuite) TestInspectImageFilterInt(c *check.C) {
	imageTest := "emptyfs"
	out, err := inspectField(imageTest, "Size")
	c.Assert(err, check.IsNil)

	size, err := strconv.Atoi(out)
	if err != nil {
		c.Fatalf("failed to inspect size of the image: %s, %v", out, err)
	}

	//now see if the size turns out to be the same
	formatStr := fmt.Sprintf("--format='{{eq .Size %d}}'", size)
	imagesCmd := exec.Command(dockerBinary, "inspect", formatStr, imageTest)
	out, exitCode, err := runCommandWithOutput(imagesCmd)
	if exitCode != 0 || err != nil {
		c.Fatalf("failed to inspect image: %s, %v", out, err)
	}
	if result, err := strconv.ParseBool(strings.TrimSuffix(out, "\n")); err != nil || !result {
		c.Fatalf("Expected size: %d for image: %s but received size: %s", size, imageTest, strings.TrimSuffix(out, "\n"))
	}
}

func (s *DockerSuite) TestInspectContainerFilterInt(c *check.C) {
	runCmd := exec.Command(dockerBinary, "run", "-i", "-a", "stdin", "busybox", "cat")
	runCmd.Stdin = strings.NewReader("blahblah")
	out, _, _, err := runCommandWithStdoutStderr(runCmd)
	if err != nil {
		c.Fatalf("failed to run container: %v, output: %q", err, out)
	}

	id := strings.TrimSpace(out)

	out, err = inspectField(id, "State.ExitCode")
	c.Assert(err, check.IsNil)

	exitCode, err := strconv.Atoi(out)
	if err != nil {
		c.Fatalf("failed to inspect exitcode of the container: %s, %v", out, err)
	}

	//now get the exit code to verify
	formatStr := fmt.Sprintf("--format='{{eq .State.ExitCode %d}}'", exitCode)
	runCmd = exec.Command(dockerBinary, "inspect", formatStr, id)
	out, _, err = runCommandWithOutput(runCmd)
	if err != nil {
		c.Fatalf("failed to inspect container: %s, %v", out, err)
	}
	if result, err := strconv.ParseBool(strings.TrimSuffix(out, "\n")); err != nil || !result {
		c.Fatalf("Expected exitcode: %d for container: %s", exitCode, id)
	}
}

func compareInspectValues(c *check.C, name string, local, remote interface{}) {
	additionalLocalAttributes := map[string]struct{}{
		"VirtualSize": {},
	}

	isRootObject := len(name) <= 4

	if reflect.TypeOf(local) != reflect.TypeOf(remote) {
		c.Errorf("types don't match for %q: %T != %T", name, local, remote)
		return
	}
	switch local.(type) {
	case bool:
		lVal := local.(bool)
		rVal := remote.(bool)
		if lVal != rVal {
			c.Errorf("local value differs from remote for %q: %t != %t", name, lVal, rVal)
		}
	case float64:
		lVal := local.(float64)
		rVal := remote.(float64)
		if lVal != rVal {
			c.Errorf("local value differs from remote for %q: %f != %f", name, lVal, rVal)
		}
	case string:
		lVal := local.(string)
		rVal := remote.(string)
		if lVal != rVal {
			c.Errorf("local value differs from remote for %q: %q != %q", name, lVal, rVal)
		}
	// JSON array
	case []interface{}:
		lVal := local.([]interface{})
		rVal := remote.([]interface{})
		if len(lVal) != len(rVal) {
			c.Errorf("array length differs between local and remote for %q: %d != %d", name, len(lVal), len(rVal))
		}
		for i := 0; i < len(lVal) && i < len(rVal); i++ {
			compareInspectValues(c, fmt.Sprintf("%s[%d]", name, i), lVal[i], rVal[i])
		}
	// JSON object
	case map[string]interface{}:
		lMap := local.(map[string]interface{})
		rMap := remote.(map[string]interface{})
		if isRootObject && len(lMap) != len(rMap)+len(additionalLocalAttributes) {
			c.Errorf("got unexpected number of root object's attributes from remote inpect %q: %d != %d", name, len(lMap), len(rMap)+len(additionalLocalAttributes))
		} else if !isRootObject && len(lMap) != len(rMap) {
			c.Errorf("map length differs between local and remote for %q: %d != %d", name, len(lMap), len(rMap))
		}
		for key, lVal := range lMap {
			itemName := fmt.Sprintf("%s.%s", name, key)
			rVal, ok := rMap[key]
			if ok {
				compareInspectValues(c, itemName, lVal, rVal)
			} else if _, exists := additionalLocalAttributes[key]; !isRootObject || !exists {
				c.Errorf("attribute %q present in local but not in remote object", itemName)
			}
		}
		for key := range rMap {
			if _, ok := lMap[key]; !ok {
				c.Errorf("attribute \"%s.%s\" present in remote but not in local object", name, key)
			}
		}
	case nil:
		if local != remote {
			c.Errorf("local value differs from remote for %q: %v (%T) != %v (%T)", name, local, local, remote, remote)
		}
	default:
		c.Fatalf("got unexpected type (%T) for %q", local, name)
	}
}

func (s *DockerRegistrySuite) TestInspectRemoteRepository(c *check.C) {
	var (
		localValue  []interface{}
		remoteValue []interface{}
	)
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)
	// tag the image and upload it to the private registry
	tagCmd := exec.Command(dockerBinary, "tag", "busybox", repoName)
	if out, _, err := runCommandWithOutput(tagCmd); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}

	inspectCmd := exec.Command(dockerBinary, "inspect", repoName)
	localOut, _, err := runCommandWithOutput(inspectCmd)
	if err != nil {
		c.Fatalf("failed to inspect local busybox image : %s, %v", localOut, err)
	}
	pushCmd := exec.Command(dockerBinary, "push", repoName)
	if out, _, err := runCommandWithOutput(pushCmd); err != nil {
		c.Fatalf("pushing the image to the private registry has failed: %s, %v", out, err)
	}
	inspectCmd = exec.Command(dockerBinary, "inspect", "-r", repoName)
	remoteOut, _, err := runCommandWithOutput(inspectCmd)
	if err != nil {
		c.Fatalf("failed to inspect remote busybox image : %s, %v", remoteOut, err)
	}

	if err = json.Unmarshal([]byte(localOut), &localValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}

	if err = json.Unmarshal([]byte(remoteOut), &remoteValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}

	compareInspectValues(c, "a", localValue, remoteValue)

	deleteImages(repoName)

	// local inspect shall fail now
	inspectCmd = exec.Command(dockerBinary, "inspect", repoName)
	localOut, _, err = runCommandWithOutput(inspectCmd)
	if err == nil {
		c.Fatalf("inspect on removed local images should have failed: %s", localOut)
	}

	// remote inspect shall still succeed
	inspectCmd = exec.Command(dockerBinary, "inspect", "-r", repoName)
	remoteOut2, _, err := runCommandWithOutput(inspectCmd)
	if err != nil {
		c.Fatalf("failed to inspect remote busybox image : %s, %v", remoteOut2, err)
	}

	if remoteOut != remoteOut2 {
		c.Fatalf("remote inspect should produce identical output as before:\nfirst run: %s\n\nsecond run: %s", remoteOut, remoteOut2)
	}
}

func (s *DockerRegistrySuite) TestInspectImageFromAdditionalRegistry(c *check.C) {
	var (
		localValue  []interface{}
		remoteValue []interface{}
	)
	d := NewDaemon(c)
	daemonArgs := []string{"--add-registry=" + s.reg.url}
	if err := d.StartWithBusybox(daemonArgs...); err != nil {
		c.Fatalf("we should have been able to start the daemon with passing { %s } flags: %v", strings.Join(daemonArgs, ", "), err)
	}
	defer d.Stop()

	repoName := fmt.Sprintf("dockercli/busybox")
	fqn := s.reg.url + "/" + repoName
	// tag the image and upload it to the private registry
	if out, err := d.Cmd("tag", "busybox", fqn); err != nil {
		c.Fatalf("image tagging failed: %s, %v", out, err)
	}

	localOut, err := d.Cmd("inspect", repoName)
	if err != nil {
		c.Fatalf("failed to inspect local busybox image: %s, %v", localOut, err)
	}

	remoteOut, err := d.Cmd("inspect", "-r", repoName)
	if err == nil {
		c.Fatalf("inspect of remote image should have failed: %s", remoteOut)
	}

	if out, err := d.Cmd("push", fqn); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", fqn, err, out)
	}

	remoteOut, err = d.Cmd("inspect", "-r", repoName)
	if err != nil {
		c.Fatalf("failed to inspect remote image: %s, %v", localOut, err)
	}

	if err = json.Unmarshal([]byte(localOut), &localValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}
	if err = json.Unmarshal([]byte(remoteOut), &remoteValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}
	compareInspectValues(c, "a", localValue, remoteValue)

	deleteImages(fqn)

	remoteOut2, err := d.Cmd("inspect", "-r", fqn)
	if err != nil {
		c.Fatalf("failed to inspect remote busybox image: %s, %v", remoteOut2, err)
	}

	if remoteOut != remoteOut2 {
		c.Fatalf("remote inspect should produce identical output as before:\nfirst run: %s\n\nsecond run: %s", remoteOut, remoteOut2)
	}
}

func (s *DockerRegistrySuite) TestInspectNonExistentRepository(c *check.C) {
	repoName := fmt.Sprintf("%s/foo/non-existent", s.reg.url)

	inspectCmd := exec.Command(dockerBinary, "inspect", repoName)
	out, _, err := runCommandWithOutput(inspectCmd)
	if err == nil {
		c.Error("inspecting non-existent image should have failed", out)
	} else if !strings.Contains(strings.ToLower(out), "no such image or container") {
		c.Errorf("got unexpected error message: %v", out)
	}

	inspectCmd = exec.Command(dockerBinary, "inspect", "-r", repoName)
	out, _, err = runCommandWithOutput(inspectCmd)
	if err == nil {
		c.Error("inspecting non-existent image should have failed", out)
	} else if !strings.Contains(strings.ToLower(out), "no such image:") {
		c.Errorf("got unexpected error message: %v", out)
	}
}
