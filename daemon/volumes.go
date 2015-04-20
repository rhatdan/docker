package daemon

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/execdriver"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/docker/libcontainer/label"
)

type volumeMount struct {
	containerPath string
	hostPath      string
	writable      bool
	copyData      bool
	from          string
}

func (container *Container) prepareVolumes() error {
	if container.Volumes == nil || len(container.Volumes) == 0 {
		container.Volumes = make(map[string]string)
		container.VolumesRW = make(map[string]bool)
	}

	if len(container.hostConfig.VolumesFrom) > 0 && container.AppliedVolumesFrom == nil {
		container.AppliedVolumesFrom = make(map[string]struct{})
	}
	return container.createVolumes()
}

func (container *Container) createVolumes() error {
	mounts := make(map[string]*volumeMount)

	// get the normal volumes
	for path := range container.Config.Volumes {
		path = filepath.Clean(path)
		// skip if there is already a volume for this container path
		if _, exists := container.Volumes[path]; exists {
			continue
		}

		realPath, err := container.getResourcePath(path)
		if err != nil {
			return err
		}
		if stat, err := os.Stat(realPath); err == nil {
			if !stat.IsDir() {
				return fmt.Errorf("can't mount to container path, file exists - %s", path)
			}
		}

		mnt := &volumeMount{
			containerPath: path,
			writable:      true,
			copyData:      true,
		}
		mounts[mnt.containerPath] = mnt
	}

	// Get all the bind mounts
	// track bind paths separately due to #10618
	bindPaths := make(map[string]struct{})
	for _, spec := range container.hostConfig.Binds {
		mnt, err := parseBindMountSpec(spec, container.MountLabel)
		if err != nil {
			return err
		}

		// #10618
		if _, exists := bindPaths[mnt.containerPath]; exists {
			return fmt.Errorf("Duplicate volume mount %s", mnt.containerPath)
		}

		bindPaths[mnt.containerPath] = struct{}{}
		mounts[mnt.containerPath] = mnt
	}

	// Get volumes from
	for _, from := range container.hostConfig.VolumesFrom {
		cID, mode, err := parseVolumesFromSpec(from)
		if err != nil {
			return err
		}
		if _, exists := container.AppliedVolumesFrom[cID]; exists {
			// skip since it's already been applied
			continue
		}

		c, err := container.daemon.Get(cID)
		if err != nil {
			return fmt.Errorf("container %s not found, impossible to mount its volumes", cID)
		}

		for _, mnt := range c.volumeMounts() {
			mnt.writable = mnt.writable && (mode == "rw")
			mnt.from = cID
			mounts[mnt.containerPath] = mnt
		}
	}

	for _, mnt := range mounts {
		containerMntPath, err := symlink.FollowSymlinkInScope(filepath.Join(container.basefs, mnt.containerPath), container.basefs)
		if err != nil {
			return err
		}

		runtime.LockOSThread()
		defer resetLabeling()

		if err := label.SetFileCreateLabel(container.MountLabel); err != nil {
			return fmt.Errorf("Unable to setup default labeling for volume creation %s: %v", mnt.hostPath, err)
		}

		// Create the actual volume
		v, err := container.daemon.volumes.FindOrCreateVolume(mnt.hostPath, mnt.writable)
		if err != nil {
			return err
		}

		if err := resetLabeling(); err != nil {
			return err
		}

		container.VolumesRW[mnt.containerPath] = mnt.writable
		container.Volumes[mnt.containerPath] = v.Path
		v.AddContainer(container.ID)
		if mnt.from != "" {
			container.AppliedVolumesFrom[mnt.from] = struct{}{}
		}

		if mnt.writable && mnt.copyData {
			// Copy whatever is in the container at the containerPath to the volume
			copyExistingContents(containerMntPath, v.Path)
		}
	}

	return nil
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

func (container *Container) VolumePaths() map[string]struct{} {
	var paths = make(map[string]struct{})
	for _, path := range container.Volumes {
		paths[path] = struct{}{}
	}
	return paths
}

func (container *Container) registerVolumes() {
	for path := range container.VolumePaths() {
		if v := container.daemon.volumes.Get(path); v != nil {
			v.AddContainer(container.ID)
			continue
		}

		// if container was created with an old daemon, this volume may not be registered so we need to make sure it gets registered
		writable := true
		if rw, exists := container.VolumesRW[path]; exists {
			writable = rw
		}
		runtime.LockOSThread()
		defer resetLabeling()

		if err := label.SetFileCreateLabel(container.MountLabel); err != nil {
			logrus.Debugf("Unable to setup default labeling for volume creation %s: %v", path, err)
			continue

		}

		// Create the actual volume
		v, err := container.daemon.volumes.FindOrCreateVolume(path, writable)
		if err != nil {
			logrus.Debugf("error registering volume %s: %v", path, err)
			continue
		}
		if err := resetLabeling(); err != nil {
			logrus.Debugf("Unable to reset labeling %s: %v", path, err)
		}

		v.AddContainer(container.ID)
	}
}

func (container *Container) derefVolumes() {
	for path := range container.VolumePaths() {
		vol := container.daemon.volumes.Get(path)
		if vol == nil {
			logrus.Debugf("Volume %s was not found and could not be dereferenced", path)
			continue
		}
		vol.RemoveContainer(container.ID)
	}
}
func resetLabeling() error {
	err := label.SetFileCreateLabel("")
	runtime.UnlockOSThread()
	return err
}

func parseBindMountSpec(spec string, mountLabel string) (*volumeMount, error) {
	arr := strings.Split(spec, ":")

	mnt := &volumeMount{}
	switch len(arr) {
	case 2:
		mnt.hostPath = arr[0]
		mnt.containerPath = arr[1]
		mnt.writable = true
	case 3:
		mnt.hostPath = arr[0]
		mnt.containerPath = arr[1]
		mode := arr[2]
		if !validMountMode(mode) {
			return nil, fmt.Errorf("Invalid volume specification: %s", spec)
		}
		mnt.writable = rwModes[mode]
		if strings.ContainsAny(mode, "zZ") {
			if err := label.Relabel(mnt.hostPath, mountLabel, mode); err != nil {
				return nil, err
			}
		}

	default:
		return nil, fmt.Errorf("Invalid volume specification: %s", spec)
	}

	if !filepath.IsAbs(mnt.hostPath) {
		return nil, fmt.Errorf("cannot bind mount volume: %s volume paths must be absolute.", mnt.hostPath)
	}

	mnt.hostPath = filepath.Clean(mnt.hostPath)
	mnt.containerPath = filepath.Clean(mnt.containerPath)
	return mnt, nil
}

func parseVolumesFromSpec(spec string) (string, string, error) {
	specParts := strings.SplitN(spec, ":", 2)
	if len(specParts) == 0 {
		return "", "", fmt.Errorf("malformed volumes-from specification: %s", spec)
	}

	var (
		id   = specParts[0]
		mode = "rw"
	)
	if len(specParts) == 2 {
		mode = specParts[1]
		if !validMountMode(mode) {
			return "", "", fmt.Errorf("invalid mode for volumes-from: %s", mode)
		}
	}
	return id, mode, nil
}

var rwModes = map[string]bool{
	"rw":   true,
	"rw,Z": true,
	"rw,z": true,
	"z,rw": true,
	"Z,rw": true,
	"Z":    true,
	"z":    true,
}
var roModes = map[string]bool{
	"ro":   true,
	"ro,Z": true,
	"ro,z": true,
	"z,ro": true,
	"Z,ro": true,
}

func validMountMode(mode string) bool {
	return roModes[mode] || rwModes[mode]
}

func (container *Container) specialMounts() []execdriver.Mount {
	var mounts []execdriver.Mount
	if container.ResolvConfPath != "" {
		label.SetFileLabel(container.ResolvConfPath, container.MountLabel)
		mounts = append(mounts, execdriver.Mount{Source: container.ResolvConfPath, Destination: "/etc/resolv.conf", Writable: true, Private: true})
	}
	if container.HostnamePath != "" {
		label.SetFileLabel(container.HostnamePath, container.MountLabel)
		mounts = append(mounts, execdriver.Mount{Source: container.HostnamePath, Destination: "/etc/hostname", Writable: true, Private: true})
	}
	if container.HostsPath != "" {
		label.SetFileLabel(container.HostsPath, container.MountLabel)
		mounts = append(mounts, execdriver.Mount{Source: container.HostsPath, Destination: "/etc/hosts", Writable: true, Private: true})
	}
	return mounts
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

func (container *Container) setupMounts() error {
	mounts := []execdriver.Mount{}

	if container.hostConfig.MountRun && container.Volumes["/run"] == "" {
		mounts = append(mounts, execdriver.Mount{Source: "tmpfs", Destination: "/run", Writable: true, Private: true})
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
			Writable:    container.VolumesRW[path],
		})
	}

	mounts = append(mounts, container.specialMounts()...)

	if container.Volumes["/var"] == "" &&
		container.Volumes["/var/log"] == "" &&
		container.Volumes["/var/log/journal"] == "" {
		if journalPath, err := container.setupJournal(); err != nil {
			return err
		} else {
			if journalPath != "" {
				label.Relabel(journalPath, container.MountLabel, "Z")
				mounts = append(mounts, execdriver.Mount{Source: journalPath, Destination: journalPath, Writable: true, Private: true})
			}
		}
	}

	container.command.Mounts = mounts
	return nil
}

func (container *Container) volumeMounts() map[string]*volumeMount {
	mounts := make(map[string]*volumeMount)

	for containerPath, path := range container.Volumes {
		v := container.daemon.volumes.Get(path)
		if v == nil {
			// This should never happen
			logrus.Debugf("reference by container %s to non-existent volume path %s", container.ID, path)
			continue
		}
		mounts[containerPath] = &volumeMount{hostPath: path, containerPath: containerPath, writable: container.VolumesRW[containerPath]}
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

func (container *Container) mountVolumes() error {
	for dest, source := range container.Volumes {
		v := container.daemon.volumes.Get(source)
		if v == nil {
			return fmt.Errorf("could not find volume for %s:%s, impossible to mount", source, dest)
		}

		destPath, err := container.getResourcePath(dest)
		if err != nil {
			return err
		}

		if err := mount.Mount(source, destPath, "bind", "rbind,rw"); err != nil {
			return fmt.Errorf("error while mounting volume %s: %v", source, err)
		}
	}

	for _, mnt := range container.specialMounts() {
		destPath, err := container.getResourcePath(mnt.Destination)
		if err != nil {
			return err
		}
		if err := mount.Mount(mnt.Source, destPath, "bind", "bind,rw"); err != nil {
			return fmt.Errorf("error while mounting volume %s: %v", mnt.Source, err)
		}
	}
	return nil
}

func (container *Container) unmountVolumes() {
	for dest := range container.Volumes {
		destPath, err := container.getResourcePath(dest)
		if err != nil {
			logrus.Errorf("error while unmounting volumes %s: %v", destPath, err)
			continue
		}
		if err := mount.ForceUnmount(destPath); err != nil {
			logrus.Errorf("error while unmounting volumes %s: %v", destPath, err)
			continue
		}
	}

	for _, mnt := range container.specialMounts() {
		destPath, err := container.getResourcePath(mnt.Destination)
		if err != nil {
			logrus.Errorf("error while unmounting volumes %s: %v", destPath, err)
			continue
		}
		if err := mount.ForceUnmount(destPath); err != nil {
			logrus.Errorf("error while unmounting volumes %s: %v", destPath, err)
		}
	}
}
