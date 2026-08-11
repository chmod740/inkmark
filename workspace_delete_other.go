//go:build !darwin && !linux && !windows

package main

import (
	"errors"
	"io/fs"
	"os"
)

func workspaceDeletionRootFilesystemIdentity(_ *os.Root) (uint64, error) {
	return 0, errors.New("safe recursive workspace deletion is not supported on this platform")
}

func workspaceDeletionEntryFilesystemMatches(_ *os.Root, _ string, _ fs.FileInfo, _ uint64) (bool, error) {
	return false, errors.New("safe recursive workspace deletion is not supported on this platform")
}

func workspaceDeletionPlatformSafe(_ fs.FileInfo) bool {
	return true
}
