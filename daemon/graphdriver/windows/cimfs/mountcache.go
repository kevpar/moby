package cimfs

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func (d *Driver) releaseCacheMount(id string) (bool, error) {
	mountFilePath := filepath.Join(d.layerDirPath(id), "mount.json")

	mount, err := getMountInfo(mountFilePath)
	if err != nil {
		return false, errors.Wrap(err, "failed to get mount info")
	}

	if mount.RefCount == 0 {
		return false, errors.Wrap(err, "tried to release an already freed cached mount")
	}
	mount.RefCount--

	if mount.RefCount > 0 {
		if err := setMountInfo(mountFilePath, mount); err != nil {
			return false, errors.Wrap(err, "failed to set mount info on file")
		}
		return false, nil
	} else {
		if err := os.Remove(mountFilePath); err != nil {
			return false, errors.Wrap(err, "failed to delete mount info file")
		}
		return true, nil
	}
}

func (d *Driver) getCacheMount(id string) (string, bool, error) {
	mountFilePath := filepath.Join(d.layerDirPath(id), "mount.json")

	mount, err := getMountInfo(mountFilePath)
	if err != nil {
		return "", false, errors.Wrap(err, "failed to get mount info from file")
	}

	if mount.RefCount == 0 {
		return "", false, nil
	}

	mount.RefCount++
	if err := setMountInfo(mountFilePath, mount); err != nil {
		return "", false, errors.Wrap(err, "failed to set mount info on file")
	}

	return mount.Path, true, nil
}

func (d *Driver) setCacheMount(id string, path string) error {
	mountFilePath := filepath.Join(d.layerDirPath(id), "mount.json")

	mount, err := getMountInfo(mountFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to get mount info from file")
	}

	if mount.RefCount != 0 {
		return errors.New("attempt to set cached mount for an already mounted layer")
	}

	if err := setMountInfo(mountFilePath, mountInfo{path, 1}); err != nil {
		return errors.Wrap(err, "failed to set mount info")
	}

	return nil
}

func getMountInfo(path string) (mountInfo, error) {
	content, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return mountInfo{}, nil
	} else if err != nil {
		return mountInfo{}, errors.Wrap(err, "failed to read mount info file")
	}

	var mount mountInfo
	if err := json.Unmarshal(content, &mount); err != nil {
		return mountInfo{}, errors.Wrap(err, "failed to unmarshal mount info JSON")
	}

	return mount, nil
}

func setMountInfo(path string, mount mountInfo) error {
	content, err := json.Marshal(&mount)
	if err != nil {
		return errors.Wrap(err, "failed to marshal mount info as JSON")
	}

	if err := ioutil.WriteFile(path, content, 0600); err != nil {
		return errors.Wrap(err, "failed to write mount info JSON to file")
	}

	return nil
}
