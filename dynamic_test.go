// FILE: lixenwraith/config/dynamic_test.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiFormatLoading tests loading different config formats
func TestMultiFormatLoading(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test config in different formats
	tomlConfig := `
[server]
host = "toml-host"
port = 8080

[database]
url = "postgres://localhost/toml"
`

	jsonConfig := `{
		"server": {
			"host": "json-host",
			"port": 9090
		},
		"database": {
			"url": "postgres://localhost/json"
		}
	}`

	yamlConfig := `
server:
  host: yaml-host
  port: 7070
database:
  url: postgres://localhost/yaml
`

	// Write config files
	tomlPath := filepath.Join(tmpDir, "config.toml")
	jsonPath := filepath.Join(tmpDir, "config.json")
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlConfig), 0644))
	require.NoError(t, os.WriteFile(jsonPath, []byte(jsonConfig), 0644))
	require.NoError(t, os.WriteFile(yamlPath, []byte(yamlConfig), 0644))

	t.Run("AutoDetectFormats", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "")
		cfg.Register("server.port", 0)
		cfg.Register("database.url", "")

		// Test TOML
		cfg.SetFileFormat("auto")
		require.NoError(t, cfg.LoadFile(tomlPath))
		host, _ := cfg.Get("server.host")
		assert.Equal(t, "toml-host", host)

		// Test JSON
		require.NoError(t, cfg.LoadFile(jsonPath))
		host, _ = cfg.Get("server.host")
		assert.Equal(t, "json-host", host)
		port, _ := cfg.Get("server.port")
		// JSON number should be preserved as json.Number but convertible
		switch v := port.(type) {
		case json.Number:
			// Expected for raw value
			assert.Equal(t, json.Number("9090"), v)
		case int64:
			// Expected after decode hook conversion
			assert.Equal(t, int64(9090), v)
		case float64:
			// Alternative conversion
			assert.Equal(t, float64(9090), v)
		default:
			t.Errorf("Unexpected type for port: %T", port)
		}

		// Test YAML
		require.NoError(t, cfg.LoadFile(yamlPath))
		host, _ = cfg.Get("server.host")
		assert.Equal(t, "yaml-host", host)
	})

	t.Run("ExplicitFormat", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "")

		// Force JSON parsing on .conf file
		confPath := filepath.Join(tmpDir, "config.conf")
		require.NoError(t, os.WriteFile(confPath, []byte(jsonConfig), 0644))

		cfg.SetFileFormat("json")
		require.NoError(t, cfg.LoadFile(confPath))

		host, _ := cfg.Get("server.host")
		assert.Equal(t, "json-host", host)
	})

	t.Run("ContentDetection", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "")

		// Ambiguous extension
		ambigPath := filepath.Join(tmpDir, "config.conf")
		require.NoError(t, os.WriteFile(ambigPath, []byte(yamlConfig), 0644))

		cfg.SetFileFormat("auto")
		require.NoError(t, cfg.LoadFile(ambigPath))

		host, _ := cfg.Get("server.host")
		assert.Equal(t, "yaml-host", host)
	})
}

// TestDynamicFormatSwitching tests runtime format changes
func TestDynamicFormatSwitching(t *testing.T) {
	tmpDir := t.TempDir()

	// Create configs in different formats with same structure
	configs := map[string]string{
		"toml": `value = "from-toml"`,
		"json": `{"value": "from-json"}`,
		"yaml": `value: from-yaml`,
	}

	cfg := New()
	cfg.Register("value", "default")

	for format, content := range configs {
		t.Run(format, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, "config."+format)
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

			// Set format and load
			require.NoError(t, cfg.SetFileFormat(format))
			require.NoError(t, cfg.LoadFile(filePath))

			val, _ := cfg.Get("value")
			assert.Equal(t, "from-"+format, val)
		})
	}
}

// TestWatchFileFormatSwitch tests watching different file formats
func TestWatchFileFormatSwitch(t *testing.T) {
	tmpDir := t.TempDir()

	tomlPath := filepath.Join(tmpDir, "config.toml")
	jsonPath := filepath.Join(tmpDir, "config.json")

	require.NoError(t, os.WriteFile(tomlPath, []byte(`value = "toml-1"`), 0644))
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"value": "json-1"}`), 0644))

	cfg := New()
	cfg.Register("value", "default")

	// Configure fast polling for test
	opts := WatchOptions{
		PollInterval: testPollInterval, // Fast polling for tests
		Debounce:     testDebounce,     // Short debounce
		MaxWatchers:  10,
	}

	// Start watching TOML
	cfg.SetFileFormat("auto")
	require.NoError(t, cfg.LoadFile(tomlPath))
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Wait for watcher to start
	require.Eventually(t, func() bool {
		return cfg.IsWatching()
	}, 4*testDebounce, 2*SpinWaitInterval)

	val, _ := cfg.Get("value")
	assert.Equal(t, "toml-1", val)

	// Switch to JSON with format hint
	require.NoError(t, cfg.WatchFile(jsonPath, "json"))

	// Wait for new watcher to start
	require.Eventually(t, func() bool {
		return cfg.IsWatching()
	}, 4*testDebounce, 2*SpinWaitInterval)

	// Get watch channel AFTER switching files
	changes := cfg.Watch()

	val, _ = cfg.Get("value")
	assert.Equal(t, "json-1", val)

	// Update JSON file
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"value": "json-2"}`), 0644))

	// Wait for change notification
	select {
	case path := <-changes:
		assert.Equal(t, "value", path)
		// Wait a bit for value to be updated
		require.Eventually(t, func() bool {
			val, _ := cfg.Get("value")
			return val == "json-2"
		}, testEventuallyTimeout, 2*SpinWaitInterval)
	case <-time.After(testWatchTimeout):
		t.Error("Timeout waiting for JSON file change")
	}

	// Update old TOML file - should NOT trigger notification
	require.NoError(t, os.WriteFile(tomlPath, []byte(`value = "toml-2"`), 0644))

	// Should not receive notification from old file
	select {
	case <-changes:
		t.Error("Should not receive changes from old TOML file")
	case <-time.After(testPollWindow):
		// Expected - no change notification
	}
}

// TestSecurityOptions tests security features
func TestSecurityOptions(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("PathTraversal", func(t *testing.T) {
		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			PreventPathTraversal: true,
		})

		// Test various malicious paths
		maliciousPaths := []string{
			"../../../etc/passwd",
			"./../etc/passwd",
			"config/../../../etc/passwd",
			filepath.Join("..", "..", "etc", "passwd"),
		}

		for _, malPath := range maliciousPaths {
			err := cfg.LoadFile(malPath)
			assert.Error(t, err, "Should reject path: %s", malPath)
			assert.Contains(t, err.Error(), "path traversal")
		}

		// Valid paths should work
		validPath := filepath.Join(tmpDir, "config.toml")
		os.WriteFile(validPath, []byte(`test = "value"`), 0644)
		cfg.Register("test", "")

		err := cfg.LoadFile(validPath)
		assert.NoError(t, err, "Should accept valid absolute path")
	})

	t.Run("FileSizeLimit", func(t *testing.T) {
		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			MaxFileSize: 100, // 100 bytes limit
		})

		// Create large file
		largePath := filepath.Join(tmpDir, "large.toml")
		largeContent := make([]byte, 1024)
		for i := range largeContent {
			largeContent[i] = 'a'
		}
		require.NoError(t, os.WriteFile(largePath, largeContent, 0644))

		err := cfg.LoadFile(largePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum size")
	})

	t.Run("FileOwnership", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping ownership test on Windows")
		}

		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			EnforceFileOwnership: true,
		})

		// Create file owned by current user (should succeed)
		ownedPath := filepath.Join(tmpDir, "owned.toml")
		require.NoError(t, os.WriteFile(ownedPath, []byte(`test = "value"`), 0644))

		cfg.Register("test", "")
		err := cfg.LoadFile(ownedPath)
		assert.NoError(t, err)
	})
}

// waitForWatchingState waits for watcher state, preventing race conditions of goroutine start and test check
func waitForWatchingState(t *testing.T, cfg *Config, expected bool, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		return cfg.IsWatching() == expected
	}, testEventuallyTimeout, 2*SpinWaitInterval, msgAndArgs...)
}

// TestBuilderWithFormat tests Builder integration
func TestBuilderWithFormat(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "config.json")

	jsonConfig := `{
		"server": {
			"host": "builder-host",
			"port": 8080
		}
	}`
	require.NoError(t, os.WriteFile(jsonPath, []byte(jsonConfig), 0644))

	type Config struct {
		Server struct {
			Host string `json:"host" toml:"host"`
			Port int    `json:"port" toml:"port"`
		} `json:"server" toml:"server"`
	}

	defaults := &Config{}
	defaults.Server.Host = "default-host"
	defaults.Server.Port = 3000

	cfg, err := NewBuilder().
		WithDefaults(defaults).
		WithFile(jsonPath).
		WithFileFormat("json").
		WithTagName("toml"). // Use toml tags for registration
		WithSecurityOptions(SecurityOptions{
			PreventPathTraversal: true,
			MaxFileSize:          1024 * 1024, // 1MB
		}).
		Build()

	require.NoError(t, err)

	// Check the value was loaded
	host, exists := cfg.Get("server.host")
	assert.True(t, exists, "server.host should exist")
	assert.Equal(t, "builder-host", host)

	port, exists := cfg.Get("server.port")
	assert.True(t, exists, "server.port should exist")
	// Handle json.Number or converted int
	switch v := port.(type) {
	case json.Number:
		p, _ := v.Int64()
		assert.Equal(t, int64(8080), p)
	case int64:
		assert.Equal(t, int64(8080), v)
	case float64:
		assert.Equal(t, float64(8080), v)
	default:
		t.Errorf("Unexpected type for port: %T", port)
	}
}

// BenchmarkFormatParsing benchmarks different format parsing speeds
func BenchmarkFormatParsing(b *testing.B) {
	tmpDir := b.TempDir()

	// Create test data
	configs := map[string]string{
		"toml": `
[server]
host = "localhost"
port = 8080
[database]
url = "postgres://localhost/db"
[cache]
ttl = 300
`,
		"json": `{
			"server": {"host": "localhost", "port": 8080},
			"database": {"url": "postgres://localhost/db"},
			"cache": {"ttl": 300}
		}`,
		"yaml": `
server:
  host: localhost
  port: 8080
database:
  url: postgres://localhost/db
cache:
  ttl: 300
`,
	}

	for format, content := range configs {
		b.Run(format, func(b *testing.B) {
			path := filepath.Join(tmpDir, "bench."+format)
			os.WriteFile(path, []byte(content), 0644)

			cfg := New()
			cfg.Register("server.host", "")
			cfg.Register("server.port", 0)
			cfg.Register("database.url", "")
			cfg.Register("cache.ttl", 0)
			cfg.SetFileFormat(format)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cfg.LoadFile(path)
			}
		})
	}
}