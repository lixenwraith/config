package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func manualWatcher(t *testing.T, c *Config, path string) *watcher {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{ctx: ctx, cancel: cancel, opts: WatchOptions{ReloadTimeout: time.Second, MaxWatchers: 10}, filePath: path, watchers: make(map[int64]chan string), read: readConfigFile}
	w.watching.Store(true)
	c.mutex.Lock()
	c.watcher = w
	c.mutex.Unlock()
	t.Cleanup(c.StopAutoUpdate)
	return w
}

func watcherFixture(t *testing.T) (*Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("value=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New()
	if err := c.Register("value", uint8(0)); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	return c, path
}

func TestReloadCannotPublishAfterStopTimeoutOrNewLoad(t *testing.T) {
	for _, mode := range []string{"stop", "timeout", "new-load"} {
		t.Run(mode, func(t *testing.T) {
			c, path := watcherFixture(t)
			w := manualWatcher(t, c, path)
			ch := w.subscribe()
			if err := os.WriteFile(path, []byte("value=2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			staged, err := readConfigFile(context.Background(), path, fileSettings{})
			if err != nil {
				t.Fatal(err)
			}
			w.read = func(ctx context.Context, _ string, _ fileSettings) (*fileSnapshot, error) {
				switch mode {
				case "stop":
					c.StopAutoUpdate()
				case "timeout":
					<-ctx.Done()
				case "new-load":
					if err := os.WriteFile(path, []byte("value=3\n"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := c.LoadFile(path); err != nil {
						t.Fatal(err)
					}
				}
				return staged, nil // Deliberately complete a stale read successfully.
			}
			if mode == "timeout" {
				w.opts.ReloadTimeout = time.Millisecond
			}
			w.checkAndReload(c)
			want := int64(1)
			if mode == "new-load" {
				want = 3
			}
			if value, _ := c.Get("value"); value != want {
				t.Fatalf("stale publication: %v want %v", value, want)
			}
			if mode == "timeout" {
				select {
				case event := <-ch:
					if event != EventReloadTimeout {
						t.Fatal(event)
					}
				default:
					t.Fatal("timeout event missing")
				}
			}
			if mode == "stop" {
				if _, ok := <-ch; ok {
					t.Fatal("subscriber not closed by stop")
				}
			}
		})
	}
}

func TestWatcherDebouncesErrorsAndRecovers(t *testing.T) {
	c, path := watcherFixture(t)
	w := manualWatcher(t, c, path)
	w.opts.Debounce = time.Hour
	ch := w.subscribe()
	if err := os.WriteFile(path, []byte("value="), 0600); err != nil {
		t.Fatal(err)
	}
	w.checkAndReload(c)
	select {
	case event := <-ch:
		t.Fatalf("transient error emitted: %s", event)
	default:
	}
	if err := os.WriteFile(path, []byte("value=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w.checkAndReload(c)
	w.observedAt = time.Now().Add(-2 * time.Hour)
	w.checkAndReload(c)
	if value, _ := c.Get("value"); value != int64(2) {
		t.Fatal(value)
	}
	if event := <-ch; event != "value" {
		t.Fatal(event)
	}
	// A stable invalid edit is reported once and leaves the old value intact.
	writeTestFile(t, path, []byte("value=256\n"), 0600)
	w.checkAndReload(c)
	w.observedAt = time.Now().Add(-2 * time.Hour)
	w.checkAndReload(c)
	w.checkAndReload(c)
	select {
	case <-ch:
	default:
		t.Fatal("missing invalid edit event")
	}
	select {
	case event := <-ch:
		t.Fatalf("duplicate invalid edit: %s", event)
	default:
	}
	if value, _ := c.Get("value"); value != int64(2) {
		t.Fatal("invalid value published")
	}
}

func TestWatcherRetriesTimedOutUnchangedContent(t *testing.T) {
	for _, mode := range []string{"read-error", "late-result"} {
		t.Run(mode, func(t *testing.T) {
			c, path := watcherFixture(t)
			w := manualWatcher(t, c, path)
			w.opts.ReloadTimeout = 20 * time.Millisecond
			ch := w.subscribe()
			if err := os.WriteFile(path, []byte("value=2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			staged, err := readConfigFile(context.Background(), path, fileSettings{})
			if err != nil {
				t.Fatal(err)
			}
			attempts := 0
			w.read = func(ctx context.Context, _ string, _ fileSettings) (*fileSnapshot, error) {
				attempts++
				if attempts <= 2 {
					<-ctx.Done()
					if mode == "read-error" {
						return nil, ctx.Err()
					}
				}
				return staged, nil
			}
			w.checkAndReload(c)
			select {
			case event := <-ch:
				if event != EventReloadTimeout {
					t.Fatal(event)
				}
			default:
				t.Fatal("timeout event missing")
			}
			w.checkAndReload(c)
			if value, _ := c.Get("value"); value != int64(1) {
				t.Fatal("timed-out result published")
			}
			select {
			case event := <-ch:
				t.Fatalf("duplicate timeout: %s", event)
			default:
			}
			w.checkAndReload(c)
			if value, _ := c.Get("value"); value != int64(2) {
				t.Fatal("unchanged content was not retried after timeout")
			}
			select {
			case event := <-ch:
				if event != "value" {
					t.Fatal(event)
				}
			default:
				t.Fatal("recovery event missing")
			}
		})
	}
}

func TestWatcherSameMetadataReplacementAndRecreation(t *testing.T) {
	c, path := watcherFixture(t)
	w := manualWatcher(t, c, path)
	ch := w.subscribe()
	w.checkAndReload(c)
	info, _ := os.Stat(path)
	replacement := path + ".new"
	writeTestFile(t, replacement, []byte("value=2\n"), 0600)
	_ = os.Chtimes(replacement, info.ModTime(), info.ModTime())
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	w.checkAndReload(c)
	if value, _ := c.Get("value"); value != int64(2) {
		t.Fatal("replacement missed")
	}
	if event := <-ch; event != "value" {
		t.Fatal(event)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	w.checkAndReload(c)
	w.checkAndReload(c)
	if event := <-ch; event != EventFileDeleted {
		t.Fatal(event)
	}
	select {
	case event := <-ch:
		t.Fatalf("repeated deletion: %s", event)
	default:
	}
	writeTestFile(t, path, []byte("value=3\n"), 0600)
	w.checkAndReload(c)
	if value, _ := c.Get("value"); value != int64(3) {
		t.Fatal("recreation missed")
	}
	c.StopAutoUpdate()
	for range ch {
	} // Stop closes every subscriber synchronously.
	c.AutoUpdateWithOptions(WatchOptions{PollInterval: time.Hour})
	if !c.IsWatching() {
		t.Fatal("restart failed")
	}
}

func TestBuilderDoesNotHideInvalidSourceBehindMissingFile(t *testing.T) {
	var target struct {
		Port uint8 `toml:"port"`
	}
	c, err := NewBuilder().WithTarget(&target).WithFile(filepath.Join(t.TempDir(), "missing.toml")).WithArgs([]string{"--port=256"}).Build()
	if err == nil || c != nil {
		t.Fatalf("missing file hid invalid CLI: %v", err)
	}
}
