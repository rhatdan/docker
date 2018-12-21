// +build linux freebsd

package initlayer // import "github.com/docker/docker/daemon/initlayer"

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/containerfs"
	"github.com/docker/docker/pkg/idtools"
	"golang.org/x/sys/unix"
)

// Setup populates a directory with mountpoints suitable
// for bind-mounting things into the container.
//
// This extra layer is used by all containers as the top-most ro layer. It protects
// the container from unwanted side-effects on the rw layer.
func Setup(initLayerFs containerfs.ContainerFS, rootIdentity idtools.Identity) error {
	// Since all paths are local to the container, we can just extract initLayerFs.Path()
	initLayer := initLayerFs.Path()

	for pth, typ := range map[string]string{
		"/dev/pts":         "dir",
		"/dev/shm":         "dir",
		"/proc":            "dir",
		"/sys":             "dir",
		"/.dockerenv":      "file",
		"/etc/resolv.conf": "file",
		"/etc/hosts":       "file",
		"/etc/hostname":    "file",
		"/dev/console":     "file",
		"/etc/mtab":        "/proc/mounts",
	} {
		parts := strings.Split(pth, "/")
		prev := "/"
		for _, p := range parts[1:] {
			prev = filepath.Join(prev, p)
			unix.Unlink(filepath.Join(initLayer, prev))
		}

		tpath := filepath.Join(initLayer, pth)
		if stat, err := os.Stat(tpath); err != nil {
			if os.IsNotExist(err) {
				if err := idtools.MkdirAllAndChownNew(filepath.Join(initLayer, filepath.Dir(pth)), 0755, rootIdentity); err != nil {
					return err
				}
				switch typ {
				case "dir":
					if err := idtools.MkdirAllAndChownNew(tpath, 0755, rootIdentity); err != nil {
						return err
					}
				case "file":
					f, err := os.OpenFile(tpath, os.O_CREATE, 0755)
					if err != nil {
						return err
					}
					f.Chown(rootIdentity.UID, rootIdentity.GID)
					f.Close()
				default:
					// Replace pth only if it is not a symbolic link
					if stat.Mode()&os.ModeSymlink != 0 {
						if err := os.Symlink(typ, tpath); err != nil {
							return err
						}
					}
				}
			} else {
				return err
			}
		}
	}

	// Layer is ready to use, if it wasn't before.
	return nil
}
