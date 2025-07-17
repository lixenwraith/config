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

func TestAutoUpdate(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	initialConfig := `
[server]
port = 8080
host = "localhost"

[features]
enabled = true
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatal("Failed to write initial config:", err)
	}

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
	if err != nil {
		t.Fatal("Failed to build config:", err)
	}

	// Verify initial values
	if port, _ := cfg.Get("server.port"); port.(int64) != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}

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

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatal("Failed to update config:", err)
	}

	// Wait for changes to be detected
	time.Sleep(300 * time.Millisecond)

	// Verify new values
	if port, _ := cfg.Get("server.port"); port.(int64) != 9090 {
		t.Errorf("Expected port 9090 after update, got %d", port)
	}

	if host, _ := cfg.Get("server.host"); host.(string) != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0 after update, got %s", host)
	}

	if enabled, _ := cfg.Get("features.enabled"); enabled.(bool) != false {
		t.Errorf("Expected features.enabled to be false after update")
	}

	// Check that changes were notified
	mu.Lock()
	defer mu.Unlock()

	expectedChanges := []string{"server.port", "server.host", "features.enabled"}
	for _, path := range expectedChanges {
		if !changedPaths[path] {
			t.Errorf("Expected change notification for %s", path)
		}
	}
}

func TestWatchFileDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create initial config
	if err := os.WriteFile(configPath, []byte(`test = "value"`), 0644); err != nil {
		t.Fatal("Failed to write config:", err)
	}

	cfg := New()
	cfg.Register("test", "default")

	if err := cfg.LoadFile(configPath); err != nil {
		t.Fatal("Failed to load config:", err)
	}

	// Enable watching
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()

	// Delete file
	if err := os.Remove(configPath); err != nil {
		t.Fatal("Failed to delete config:", err)
	}

	// Wait for deletion detection
	select {
	case path := <-changes:
		if path != "file_deleted" {
			t.Errorf("Expected file_deleted, got %s", path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for deletion notification")
	}
}

func TestWatchPermissionChange(t *testing.T) {
	// Skip on Windows where permission model is different
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create config with specific permissions
	if err := os.WriteFile(configPath, []byte(`test = "value"`), 0644); err != nil {
		t.Fatal("Failed to write config:", err)
	}

	cfg := New()
	cfg.Register("test", "default")

	if err := cfg.LoadFile(configPath); err != nil {
		t.Fatal("Failed to load config:", err)
	}

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
	if err := os.Chmod(configPath, 0666); err != nil {
		t.Fatal("Failed to change permissions:", err)
	}

	// Wait for permission change detection
	select {
	case path := <-changes:
		if path != "permissions_changed" {
			t.Errorf("Expected permissions_changed, got %s", path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for permission change notification")
	}
}

func TestMaxWatchers(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")
	if err := os.WriteFile(configPath, []byte(`test = "value"`), 0644); err != nil {
		t.Fatal("Failed to write config:", err)
	}

	if err := cfg.LoadFile(configPath); err != nil {
		t.Fatal("Failed to load config:", err)
	}

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
		select {
		case _, ok := <-ch:
			if !ok && i < 3 {
				t.Errorf("Channel %d should be open", i)
			} else if ok && i == 3 {
				t.Error("Channel 3 should be closed (max watchers exceeded)")
			}
		default:
			// Channel is open and empty, expected for first 3
			if i == 3 {
				// Try to receive with timeout to verify it's closed
				select {
				case _, ok := <-ch:
					if ok {
						t.Error("Channel 3 should be closed")
					}
				case <-time.After(10 * time.Millisecond):
					t.Error("Channel 3 should be closed immediately")
				}
			}
		}
	}
}

func TestDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.toml")

	// Create initial config
	if err := os.WriteFile(configPath, []byte(`value = 1`), 0644); err != nil {
		t.Fatal("Failed to write config:", err)
	}

	cfg := New()
	cfg.Register("value", 0)

	if err := cfg.LoadFile(configPath); err != nil {
		t.Fatal("Failed to load config:", err)
	}

	// Enable watching with longer debounce
	opts := WatchOptions{
		PollInterval: 50 * time.Millisecond,
		Debounce:     200 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	changes := cfg.Watch()
	changeCount := 0

	go func() {
		for range changes {
			changeCount++
		}
	}()

	// Make rapid changes
	for i := 2; i <= 5; i++ {
		content := fmt.Sprintf(`value = %d`, i)
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatal("Failed to write config:", err)
		}
		time.Sleep(50 * time.Millisecond) // Less than debounce period
	}

	// Wait for debounce to complete
	time.Sleep(300 * time.Millisecond)

	// Should only see one change due to debounce
	if changeCount != 1 {
		t.Errorf("Expected 1 change due to debounce, got %d", changeCount)
	}

	// Verify final value
	val, _ := cfg.Get("value")
	if val.(int64) != 5 {
		t.Errorf("Expected final value 5, got %d", val)
	}
}

// Benchmark file watching overhead
func BenchmarkWatchOverhead(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench.toml")

	// Create config with many values
	var configContent string
	for i := 0; i < 100; i++ {
		configContent += fmt.Sprintf("value%d = %d\n", i, i)
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		b.Fatal("Failed to write config:", err)
	}

	cfg := New()
	for i := 0; i < 100; i++ {
		cfg.Register(fmt.Sprintf("value%d", i), 0)
	}

	if err := cfg.LoadFile(configPath); err != nil {
		b.Fatal("Failed to load config:", err)
	}

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