//go:build linux

package main

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func renameWorkspaceEntryNoReplace(parent *os.Root, sourceName string, destinationName string) error {
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	directoryFD := int(directory.Fd())
	err = unix.Renameat2(directoryFD, sourceName, directoryFD, destinationName, unix.RENAME_NOREPLACE)
	runtime.KeepAlive(directory)
	return err
}
