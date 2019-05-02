package cimfs

import (
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

type FileInfo struct {
	Size int64

	CreationTime   windows.Filetime
	LastWriteTime  windows.Filetime
	ChangeTime     windows.Filetime
	LastAccessTime windows.Filetime

	Attributes uint32

	SecurityDescriptor []byte
	ReparseData        []byte
	EAs                []winio.ExtendedAttribute
}

type fileInfoInternal struct {
	Size     uint32
	FileSize int64

	CreationTime   windows.Filetime
	LastWriteTime  windows.Filetime
	ChangeTime     windows.Filetime
	LastAccessTime windows.Filetime

	Attributes uint32

	SecurityDescriptorBuffer unsafe.Pointer
	SecurityDescriptorSize   uint32

	ReparseDataBuffer unsafe.Pointer
	ReparseDataSize   uint32

	EAs     unsafe.Pointer
	EACount uint32
}

type eaInternal struct {
	Name       string
	NameLength uint32

	Flags uint8

	Buffer     unsafe.Pointer
	BufferSize uint32
}

type imageHandle uintptr
type streamHandle uintptr

type Image struct {
	handle       imageHandle
	activeStream streamHandle
}

func Open(path string) (*Image, error) {
	var handle imageHandle
	if err := cimInitializeImage(path, 0, &handle); err != nil {
		return nil, err
	}

	return &Image{handle: handle}, nil
}

func (cim *Image) AddFile(path string, info *FileInfo) error {
	infoInternal := &fileInfoInternal{
		FileSize:       info.Size,
		CreationTime:   info.CreationTime,
		LastWriteTime:  info.LastWriteTime,
		ChangeTime:     info.ChangeTime,
		LastAccessTime: info.LastAccessTime,
		Attributes:     info.Attributes,
	}

	if len(info.SecurityDescriptor) > 0 {
		infoInternal.SecurityDescriptorBuffer = unsafe.Pointer(&info.SecurityDescriptor[0])
		infoInternal.SecurityDescriptorSize = uint32(len(info.SecurityDescriptor))
	}

	if len(info.ReparseData) > 0 {
		infoInternal.ReparseDataBuffer = unsafe.Pointer(&info.ReparseData[0])
		infoInternal.ReparseDataSize = uint32(len(info.ReparseData))
	}

	easInternal := []eaInternal{}
	for _, ea := range info.EAs {
		eaInternal := eaInternal{
			Name:       ea.Name,
			NameLength: uint32(len(ea.Name)),
			Flags:      ea.Flags,
		}

		if len(ea.Value) > 0 {
			eaInternal.Buffer = unsafe.Pointer(&ea.Value[0])
			eaInternal.BufferSize = uint32(len(ea.Value))
		}

		easInternal = append(easInternal, eaInternal)
	}

	return cimAddFile(cim.handle, path, infoInternal, 0, &cim.activeStream)
}

func (cim *Image) Write(p []byte) (int, error) {
	if cim.activeStream == 0 {
		return 0, errors.New("No active stream")
	}

	// TODO: pass p directly to gen'd syscall
	err := cimWriteStream(cim.activeStream, uintptr(unsafe.Pointer(&p[0])), uint64(len(p)))
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (cim *Image) CloseStream() error {
	return cimFinalizeStream(cim.activeStream)
}

func (cim *Image) Close(path string) error {
	return cimFinalizeImage(cim.handle, path)
}

func (cim *Image) RemoveFile(path string) error {
	return cimRemoveFile(cim.handle, path)
}

func MountImage(path string, g *guid.GUID) error {
	return cimMountImage(path, g)
}

func UnmountImage(g *guid.GUID) error {
	return cimUnmountImage(g)
}
