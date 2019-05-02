//+build windows

package cimfs //import "github.com/docker/docker/daemon/graphdriver/windows/cimfs"

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/containerfs"
	"github.com/docker/docker/pkg/idtools"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func init() {
	graphdriver.Register("cimfs", InitDriver)
}

func InitDriver(root string, options []string, uidMaps, gidMaps []idtools.IDMap) (graphdriver.Driver, error) {
	logrus.Info("cimfs.InitDriver")

	layersRoot := filepath.Join(root, "layers")
	if err := idtools.MkdirAllAndChown(layersRoot, 0700, idtools.Identity{UID: 0, GID: 0}); err != nil {
		return nil, err
	}
	mountsRoot := filepath.Join(root, "mounts")
	if err := idtools.MkdirAllAndChown(mountsRoot, 0700, idtools.Identity{UID: 0, GID: 0}); err != nil {
		return nil, err
	}

	return &Driver{
		layersRoot: layersRoot,
		mountsRoot: mountsRoot,
	}, nil
}

type Driver struct {
	layersRoot string
	mountsRoot string
}

func (d *Driver) String() string {
	return "cimfs"
}

func (d *Driver) layerDirPath(id string) string {
	return filepath.Join(d.layersRoot, id)
}

func (d *Driver) layerCimPath(id string) string {
	return filepath.Join(d.layerDirPath(id), "layer.cimfs")
}

func (d *Driver) layerScratchPath(id string) string {
	return filepath.Join(d.layerDirPath(id), "sandbox.vhdx")
}

func (d *Driver) mountDirPath(id string) string {
	return filepath.Join(d.mountsRoot, id)
}

func (d *Driver) getLayerChain(id string) ([]string, error) {
	path := filepath.Join(d.layerDirPath(id), "layerchain.json")
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read layer chain file")
	}

	var layerChain []string
	if err := json.Unmarshal(content, &layerChain); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal layer chain JSON")
	}

	return layerChain, nil
}

func (d *Driver) setLayerChain(id string, chain []string) error {
	content, err := json.Marshal(&chain)
	if err != nil {
		return errors.Wrap(err, "failed to marshal layer chain as JSON")
	}

	path := filepath.Join(d.layerDirPath(id), "layerchain.json")
	if err := ioutil.WriteFile(path, content, 0600); err != nil {
		return errors.Wrap(err, "failed to write layer chain JSON to file")
	}

	return nil
}

func (d *Driver) updateLayerChain(id string, parent string) error {
	chain := []string{}
	if parent != "" {
		chain = append(chain, parent)

		parentChain, err := d.getLayerChain(parent)
		if err != nil {
			return err
		}
		chain = append(chain, parentChain...)
	}
	return d.setLayerChain(id, chain)
}

func createVhdx(path string, sizeGB uint32) error {
	if err := vhd.CreateVhdx(path, sizeGB, 1); err != nil {
		return errors.Wrap(err, "failed to create VHD")
	}

	vhd, err := vhd.OpenVirtualDisk(path, vhd.VirtualDiskAccessAll, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		return errors.Wrap(err, "failed to open VHD")
	}

	if err := hcsFormatWritableLayerVhd(uintptr(vhd)); err != nil {
		return errors.Wrap(err, "failed to format VHD")
	}

	if err := windows.CloseHandle(windows.Handle(vhd)); err != nil {
		return errors.Wrap(err, "failed to close VHD")
	}

	return nil
}

func (d *Driver) CreateReadWrite(id, parent string, opts *graphdriver.CreateOpts) error {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.CreateReadWrite")

	idtools.MkdirAllAndChown(d.layerDirPath(id), 0700, idtools.Identity{})
	createVhdx(d.layerScratchPath(id), 20)
	// TODO: resize scratch vhd

	return d.updateLayerChain(id, parent)
}

func (d *Driver) Create(id, parent string, opts *graphdriver.CreateOpts) error {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.Create")

	idtools.MkdirAllAndChown(d.layerDirPath(id), 0700, idtools.Identity{})

	return d.updateLayerChain(id, parent)
}

func (d *Driver) Remove(id string) error {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.Remove")
	return nil
}

func (d *Driver) Get(id, mountLabel string) (fs containerfs.ContainerFS, err error) {
	logrus.WithFields(logrus.Fields{
		"id":         id,
		"mountLabel": mountLabel,
	}).Info("cimfs.Driver.Get")

	layerChain, err := d.getLayerChain(id)
	if err != nil {
		return nil, err
	}

	layersToMount := []string{}

	if _, err := os.Stat(d.layerScratchPath(id)); err == nil {
		// vhd exists, pass as=is to activatelayer
	} else if os.IsNotExist(err) {
		// no vhd, need to mount this layer as well
		layersToMount = append(layersToMount, id)
	} else {
		return nil, errors.Wrap(err, "failed to stat scratch VHD")
	}

	layersToMount = append(layersToMount, layerChain...)

	for _, layer := range layersToMount {
		g, err := hcsshim.NameToGuid(layer)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert name to GUID")
		}

		g2 := guid.GUID{}
		g2.Data1 = binary.LittleEndian.Uint32(g[0:4])
		g2.Data2 = binary.LittleEndian.Uint16(g[4:6])
		g2.Data3 = binary.LittleEndian.Uint16(g[6:8])
		copy(g2.Data4[:], g[8:])

		if err := cimfs.MountImage(d.layerCimPath(layer), &g2); err != nil {
			return nil, errors.Wrap(err, "failed to mount CimFS")
		}
	}

	return nil, nil
}

func (d *Driver) Put(id string) error {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.Put")

	return nil
}

func (d *Driver) Exists(id string) bool {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.Exists")

	panic("not implemented")
}

func (d *Driver) Status() [][2]string {
	logrus.Info("cimfs.Driver.Status")

	return nil
}

func (d *Driver) GetMetadata(id string) (map[string]string, error) {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.GetMetadata")

	panic("not implemented")
}

func (d *Driver) Cleanup() error {
	logrus.Info("cimfs.Driver.Cleanup")
	return nil
}

func (d *Driver) Diff(id, parent string) (io.ReadCloser, error) {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.Diff")

	panic("not implemented")
}

func (d *Driver) Changes(id, parent string) ([]archive.Change, error) {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.Changes")

	panic("not implemented")
}

func timeToFiletime(t time.Time) windows.Filetime {
	return windows.NsecToFiletime(t.UnixNano())
}

const maxNanoSecondIntSize = 9

func parsePAXTime(t string) (time.Time, error) {
	buf := []byte(t)
	pos := bytes.IndexByte(buf, '.')
	var seconds, nanoseconds int64
	var err error
	if pos == -1 {
		seconds, err = strconv.ParseInt(t, 10, 0)
		if err != nil {
			return time.Time{}, err
		}
	} else {
		seconds, err = strconv.ParseInt(string(buf[:pos]), 10, 0)
		if err != nil {
			return time.Time{}, err
		}
		nano_buf := string(buf[pos+1:])
		// Pad as needed before converting to a decimal.
		// For example .030 -> .030000000 -> 30000000 nanoseconds
		if len(nano_buf) < maxNanoSecondIntSize {
			// Right pad
			nano_buf += strings.Repeat("0", maxNanoSecondIntSize-len(nano_buf))
		} else if len(nano_buf) > maxNanoSecondIntSize {
			// Right truncate
			nano_buf = nano_buf[:maxNanoSecondIntSize]
		}
		nanoseconds, err = strconv.ParseInt(string(nano_buf), 10, 0)
		if err != nil {
			return time.Time{}, err
		}
	}
	ts := time.Unix(seconds, nanoseconds)
	return ts, nil
}

func (d *Driver) ApplyDiff(id, parent string, diff io.Reader) (size int64, err error) {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.ApplyDiff")

	cimFSPath := d.layerCimPath(id)

	cim, err := cimfs.Open(cimFSPath)
	if err != nil {
		return 0, errors.Wrap(err, "failed to open CimFS")
	}

	tr := tar.NewReader(diff)
	h, err := tr.Next()
	for err == nil {

		base := path.Base(h.Name) // TODO: filepath or path for tar path?
		if strings.HasPrefix(base, archive.WhiteoutPrefix) {
			// name := path.Join(path.Dir(h.Name), base[len(archive.WhiteoutPrefix):])
			// write tombstone
			panic("not implemented")
		} else {
			// if link: err = w.AddLink(filepath.FromSlash(hdr.Name), filepath.FromSlash(hdr.Linkname))

			newPath := filepath.FromSlash(h.Name)
			logrus.Debug("CimFS ApplyDiff: found item: ", newPath)

			creationTime := time.Unix(0, 0)
			if creationTimeStr, ok := h.PAXRecords["LIBARCHIVE.creationtime"]; ok {
				creationTime, err = parsePAXTime(creationTimeStr)
				if err != nil {
					return 0, errors.Wrap(err, "failed to parse creation time")
				}
			}

			attrs := uint64(0)
			if attrsStr, ok := h.PAXRecords["MSWINDOWS.fileattr"]; ok {
				attrs, err = strconv.ParseUint(attrsStr, 10, 32)
				if err != nil {
					return 0, errors.Wrap(err, "failed to parse attributes")
				}
			} else {
				if h.Typeflag == tar.TypeDir {
					attrs |= windows.FILE_ATTRIBUTE_DIRECTORY
				}
			}

			// TODO: need support for SDDL extension?
			sd := []byte{}
			if sdStr, ok := h.PAXRecords["MSWINDOWS.rawsd"]; ok {
				sd, err = base64.StdEncoding.DecodeString(sdStr)
				if err != nil {
					return 0, errors.Wrap(err, "failed to parse security descriptor")
				}
			}

			xattrPrefix := "MSWINDOWS.xattr."
			var eas []winio.ExtendedAttribute
			for k, v := range h.PAXRecords {
				if !strings.HasPrefix(k, xattrPrefix) {
					continue
				}
				data, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					return 0, errors.Wrap(err, "failed to parse extended attribute")
				}
				eas = append(eas, winio.ExtendedAttribute{
					Name:  k[len(xattrPrefix):],
					Value: data,
				})
			}

			info := &cimfs.FileInfo{
				Size:               h.Size,
				CreationTime:       timeToFiletime(creationTime),
				LastWriteTime:      timeToFiletime(h.ModTime),
				ChangeTime:         timeToFiletime(h.ChangeTime),
				LastAccessTime:     timeToFiletime(h.AccessTime),
				Attributes:         uint32(attrs),
				SecurityDescriptor: sd,
				EAs:                eas,
			}

			logrus.WithField("FileInfo", info).Debugf("Adding file to CimFS layer")
			if err := cim.AddFile(newPath, info); err != nil {
				return 0, errors.Wrap(err, "failed to add new CimFS file")
			}

			if _, err := io.Copy(cim, tr); err != nil {
				return 0, errors.Wrap(err, "failed to write to CimFS stream")
			}

			if err := cim.CloseStream(); err != nil {
				return 0, errors.Wrap(err, "failed to close CimFS stream")
			}
		}

		h, err = tr.Next()
	}

	// TODO: need to restore dir file times?

	if err != io.EOF {
		return 0, errors.Wrap(err, "failed iterating tar entries")
	}

	return 0, cim.Close(cimFSPath)
}

func (d *Driver) DiffSize(id, parent string) (size int64, err error) {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.DiffSize")

	panic("not implemented")
}
