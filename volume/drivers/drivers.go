package volumedrivers

import (
	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/volume/drivers/aws"
	"github.com/libopenstorage/openstorage/volume/drivers/btrfs"
	"github.com/libopenstorage/openstorage/volume/drivers/buse"
	"github.com/libopenstorage/openstorage/volume/drivers/coprhd"
	"github.com/libopenstorage/openstorage/volume/drivers/nfs"
	"github.com/libopenstorage/openstorage/volume/drivers/pwx"
	"github.com/libopenstorage/openstorage/volume/drivers/vfs"
)

// Driver is the description of a supported OST driver. New Drivers are added to
// the drivers array
type Driver struct {
	DriverType api.DriverType
	Name       string
}

var (
	// AllDrivers is a slice of all existing known Drivers.
	AllDrivers = []Driver{
		// AWS driver provisions storage from EBS.
		{DriverType: aws.Type, Name: aws.Name},
		// NFS driver provisions storage from an NFS server.
		{DriverType: nfs.Type, Name: nfs.Name},
		// BTRFS driver provisions storage from local btrfs.
		{DriverType: btrfs.Type, Name: btrfs.Name},
		// PWX driver provisions storage from PWX cluster.
		{DriverType: pwx.Type, Name: pwx.Name},
		// VFS driver provisions storage from local filesystem
		{DriverType: vfs.Type, Name: vfs.Name},
		// BUSE driver provisions storage from local volumes and implements block in user space.
		{DriverType: buse.Type, Name: buse.Name},
		// COPRHD driver
		{DriverType: coprhd.Type, Name: coprhd.Name},
	}
)
