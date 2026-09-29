// FILE: lixenwraith/config/loader_test.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		writeTestFile(t, configFile, []byte(content), 0644)

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
		mustNoError(t, err)

		// Verify loaded values
		host, _ := cfg.Get("server.host")
		checkEqual(t, host, "example.com")

		port, _ := cfg.Get("server.port")
		checkEqual(t, port, int64(9000))

		enabled, _ := cfg.Get("server.enabled")
		checkEqual(t, enabled, true)

		cert, _ := cfg.Get("server.tls.cert")
		checkEqual(t, cert, "/path/to/cert.pem")

		// Arrays are loaded as []any
		connections, _ := cfg.Get("database.connections")
		checkEqual(t, connections, []any{int64(1), int64(2), int64(3)})
	})

	t.Run("InvalidTOMLFile", func(t *testing.T) {
		configFile := filepath.Join(tmpDir, "invalid.toml")
		writeTestFile(t, configFile, []byte(`invalid = toml content`), 0644)

		cfg := New()
		err := cfg.LoadFile(configFile)
		checkErrorContains(t, err, "failed to parse TOML")
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		cfg := New()
		err := cfg.LoadFile("/non/existent/file.toml")
		if err == nil {
			t.Errorf("expected an error")
		}
		if err := err; !errors.Is(err, ErrConfigNotFound) {
			t.Errorf("error = %v, want errors.Is(_, %v)", err, ErrConfigNotFound)
		}
	})

	t.Run("UnregisteredPathsIgnored", func(t *testing.T) {
		configFile := filepath.Join(tmpDir, "extra.toml")
		writeTestFile(t, configFile, []byte(`
registered = "value"
unregistered = "ignored"
`), 0644)

		cfg := New()
		cfg.Register("registered", "")

		err := cfg.LoadFile(configFile)
		mustNoError(t, err)

		val, exists := cfg.Get("registered")
		if !exists {
			t.Errorf("exists should be true")
		}
		checkEqual(t, val, "value")

		_, exists = cfg.Get("unregistered")
		if exists {
			t.Errorf("exists should be false")
		}
	})
}

// TestEnvironmentLoading tests environment variable loading
func TestEnvironmentLoading(t *testing.T) {
	t.Run("DefaultEnvTransform", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "localhost")
		cfg.Register("server.port", 8080)
		cfg.Register("enable_debug", false)

		t.Setenv("APP_SERVER_HOST", "envhost")
		t.Setenv("APP_SERVER_PORT", "9090")
		t.Setenv("APP_ENABLE_DEBUG", "true")

		err := cfg.LoadEnv("APP_")
		mustNoError(t, err)

		host, _ := cfg.Get("server.host")
		checkEqual(t, host, "envhost")

		port, _ := cfg.Get("server.port")
		checkEqual(t, port, "9090") // String from env

		debug, _ := cfg.Get("enable_debug")
		checkEqual(t, debug, "true") // String from env
	})

	t.Run("CustomEnvTransform", func(t *testing.T) {
		cfg := New()
		cfg.Register("db.host", "localhost")

		t.Setenv("DATABASE_HOSTNAME", "customhost")

		opts := LoadOptions{
			Sources: []Source{SourceEnv, SourceDefault},
			EnvTransform: func(path string) string {
				if path == "db.host" {
					return "DATABASE_HOSTNAME"
				}
				return path
			},
		}

		err := cfg.loadWithOptions("", nil, opts)
		mustNoError(t, err)

		host, _ := cfg.Get("db.host")
		checkEqual(t, host, "customhost")
	})

	t.Run("EnvWhitelist", func(t *testing.T) {
		cfg := New()
		cfg.Register("allowed.path", "default1")
		cfg.Register("blocked.path", "default2")

		t.Setenv("ALLOWED_PATH", "env1")
		t.Setenv("BLOCKED_PATH", "env2")

		opts := LoadOptions{
			Sources:      []Source{SourceEnv, SourceDefault},
			EnvWhitelist: map[string]bool{"allowed.path": true},
		}

		err := cfg.loadWithOptions("", nil, opts)
		mustNoError(t, err)

		allowed, _ := cfg.Get("allowed.path")
		checkEqual(t, allowed, "env1")

		blocked, _ := cfg.Get("blocked.path")
		checkEqual(t, blocked, "default2") // Should not load from env
	})

	t.Run("DiscoverEnv", func(t *testing.T) {
		cfg := New()
		cfg.Register("test.one", "")
		cfg.Register("test.two", "")
		cfg.Register("other.value", "")

		t.Setenv("PREFIX_TEST_ONE", "value1")
		t.Setenv("PREFIX_TEST_TWO", "value2")
		t.Setenv("PREFIX_OTHER_VALUE", "value3")
		t.Setenv("UNRELATED_VAR", "ignored")

		discovered := cfg.DiscoverEnv("PREFIX_")
		if got := len(discovered); got != 3 {
			t.Errorf("length = %d, want %d", got, 3)
		}
		checkEqual(t, discovered["test.one"], "PREFIX_TEST_ONE")
		checkEqual(t, discovered["test.two"], "PREFIX_TEST_TWO")
		checkEqual(t, discovered["other.value"], "PREFIX_OTHER_VALUE")
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
			mustNoError(t, err)

			// Verify values
			for path, expected := range tt.expected {
				if path != "" {
					val, exists := cfg.Get(path)
					if !exists {
						t.Errorf("exists should be true: %s", fmt.Sprintf("Path %s should exist", path))
					}
					checkEqual(t, val, expected)
				}
			}
		})
	}

	t.Run("InvalidKeySegment", func(t *testing.T) {
		result, err := parseArgs([]string{"--invalid!key=value"})
		checkErrorContains(t, err, "invalid command-line key segment")
		if got := result; got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

// TestLoadWithOptions tests complete loading with multiple sources
func TestLoadWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")
	writeTestFile(t, configFile, []byte(`
[server]
host = "filehost"
port = 8080
`), 0644)

	t.Setenv("TEST_SERVER_HOST", "envhost")
	t.Setenv("TEST_SERVER_PORT", "9090")

	cfg := New()
	cfg.Register("server.host", "defaulthost")
	cfg.Register("server.port", 3000)

	args := []string{"--server.port=7070"}

	opts := LoadOptions{
		Sources:   []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault},
		EnvPrefix: "TEST_",
	}

	err := cfg.loadWithOptions(configFile, args, opts)
	mustNoError(t, err)

	// CLI should win
	port, _ := cfg.Get("server.port")
	checkEqual(t, port, "7070")

	// ENV should win over file
	host, _ := cfg.Get("server.host")
	checkEqual(t, host, "envhost")

	// Test source inspection
	sources := cfg.GetSources("server.port")
	checkEqual(t, sources[SourceCLI], "7070")
	checkEqual(t, sources[SourceEnv], "9090")
	checkEqual(t, sources[SourceFile], int64(8080))
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
		mustNoError(t, err)

		// Verify file exists and is readable
		content, err := os.ReadFile(savePath)
		mustNoError(t, err)
		if got := string(content); !strings.Contains(got, "savehost") {
			t.Errorf("unexpected substring membership: %q in %q", "savehost", got)
		}
		if got := string(content); !strings.Contains(got, "9999") {
			t.Errorf("unexpected substring membership: %q in %q", "9999", got)
		}

		// Load into new config to verify
		cfg2 := New()
		cfg2.Register("server.host", "")
		cfg2.Register("server.port", 0)
		err = cfg2.LoadFile(savePath)
		mustNoError(t, err)

		host, _ := cfg2.Get("server.host")
		checkEqual(t, host, "savehost")
	})

	t.Run("SaveSpecificSource", func(t *testing.T) {
		cfg.SetSource(SourceEnv, "server.host", "envhost")
		cfg.SetSource(SourceEnv, "server.port", "7777")
		cfg.SetSource(SourceFile, "server.port", "6666")

		savePath := filepath.Join(tmpDir, "env-only.toml")
		err := cfg.SaveSource(savePath, SourceEnv)
		mustNoError(t, err)

		content, err := os.ReadFile(savePath)
		mustNoError(t, err)
		if got := string(content); !strings.Contains(got, "envhost") {
			t.Errorf("unexpected substring membership: %q in %q", "envhost", got)
		}
		if got := string(content); !strings.Contains(got, "7777") {
			t.Errorf("unexpected substring membership: %q in %q", "7777", got)
		}
		if got := string(content); strings.Contains(got, "6666") {
			t.Errorf("unexpected substring membership: %q in %q", "6666", got)
		}
	})

	t.Run("SaveToNonExistentDirectory", func(t *testing.T) {
		savePath := filepath.Join(tmpDir, "new", "dir", "config.toml")
		err := cfg.Save(savePath)
		mustNoError(t, err)

		// Verify file was created
		_, err = os.Stat(savePath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
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

	if got := len(exports); got != 2 {
		t.Errorf("length = %d, want %d", got, 2)
	}
	checkEqual(t, exports["APP_SERVER_HOST"], "exporthost")
	checkEqual(t, exports["APP_FEATURE_ENABLED"], "true")
	if _, ok := exports["APP_SERVER_PORT"]; ok {
		t.Errorf("unexpected map membership for %q", "APP_SERVER_PORT")
	} // Still default
}
