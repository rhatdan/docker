package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-check/check"
)

func (s *DockerSuite) TestInspectApiContainerResponse(c *check.C) {
	out, _ := dockerCmd(c, "run", "-d", "busybox", "true")

	cleanedContainerID := strings.TrimSpace(out)
	keysBase := []string{"Id", "State", "Created", "Path", "Args", "Config", "Image", "NetworkSettings",
		"ResolvConfPath", "HostnamePath", "HostsPath", "LogPath", "Name", "Driver", "ExecDriver", "MountLabel", "ProcessLabel", "GraphDriver"}

	cases := []struct {
		version string
		keys    []string
	}{
		{"1.20", append(keysBase, "Mounts")},
		{"1.19", append(keysBase, "Volumes", "VolumesRW")},
	}

	for _, cs := range cases {
		endpoint := fmt.Sprintf("/v%s/containers/%s/json", cs.version, cleanedContainerID)

		status, body, err := sockRequest("GET", endpoint, nil)
		c.Assert(status, check.Equals, http.StatusOK)
		c.Assert(err, check.IsNil)

		var inspectJSON map[string]interface{}
		if err = json.Unmarshal(body, &inspectJSON); err != nil {
			c.Fatalf("unable to unmarshal body for version %s: %v", cs.version, err)
		}

		for _, key := range cs.keys {
			if _, ok := inspectJSON[key]; !ok {
				c.Fatalf("%s does not exist in response for version %s", key, cs.version)
			}
		}

		//Issue #6830: type not properly converted to JSON/back
		if _, ok := inspectJSON["Path"].(bool); ok {
			c.Fatalf("Path of `true` should not be converted to boolean `true` via JSON marshalling")
		}
	}
}

func compareInspectValues(c *check.C, name string, fst, snd interface{}, localVsRemote bool) {
	additionalLocalAttributes := map[string]struct{}{}
	additionalRemoteAttributes := map[string]struct{}{}
	if localVsRemote {
		additionalLocalAttributes = map[string]struct{}{
			"GraphDriver": {},
			"VirtualSize": {},
		}
		additionalRemoteAttributes = map[string]struct{}{
			"Registry": {},
			"Digest":   {},
			"Tag":      {},
		}
	}

	isRootObject := len(name) <= 1

	if reflect.TypeOf(fst) != reflect.TypeOf(snd) {
		c.Errorf("types don't match for %q: %T != %T", name, fst, snd)
		return
	}
	switch fst.(type) {
	case bool:
		lVal := fst.(bool)
		rVal := snd.(bool)
		if lVal != rVal {
			c.Errorf("fst value differs from snd for %q: %t != %t", name, lVal, rVal)
		}
	case float64:
		lVal := fst.(float64)
		rVal := snd.(float64)
		if lVal != rVal {
			c.Errorf("fst value differs from snd for %q: %f != %f", name, lVal, rVal)
		}
	case string:
		lVal := fst.(string)
		rVal := snd.(string)
		if lVal != rVal {
			c.Errorf("fst value differs from snd for %q: %q != %q", name, lVal, rVal)
		}
	// JSON array
	case []interface{}:
		lVal := fst.([]interface{})
		rVal := snd.([]interface{})
		if len(lVal) != len(rVal) {
			c.Errorf("array length differs between fst and snd for %q: %d != %d", name, len(lVal), len(rVal))
		}
		for i := 0; i < len(lVal) && i < len(rVal); i++ {
			compareInspectValues(c, fmt.Sprintf("%s[%d]", name, i), lVal[i], rVal[i], localVsRemote)
		}
	// JSON object
	case map[string]interface{}:
		lMap := fst.(map[string]interface{})
		rMap := snd.(map[string]interface{})
		if isRootObject && len(lMap)-len(additionalLocalAttributes) != len(rMap)-len(additionalRemoteAttributes) {
			c.Errorf("got unexpected number of root object's attributes from snd inpect %q: %d != %d", name, len(lMap)-len(additionalLocalAttributes), len(rMap)-len(additionalRemoteAttributes))
		} else if !isRootObject && len(lMap) != len(rMap) {
			c.Errorf("map length differs between fst and snd for %q: %d != %d", name, len(lMap), len(rMap))
		}
		for key, lVal := range lMap {
			itemName := fmt.Sprintf("%s.%s", name, key)
			rVal, ok := rMap[key]
			if ok {
				compareInspectValues(c, itemName, lVal, rVal, localVsRemote)
			} else if _, exists := additionalLocalAttributes[key]; !isRootObject || !localVsRemote || !exists {
				c.Errorf("attribute %q present in fst but not in snd object", itemName)
			}
		}
		for key := range rMap {
			if _, ok := lMap[key]; !ok {
				if _, exists := additionalRemoteAttributes[key]; !isRootObject || !localVsRemote || !exists {
					c.Errorf("attribute \"%s.%s\" present in snd but not in fst object", name, key)
				}
			}
		}
	case nil:
		if fst != snd {
			c.Errorf("fst value differs from snd for %q: %v (%T) != %v (%T)", name, fst, fst, snd, snd)
		}
	default:
		c.Fatalf("got unexpected type (%T) for %q", fst, name)
	}
}

func apiCallInspectImage(c *check.C, d *Daemon, repoName string, remote, shouldFail bool) (value interface{}, status int, err error) {
	suffix := ""
	if remote {
		suffix = "?remote=1"
	}
	endpoint := fmt.Sprintf("/v1.20/images/%s/json%s", repoName, suffix)
	status, body, err := func() (int, []byte, error) {
		if d == nil {
			return sockRequest("GET", endpoint, nil)
		}
		return d.sockRequest("GET", endpoint, nil)
	}()
	if shouldFail {
		c.Assert(status, check.Not(check.Equals), http.StatusOK)
		if err == nil {
			err = fmt.Errorf("%s", bytes.TrimSpace(body))
		}
	} else {
		c.Assert(status, check.Equals, http.StatusOK)
		c.Assert(err, check.IsNil)
		if err = json.Unmarshal(body, &value); err != nil {
			what := "local"
			if remote {
				what = "remote"
			}
			c.Fatalf("failed to parse result for %s image %q: %v", what, repoName, err)
		}
	}
	return
}

func (s *DockerRegistrySuite) TestInspectApiRemoteImage(c *check.C) {
	repoName := fmt.Sprintf("%v/dockercli/busybox", s.reg.url)
	// tag the image and upload it to the private registry
	dockerCmd(c, "tag", "busybox", repoName)
	defer deleteImages(repoName)

	dockerCmd(c, "push", repoName)
	localValue, _, _ := apiCallInspectImage(c, nil, repoName, false, false)
	remoteValue, _, _ := apiCallInspectImage(c, nil, repoName, true, false)
	compareInspectValues(c, "a", localValue, remoteValue, true)

	deleteImages(repoName)

	// local inspect shall fail now
	_, status, _ := apiCallInspectImage(c, nil, repoName, false, true)
	c.Assert(status, check.Equals, http.StatusNotFound)

	// remote inspect shall still succeed
	remoteValue2, _, _ := apiCallInspectImage(c, nil, repoName, true, false)
	compareInspectValues(c, "a", localValue, remoteValue2, true)
}

func (s *DockerRegistrySuite) TestInspectApiImageFromAdditionalRegistry(c *check.C) {
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

	localValue, _, _ := apiCallInspectImage(c, d, repoName, false, false)

	_, status, _ := apiCallInspectImage(c, d, repoName, true, true)
	c.Assert(status, check.Equals, http.StatusNotFound)

	if out, err := d.Cmd("push", fqn); err != nil {
		c.Fatalf("failed to push image %s: error %v, output %q", fqn, err, out)
	}

	remoteValue, _, _ := apiCallInspectImage(c, d, repoName, true, false)
	compareInspectValues(c, "a", localValue, remoteValue, true)

	if out, err := d.Cmd("rmi", fqn); err != nil {
		c.Fatalf("failed to remove image %s: %s, %v", fqn, out, err)
	}

	remoteValue2, _, _ := apiCallInspectImage(c, d, fqn, true, false)
	compareInspectValues(c, "a", localValue, remoteValue2, true)
}

func (s *DockerRegistrySuite) TestInspectApiNonExistentRepository(c *check.C) {
	repoName := fmt.Sprintf("%s/foo/non-existent", s.reg.url)

	_, status, err := apiCallInspectImage(c, nil, repoName, false, true)
	c.Assert(status, check.Equals, http.StatusNotFound)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err.Error(), check.Matches, `(?i)no such image.*`)

	_, status, err = apiCallInspectImage(c, nil, repoName, true, true)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err.Error(), check.Matches, `(?i).*(not found|no such image|no tags available).*`)
}
