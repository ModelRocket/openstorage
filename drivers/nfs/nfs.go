package nfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pborman/uuid"

	"github.com/portworx/kvdb"

	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/config"
	"github.com/libopenstorage/openstorage/pkg/mount"
	"github.com/libopenstorage/openstorage/pkg/seed"
	"github.com/libopenstorage/openstorage/proto/openstorage"
	"github.com/libopenstorage/openstorage/volume"
)

const (
	Name         = "nfs"
	Type         = volume.File
	NfsDBKey     = "OpenStorageNFSKey"
	nfsMountPath = "/var/lib/openstorage/nfs/"
	nfsBlockFile = ".blockdevice"
)

// Implements the open storage volume interface.
type driver struct {
	*volume.DefaultEnumerator
	nfsServer string
	nfsPath   string
	mounter   mount.Manager
}

func copyFile(source string, dest string) (err error) {
	sourcefile, err := os.Open(source)
	if err != nil {
		return err
	}

	defer sourcefile.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}

	defer destfile.Close()

	_, err = io.Copy(destfile, sourcefile)
	if err == nil {
		sourceinfo, err := os.Stat(source)
		if err != nil {
			err = os.Chmod(dest, sourceinfo.Mode())
		}

	}

	return
}

func copyDir(source string, dest string) (err error) {
	// get properties of source dir
	sourceinfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	// create dest dir

	err = os.MkdirAll(dest, sourceinfo.Mode())
	if err != nil {
		return err
	}

	directory, _ := os.Open(source)

	objects, err := directory.Readdir(-1)

	for _, obj := range objects {

		sourcefilepointer := source + "/" + obj.Name()

		destinationfilepointer := dest + "/" + obj.Name()

		if obj.IsDir() {
			// create sub-directories - recursively
			err = copyDir(sourcefilepointer, destinationfilepointer)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			// perform copy
			err = copyFile(sourcefilepointer, destinationfilepointer)
			if err != nil {
				fmt.Println(err)
			}
		}

	}
	return
}

func Init(params volume.DriverParams) (volume.VolumeDriver, error) {
	path, ok := params["path"]
	if !ok {
		return nil, errors.New("No NFS path provided")
	}

	server, ok := params["server"]
	if !ok {
		log.Printf("No NFS server provided, will attempt to bind mount %s", path)
	} else {
		log.Printf("NFS driver initializing with %s:%s ", server, path)
	}

	// Create a mount manager for this NFS server. Blank sever is OK.
	mounter, err := mount.New(mount.NFSMount, server)
	if err != nil {
		log.Warnf("Failed to create mount manager for server: %v (%v)", server, err)
		return nil, err
	}

	inst := &driver{
		DefaultEnumerator: volume.NewDefaultEnumerator(Name, kvdb.Instance()),
		nfsServer:         server,
		nfsPath:           path,
		mounter:           mounter,
	}

	err = os.MkdirAll(nfsMountPath, 0744)
	if err != nil {
		return nil, err
	}
	src := inst.nfsPath
	if server != "" {
		src = ":" + inst.nfsPath
	}

	// If src is already mounted at dest, leave it be.
	mountExists, err := mounter.Exists(src, nfsMountPath)
	if !mountExists {
		// Mount the nfs server locally on a unique path.
		syscall.Unmount(nfsMountPath, 0)
		if server != "" {
			err = syscall.Mount(src, nfsMountPath, openstorage.FSType_FS_TYPE_NFS.SimpleString(), 0, "nolock,addr="+inst.nfsServer)
		} else {
			err = syscall.Mount(src, nfsMountPath, "", syscall.MS_BIND, "")
		}
		if err != nil {
			log.Printf("Unable to mount %s:%s at %s (%+v)", inst.nfsServer, inst.nfsPath, nfsMountPath, err)
			return nil, err
		}
	}

	volumeInfo, err := inst.DefaultEnumerator.Enumerate(
		&openstorage.VolumeLocator{},
		nil)
	if err == nil {
		for _, info := range volumeInfo {
			if info.Status == "" {
				info.Status = api.Up
				inst.UpdateVol(&info)
			}
		}
	} else {
		log.Println("Could not enumerate Volumes, ", err)
	}

	log.Println("NFS initialized and driver mounted at: ", nfsMountPath)
	return inst, nil
}

func (d *driver) String() string {
	return Name
}

func (d *driver) Type() volume.DriverType {
	return Type
}

// Status diagnostic information
func (d *driver) Status() [][2]string {
	return [][2]string{}
}

func (d *driver) Create(locator *openstorage.VolumeLocator, source *openstorage.VolumeSource, spec *openstorage.VolumeSpec) (string, error) {
	volumeID := uuid.New()
	volumeID = strings.TrimSuffix(volumeID, "\n")

	// Create a directory on the NFS server with this UUID.
	volPath := path.Join(nfsMountPath, volumeID)
	err := os.MkdirAll(volPath, 0744)
	if err != nil {
		log.Println(err)
		return api.BadVolumeID, err
	}
	if source != nil {
		if len(source.SeedUri) != 0 {
			seed, err := seed.New(source.SeedUri, spec.Labels)
			if err != nil {
				log.Warnf("Failed to initailize seed from %q : %v",
					source.SeedUri, err)
				return api.BadVolumeID, err
			}
			err = seed.Load(path.Join(volPath, config.DataDir))
			if err != nil {
				log.Warnf("Failed to  seed from %q to %q: %v",
					source.SeedUri, nfsMountPath, err)
				return api.BadVolumeID, err
			}
		}
	}

	f, err := os.Create(path.Join(nfsMountPath, volumeID+nfsBlockFile))
	if err != nil {
		log.Println(err)
		return api.BadVolumeID, err
	}
	defer f.Close()

	err = f.Truncate(int64(spec.SizeBytes))
	if err != nil {
		log.Println(err)
		return api.BadVolumeID, err
	}

	v := &api.Volume{
		ID:         volumeID,
		Source:     source,
		Locator:    locator,
		Ctime:      time.Now(),
		Spec:       spec,
		LastScan:   time.Now(),
		Format:     openstorage.FSType_FS_TYPE_NFS,
		State:      api.VolumeAvailable,
		Status:     api.Up,
		DevicePath: path.Join(nfsMountPath, volumeID+nfsBlockFile),
	}

	err = d.CreateVol(v)
	if err != nil {
		return api.BadVolumeID, err
	}
	return v.ID, err
}

func (d *driver) Delete(volumeID string) error {
	v, err := d.GetVol(volumeID)
	if err != nil {
		log.Println(err)
		return err
	}

	// Delete the simulated block volume
	os.Remove(v.DevicePath)

	// Delete the directory on the nfs server.
	os.RemoveAll(path.Join(nfsMountPath, volumeID))

	err = d.DeleteVol(volumeID)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func (d *driver) Mount(volumeID string, mountpath string) error {
	v, err := d.GetVol(volumeID)
	if err != nil {
		log.Println(err)
		return err
	}

	srcPath := path.Join(":", d.nfsPath, volumeID)
	mountExists, err := d.mounter.Exists(srcPath, mountpath)
	if !mountExists {
		syscall.Unmount(mountpath, 0)
		// TODO(pedge): fs type simple string could result in "none"
		err = syscall.Mount(path.Join(nfsMountPath, volumeID), mountpath, v.Spec.FsType.SimpleString(), syscall.MS_BIND, "")
		if err != nil {
			log.Printf("Cannot mount %s at %s because %+v",
				path.Join(nfsMountPath, volumeID), mountpath, err)
			return err
		}
	}

	v.AttachPath = mountpath
	err = d.UpdateVol(v)

	return err
}

func (d *driver) Unmount(volumeID string, mountpath string) error {
	v, err := d.GetVol(volumeID)
	if err != nil {
		return err
	}
	if v.AttachPath == "" {
		return fmt.Errorf("Device %v not mounted", volumeID)
	}
	err = syscall.Unmount(v.AttachPath, 0)
	if err != nil {
		return err
	}
	v.AttachPath = ""
	err = d.UpdateVol(v)
	return err
}

func (d *driver) Snapshot(volumeID string, readonly bool, locator *openstorage.VolumeLocator) (string, error) {
	volIDs := make([]string, 1)
	volIDs[0] = volumeID
	vols, err := d.Inspect(volIDs)
	if err != nil {
		return api.BadVolumeID, nil
	}
	source := &openstorage.VolumeSource{ParentVolumeId: volumeID}
	newVolumeID, err := d.Create(locator, source, vols[0].Spec)
	if err != nil {
		return api.BadVolumeID, nil
	}

	// NFS does not support snapshots, so just copy the files.
	err = copyDir(nfsMountPath+volumeID, nfsMountPath+newVolumeID)
	if err != nil {
		d.Delete(newVolumeID)
		return api.BadVolumeID, nil
	}

	return newVolumeID, nil
}

func (d *driver) Attach(volumeID string) (string, error) {
	return path.Join(nfsMountPath, volumeID+nfsBlockFile), nil
}

func (d *driver) Detach(volumeID string) error {
	return nil
}

func (d *driver) Stats(volumeID string) (api.Stats, error) {
	return api.Stats{}, volume.ErrNotSupported
}

func (d *driver) Alerts(volumeID string) (api.Alerts, error) {
	return api.Alerts{}, volume.ErrNotSupported
}

func (d *driver) Shutdown() {
	log.Printf("%s Shutting down", Name)
	syscall.Unmount(nfsMountPath, 0)
}

func init() {
	// Register ourselves as an openstorage volume driver.
	volume.Register(Name, Init)
}
