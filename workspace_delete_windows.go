//go:build windows

package main

import (
	"io/fs"
	"os"
	"syscall"
)

func workspaceDeletionRootFilesystemIdentity(_ *os.Root) (uint64, error) {
	// Windows mount points and junctions are reparse points and are rejected by
	// workspaceDeletionPlatformSafe before recursion. There is no remaining
	// path by which a normal directory descends into another volume.
	return 0, nil
}

func workspaceDeletionEntryFilesystemMatches(_ *os.Root, _ string, info fs.FileInfo, _ uint64) (bool, error) {
	return workspaceDeletionPlatformSafe(info), nil
}

func workspaceDeletionPlatformSafe(info fs.FileInfo) bool {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
