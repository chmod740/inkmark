//go:build darwin

package main

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

func workspaceDeletionRootFilesystemIdentity(root *os.Root) (uint64, error) {
	info, err := root.Stat(".")
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("cannot identify the workspace filesystem")
	}
	return uint64(stat.Dev), nil
}

func workspaceDeletionEntryFilesystemMatches(_ *os.Root, _ string, info fs.FileInfo, filesystemID uint64) (bool, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, errors.New("cannot identify a deletion entry filesystem")
	}
	return uint64(stat.Dev) == filesystemID, nil
}

func workspaceDeletionPlatformSafe(_ fs.FileInfo) bool {
	return true
}
