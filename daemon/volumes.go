package daemon

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/execdriver"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/symlink"
	"github.com/docker/docker/volumes"
	"github.com/docker/libcontainer/label"
)

type Mount struct {
	MountToPath string
	container   *Container
	volume      *volumes.Volume
	Mode        string
	copyData    bool
	from        *Container
}

func (mnt *Mount) Export(resource string) (io.ReadCloser, error) {
	var name string
	if resource == mnt.MountToPath[1:] {
		name = filepath.Base(resource)
	}
	path, err := filepath.Rel(mnt.MountToPath[1:], resource)
	if err != nil {
		return nil, err
	}
	return mnt.volume.Export(path, name)
}

func (container *Container) prepareVolumes() error {
	if container.Volumes == nil || len(container.Volumes) == 0 {
		container.Volumes = make(map[string]string)
		container.VolumesRW = make(map[string]bool)
		container.VolumesMode = make(map[string]string)
	}

	return container.createVolumes()
}

// sortedVolumeMounts returns the list of container volume mount points sorted in lexicographic order
func (container *Container) sortedVolumeMounts() []string {
	var mountPaths []string
	for path := range container.Volumes {
		mountPaths = append(mountPaths, path)
	}

	sort.Strings(mountPaths)
	return mountPaths
}

func (container *Container) createVolumes() error {
	mounts, err := container.parseVolumeMountConfig()
	if err != nil {
		return err
	}

	for _, mnt := range mounts {
		if err := mnt.initialize(); err != nil {
			return err
		}
	}

	// On every start, this will apply any new `VolumesFrom` entries passed in via HostConfig, which may override volumes set in `create`
	return container.applyVolumesFrom()
}

func (m *Mount) initialize() error {
	// No need to initialize anything since it's already been initialized
	if hostPath, exists := m.container.Volumes[m.MountToPath]; exists {
		// If this is a bind-mount/volumes-from, maybe it was passed in at start instead of create
		// We need to make sure bind-mounts/volumes-from passed on start can override existing ones.
		if !m.volume.IsBindMount && m.from == nil {
			return nil
		}
		if m.volume.Path == hostPath {
			return nil
		}

		// Make sure we remove these old volumes we don't actually want now.
		// Ignore any errors here since this is just cleanup, maybe someone volumes-from'd this volume
		v := m.container.daemon.volumes.Get(hostPath)
		v.RemoveContainer(m.container.ID)
		m.container.daemon.volumes.Delete(v.Path)
	}

	// This is the full path to container fs + mntToPath
	containerMntPath, err := symlink.FollowSymlinkInScope(filepath.Join(m.container.basefs, m.MountToPath), m.container.basefs)
	if err != nil {
		return err
	}
	m.container.VolumesRW[m.MountToPath] = volumes.Writable(m.Mode)
	m.container.VolumesMode[m.MountToPath] = m.Mode
	m.container.Volumes[m.MountToPath] = m.volume.Path
	m.volume.AddContainer(m.container.ID)
	if volumes.Writable(m.Mode) && m.copyData {
		// Copy whatever is in the container at the mntToPath to the volume
		copyExistingContents(containerMntPath, m.volume.Path)
	}

	return nil
}

func (container *Container) VolumePaths() map[string]struct{} {
	var paths = make(map[string]struct{})
	for _, path := range container.Volumes {
		paths[path] = struct{}{}
	}
	return paths
}

func (container *Container) registerVolumes() {
	var (
		exists bool
		mode   string
	)

	for path := range container.VolumePaths() {
		if v := container.daemon.volumes.Get(path); v != nil {
			v.AddContainer(container.ID)
			continue
		}

		// if container was created with an old daemon, this volume may not be registered so we need to make sure it gets registered
		if mode, exists = container.VolumesMode[path]; exists {
			mode = "w"
		}
		v, err := container.daemon.volumes.FindOrCreateVolume(path, mode)
		if err != nil {
			log.Debugf("error registering volume %s: %v", path, err)
			continue
		}
		v.AddContainer(container.ID)
	}
}

func (container *Container) derefVolumes() {
	for path := range container.VolumePaths() {
		vol := container.daemon.volumes.Get(path)
		if vol == nil {
			log.Debugf("Volume %s was not found and could not be dereferenced", path)
			continue
		}
		vol.RemoveContainer(container.ID)
	}
}

func (container *Container) parseVolumeMountConfig() (map[string]*Mount, error) {
	var mounts = make(map[string]*Mount)
	// Get all the bind mounts
	for _, spec := range container.hostConfig.Binds {
		path, mountToPath, mode, err := parseBindMountSpec(spec)
		if err != nil {
			return nil, err
		}
		// Check if a volume already exists for this and use it
		vol, err := container.daemon.volumes.FindOrCreateVolume(path, mode)
		if err != nil {
			return nil, err
		}
		mounts[mountToPath] = &Mount{
			container:   container,
			volume:      vol,
			MountToPath: mountToPath,
			Mode:        mode,
		}
	}

	// Get the rest of the volumes
	for path := range container.Config.Volumes {
		// Check if this is already added as a bind-mount
		path = filepath.Clean(path)
		if _, exists := mounts[path]; exists {
			continue
		}

		// Check if this has already been created
		if _, exists := container.Volumes[path]; exists {
			continue
		}

		vol, err := container.daemon.volumes.FindOrCreateVolume("", "w")
		if err != nil {
			return nil, err
		}
		mounts[path] = &Mount{
			container:   container,
			MountToPath: path,
			volume:      vol,
			Mode:        "w",
			copyData:    true,
		}
	}

	return mounts, nil
}

func parseBindMountSpec(spec string) (string, string, string, error) {
	var (
		path, mountToPath string
		mode              string
		arr               = strings.Split(spec, ":")
	)

	switch len(arr) {
	case 2:
		path = arr[0]
		mountToPath = arr[1]
	case 3:
		if !volumes.ValidMode(arr[2], true) {
			return "", "", "", fmt.Errorf("Invalid volume options specification: %s", spec)
		}
		path = arr[0]
		mountToPath = arr[1]
		mode = arr[2]
	default:
		return "", "", "", fmt.Errorf("Invalid volume specification: %s", spec)
	}

	if !filepath.IsAbs(path) {
		return "", "", "", fmt.Errorf("cannot bind mount volume: %s volume paths must be absolute.", path)
	}

	path = filepath.Clean(path)
	mountToPath = filepath.Clean(mountToPath)
	return path, mountToPath, mode, nil
}

func parseVolumesFromSpec(spec string) (string, string, error) {
	specParts := strings.SplitN(spec, ":", 2)
	if len(specParts) == 0 {
		return "", "", fmt.Errorf("malformed volumes-from specification: %s", spec)
	}

	var (
		id   = specParts[0]
		mode = "w"
	)

	if len(specParts) == 2 {
		mode = specParts[1]
		if !volumes.ValidMode(mode, false) {
			return "", "", fmt.Errorf("invalid mode for volumes-from: %s", mode)
		}
	}
	return id, mode, nil
}

func (container *Container) applyVolumesFrom() error {
	volumesFrom := container.hostConfig.VolumesFrom
	if len(volumesFrom) > 0 && container.AppliedVolumesFrom == nil {
		container.AppliedVolumesFrom = make(map[string]struct{})
	}

	mountGroups := make(map[string][]*Mount)

	for _, spec := range volumesFrom {
		id, mode, err := parseVolumesFromSpec(spec)
		if err != nil {
			return err
		}
		if _, exists := container.AppliedVolumesFrom[id]; exists {
			// Don't try to apply these since they've already been applied
			continue
		}

		c := container.daemon.Get(id)
		if c == nil {
			return fmt.Errorf("container %s not found, impossible to mount its volumes", id)
		}

		var (
			fromMounts = c.VolumeMounts()
			mounts     []*Mount
		)

		for _, mnt := range fromMounts {
			if !(volumes.Writable(mode) && volumes.Writable(mnt.Mode)) {
				mnt.Mode = volumes.MakeReadOnly(mnt.Mode)
			}
			mounts = append(mounts, mnt)
		}
		mountGroups[id] = mounts
	}

	for id, mounts := range mountGroups {
		for _, mnt := range mounts {
			mnt.from = mnt.container
			mnt.container = container
			if err := mnt.initialize(); err != nil {
				return err
			}
		}
		container.AppliedVolumesFrom[id] = struct{}{}
	}
	return nil
}

func (container *Container) setupMounts() error {
	mounts := []execdriver.Mount{
		{Source: container.ResolvConfPath, Destination: "/etc/resolv.conf", Mode: "w", Private: true},
	}
	if container.HostnamePath != "" {
		mounts = append(mounts, execdriver.Mount{Source: container.HostnamePath, Destination: "/etc/hostname", Mode: "w", Private: true})
	}

	if container.HostsPath != "" {
		mounts = append(mounts, execdriver.Mount{Source: container.HostsPath, Destination: "/etc/hosts", Mode: "w", Private: true})
	}

	for _, m := range mounts {
		if err := label.SetFileLabel(m.Source, container.MountLabel); err != nil {
			return err
		}
	}

	if container.hostConfig.MountRun {
		mounts = append(mounts, execdriver.Mount{Source: "tmpfs", Destination: "/run", Mode: "w", Private: true})
	}

	// Mount user specified volumes
	// Note, these are not private because you may want propagation of (un)mounts from host
	// volumes. For instance if you use -v /usr:/usr and the host later mounts /usr/share you
	// want this new mount in the container
	// These mounts must be ordered based on the length of the path that it is being mounted to (lexicographic)
	for _, path := range container.sortedVolumeMounts() {
		mounts = append(mounts, execdriver.Mount{
			Source:      container.Volumes[path],
			Destination: path,
			Mode:        container.VolumesMode[path],
		})
	}

	secretsPath, err := container.secretsPath()
	if err != nil {
		return err
	}

	mounts = append(mounts, execdriver.Mount{
		Source:      secretsPath,
		Destination: "/run/secrets",
		Mode:        "w",
	})

	container.command.Mounts = mounts
	return nil
}

func (container *Container) VolumeMounts() map[string]*Mount {
	mounts := make(map[string]*Mount)

	for mountToPath, path := range container.Volumes {
		if v := container.daemon.volumes.Get(path); v != nil {
			mounts[mountToPath] = &Mount{volume: v, container: container, MountToPath: mountToPath, Mode: container.VolumesMode[mountToPath]}
		}
	}

	return mounts
}

func copyExistingContents(source, destination string) error {
	volList, err := ioutil.ReadDir(source)
	if err != nil {
		return err
	}

	if len(volList) > 0 {
		srcList, err := ioutil.ReadDir(destination)
		if err != nil {
			return err
		}

		if len(srcList) == 0 {
			// If the source volume is empty copy files from the root into the volume
			if err := chrootarchive.CopyWithTar(source, destination); err != nil {
				return err
			}
		}
	}

	return copyOwnership(source, destination)
}

// copyOwnership copies the permissions and uid:gid of the source file
// into the destination file
func copyOwnership(source, destination string) error {
	var stat syscall.Stat_t

	if err := syscall.Stat(source, &stat); err != nil {
		return err
	}

	if err := os.Chown(destination, int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}

	return os.Chmod(destination, os.FileMode(stat.Mode))
}
