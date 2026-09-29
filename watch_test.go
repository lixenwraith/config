// FILE: lixenwraith/config/watch_test.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Real watcher tests wait for events; debounce and timeout cases use manualWatcher.
const (
	testPollInterval = DefaultPollInterval / 10
	testDebounce     = DefaultDebounce / 10
	testWatchTimeout = 2 * DefaultPollInterval
)

// TestAutoUpdate tests automatic configuration reloading
func TestAutoUpdate(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	initialConfig := `
[server]
port = 8080
host = "localhost"

[features]
enabled = true
`
	writeTestFile(t, configPath, []byte(initialConfig), 0644)

	// Create config with defaults
	type TestConfig struct {
		Server struct {
			Port int    `toml:"port"`
			Host string `toml:"host"`
		} `toml:"server"`
		Features struct {
			Enabled bool `toml:"enabled"`
		} `toml:"features"`
	}

	defaults := &TestConfig{}
	defaults.Server.Port = 3000
	defaults.Server.Host = "0.0.0.0"

	// Build config
	cfg, err := NewBuilder().
		WithDefaults(defaults).
		WithFile(configPath).
		Build()
	mustNoError(t, err)

	// Verify initial values
	port, exists := cfg.Get("server.port")
	if !exists {
		t.Errorf("exists should be true")
	}
	checkEqual(t, port, int64(8080))

	// Enable auto-update with fast polling
	opts := WatchOptions{
		PollInterval: testPollInterval,
		Debounce:     testDebounce,
		MaxWatchers:  10,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Start watching
	changes := cfg.Watch()

	// Update config file
	updatedConfig := `
[server]
port = 9090
host = "0.0.0.0"

[features]
enabled = false
`
	writeTestFile(t, configPath, []byte(updatedConfig), 0644)

	// Wait for publication events, without a collector goroutine or fixed sleep.
	changedPaths := make(map[string]bool)
	for range 3 {
		changedPaths[receiveWatchEvent(t, changes)] = true
	}

	// Verify new values
	port, _ = cfg.Get("server.port")
	checkEqual(t, port, int64(9090))

	host, _ := cfg.Get("server.host")
	checkEqual(t, host, "0.0.0.0")

	enabled, _ := cfg.Get("features.enabled")
	checkEqual(t, enabled, false)

	// Check that changes were notified
	expectedChanges := []string{"server.port", "server.host", "features.enabled"}
	for _, path := range expectedChanges {
		if !(changedPaths[path]) {
			t.Errorf("changedPaths[path] should be true: %s", fmt.Sprintf("Expected change notification for %s", path))
		}
	}
}

// TestWatchFileDeleted tests behavior when config file is deleted
func TestWatchFileDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create initial config
	writeTestFile(t, configPath, []byte(`test = "value"`), 0644)

	cfg := New()
	cfg.Register("test", "default")

	mustNoError(t, cfg.LoadFile(configPath))

	// Enable watching
	opts := WatchOptions{
		PollInterval: testPollInterval,
		Debounce:     testDebounce,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	// Delete file
	mustNoError(t, os.Remove(configPath))

	checkEqual(t, receiveWatchEvent(t, changes), EventFileDeleted)
}

// TestWatchPermissionChange tests permission change detection
func TestWatchPermissionChange(t *testing.T) {
	// Skip on Windows where permission model is different
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create config with specific permissions
	writeTestFile(t, configPath, []byte(`test = "value"`), 0644)

	cfg := New()
	cfg.Register("test", "default")
	mustNoError(t, cfg.LoadFile(configPath))

	// Enable watching with permission verification
	opts := WatchOptions{
		PollInterval:      testPollInterval,
		Debounce:          testDebounce,
		VerifyPermissions: true,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	// Change permissions to world-writable (security risk)
	mustNoError(t, os.Chmod(configPath, 0666))

	checkEqual(t, receiveWatchEvent(t, changes), EventPermissionsChanged)
}

// TestMaxWatchers tests watcher limit enforcement
func TestMaxWatchers(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	writeTestFile(t, configPath, []byte(`test = "value"`), 0644)
	mustNoError(t, cfg.LoadFile(configPath))

	// Enable watching with low max watchers
	opts := WatchOptions{
		PollInterval: testPollInterval,
		MaxWatchers:  3,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	for i := range 4 {
		ch := cfg.Watch()
		select {
		case _, ok := <-ch:
			if ok || i < 3 {
				t.Errorf("subscriber %d closed unexpectedly or received a spurious event", i)
			}
		default:
			if i == 3 {
				t.Error("excess subscriber was not closed synchronously")
			}
		}
	}

	// Verify watcher count
	checkEqual(t, cfg.WatcherCount(), 3)
}

// TestRapidDebounce tests that rapid changes are debounced
func TestRapidDebounce(t *testing.T) {
	cfg, path := watcherFixture(t)
	w := manualWatcher(t, cfg, path)
	w.opts.Debounce = time.Hour
	changes := w.subscribe()
	for i := 2; i <= 5; i++ {
		writeTestFile(t, path, []byte(fmt.Sprintf("value = %d", i)), 0600)
		w.checkAndReload(cfg)
		if len(changes) != 0 {
			t.Fatal("change published before the quiet interval")
		}
	}
	w.observedAt = time.Now().Add(-2 * time.Hour)
	w.checkAndReload(cfg)
	checkEqual(t, receiveWatchEvent(t, changes), "value")
	if len(changes) != 0 {
		t.Fatal("debounced changes emitted multiple events")
	}
	value, _ := cfg.Get("value")
	checkEqual(t, value, int64(5))
}

// TestWatchWithoutFile tests watching behavior when no file is configured
func TestWatchWithoutFile(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	// No file loaded, watch should return closed channel
	ch := cfg.Watch()

	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("ok should be false: %s", "Channel should be closed when no file to watch")
		}
	case <-time.After(10 * time.Millisecond):
		t.Error("Channel should be closed immediately")
	}

	if cfg.IsWatching() {
		t.Errorf("cfg.IsWatching() should be false")
	}
}

// TestConcurrentWatchOperations tests thread safety of watch operations
func TestConcurrentWatchOperations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	writeTestFile(t, configPath, []byte(`value = 1`), 0644)

	cfg := New()
	cfg.Register("value", 0)
	mustNoError(t, cfg.LoadFile(configPath))

	opts := WatchOptions{
		PollInterval: testDebounce,
		MaxWatchers:  50,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Start multiple watchers concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ch := cfg.Watch()
			if ch == nil {
				errors <- fmt.Errorf("watcher %d: got nil channel", id)
				return
			}

			// Try to receive
			select {
			case <-ch:
				// OK, got a change
			case <-time.After(2 * SpinWaitInterval):
				// OK, no changes yet
			}
		}(i)
	}

	// Concurrent config updates
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			content := fmt.Sprintf(`value = %d`, id+10)
			if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
				errors <- fmt.Errorf("writer %d: %v", id, err)
			}
		}(i)
	}

	// Check IsWatching concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			if !cfg.IsWatching() {
				errors <- fmt.Errorf("checker %d: IsWatching returned false", id)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	if got := len(errs); got != 0 {
		t.Errorf("length = %d, want %d: %s", got, 0, "Concurrent operations should not produce errors")
	}
}

// TestStopAutoUpdate tests clean shutdown of watcher
func TestStopAutoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	writeTestFile(t, configPath, []byte(`test = "value"`), 0644)

	cfg := New()
	cfg.Register("test", "value")
	mustNoError(t, cfg.LoadFile(configPath))

	// Start watching
	cfg.AutoUpdate()
	checkWatchingState(t, cfg, true, "Watcher should be active after first start")

	ch := cfg.Watch()

	// Stop watching
	cfg.StopAutoUpdate()

	// Verify stopped
	checkWatchingState(t, cfg, false, "Watcher should be inactive after stop")
	checkEqual(t, cfg.WatcherCount(), 0)

	// Channel should eventually close
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("ok should be false: %s", "Channel should be closed after stop")
		}
	case <-time.After(ShutdownTimeout):
		t.Fatal("watch channel did not close after stop")
	}

	// Starting again should work
	cfg.AutoUpdate()
	checkWatchingState(t, cfg, true, "Watcher should be active after restart")
	cfg.StopAutoUpdate()
}

// BenchmarkWatchOverhead benchmarks the overhead of file watching
func BenchmarkWatchOverhead(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench.toml")

	// Create config with many values
	var configContent string
	for i := 0; i < 100; i++ {
		configContent += fmt.Sprintf("value%d = %d\n", i, i)
	}
	writeTestFile(b, configPath, []byte(configContent), 0644)

	cfg := New()
	for i := 0; i < 100; i++ {
		cfg.Register(fmt.Sprintf("value%d", i), 0)
	}
	mustNoError(b, cfg.LoadFile(configPath))

	// Enable watching
	opts := WatchOptions{
		PollInterval: testPollInterval,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Benchmark value retrieval with watching enabled
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cfg.Get(fmt.Sprintf("value%d", i%100))
	}
}
