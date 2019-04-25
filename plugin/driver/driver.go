package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const (
	PluginName  = "gcsfs"
	BucketMount = "/mnt/buckets"
	KeyMount    = "/mnt/keys"
	KeyStorage  = "/tmp/keys"
)

type GCSVolume struct {
	bucket  string
	count   int
	flags   []string
	keyFile string
	mounted bool
	path    string
}

type GCSDriver struct {
	m       *sync.Mutex
	volumes map[string]*GCSVolume
}

func NewGCSDriver() GCSDriver {
	return GCSDriver{
		m:       &sync.Mutex{},
		volumes: make(map[string]*GCSVolume),
	}
}

func (d GCSDriver) Create(r *volume.CreateRequest) error {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Creating volume %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		v.count++
		return nil
	}

	// If a bucket was not set, default to the volume name for a better experience.
	bucket, bucketSelected := r.Options["bucket"]
	if !bucketSelected {
		log.Warningf("A bucket was not selected, defaulting to %s", r.Name)
		bucket = r.Name
	}

	// If a key was set but does not exist, assume it is the raw JSON contents.
	// If a key was not set, default to <volume name>.json in the key mount location.
	keyFile, keyFileSelected := r.Options["key"]
	if keyFileSelected {
		keyPath := filepath.Join(KeyMount, keyFile)
		if _, err := os.Lstat(keyPath); err == nil {
			keyFile = keyPath
		} else {
			log.Infof("Error accessing the key file for bucket %s, will assume it is the key contents", bucket)
			keyFile, err = createKeyFile(keyFile)
			if err != nil {
				return err
			}
		}
	} else {
		keyName := fmt.Sprintf("%s.json", r.Name)
		log.Warningf("A key file was not selected, defaulting to %s/%s", KeyMount, keyName)
		keyFile = filepath.Join(KeyMount, keyName)
	}

	allFlags := []string{fmt.Sprintf("--key-file=%s", keyFile), "-o=allow_other"}
	if flags, ok := r.Options["flags"]; ok {
		parsedFlags := strings.Fields(flags)
		if parsedFlags != nil {
			allFlags = append(allFlags, parsedFlags...)
		}
	}

	volumePath := filepath.Join(BucketMount, r.Name)
	if err := createDir(volumePath); err != nil {
		return err
	}

	v := &GCSVolume{
		bucket:  bucket,
		count:   1,
		flags:   allFlags,
		keyFile: keyFile,
		mounted: false,
		path:    volumePath,
	}
	d.volumes[r.Name] = v

	if debug, ok := r.Options["debug"]; ok {
		timeout, err := strconv.Atoi(debug)
		if err != nil {
			return fmt.Errorf("debug timeout (%s) is not an integer", debug)
		}

		es := debugVolume(v, timeout)
		return fmt.Errorf("debug:\n%s", es.String())
	}

	return nil
}

func (d GCSDriver) List() (*volume.ListResponse, error) {
	d.m.Lock()
	defer d.m.Unlock()

	log.Info("Listing volumes")

	var volumes []*volume.Volume

	for name, v := range d.volumes {
		volumes = append(volumes, &volume.Volume{
			Name:       name,
			Mountpoint: v.path,
		})
	}

	return &volume.ListResponse{Volumes: volumes}, nil
}

func (d GCSDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Getting volume %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		return &volume.GetResponse{
			Volume: &volume.Volume{
				Name:       r.Name,
				Mountpoint: v.path,
			},
		}, nil
	}

	return nil, fmt.Errorf("unknown volume %s", r.Name)
}

func (d GCSDriver) Remove(r *volume.RemoveRequest) error {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Removing volume %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		v.count--

		if v.count == 0 {
			keyFile := v.keyFile
			delete(d.volumes, r.Name)

			location := filepath.Dir(v.keyFile)
			if location == KeyStorage {
				err := os.Remove(keyFile)
				if err != nil {
					log.Warningf("Error removing temporary key file %s: %s", keyFile, err)
				}
			}
		} else {
			log.Infof("Will not remove volume %s as it is still in use by %d container(s)", r.Name, v.count)
		}
	}

	return nil
}

func (d GCSDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Getting the path for volume %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		return &volume.PathResponse{Mountpoint: v.path}, nil
	}

	return nil, fmt.Errorf("unknown volume %s", r.Name)
}

func (d GCSDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Mounting volume %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		if !v.mounted {
			cmd := exec.Command("gcsfuse", v.flags...)
			cmd.Args = append(cmd.Args, v.bucket, v.path)

			out, err := cmd.CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("error (%s) mounting bucket %s:\n%s", err, v.bucket, out)
			}

			v.mounted = true
		}
		return &volume.MountResponse{Mountpoint: v.path}, nil
	}

	return nil, fmt.Errorf("unknown volume %s", r.Name)
}

func (d GCSDriver) Unmount(r *volume.UnmountRequest) error {
	d.m.Lock()
	defer d.m.Unlock()

	log.Infof("Un-mounting %s", r.Name)

	if v, ok := d.volumes[r.Name]; ok {
		if v.count > 1 {
			log.Infof("Will not remove volume %s as it is still in use by %d containers", r.Name, v.count)
		} else {
			cmd := exec.Command("fusermount", "-u", v.path)

			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("error (%s) un-mounting bucket %s:\n%s", err, v.bucket, out)
			}

			v.mounted = false
		}
	}

	return fmt.Errorf("unknown volume %s", r.Name)
}

func (d GCSDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "global",
		},
	}
}

func debugVolume(v *GCSVolume, timeout int) (sb strings.Builder) {
	var b bytes.Buffer

	cmd := exec.Command("gcsfuse", v.flags...)
	cmd.Args = append(cmd.Args, "--foreground", "--debug_fuse", "--debug_gcs", v.bucket, v.path)
	cmd.Stdout = &b
	cmd.Stderr = &b

	sb.WriteString(fmt.Sprintf("Command: %s\nKey file: %s\n", cmd.Args, v.keyFile))

	err := cmd.Start()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error (%s) mounting bucket\n", err))
		return sb
	}

	defer func(c exec.Cmd) {
		_ = c.Process.Kill()

		ucmd := exec.Command("fusermount", "-u", v.path)
		if uout, err := ucmd.CombinedOutput(); err != nil {
			sb.WriteString(fmt.Sprintf("Error (%s) un-mounting bucket: %s\n", err, uout))
		}

		sb.WriteString(fmt.Sprintf("Mount output:\n%s", b.String()))
	}(*cmd)

	time.Sleep(time.Duration(timeout) * time.Second)
	stat, err := os.Lstat(v.path)
	if err != nil {
		sb.WriteString(fmt.Sprintf("os.Lstat: %s\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("os.Lstat: success\nis dir: %v\n", stat.IsDir()))
	}

	paths, err := ioutil.ReadDir(v.path)
	if err != nil {
		sb.WriteString(fmt.Sprintf("ioutil.ReadDir: %s\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("ioutil.ReadDir entries: %v\n", len(paths)))
	}

	return sb
}

func createKeyFile(s string) (string, error) {
	tmpfile, err := ioutil.TempFile(KeyStorage, "")
	if err != nil {
		return "", fmt.Errorf("error creating temporary key file: %s", err)
	}

	keyFile := tmpfile.Name()
	content := []byte(s)

	if _, err := tmpfile.Write(content); err != nil {
		return "", fmt.Errorf("error writing to temporary key file %s: %s", keyFile, err)
	}

	if err := tmpfile.Close(); err != nil {
		return "", fmt.Errorf("error closing temporary key file %s: %s", keyFile, err)
	}

	return keyFile, nil
}

func createDir(d string) error {
	stat, err := os.Lstat(d)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(d, os.ModePerm); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if stat != nil && !stat.IsDir() {
		return fmt.Errorf("%s already exists and is not a directory", d)
	}

	return nil
}
