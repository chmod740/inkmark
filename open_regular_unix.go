//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// openReadOnlyNonBlocking keeps special files such as FIFOs from blocking the
// UI thread before readDocumentFromFile can reject them as non-regular files.
func openReadOnlyNonBlocking(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	return file, nil
}

func openRootReadOnlyNonBlocking(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
}

func openRootWriteOnlyNonBlocking(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_WRONLY|unix.O_NONBLOCK, 0)
}
