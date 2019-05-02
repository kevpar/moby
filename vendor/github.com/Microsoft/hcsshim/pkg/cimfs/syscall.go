package cimfs

import (
	"github.com/Microsoft/go-winio/pkg/guid"
)

type g = guid.GUID

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go syscall.go

// CimAddStream
// CimAddLink
// CimRemoveFile

//sys cimMountImage(cimPath string, volumeID *g) (hr error) = cimfs.CimMountImage
//sys cimUnmountImage(volumeID *g) (hr error) = cimfs.CimUnmountImage

//sys cimInitializeImage(cimPath string, flags uint32, cimFSHandle *imageHandle) (hr error) = cimfs.CimInitializeImage
//sys cimFinalizeImage(cimFSHandle imageHandle, cimPath string) (hr error) = cimfs.CimFinalizeImage

//sys cimAddFile(cimFSHandle imageHandle, path string, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) = cimfs.CimAddFile
//sys cimFinalizeStream(cimStreamHandle streamHandle) (hr error) = cimfs.CimFinalizeStream
//sys cimWriteStream(cimStreamHandle streamHandle, buffer uintptr, bufferSize uint64) (hr error) = cimfs.CimWriteStream
//sys cimRemoveFile(cimFSHandle imageHandle, path string) (hr error) = cimfs.CimRemoveFile
