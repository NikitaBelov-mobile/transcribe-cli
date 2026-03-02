//go:build windows

package app

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetShortPathNameW = kernel32.NewProc("GetShortPathNameW")
)

func tryWindowsShortPath(path string) (string, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, 32768)
	r0, _, e1 := procGetShortPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r0 == 0 {
		if e1 != syscall.Errno(0) {
			return "", e1
		}
		return "", errors.New("GetShortPathNameW returned empty path")
	}
	return syscall.UTF16ToString(buf[:r0]), nil
}
