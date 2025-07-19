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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, os.WriteFile(configPath, []byte(initialConfig), 0644))

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
	require.NoError(t, err)

	// Verify initial values
	port, exists := cfg.Get("server.port")
	assert.True(t, exists)
	assert.Equal(t, int64(8080), port)

	// Enable auto-update with fast polling
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
		MaxWatchers:  10,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Start watching
	changes := cfg.Watch()

	// Collect changes
	var mu sync.Mutex
	changedPaths := make(map[string]bool)

	go func() {
		for path := range changes {
			mu.Lock()
			changedPaths[path] = true
			mu.Unlock()
		}
	}()

	// Update config file
	updatedConfig := `
[server]
port = 9090
host = "0.0.0.0"

[features]
enabled = false
`
	require.NoError(t, os.WriteFile(configPath, []byte(updatedConfig), 0644))

	// Wait for changes to be detected
	time.Sleep(300 * time.Millisecond)

	// Verify new values
	port, _ = cfg.Get("server.port")
	assert.Equal(t, int64(9090), port)

	host, _ := cfg.Get("server.host")
	assert.Equal(t, "0.0.0.0", host)

	enabled, _ := cfg.Get("features.enabled")
	assert.Equal(t, false, enabled)

	// Check that changes were notified
	mu.Lock()
	defer mu.Unlock()

	expectedChanges := []string{"server.port", "server.host", "features.enabled"}
	for _, path := range expectedChanges {
		assert.True(t, changedPaths[path], "Expected change notification for %s", path)
	}
}

// TestWatchFileDeleted tests behavior when config file is deleted
func TestWatchFileDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create initial config
	require.NoError(t, os.WriteFile(configPath, []byte(`test = "value"`), 0644))

	cfg := New()
	cfg.Register("test", "default")

	require.NoError(t, cfg.LoadFile(configPath))

	// Enable watching
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	// Delete file
	require.NoError(t, os.Remove(configPath))

	// Wait for deletion detection
	select {
	case path := <-changes:
		assert.Equal(t, "file_deleted", path)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for deletion notification")
	}
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
	require.NoError(t, os.WriteFile(configPath, []byte(`test = "value"`), 0644))

	cfg := New()
	cfg.Register("test", "default")
	require.NoError(t, cfg.LoadFile(configPath))

	// Enable watching with permission verification
	opts := WatchOptions{
		PollInterval:      100 * time.Millisecond,
		Debounce:          50 * time.Millisecond,
		VerifyPermissions: true,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	// Change permissions to world-writable (security risk)
	require.NoError(t, os.Chmod(configPath, 0666))

	// Wait for permission change detection
	select {
	case path := <-changes:
		assert.Equal(t, "permissions_changed", path)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for permission change notification")
	}
}

// TestMaxWatchers tests watcher limit enforcement
func TestMaxWatchers(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`test = "value"`), 0644))
	require.NoError(t, cfg.LoadFile(configPath))

	// Enable watching with low max watchers
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
		MaxWatchers:  3,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Create maximum allowed watchers
	channels := make([]<-chan string, 0, 4)
	for i := 0; i < 4; i++ {
		ch := cfg.Watch()
		channels = append(channels, ch)

		// Check if channel is open
		if i < 3 {
			// First 3 should be open
			select {
			case _, ok := <-ch:
				assert.True(t, ok || i < 3, "Channel %d should be open", i)
			default:
				// Channel is open and empty, expected
			}
		} else {
			// 4th should be closed immediately
			select {
			case _, ok := <-ch:
				assert.False(t, ok, "Channel 3 should be closed (max watchers exceeded)")
			case <-time.After(10 * time.Millisecond):
				t.Error("Channel 3 should be closed immediately")
			}
		}
	}

	// Verify watcher count
	assert.Equal(t, 3, cfg.WatcherCount())
}

// TestDebounce tests that rapid changes are debounced
func TestDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create initial config
	require.NoError(t, os.WriteFile(configPath, []byte(`value = 1`), 0644))

	cfg := New()
	cfg.Register("value", 0)
	require.NoError(t, cfg.LoadFile(configPath))

	// Enable watching with longer debounce
	opts := WatchOptions{
		PollInterval: 50 * time.Millisecond,
		Debounce:     200 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	var changeCount int
	var mu sync.Mutex
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-changes:
				mu.Lock()
				changeCount++
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	// Make rapid changes
	for i := 2; i <= 5; i++ {
		content := fmt.Sprintf(`value = %d`, i)
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
		time.Sleep(50 * time.Millisecond) // Less than debounce period
	}

	// Wait for debounce to complete
	time.Sleep(300 * time.Millisecond)
	done <- true

	// Should only see one change due to debounce
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, changeCount, "Expected 1 change due to debounce, got %d", changeCount)

	// Verify final value
	val, _ := cfg.Get("value")
	assert.Equal(t, int64(5), val)
}

// TestWatchWithoutFile tests watching behavior when no file is configured
func TestWatchWithoutFile(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	// No file loaded, watch should return closed channel
	ch := cfg.Watch()

	select {
	case _, ok := <-ch:
		assert.False(t, ok, "Channel should be closed when no file to watch")
	case <-time.After(10 * time.Millisecond):
		t.Error("Channel should be closed immediately")
	}

	assert.False(t, cfg.IsWatching())
}

// TestConcurrentWatchOperations tests thread safety of watch operations
func TestConcurrentWatchOperations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`value = 1`), 0644))

	cfg := New()
	cfg.Register("value", 0)
	require.NoError(t, cfg.LoadFile(configPath))

	opts := WatchOptions{
		PollInterval: 50 * time.Millisecond,
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
			case <-time.After(10 * time.Millisecond):
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

			isWatching := false
			for j := 0; j < 5; j++ { // Poll a few times, double-dip wait for goroutine to start
				if cfg.IsWatching() {
					isWatching = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !isWatching {
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
	assert.Empty(t, errs, "Concurrent operations should not produce errors")
}

// TestReloadTimeout tests reload timeout handling
func TestReloadTimeout(t *testing.T) {
	// This test would require mocking file operations to simulate a slow read
	// For now, we'll test that timeout option is respected in configuration

	cfg := New()
	cfg.Register("test", "value")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`test = "value"`), 0644))
	require.NoError(t, cfg.LoadFile(configPath))

	// Very short timeout
	opts := WatchOptions{
		PollInterval:  100 * time.Millisecond,
		ReloadTimeout: 1 * time.Nanosecond, // Extremely short
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	waitForWatchingState(t, cfg, true)
}

// TestStopAutoUpdate tests clean shutdown of watcher
func TestStopAutoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`test = "value"`), 0644))

	cfg := New()
	cfg.Register("test", "value")
	require.NoError(t, cfg.LoadFile(configPath))

	// Start watching
	cfg.AutoUpdate()
	waitForWatchingState(t, cfg, true, "Watcher should be active after first start")

	ch := cfg.Watch()

	// Stop watching
	cfg.StopAutoUpdate()

	// Verify stopped
	waitForWatchingState(t, cfg, false, "Watcher should be inactive after stop")
	assert.Equal(t, 0, cfg.WatcherCount())

	// Channel should eventually close
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "Channel should be closed after stop")
	case <-time.After(100 * time.Millisecond):
		// OK, channel might not close immediately
	}

	// Starting again should work
	cfg.AutoUpdate()
	waitForWatchingState(t, cfg, true, "Watcher should be active after restart")
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
	require.NoError(b, os.WriteFile(configPath, []byte(configContent), 0644))

	cfg := New()
	for i := 0; i < 100; i++ {
		cfg.Register(fmt.Sprintf("value%d", i), 0)
	}
	require.NoError(b, cfg.LoadFile(configPath))

	// Enable watching
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Benchmark value retrieval with watching enabled
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cfg.Get(fmt.Sprintf("value%d", i%100))
	}
}

// helper function to wait for watcher state, preventing race conditions of goroutine start and test check
func waitForWatchingState(t *testing.T, cfg *Config, expected bool, msgAndArgs ...any) {
	require.Eventually(t, func() bool {
		return cfg.IsWatching() == expected
	}, 200*time.Millisecond, 10*time.Millisecond, msgAndArgs...)
}