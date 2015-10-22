// +build linux

package dockerhooks

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/opencontainers/runc/libcontainer/configs"
)

const (
	hookDirPath = "/usr/libexec/docker/hooks.d"
)

func Prestart(state configs.HookState) error {
	hooks, hooksPath, err := getHooks()
	if err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	for _, item := range hooks {
		if item.Mode().IsRegular() {
			if err := runHook(path.Join(hookDirPath, item.Name()), "prestart", hooksPath, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func Poststop(state configs.HookState) error {
	hooks, hooksPath, err := getHooks()
	if err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	for i := len(hooks) - 1; i >= 0; i-- {
		fn := hooks[i].Name()
		for _, item := range hooks {
			if item.Mode().IsRegular() && fn == item.Name() {
				if err := runHook(path.Join(hookDirPath, item.Name()), "poststop", hooksPath, b); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func getHooks() ([]os.FileInfo, string, error) {
	hooksPath := os.Getenv("DOCKER_HOOKS_PATH")
	if hooksPath == "" {
		hooksPath = "/usr/libexec/docker/hooks.d"
	}

	// find any hooks executables
	if _, err := os.Stat(hookDirPath); os.IsNotExist(err) {
		return nil, "", nil
	}

	hooks, err := ioutil.ReadDir(hookDirPath)
	return hooks, hooksPath, err
}

func runHook(hookfile string, hookType string, hooksPath string, stdinBytes []byte) error {
	cmd := exec.Cmd{
		Path: hookfile,
		Args: []string{hookType},
		Env: []string{
			"container=docker",
			"DOCKER_HOOKS_PATH=", hooksPath,
		},
		Stdin: bytes.NewReader(stdinBytes),
	}
	return cmd.Run()
}
