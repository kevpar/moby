// Code generated mksyscall_windows.exe DO NOT EDIT

package cimfs

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var _ unsafe.Pointer

// Do the interface allocations only once for common
// Errno values.
const (
	errnoERROR_IO_PENDING = 997
)

var (
	errERROR_IO_PENDING error = syscall.Errno(errnoERROR_IO_PENDING)
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case errnoERROR_IO_PENDING:
		return errERROR_IO_PENDING
	}
	// TODO: add more here, after collecting data on the common
	// error values see on Windows. (perhaps when running
	// all.bat?)
	return e
}

var (
	modcimfs = windows.NewLazySystemDLL("cimfs.dll")

	procCimMountImage      = modcimfs.NewProc("CimMountImage")
	procCimUnmountImage    = modcimfs.NewProc("CimUnmountImage")
	procCimInitializeImage = modcimfs.NewProc("CimInitializeImage")
	procCimFinalizeImage   = modcimfs.NewProc("CimFinalizeImage")
	procCimAddFile         = modcimfs.NewProc("CimAddFile")
	procCimFinalizeStream  = modcimfs.NewProc("CimFinalizeStream")
	procCimWriteStream     = modcimfs.NewProc("CimWriteStream")
	procCimRemoveFile      = modcimfs.NewProc("CimRemoveFile")
	procCimAddLink         = modcimfs.NewProc("CimAddLink")
)

func cimMountImage(cimPath string, volumeID *g) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimMountImage(_p0, volumeID)
}

func _cimMountImage(cimPath *uint16, volumeID *g) (hr error) {
	r0, _, _ := syscall.Syscall(procCimMountImage.Addr(), 2, uintptr(unsafe.Pointer(cimPath)), uintptr(unsafe.Pointer(volumeID)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimUnmountImage(volumeID *g) (hr error) {
	r0, _, _ := syscall.Syscall(procCimUnmountImage.Addr(), 1, uintptr(unsafe.Pointer(volumeID)), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimInitializeImage(cimPath string, flags uint32, cimFSHandle *imageHandle) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimInitializeImage(_p0, flags, cimFSHandle)
}

func _cimInitializeImage(cimPath *uint16, flags uint32, cimFSHandle *imageHandle) (hr error) {
	r0, _, _ := syscall.Syscall(procCimInitializeImage.Addr(), 3, uintptr(unsafe.Pointer(cimPath)), uintptr(flags), uintptr(unsafe.Pointer(cimFSHandle)))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimFinalizeImage(cimFSHandle imageHandle, cimPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(cimPath)
	if hr != nil {
		return
	}
	return _cimFinalizeImage(cimFSHandle, _p0)
}

func _cimFinalizeImage(cimFSHandle imageHandle, cimPath *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimFinalizeImage.Addr(), 2, uintptr(cimFSHandle), uintptr(unsafe.Pointer(cimPath)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimAddFile(cimFSHandle imageHandle, path string, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(path)
	if hr != nil {
		return
	}
	return _cimAddFile(cimFSHandle, _p0, file, flags, cimStreamHandle)
}

func _cimAddFile(cimFSHandle imageHandle, path *uint16, file *fileInfoInternal, flags uint32, cimStreamHandle *streamHandle) (hr error) {
	r0, _, _ := syscall.Syscall6(procCimAddFile.Addr(), 5, uintptr(cimFSHandle), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(file)), uintptr(flags), uintptr(unsafe.Pointer(cimStreamHandle)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimFinalizeStream(cimStreamHandle streamHandle) (hr error) {
	r0, _, _ := syscall.Syscall(procCimFinalizeStream.Addr(), 1, uintptr(cimStreamHandle), 0, 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimWriteStream(cimStreamHandle streamHandle, buffer uintptr, bufferSize uint32) (hr error) {
	r0, _, _ := syscall.Syscall(procCimWriteStream.Addr(), 3, uintptr(cimStreamHandle), uintptr(buffer), uintptr(bufferSize))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimRemoveFile(cimFSHandle imageHandle, path string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(path)
	if hr != nil {
		return
	}
	return _cimRemoveFile(cimFSHandle, _p0)
}

func _cimRemoveFile(cimFSHandle imageHandle, path *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimRemoveFile.Addr(), 2, uintptr(cimFSHandle), uintptr(unsafe.Pointer(path)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func cimAddLink(cimFSHandle imageHandle, existingPath string, targetPath string) (hr error) {
	var _p0 *uint16
	_p0, hr = syscall.UTF16PtrFromString(existingPath)
	if hr != nil {
		return
	}
	var _p1 *uint16
	_p1, hr = syscall.UTF16PtrFromString(targetPath)
	if hr != nil {
		return
	}
	return _cimAddLink(cimFSHandle, _p0, _p1)
}

func _cimAddLink(cimFSHandle imageHandle, existingPath *uint16, targetPath *uint16) (hr error) {
	r0, _, _ := syscall.Syscall(procCimAddLink.Addr(), 3, uintptr(cimFSHandle), uintptr(unsafe.Pointer(existingPath)), uintptr(unsafe.Pointer(targetPath)))
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}
