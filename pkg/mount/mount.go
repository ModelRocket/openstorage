// +build linux

package mount

import (
	"errors"
	"fmt"
	"sync"
	"syscall"

	"go.pedge.io/dlog"
)

// Mangager defines the interface for keep track of volume driver mounts.
type Manager interface {
	// String representation of the mount table
	String() string
	// Load mount table for all devices that match this identifier
	Load(source string) error
	// Inspect mount table for specified source. ErrEnoent may be returned.
	Inspect(source string) []*PathInfo
	// Mounts returns paths for specified source.
	Mounts(source string) []string
	// HasMounts determines returns the number of mounts for the source.
	HasMounts(source string) int
	// Exists returns true if the device is mounted at specified path.
	// returned if the device does not exists.
	Exists(source, path string) (bool, error)
	// GetDevPath scans mount for a specified mountPath and returns the devPath
	// if found or returnes an ErrEnoent
	GetDevPath(mountPath string) (string, error)
	// Mount device at mountpoint or increment refcnt if device is already mounted
	// at specified mountpoint.
	Mount(minor int, device, path, fs string, flags uintptr, data string) error
	// Unmount device at mountpoint or decrement refcnt. If device has no
	// mountpoints left after this operation, it is removed from the matrix.
	// ErrEnoent is returned if the device or mountpoint for the device is not found.
	Unmount(source, path string) error
}

// MountImpl backend implementation for Mount/Unmount calls
type MountImpl interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
}

type MountType int

const (
	DeviceMount MountType = 1 << iota
	NFSMount
)

var (
	// ErrExist is returned if path is already mounted to a different device.
	ErrExist = errors.New("Mountpath already exists")
	// ErrEnoent is returned for a non existent mount point
	ErrEnoent = errors.New("Mountpath is not mounted")
	// ErrEinval is returned is fields for an entry do no match
	// existing fields
	ErrEinval = errors.New("Invalid arguments for mount entry")
	// ErrUnsupported is returned for an operation or a mount type not suppored.
	ErrUnsupported = errors.New("Not supported")
)

// DeviceMap map device name to Info
type DeviceMap map[string]*Info

// PathMap map path name to device
type PathMap map[string]string

// PathInfo is a reference counted path
type PathInfo struct {
	Path string
	ref  int
}

// Info per device
type Info struct {
	sync.Mutex
	Device     string
	Minor      int
	Mountpoint []*PathInfo
	Fs         string
}

// Mounter implements Ops and keeps track of active mounts for volume drivers.
type Mounter struct {
	sync.Mutex
	mountImpl MountImpl
	mounts    DeviceMap
	paths     PathMap
}

// DefaultMounter defaults to syscall implementation.
type DefaultMounter struct {
}

// Mount default mount implementation is syscall.
func (m *DefaultMounter) Mount(
	source string,
	target string,
	fstype string,
	flags uintptr,
	data string,
) error {
	return syscall.Mount(source, target, fstype, flags, data)
}

// Unmount default unmount implementation is syscall.
func (m *DefaultMounter) Unmount(target string, flags int) error {
	return syscall.Unmount(target, flags)
}

// String representation of Mounter
func (m *Mounter) String() string {
	return fmt.Sprintf("%#v", *m)
}

// Inspect mount table for device
func (m *Mounter) Inspect(devPath string) []*PathInfo {
	m.Lock()
	defer m.Unlock()

	v, ok := m.mounts[devPath]
	if !ok {
		return nil
	}
	return v.Mountpoint
}

// Inspect mount table for device
func (m *Mounter) Mounts(devPath string) []string {
	m.Lock()
	defer m.Unlock()

	v, ok := m.mounts[devPath]
	if !ok {
		return nil
	}

	mounts := make([]string, len(v.Mountpoint))
	for i, v := range v.Mountpoint {
		mounts[i] = v.Path
	}

	return mounts
}

// HasMounts determines returns the number of mounts for the device.
func (m *Mounter) HasMounts(devPath string) int {
	m.Lock()
	defer m.Unlock()

	v, ok := m.mounts[devPath]
	if !ok {
		return 0
	}
	return len(v.Mountpoint)
}

// Exists scans mountpaths for specified device and returns true if path is one of the
// mountpaths. ErrEnoent may be retuned if the device is not found
func (m *Mounter) Exists(devPath string, path string) (bool, error) {
	m.Lock()
	defer m.Unlock()

	v, ok := m.mounts[devPath]
	if !ok {
		return false, ErrEnoent
	}
	for _, p := range v.Mountpoint {
		if p.Path == path {
			return true, nil
		}
	}
	return false, nil
}

// GetDevPath scans mount for a specified mountPath and returns the devPath
// if found or returnes an ErrEnoent
func (m *Mounter) GetDevPath(mountPath string) (string, error) {
	m.Lock()
	defer m.Unlock()

	for k, v := range m.mounts {
		for _, p := range v.Mountpoint {
			if p.Path == mountPath {
				return k, nil
			}
		}
	}
	return "", ErrEnoent
}

// Mount new mountpoint for specified device.
func (m *Mounter) Mount(minor int, device, path, fs string, flags uintptr, data string) error {
	m.Lock()

	dev, ok := m.paths[path]
	if ok && dev != device {
		dlog.Warnf("cannot mount %q,  device %q is mounted at %q", device, dev, path)
		m.Unlock()
		return ErrExist
	}
	info, ok := m.mounts[device]
	if !ok {
		info = &Info{
			Device:     device,
			Mountpoint: make([]*PathInfo, 0),
			Minor:      minor,
			Fs:         fs,
		}
	}
	m.mounts[device] = info
	m.Unlock()
	info.Lock()
	defer info.Unlock()

	// Validate input params
	if fs != info.Fs {
		dlog.Warnf("%s Existing mountpoint has fs %q cannot change to %q",
			device, info.Fs, fs)
		return ErrEinval
	}

	// Try to find the mountpoint. If it already exists, then increment refcnt
	for _, p := range info.Mountpoint {
		if p.Path == path {
			p.ref++
			return nil
		}
	}
	// The device is not mounted at path, mount it and add to its mountpoints.
	err := m.mountImpl.Mount(device, path, fs, flags, data)
	if err != nil {
		return err
	}
	info.Mountpoint = append(info.Mountpoint, &PathInfo{Path: path, ref: 1})
	m.paths[path] = device
	return nil
}

func (m *Mounter) maybeRemoveDevice(device string) {
	m.Lock()
	defer m.Unlock()
	if info, ok := m.mounts[device]; ok {
		// If the device has no more mountpoints, remove it from the map
		if len(info.Mountpoint) == 0 {
			delete(m.mounts, device)
		}
	}
}

// Unmount device at mountpoint or decrement refcnt. If device has no
// mountpoints left after this operation, it is removed from the matrix.
// ErrEnoent is returned if the device or mountpoint for the device is not found.
func (m *Mounter) Unmount(device, path string) error {
	m.Lock()

	info, ok := m.mounts[device]
	if !ok {
		m.Unlock()
		return ErrEnoent
	}
	m.Unlock()
	info.Lock()
	defer info.Unlock()
	for i, p := range info.Mountpoint {
		if p.Path == path {
			p.ref--
			// Unmount only if refcnt is 0
			if p.ref == 0 {
				err := m.mountImpl.Unmount(path, 0)
				if err != nil {
					p.ref++
					return err
				}
				if _, pathExists := m.paths[path]; pathExists {
					delete(m.paths, path)
				} else {
					dlog.Warnf("Path %q for device %q does not exist in pathMap", path, device)
				}
				// Blow away this mountpoint.
				info.Mountpoint[i] = info.Mountpoint[len(info.Mountpoint)-1]
				info.Mountpoint = info.Mountpoint[0 : len(info.Mountpoint)-1]
				m.maybeRemoveDevice(device)
			}
			return nil
		}
	}
	return ErrEnoent
}

func New(mounterType MountType, mountImpl MountImpl, identifier string) (Manager, error) {

	if mountImpl == nil {
		mountImpl = &DefaultMounter{}
	}

	switch mounterType {
	case DeviceMount:
		return NewDeviceMounter(identifier, mountImpl)
	case NFSMount:
		return NewNFSMounter(identifier, mountImpl)
	}
	return nil, ErrUnsupported
}
