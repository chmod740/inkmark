//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

func workspaceLegacyDirectoryNameForTest() string {
	return "legacy-portable"
}

func workspaceRootReplacementUnavailableForTest(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
