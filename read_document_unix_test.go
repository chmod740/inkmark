//go:build !windows

package main

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadDocumentRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.md")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	completed := make(chan error, 1)
	go func() {
		_, err := readDocument(path)
		completed <- err
	}()
	select {
	case err := <-completed:
		if err == nil {
			t.Fatal("expected FIFO to be rejected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reading a FIFO blocked")
	}
}
