package template

import (
	"syscall"

	"github.com/opencontainers/runc/libcontainer/apparmor"
	"github.com/opencontainers/runc/libcontainer/configs"
)

const defaultMountFlags = syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

// New returns the docker default configuration for libcontainer
func New() *configs.Config {
	container := &configs.Config{
		Capabilities: []string{
			"CHOWN",
			"DAC_OVERRIDE",
			"FSETID",
			"FOWNER",
			"MKNOD",
			"NET_RAW",
			"SETGID",
			"SETUID",
			"SETFCAP",
			"SETPCAP",
			"NET_BIND_SERVICE",
			"SYS_CHROOT",
			"KILL",
			"AUDIT_WRITE",
		},
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: "NEWNS"},
			{Type: "NEWUTS"},
			{Type: "NEWIPC"},
			{Type: "NEWPID"},
			{Type: "NEWNET"},
		}),
		Cgroups: &configs.Cgroup{
			Parent:           "docker",
			AllowAllDevices:  false,
			MemorySwappiness: -1,
		},
		Mounts: []*configs.Mount{
			{
				Source:      "proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "tmpfs",
				Destination: "/dev",
				Device:      "tmpfs",
				Flags:       syscall.MS_NOSUID | syscall.MS_STRICTATIME,
				Data:        "mode=755",
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620,gid=5",
			},
			{
				Source:      "sysfs",
				Destination: "/sys",
				Device:      "sysfs",
				Flags:       defaultMountFlags | syscall.MS_RDONLY,
			},
			{
				Source:      "cgroup",
				Destination: "/sys/fs/cgroup",
				Device:      "cgroup",
				Flags:       defaultMountFlags | syscall.MS_RDONLY,
			},
		},
		MaskPaths: []string{
			"/proc/kcore",
			"/proc/latency_stats",
			"/proc/timer_stats",
		},
		ReadonlyPaths: []string{
			"/proc/asound",
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		},
	}

	if apparmor.IsEnabled() {
		container.AppArmorProfile = "docker-default"
	}
	container.Hooks = &configs.Hooks{}
	cmd := configs.Command{
		Path: "/usr/libexec/docker/dockerhooks",
		Args: []string{"prestart"},
		Env:  []string{"container=docker"},
	}
	container.Hooks.Prestart = append(container.Hooks.Prestart, configs.NewCommandHook(cmd))
	cmd = configs.Command{
		Path: "/usr/libexec/docker/dockerhooks",
		Args: []string{"poststop"},
		Env:  []string{"container=docker"},
	}
	container.Hooks.Poststop = append(container.Hooks.Poststop, configs.NewCommandHook(cmd))

	return container
}
