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

const (
	hookDirPath = "/usr/libexec/docker/hooks.d"
)

func getHooks() ([]os.FileInfo, error) {
	// find any hooks executables
	if _, err := os.Stat(hookDirPath); os.IsNotExist(err) {
		return nil, nil
	}
	return ioutil.ReadDir(hookDirPath)
}

func prestart(hooks []os.FileInfo, stdinbytes []byte) error {
	for _, item := range hooks {
		if item.Mode().IsRegular() {
			if err := run(path.Join(hookDirPath, item.Name()), stdinbytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func poststop(hooks []os.FileInfo, stdinbytes []byte) error {
	for i := len(hooks) - 1; i >= 0; i-- {
		fn := hooks[i].Name()
		for _, item := range hooks {
			if item.Mode().IsRegular() && fn == item.Name() {
				if err := run(path.Join(hookDirPath, item.Name()), stdinbytes); err != nil {
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

	hooks, err = getHooks()
	check(err)
	if len(hooks) == 0 {
		return
	}

	incoming, err = ioutil.ReadAll(os.Stdin)
	check(err)

	switch os.Args[0] {
	case "prestart":
		err := prestart(hooks, incoming)
		check(err)
	case "poststop":
		err := poststop(hooks, incoming)
		check(err)
	default:
		log.Fatalf("ERROR: Invalid argument %v", os.Args[0])
	}
}
