// +build linux

package main

import (
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
	"os/exec"
	"path"
)

func getHooks() (string, []os.FileInfo, error) {
	hookPath := os.Getenv("DOCKER_HOOKS_PATH")
	if hookPath != "" {
		hookPath = "hooks.d"
	}
	// find any hooks executables
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		return "", nil, nil
	}
	hooks, err := ioutil.ReadDir(hookPath)
	return hookPath, hooks, err
}

func prestart(hooks []os.FileInfo, hookPath string, stdinbytes []byte) error {
	for _, item := range hooks {
		if item.Mode().IsRegular() {
			if err := run(path.Join(hookPath, item.Name()), stdinbytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func poststop(hooks []os.FileInfo, hookPath string, stdinbytes []byte) error {
	for i := len(hooks) - 1; i >= 0; i-- {
		fn := hooks[i].Name()
		for _, item := range hooks {
			if item.Mode().IsRegular() && fn == item.Name() {
				if err := run(path.Join(hookPath, item.Name()), stdinbytes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func run(hookfile string, incoming []byte) error {
	cmd := exec.Command(hookfile)
	cmd.Args = os.Args
	cmd.Env = os.Environ()
	log.Print("Executing docker hook: ", hookfile, cmd.Args, cmd.Env)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	stdinPipe.Write(incoming)
	return nil
}

func check(err error) {
	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}

func main() {
	var (
		err       error
		hooks     []os.FileInfo
		incoming  []byte
		logwriter *syslog.Writer
	)

	logwriter, err = syslog.New(syslog.LOG_NOTICE, "docker-hooks")
	if err == nil {
		log.SetOutput(logwriter)
	}
	log.Print("Executing dockerhook ", os.Args[0])

	hookPath, hooks, err := getHooks()
	check(err)
	if len(hooks) == 0 {
		return
	}

	incoming, err = ioutil.ReadAll(os.Stdin)
	check(err)

	switch os.Args[0] {
	case "prestart":
		err := prestart(hooks, hookPath, incoming)
		check(err)
	case "poststop":
		err := poststop(hooks, hookPath, incoming)
		check(err)
	default:
		log.Fatalf("ERROR: Invalid argument %v", os.Args[0])
	}
}
