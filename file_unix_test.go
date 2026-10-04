//go:build unix

package config

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The open cannot block: a FIFO swapped in after the Stat fails at once
func TestOpenConfigNeverBlocks(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "config.toml")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skip(err)
	}
	done := make(chan struct{})
	go func() {
		if f, err := openConfig(fifo); err == nil {
			f.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("open blocked on a FIFO without a writer")
	}
}
