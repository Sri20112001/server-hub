//go:build windows

package transfer

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func diskUsage(path string) (total, used, available uint64, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(
		pathPtr,
		(*uint64)(unsafe.Pointer(&freeBytesAvailable)),
		(*uint64)(unsafe.Pointer(&totalBytes)),
		(*uint64)(unsafe.Pointer(&totalFreeBytes)),
	)
	if err != nil {
		return
	}
	total = totalBytes
	available = freeBytesAvailable
	used = total - totalFreeBytes
	return
}
