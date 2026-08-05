//go:build windows

package main

import "os"

func openReadOnlyNonBlocking(path string) (*os.File, error) {
	return os.Open(path)
}

func openRootReadOnlyNonBlocking(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
