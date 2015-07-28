package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
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

func (s *DockerSuite) TestInspectDefault(c *check.C) {

	//Both the container and image are named busybox. docker inspect will fetch the container JSON.
	//If the container JSON is not available, it will go for the image JSON.

	dockerCmd(c, "run", "--name=busybox", "-d", "busybox", "true")
	dockerCmd(c, "inspect", "busybox")
}

func (s *DockerSuite) TestInspectTypeFlagContainer(c *check.C) {

	//Both the container and image are named busybox. docker inspect will fetch container
	//JSON State.Running field. If the field is true, it's a container.

	dockerCmd(c, "run", "--name=busybox", "-d", "busybox", "top")

	formatStr := fmt.Sprintf("--format='{{.State.Running}}'")
	out, exitCode, err := dockerCmdWithError(c, "inspect", "--type=container", formatStr, "busybox")
	if exitCode != 0 || err != nil {
		c.Fatalf("failed to inspect container: %s, %v", out, err)
	}

	if out != "true\n" {
		c.Fatal("not a container JSON")
	}
}

func (s *DockerSuite) TestInspectTypeFlagWithNoContainer(c *check.C) {

	//Run this test on an image named busybox. docker inspect will try to fetch container
	//JSON. Since there is no container named busybox and --type=container, docker inspect will
	//not try to get the image JSON. It will throw an error.

	dockerCmd(c, "run", "-d", "busybox", "true")

	_, exitCode, err := dockerCmdWithError(c, "inspect", "--type=container", "busybox")
	if exitCode == 0 || err == nil {
		c.Fatalf("docker inspect should have failed, as there is no container named busybox")
	}
}

func (s *DockerSuite) TestInspectTypeFlagWithImage(c *check.C) {

	//Both the container and image are named busybox. docker inspect will fetch image
	//JSON as --type=image. if there is no image with name busybox, docker inspect
	//will throw an error.

	dockerCmd(c, "run", "--name=busybox", "-d", "busybox", "true")

	out, exitCode, err := dockerCmdWithError(c, "inspect", "--type=image", "busybox")
	if exitCode != 0 || err != nil {
		c.Fatalf("failed to inspect image: %s, %v", out, err)
	}

	if strings.Contains(out, "State") {
		c.Fatal("not an image JSON")
	}
}

func (s *DockerSuite) TestInspectTypeFlagWithInvalidValue(c *check.C) {

	//Both the container and image are named busybox. docker inspect will fail
	//as --type=foobar is not a valid value for the flag.

	dockerCmd(c, "run", "--name=busybox", "-d", "busybox", "true")

	out, exitCode, err := dockerCmdWithError(c, "inspect", "--type=foobar", "busybox")
	if exitCode != 0 || err != nil {
		if !strings.Contains(out, "not a valid value for --type") {
			c.Fatalf("failed to inspect image: %s, %v", out, err)
		}
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
	out, exitCode, err := dockerCmdWithError(c, "inspect", formatStr, imageTest)
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
	out, _ = dockerCmd(c, "inspect", formatStr, id)
	if result, err := strconv.ParseBool(strings.TrimSuffix(out, "\n")); err != nil || !result {
		c.Fatalf("Expected exitcode: %d for container: %s", exitCode, id)
	}
}

func (s *DockerSuite) TestInspectImageGraphDriver(c *check.C) {
	imageTest := "emptyfs"
	name, err := inspectField(imageTest, "GraphDriver.Name")
	c.Assert(err, check.IsNil)

	if name != "devicemapper" && name != "overlay" && name != "vfs" && name != "zfs" && name != "btrfs" && name != "aufs" {
		c.Fatalf("%v is not a valid graph driver name", name)
	}

	if name != "devicemapper" {
		return
	}

	deviceID, err := inspectField(imageTest, "GraphDriver.Data.DeviceId")
	c.Assert(err, check.IsNil)

	_, err = strconv.Atoi(deviceID)
	if err != nil {
		c.Fatalf("failed to inspect DeviceId of the image: %s, %v", deviceID, err)
	}

	deviceSize, err := inspectField(imageTest, "GraphDriver.Data.DeviceSize")
	c.Assert(err, check.IsNil)

	_, err = strconv.ParseUint(deviceSize, 10, 64)
	if err != nil {
		c.Fatalf("failed to inspect DeviceSize of the image: %s, %v", deviceSize, err)
	}
}

func (s *DockerSuite) TestInspectContainerGraphDriver(c *check.C) {
	out, _ := dockerCmd(c, "run", "-d", "busybox", "true")
	out = strings.TrimSpace(out)

	name, err := inspectField(out, "GraphDriver.Name")
	c.Assert(err, check.IsNil)

	if name != "devicemapper" && name != "overlay" && name != "vfs" && name != "zfs" && name != "btrfs" && name != "aufs" {
		c.Fatalf("%v is not a valid graph driver name", name)
	}

	if name != "devicemapper" {
		return
	}

	deviceID, err := inspectField(out, "GraphDriver.Data.DeviceId")
	c.Assert(err, check.IsNil)

	_, err = strconv.Atoi(deviceID)
	if err != nil {
		c.Fatalf("failed to inspect DeviceId of the image: %s, %v", deviceID, err)
	}

	deviceSize, err := inspectField(out, "GraphDriver.Data.DeviceSize")
	c.Assert(err, check.IsNil)

	_, err = strconv.ParseUint(deviceSize, 10, 64)
	if err != nil {
		c.Fatalf("failed to inspect DeviceSize of the image: %s, %v", deviceSize, err)
	}
}

func (s *DockerSuite) TestInspectBindMountPoint(c *check.C) {
	dockerCmd(c, "run", "-d", "--name", "test", "-v", "/data:/data:ro,z", "busybox", "cat")

	vol, err := inspectFieldJSON("test", "Mounts")
	c.Assert(err, check.IsNil)

	var mp []types.MountPoint
	err = unmarshalJSON([]byte(vol), &mp)
	c.Assert(err, check.IsNil)

	if len(mp) != 1 {
		c.Fatalf("Expected 1 mount point, was %v\n", len(mp))
	}

	m := mp[0]

	if m.Name != "" {
		c.Fatal("Expected name to be empty")
	}

	if m.Driver != "" {
		c.Fatal("Expected driver to be empty")
	}

	if m.Source != "/data" {
		c.Fatalf("Expected source /data, was %s\n", m.Source)
	}

	if m.Destination != "/data" {
		c.Fatalf("Expected destination /data, was %s\n", m.Destination)
	}

	if m.Mode != "ro,z" {
		c.Fatalf("Expected mode `ro,z`, was %s\n", m.Mode)
	}

	if m.RW != false {
		c.Fatalf("Expected rw to be false")
	}
}

// #14947
func (s *DockerSuite) TestInspectTimesAsRFC3339Nano(c *check.C) {
	out, _ := dockerCmd(c, "run", "-d", "busybox", "true")
	id := strings.TrimSpace(out)
	startedAt, err := inspectField(id, "State.StartedAt")
	c.Assert(err, check.IsNil)
	finishedAt, err := inspectField(id, "State.FinishedAt")
	c.Assert(err, check.IsNil)
	created, err := inspectField(id, "Created")
	c.Assert(err, check.IsNil)

	_, err = time.Parse(time.RFC3339Nano, startedAt)
	c.Assert(err, check.IsNil)
	_, err = time.Parse(time.RFC3339Nano, finishedAt)
	c.Assert(err, check.IsNil)
	_, err = time.Parse(time.RFC3339Nano, created)
	c.Assert(err, check.IsNil)

	created, err = inspectField("busybox", "Created")
	c.Assert(err, check.IsNil)

	_, err = time.Parse(time.RFC3339Nano, created)
	c.Assert(err, check.IsNil)
}

func compareInspectValues(c *check.C, name string, local, remote interface{}) {
	additionalLocalAttributes := map[string]struct{}{
		"GraphDriver": {},
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
	dockerCmd(c, "tag", "busybox", repoName)

	localOut, _ := dockerCmd(c, "inspect", repoName)

	dockerCmd(c, "push", repoName)
	remoteOut, _ := dockerCmd(c, "inspect", "-r", repoName)

	if err := json.Unmarshal([]byte(localOut), &localValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}
	if err := json.Unmarshal([]byte(remoteOut), &remoteValue); err != nil {
		c.Fatalf("failed to parse result for local busybox image: %v", err)
	}
	compareInspectValues(c, "a", localValue, remoteValue)

	deleteImages(repoName)

	// local inspect shall fail now
	inspectCmd := exec.Command(dockerBinary, "inspect", repoName)
	localOut, _, err := runCommandWithOutput(inspectCmd)
	if err == nil {
		c.Fatalf("inspect on removed local images should have failed: %s", localOut)
	}

	// remote inspect shall still succeed
	remoteOut2, _ := dockerCmd(c, "inspect", "-r", repoName)

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
	} else if !strings.Contains(strings.ToLower(out), "no such image:") && !strings.Contains(strings.ToLower(out), "no tags available") {
		c.Errorf("got unexpected error message: %v", out)
	}
}
