//go:build !darwin && !linux && !windows

package main

import (
	"errors"
	"os"
)

func renameWorkspaceEntryNoReplace(_ *os.Root, _ string, _ string) error {
	return errors.New("atomic no-replace rename is not supported on this platform")
}
