// +build darwin dragonfly freebsd linux netbsd openbsd

package utils

import (
	"os"
)

// TempDir returns the default directory to use for temporary files.
func TempDir() string {
	dir := os.Getenv("TMPDIR")
	if dir == "" {
		dir = "/var/tmp"
	}
	return dir
}
