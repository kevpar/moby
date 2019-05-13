//+build windows

package cimfs //import "github.com/docker/docker/daemon/graphdriver/windows/cimfs"

import (
	"archive/tar"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

func InitDriver(root string, options []string, uidMaps []idtools.IDMap, gidMaps []idtools.IDMap) (graphdriver.Driver, error) {
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

type mountInfo struct {
	Path     string
	RefCount uint
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

	vhd, err := vhd.OpenVirtualDisk(path, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		return errors.Wrap(err, "failed to open VHD")
	}

	if err := hcsFormatWritableLayerVhd(uintptr(vhd)); err != nil {
		return errors.Wrap(err, "failed to format VHD")
	}
	logrus.WithField("path", path).Info("Successfully formatted VHD")

	if err := windows.CloseHandle(windows.Handle(vhd)); err != nil {
		return errors.Wrap(err, "failed to close VHD")
	}

	return nil
}

func (d *Driver) CreateReadWrite(id string, parent string, opts *graphdriver.CreateOpts) (err error) {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.CreateReadWrite")
	defer func() {
		logrus.WithField("error", err).Info("cimfs.Driver.CreateReadWrite end")
	}()

	if err := idtools.MkdirAllAndChown(d.layerDirPath(id), 0700, idtools.Identity{}); err != nil {
		return errors.Wrap(err, "failed to create layer dir path")
	}
	if err := createVhdx(d.layerScratchPath(id), 20); err != nil {
		return errors.Wrap(err, "failed to create VHD")
	}
	// TODO: resize scratch vhd

	return d.updateLayerChain(id, parent)
}

func (d *Driver) Create(id string, parent string, opts *graphdriver.CreateOpts) error {
	logrus.WithFields(logrus.Fields{
		"id":     id,
		"parent": parent,
	}).Info("cimfs.Driver.Create")

	if err := idtools.MkdirAllAndChown(d.layerDirPath(id), 0700, idtools.Identity{}); err != nil {
		return errors.Wrap(err, "failed to create layer dir path")
	}

	return d.updateLayerChain(id, parent)
}

func (d *Driver) Remove(id string) error {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.Remove")
	return nil
}

func (d *Driver) Get(id string, mountLabel string) (fs containerfs.ContainerFS, err error) {
	logrus.WithFields(logrus.Fields{
		"id":         id,
		"mountLabel": mountLabel,
	}).Info("cimfs.Driver.Get")
	defer func() {
		logrus.WithField("error", err).Info("cimfs.Driver.Get end")
	}()

	if mountPath, mounted, err := d.getCacheMount(id); mounted {
		return containerfs.NewLocalContainerFS(mountPath), nil
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to check first layer mount cache")
	}

	layerChain, err := d.getLayerChain(id)
	if err != nil {
		return nil, err
	}

	layersToMount := []string{}
	finalLayers := []string{}

	if _, err := os.Stat(d.layerScratchPath(id)); err == nil {
		// vhd exists, pass as=is to activatelayer
		logrus.WithField("path", d.layerScratchPath(id)).Info("ActivateLayer")
		if err := hcsshim.ActivateLayer(hcsshim.DriverInfo{}, d.layerDirPath(id)); err != nil {
			return nil, errors.Wrap(err, "failed to ActivateLayer")
		}
		finalLayers = append(finalLayers, d.layerDirPath(id))
	} else if os.IsNotExist(err) {
		// no vhd, need to mount this layer as well
		layersToMount = append(layersToMount, id)
	} else {
		return nil, errors.Wrap(err, "failed to stat scratch VHD")
	}

	layersToMount = append(layersToMount, layerChain...)

	for _, layer := range layersToMount {
		if mountPath, mounted, err := d.getCacheMount(layer); mounted {
			finalLayers = append(finalLayers, mountPath)
			continue
		} else if err != nil {
			return nil, errors.Wrap(err, "failed to check first layer mount cache")
		}

		g, err := hcsshim.NameToGuid(layer)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert name to GUID")
		}

		g2 := guid.GUID{}
		g2.Data1 = binary.LittleEndian.Uint32(g[0:4])
		g2.Data2 = binary.LittleEndian.Uint16(g[4:6])
		g2.Data3 = binary.LittleEndian.Uint16(g[6:8])
		copy(g2.Data4[:], g[8:])

		logrus.WithFields(logrus.Fields{
			"layer": layer,
			"guid":  g2.String(),
		}).Info("Mounting CimFS layer")

		if err := cimfs.MountImage(d.layerCimPath(layer), &g2); err != nil {
			return nil, errors.Wrap(err, "failed to mount CimFS")
		}

		mountPath := fmt.Sprintf(`\\?\Volume{%s}\`, g2.String())

		if err := d.setCacheMount(layer, mountPath); err != nil {
			return nil, errors.Wrap(err, "failed to cache CimFS mount")
		}

		finalLayers = append(finalLayers, mountPath)
	}

	logrus.WithField("finalLayers", finalLayers).Info("Final layers for PrepareLayer")

	mountPath, err := hcsshim.GetLayerMountPath(hcsshim.DriverInfo{}, finalLayers[0])
	if err != nil {
		return nil, errors.Wrap(err, "failed to get first layer mount path")
	}

	logrus.WithField("mountPath", mountPath).Info("Retrieved mount path")

	if err := hcsshim.PrepareLayer(hcsshim.DriverInfo{}, finalLayers[0], finalLayers[1:]); err != nil {
		return nil, errors.Wrap(err, "failed to PrepareLayer")
	}

	if err := d.setCacheMount(id, mountPath); err != nil {
		return nil, errors.Wrap(err, "failed to cache first layer mount")
	}

	return containerfs.NewLocalContainerFS(mountPath), nil
}

func (d *Driver) Put(id string) error {
	logrus.WithFields(logrus.Fields{
		"id": id,
	}).Info("cimfs.Driver.Put")

	if shouldUnmount, err := d.releaseCacheMount(id); !shouldUnmount {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to release cached mount")
	}

	_, err := d.getLayerChain(id)
	if err != nil {
		return errors.Wrap(err, "failed to get layer chain")
	}

	// if err := hcsshim.UnprepareLayer()

	// g, err := hcsshim.NameToGuid(id)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to convert name to GUID")
	// }

	// g2 := guid.GUID{}
	// g2.Data1 = binary.LittleEndian.Uint32(g[0:4])
	// g2.Data2 = binary.LittleEndian.Uint16(g[4:6])
	// g2.Data3 = binary.LittleEndian.Uint16(g[6:8])
	// copy(g2.Data4[:], g[8:])

	// logrus.WithFields(logrus.Fields{
	// 	"layer": id,
	// 	"guid":  g2.String(),
	// }).Info("Unmounting CimFS layer")

	// if err := cimfs.UnmountImage(&g2); err != nil {
	// 	return errors.Wrap(err, "failed to unmount CimFS")
	// }

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

	m := make(map[string]string)
	m["dir"] = d.layerDirPath(id)

	return m, nil
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
