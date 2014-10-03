package ipc

import (
	"fmt"
	"os"
	"syscall"

	"github.com/docker/libcontainer/system"
)

// Ipc defines configuration for a container's ipc stack
//

// The ipc configuration can be omited from a container causing the
// container to be setup with the host's ipc stack
type Ipc struct {
	// Path to ipc namespace
	NsPath string `json:"ns_path,omitempty"`
}

// Join the IPC Namespace of specified ipc path if it exists.
// If the path does not exist then you are not joining a container.
func Initialize(ipc *Ipc) error {

	if ipc.NsPath == "" {
		return nil
	}
	f, err := os.OpenFile(ipc.NsPath, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed get IPC namespace fd: %v", err)
	}

	err = system.Setns(f.Fd(), syscall.CLONE_NEWIPC)
	f.Close()

	if err != nil {
		return fmt.Errorf("failed to setns current IPC namespace: %v", err)
	}

	return nil
}
