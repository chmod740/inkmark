//go:build !windows

package main

func workspaceLegacyDirectoryNameForTest() string {
	return "legacy:name"
}

func workspaceRootReplacementUnavailableForTest(error) bool {
	return false
}
