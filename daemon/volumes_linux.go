// +build !windows

package daemon

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/docker/daemon/execdriver"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/libcontainer/label"
)

// copyOwnership copies the permissions and uid:gid of the source file
// into the destination file
func copyOwnership(source, destination string) error {
	stat, err := system.Stat(source)
	if err != nil {
		return err
	}

	if err := os.Chown(destination, int(stat.Uid()), int(stat.Gid())); err != nil {
		return err
	}

	return os.Chmod(destination, os.FileMode(stat.Mode()))
}

func (container *Container) setupMounts() ([]execdriver.Mount, error) {
	var mounts []execdriver.Mount
	for _, m := range container.MountPoints {
		path, err := m.Setup()
		if err != nil {
			return nil, err
		}

		mounts = append(mounts, execdriver.Mount{
			Source:      path,
			Destination: m.Destination,
			Writable:    m.RW,
		})
	}

	if container.Config.Systemd {
		if container.MountPoints["/run"] == nil {
			mounts = append(mounts, execdriver.Mount{Source: "tmpfs", Destination: "/run", Writable: true, Private: true})
		}

		if container.MountPoints["/sys"] == nil &&
			container.MountPoints["/sys/fs"] == nil &&
			container.MountPoints["/sys/fs/cgroup"] == nil {
			mounts = append(mounts, execdriver.Mount{Source: "/sys/fs/cgroup", Destination: "/sys/fs/cgroup", Writable: false, Private: true})
		}

		if container.MountPoints["/var"] == nil &&
			container.MountPoints["/var/log"] == nil &&
			container.MountPoints["/var/log/journal"] == nil {
			if journalPath, err := container.setupJournal(); err != nil {
				return nil, err
			} else {
				if journalPath != "" {
					label.Relabel(journalPath, container.MountLabel, "Z")
					mounts = append(mounts, execdriver.Mount{Source: journalPath, Destination: journalPath, Writable: true, Private: true})
				}
			}
		}
	}

	mounts = sortMounts(mounts)
	return append(mounts, container.networkMounts()...), nil
}

func sortMounts(m []execdriver.Mount) []execdriver.Mount {
	sort.Sort(mounts(m))
	return m
}

type mounts []execdriver.Mount

func (m mounts) Len() int {
	return len(m)
}

func (m mounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

func (m mounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m mounts) parts(i int) int {
	return len(strings.Split(filepath.Clean(m[i].Destination), string(os.PathSeparator)))
}

func (container *Container) setupJournal() (string, error) {
	path := journalPath(container.ID)
	if path != "" {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", err
		}
	}
	return path, nil
}
