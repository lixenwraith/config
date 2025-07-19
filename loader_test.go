// FILE: lixenwraith/config/loader_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileLoading tests TOML file loading
func TestFileLoading(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("ValidTOMLFile", func(t *testing.T) {
		configFile := filepath.Join(tmpDir, "valid.toml")
		content := `
# Server configuration
[server]
host = "example.com"
port = 9000
enabled = true

[server.tls]
cert = "/path/to/cert.pem"
key = "/path/to/key.pem"

[database]
connections = [1, 2, 3]
tags = ["primary", "replica"]
`
		os.WriteFile(configFile, []byte(content), 0644)

		cfg := New()
		// Register all paths
		cfg.Register("server.host", "localhost")
		cfg.Register("server.port", 8080)
		cfg.Register("server.enabled", false)
		cfg.Register("server.tls.cert", "")
		cfg.Register("server.tls.key", "")
		cfg.Register("database.connections", []int{})
		cfg.Register("database.tags", []string{})

		err := cfg.LoadFile(configFile)
		require.NoError(t, err)

		// Verify loaded values
		host, _ := cfg.Get("server.host")
		assert.Equal(t, "example.com", host)

		port, _ := cfg.Get("server.port")
		assert.Equal(t, int64(9000), port)

		enabled, _ := cfg.Get("server.enabled")
		assert.Equal(t, true, enabled)

		cert, _ := cfg.Get("server.tls.cert")
		assert.Equal(t, "/path/to/cert.pem", cert)

		// Arrays are loaded as []any
		connections, _ := cfg.Get("database.connections")
		assert.Equal(t, []any{int64(1), int64(2), int64(3)}, connections)
	})

	t.Run("InvalidTOMLFile", func(t *testing.T) {
		configFile := filepath.Join(tmpDir, "invalid.toml")
		os.WriteFile(configFile, []byte(`invalid = toml content`), 0644)

		cfg := New()
		err := cfg.LoadFile(configFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse TOML")
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		cfg := New()
		err := cfg.LoadFile("/non/existent/file.toml")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrConfigNotFound)
	})

	t.Run("UnregisteredPathsIgnored", func(t *testing.T) {
		configFile := filepath.Join(tmpDir, "extra.toml")
		os.WriteFile(configFile, []byte(`
registered = "value"
unregistered = "ignored"
`), 0644)

		cfg := New()
		cfg.Register("registered", "")

		err := cfg.LoadFile(configFile)
		require.NoError(t, err)

		val, exists := cfg.Get("registered")
		assert.True(t, exists)
		assert.Equal(t, "value", val)

		_, exists = cfg.Get("unregistered")
		assert.False(t, exists)
	})
}

// TestEnvironmentLoading tests environment variable loading
func TestEnvironmentLoading(t *testing.T) {
	// Save and restore environment
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := splitEnvVar(e)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("DefaultEnvTransform", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "localhost")
		cfg.Register("server.port", 8080)
		cfg.Register("enable_debug", false)

		os.Setenv("APP_SERVER_HOST", "envhost")
		os.Setenv("APP_SERVER_PORT", "9090")
		os.Setenv("APP_ENABLE_DEBUG", "true")

		err := cfg.LoadEnv("APP_")
		require.NoError(t, err)

		host, _ := cfg.Get("server.host")
		assert.Equal(t, "envhost", host)

		port, _ := cfg.Get("server.port")
		assert.Equal(t, "9090", port) // String from env

		debug, _ := cfg.Get("enable_debug")
		assert.Equal(t, "true", debug) // String from env
	})

	t.Run("CustomEnvTransform", func(t *testing.T) {
		cfg := New()
		cfg.Register("db.host", "localhost")

		os.Setenv("DATABASE_HOSTNAME", "customhost")

		opts := LoadOptions{
			Sources: []Source{SourceEnv, SourceDefault},
			EnvTransform: func(path string) string {
				if path == "db.host" {
					return "DATABASE_HOSTNAME"
				}
				return path
			},
		}

		err := cfg.LoadWithOptions("", nil, opts)
		require.NoError(t, err)

		host, _ := cfg.Get("db.host")
		assert.Equal(t, "customhost", host)
	})

	t.Run("EnvWhitelist", func(t *testing.T) {
		cfg := New()
		cfg.Register("allowed.path", "default1")
		cfg.Register("blocked.path", "default2")

		os.Setenv("ALLOWED_PATH", "env1")
		os.Setenv("BLOCKED_PATH", "env2")

		opts := LoadOptions{
			Sources:      []Source{SourceEnv, SourceDefault},
			EnvWhitelist: map[string]bool{"allowed.path": true},
		}

		err := cfg.LoadWithOptions("", nil, opts)
		require.NoError(t, err)

		allowed, _ := cfg.Get("allowed.path")
		assert.Equal(t, "env1", allowed)

		blocked, _ := cfg.Get("blocked.path")
		assert.Equal(t, "default2", blocked) // Should not load from env
	})

	t.Run("DiscoverEnv", func(t *testing.T) {
		cfg := New()
		cfg.Register("test.one", "")
		cfg.Register("test.two", "")
		cfg.Register("other.value", "")

		os.Setenv("PREFIX_TEST_ONE", "value1")
		os.Setenv("PREFIX_TEST_TWO", "value2")
		os.Setenv("PREFIX_OTHER_VALUE", "value3")
		os.Setenv("UNRELATED_VAR", "ignored")

		discovered := cfg.DiscoverEnv("PREFIX_")
		assert.Len(t, discovered, 3)
		assert.Equal(t, "PREFIX_TEST_ONE", discovered["test.one"])
		assert.Equal(t, "PREFIX_TEST_TWO", discovered["test.two"])
		assert.Equal(t, "PREFIX_OTHER_VALUE", discovered["other.value"])
	})
}

// TestCLIParsing tests command-line argument parsing
func TestCLIParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]any
	}{
		{
			name: "KeyValueWithEquals",
			args: []string{"--server.host=example.com", "--server.port=9000"},
			expected: map[string]any{
				"server.host": "example.com",
				"server.port": "9000",
			},
		},
		{
			name: "KeyValueWithSpace",
			args: []string{"--server.host", "example.com", "--server.port", "9000"},
			expected: map[string]any{
				"server.host": "example.com",
				"server.port": "9000",
			},
		},
		{
			name: "BooleanFlags",
			args: []string{"--enable.debug", "--disable.cache", "false"},
			expected: map[string]any{
				"enable.debug":  "true",
				"disable.cache": "false",
			},
		},
		{
			name: "MixedFormats",
			args: []string{
				"--server.host=localhost",
				"--server.port", "8080",
				"--enable.tls",
				"--database.pool.size=10",
			},
			expected: map[string]any{
				"server.host":        "localhost",
				"server.port":        "8080",
				"enable.tls":         "true",
				"database.pool.size": "10",
			},
		},
		{
			name: "EmptyAndInvalidArgs",
			args: []string{"", "--", "---", "--=value"},
			expected: map[string]any{
				"": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := New()

			// Register expected paths
			for path := range tt.expected {
				if path != "" { // Skip empty path
					cfg.Register(path, "")
				}
			}

			err := cfg.LoadCLI(tt.args)
			require.NoError(t, err)

			// Verify values
			for path, expected := range tt.expected {
				if path != "" {
					val, exists := cfg.Get(path)
					assert.True(t, exists, "Path %s should exist", path)
					assert.Equal(t, expected, val)
				}
			}
		})
	}

	t.Run("InvalidKeySegment", func(t *testing.T) {
		result, err := parseArgs([]string{"--invalid!key=value"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid command-line key segment")
		assert.Nil(t, result)
	})
}

// TestLoadWithOptions tests complete loading with multiple sources
func TestLoadWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")
	os.WriteFile(configFile, []byte(`
[server]
host = "filehost"
port = 8080
`), 0644)

	os.Setenv("TEST_SERVER_HOST", "envhost")
	os.Setenv("TEST_SERVER_PORT", "9090")
	defer func() {
		os.Unsetenv("TEST_SERVER_HOST")
		os.Unsetenv("TEST_SERVER_PORT")
	}()

	cfg := New()
	cfg.Register("server.host", "defaulthost")
	cfg.Register("server.port", 3000)

	args := []string{"--server.port=7070"}

	opts := LoadOptions{
		Sources:   []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault},
		EnvPrefix: "TEST_",
	}

	err := cfg.LoadWithOptions(configFile, args, opts)
	require.NoError(t, err)

	// CLI should win
	port, _ := cfg.Get("server.port")
	assert.Equal(t, "7070", port)

	// ENV should win over file
	host, _ := cfg.Get("server.host")
	assert.Equal(t, "envhost", host)

	// Test source inspection
	sources := cfg.GetSources("server.port")
	assert.Equal(t, "7070", sources[SourceCLI])
	assert.Equal(t, "9090", sources[SourceEnv])
	assert.Equal(t, int64(8080), sources[SourceFile])
}

// TestAtomicSave tests atomic file saving
func TestAtomicSave(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := New()
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", 8080)
	cfg.Register("database.url", "postgres://localhost/db")

	// Set some values
	cfg.Set("server.host", "savehost")
	cfg.Set("server.port", 9999)

	t.Run("SaveCurrentState", func(t *testing.T) {
		savePath := filepath.Join(tmpDir, "saved.toml")
		err := cfg.Save(savePath)
		require.NoError(t, err)

		// Verify file exists and is readable
		content, err := os.ReadFile(savePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "savehost")
		assert.Contains(t, string(content), "9999")

		// Load into new config to verify
		cfg2 := New()
		cfg2.Register("server.host", "")
		cfg2.Register("server.port", 0)
		err = cfg2.LoadFile(savePath)
		require.NoError(t, err)

		host, _ := cfg2.Get("server.host")
		assert.Equal(t, "savehost", host)
	})

	t.Run("SaveSpecificSource", func(t *testing.T) {
		cfg.SetSource("server.host", SourceEnv, "envhost")
		cfg.SetSource("server.port", SourceEnv, "7777")
		cfg.SetSource("server.port", SourceFile, "6666")

		savePath := filepath.Join(tmpDir, "env-only.toml")
		err := cfg.SaveSource(savePath, SourceEnv)
		require.NoError(t, err)

		content, err := os.ReadFile(savePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "envhost")
		assert.Contains(t, string(content), "7777")
		assert.NotContains(t, string(content), "6666")
	})

	t.Run("SaveToNonExistentDirectory", func(t *testing.T) {
		savePath := filepath.Join(tmpDir, "new", "dir", "config.toml")
		err := cfg.Save(savePath)
		require.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(savePath)
		assert.NoError(t, err)
	})
}

// TestExportEnv tests environment variable export
func TestExportEnv(t *testing.T) {
	cfg := New()
	cfg.Register("server.host", "defaulthost")
	cfg.Register("server.port", 8080)
	cfg.Register("feature.enabled", false)

	// Only export non-default values
	cfg.Set("server.host", "exporthost")
	cfg.Set("feature.enabled", true)

	exports := cfg.ExportEnv("APP_")

	assert.Len(t, exports, 2)
	assert.Equal(t, "exporthost", exports["APP_SERVER_HOST"])
	assert.Equal(t, "true", exports["APP_FEATURE_ENABLED"])
	assert.NotContains(t, exports, "APP_SERVER_PORT") // Still default
}

// splitEnvVar splits environment variable into key and value
func splitEnvVar(env string) []string {
	parts := make([]string, 2)
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			parts[0] = env[:i]
			parts[1] = env[i+1:]
			return parts
		}
	}
	return []string{env}
}