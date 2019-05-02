package cimfs

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go syscall.go

//sys hcsFormatWritableLayerVhd(handle uintptr) (hr error) = computestorage.HcsFormatWritableLayerVhd
