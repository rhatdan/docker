// +build linux

package systemd

import (
	"encoding/hex"
	"os"

	//	"github.com/coreos/go-systemd/util/"
	"github.com/godbus/dbus"
)

var conn *dbus.Conn

// Remove once IsRunningSystemd is available in go-systemd/util
func IsRunningSystemd() bool {
	s, err := os.Stat("/run/systemd/system")
	if err != nil {
		return false
	}

	return s.IsDir()
}

// RegisterMachine with systemd on the host system
func RegisterMachine(name string, id string, pid int, root_directory string) error {
	var (
		av  []byte
		err error
	)
	if !IsRunningSystemd() {
		return nil
	}

	if conn == nil {
		conn, err = dbus.SystemBus()
		if err != nil {
			return (err)
		}
	}

	av, err = hex.DecodeString(id[0:32])
	if err != nil {
		return err
	}

	obj := conn.Object("org.freedesktop.machine1", "/org/freedesktop/machine1")
	return obj.Call("org.freedesktop.machine1.Manager.RegisterMachine", 0, name[0:64], av, "docker", "container", uint32(pid), root_directory).Err
}

// TerminateMachine registered with systemd on the host system
func TerminateMachine(name string) error {
	var (
		err error
	)
	if !IsRunningSystemd() {
		return nil
	}

	if conn == nil {
		conn, err = dbus.SystemBus()
		if err != nil {
			return (err)
		}
	}

	obj := conn.Object("org.freedesktop.machine1", "/org/freedesktop/machine1")
	return obj.Call("org.freedesktop.machine1.Manager.TerminateMachine", 0, name).Err
}
