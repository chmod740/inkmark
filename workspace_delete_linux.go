//go:build linux

package main

import (
	"errors"
	"io/fs"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func workspaceDeletionRootFilesystemIdentity(root *os.Root) (uint64, error) {
	file, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return workspaceLinuxMountID(file)
}

func workspaceDeletionEntryFilesystemMatches(root *os.Root, name string, _ fs.FileInfo, filesystemID uint64) (bool, error) {
	directory, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer directory.Close()
	var stat unix.Statx_t
	err = unix.Statx(int(directory.Fd()), name, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &stat)
	runtime.KeepAlive(directory)
	if err != nil {
		return false, err
	}
	if stat.Mask&unix.STATX_MNT_ID == 0 {
		return false, errors.New("statx did not provide a mount identifier")
	}
	return stat.Mnt_id == filesystemID, nil
}

func workspaceLinuxMountID(file *os.File) (uint64, error) {
	var stat unix.Statx_t
	err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &stat)
	runtime.KeepAlive(file)
	if err != nil {
		return 0, err
	}
	if stat.Mask&unix.STATX_MNT_ID == 0 {
		return 0, errors.New("statx did not provide a mount identifier")
	}
	return stat.Mnt_id, nil
}

func workspaceDeletionPlatformSafe(_ fs.FileInfo) bool {
	return true
}
