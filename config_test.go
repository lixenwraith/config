// FILE: lixenwraith/config/config_test.go
package config

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigCreation tests various config creation patterns
func TestConfigCreation(t *testing.T) {
	t.Run("NewWithDefaultOptions", func(t *testing.T) {
		cfg := New()
		require.NotNil(t, cfg)
		assert.NotNil(t, cfg.items)
		assert.Equal(t, []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault}, cfg.options.Sources)
	})

	t.Run("NewWithCustomOptions", func(t *testing.T) {
		opts := LoadOptions{
			Sources:   []Source{SourceEnv, SourceFile, SourceDefault},
			EnvPrefix: "MYAPP_",
			LoadMode:  LoadModeReplace,
		}
		cfg := NewWithOptions(opts)
		require.NotNil(t, cfg)
		assert.Equal(t, opts.Sources, cfg.options.Sources)
		assert.Equal(t, "MYAPP_", cfg.options.EnvPrefix)
	})
}

// TestPathRegistration tests path registration edge cases
func TestPathRegistration(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		defaultVal  any
		expectError bool
		errorMsg    string
	}{
		{"ValidSimplePath", "port", 8080, false, ""},
		{"ValidNestedPath", "server.host.name", "localhost", false, ""},
		{"EmptyPath", "", nil, true, "registration path cannot be empty"},
		{"InvalidCharacter", "server.port!", 8080, true, "invalid path segment"},
		{"InvalidDot", "server..port", 8080, true, "invalid path segment"},
		{"LeadingDot", ".server.port", 8080, true, "invalid path segment"},
		{"TrailingDot", "server.port.", 8080, true, "invalid path segment"},
		{"ValidUnderscore", "server_config.max_connections", 100, false, ""},
		{"ValidDash", "feature-flags.enable-debug", false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := New()
			err := cfg.Register(tt.path, tt.defaultVal)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				val, exists := cfg.Get(tt.path)
				assert.True(t, exists)
				assert.Equal(t, tt.defaultVal, val)
			}
		})
	}
}

// TestComplexStructRegistration tests struct registration with various tag types
func TestComplexStructRegistration(t *testing.T) {
	type DatabaseConfig struct {
		Host        string        `toml:"host" json:"db_host" yaml:"dbHost"`
		Port        int           `toml:"port" json:"db_port" yaml:"dbPort"`
		MaxConns    int           `toml:"max_connections"`
		Timeout     time.Duration `toml:"timeout"`
		EnableDebug bool          `toml:"debug" env:"DB_DEBUG"`
	}

	type ServerConfig struct {
		Name     string         `toml:"name" json:"name"`
		Database DatabaseConfig `toml:"db" json:"db"`
		Tags     []string       `toml:"tags" json:"tags"`
		Metadata map[string]any `toml:"metadata" json:"metadata"`
	}

	defaultConfig := &ServerConfig{
		Name: "test-server",
		Database: DatabaseConfig{
			Host:        "localhost",
			Port:        5432,
			MaxConns:    100,
			Timeout:     30 * time.Second,
			EnableDebug: false,
		},
		Tags:     []string{"test", "development"},
		Metadata: map[string]any{"version": "1.0"},
	}

	t.Run("TOMLTags", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStruct("", defaultConfig)
		require.NoError(t, err)

		// Verify paths registered with TOML tags
		paths := cfg.GetRegisteredPaths("")
		assert.True(t, paths["name"])
		assert.True(t, paths["db.host"])
		assert.True(t, paths["db.port"])
		assert.True(t, paths["db.max_connections"])
		assert.True(t, paths["db.timeout"])
		assert.True(t, paths["db.debug"])
		assert.True(t, paths["tags"])
		assert.True(t, paths["metadata"])

		// Verify default values
		val, _ := cfg.Get("db.timeout")
		assert.Equal(t, 30*time.Second, val)
	})

	t.Run("JSONTags", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStructWithTags("", defaultConfig, "json")
		require.NoError(t, err)

		// JSON tags should create different paths
		paths := cfg.GetRegisteredPaths("")
		assert.True(t, paths["db.db_host"])
		assert.True(t, paths["db.db_port"])
	})

	t.Run("UnsupportedTag", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStructWithTags("", defaultConfig, "xml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported tag name")
	})

	t.Run("WithPrefix", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStruct("server", defaultConfig)
		require.NoError(t, err)

		paths := cfg.GetRegisteredPaths("server.")
		assert.True(t, paths["server.name"])
		assert.True(t, paths["server.db.host"])
	})
}

// TestSourcePrecedence tests configuration source precedence
func TestSourcePrecedence(t *testing.T) {
	cfg := New()
	cfg.Register("test.value", "default")

	// Set values in different sources
	cfg.SetSource(SourceFile, "test.value", "from-file")
	cfg.SetSource(SourceEnv, "test.value", "from-env")
	cfg.SetSource(SourceCLI, "test.value", "from-cli")

	// Default precedence: CLI > Env > File > Default
	val, _ := cfg.Get("test.value")
	assert.Equal(t, "from-cli", val)

	// Remove CLI value
	cfg.ResetSource(SourceCLI)
	val, _ = cfg.Get("test.value")
	assert.Equal(t, "from-env", val)

	// Change precedence
	err := cfg.SetLoadOptions(LoadOptions{
		Sources: []Source{SourceFile, SourceEnv, SourceCLI, SourceDefault},
	})
	require.NoError(t, err)
	val, _ = cfg.Get("test.value")
	assert.Equal(t, "from-file", val)

	// Test GetSources
	sources := cfg.GetSources("test.value")
	assert.Equal(t, "from-file", sources[SourceFile])
	assert.Equal(t, "from-env", sources[SourceEnv])
}

// TestTypeConversion tests automatic type conversion through mapstructure
func TestTypeConversion(t *testing.T) {
	type TestConfig struct {
		IntValue    int64         `toml:"int"`
		FloatValue  float64       `toml:"float"`
		BoolValue   bool          `toml:"bool"`
		Duration    time.Duration `toml:"duration"`
		Time        time.Time     `toml:"time"`
		IP          net.IP        `toml:"ip"`
		IPNet       *net.IPNet    `toml:"ipnet"`
		URL         *url.URL      `toml:"url"`
		StringSlice []string      `toml:"strings"`
		IntSlice    []int         `toml:"ints"`
	}

	cfg := New()
	defaults := &TestConfig{
		IntValue:    42,
		FloatValue:  3.14,
		BoolValue:   true,
		Duration:    5 * time.Second,
		Time:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		IP:          net.ParseIP("127.0.0.1"),
		StringSlice: []string{"a", "b"},
		IntSlice:    []int{1, 2, 3},
	}

	err := cfg.RegisterStruct("", defaults)
	require.NoError(t, err)

	// Test string conversions from environment
	cfg.SetSource(SourceEnv, "int", "100")
	cfg.SetSource(SourceEnv, "float", "2.718")
	cfg.SetSource(SourceEnv, "bool", "false")
	cfg.SetSource(SourceEnv, "duration", "1m30s")
	cfg.SetSource(SourceEnv, "time", "2024-12-25T10:00:00Z")
	cfg.SetSource(SourceEnv, "ip", "192.168.1.1")
	cfg.SetSource(SourceEnv, "ipnet", "10.0.0.0/8")
	cfg.SetSource(SourceEnv, "url", "https://example.com:8080/path")
	cfg.SetSource(SourceEnv, "strings", "x,y,z")
	// cfg.SetSource("ints", SourceEnv, "7,8,9") // failure due to mapstructure limitation

	// Scan into struct
	var result TestConfig
	err = cfg.Scan(&result)
	require.NoError(t, err)

	assert.Equal(t, int64(100), result.IntValue)
	assert.Equal(t, 2.718, result.FloatValue)
	assert.Equal(t, false, result.BoolValue)
	assert.Equal(t, 90*time.Second, result.Duration)
	assert.Equal(t, "2024-12-25T10:00:00Z", result.Time.Format(time.RFC3339))
	assert.Equal(t, "192.168.1.1", result.IP.String())
	assert.Equal(t, "10.0.0.0/8", result.IPNet.String())
	assert.Equal(t, "https://example.com:8080/path", result.URL.String())
	assert.Equal(t, []string{"x", "y", "z"}, result.StringSlice)
	// Note: String to int slice conversion through env requires handling in the test
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	cfg := New()

	// Register paths
	for i := 0; i < 100; i++ {
		cfg.Register(fmt.Sprintf("path%d", i), i)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 1000)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				path := fmt.Sprintf("path%d", j)
				if _, exists := cfg.Get(path); !exists {
					errors <- fmt.Errorf("reader %d: path %s not found", id, path)
				}
			}
		}(i)
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				path := fmt.Sprintf("path%d", j)
				value := fmt.Sprintf("writer%d-value%d", id, j)
				if err := cfg.Set(path, value); err != nil {
					errors <- fmt.Errorf("writer %d: %v", id, err)
				}
			}
		}(i)
	}

	// Concurrent source changes
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sources := []Source{SourceFile, SourceEnv, SourceCLI}
			for j := 0; j < 50; j++ {
				path := fmt.Sprintf("path%d", j)
				source := sources[j%len(sources)]
				value := fmt.Sprintf("source%d-value%d", id, j)
				if err := cfg.SetSource(source, path, value); err != nil {
					errors <- fmt.Errorf("source writer %d: %v", id, err)
				}
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
	assert.Empty(t, errs, "Concurrent access should not produce errors")
}

// TestUnregister tests path unregistration
func TestUnregister(t *testing.T) {
	cfg := New()

	// Register nested paths
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", 8080)
	cfg.Register("server.tls.enabled", true)
	cfg.Register("server.tls.cert", "/path/to/cert")
	cfg.Register("database.host", "dbhost")

	t.Run("UnregisterSinglePath", func(t *testing.T) {
		err := cfg.Unregister("server.port")
		assert.NoError(t, err)
		_, exists := cfg.Get("server.port")
		assert.False(t, exists)

		// Other paths should remain
		_, exists = cfg.Get("server.host")
		assert.True(t, exists)
	})

	t.Run("UnregisterParentPath", func(t *testing.T) {
		err := cfg.Unregister("server.tls")
		assert.NoError(t, err)

		// All child paths should be removed
		_, exists := cfg.Get("server.tls.enabled")
		assert.False(t, exists)
		_, exists = cfg.Get("server.tls.cert")
		assert.False(t, exists)

		// Sibling paths should remain
		_, exists = cfg.Get("server.host")
		assert.True(t, exists)
	})

	t.Run("UnregisterNonExistentPath", func(t *testing.T) {
		err := cfg.Unregister("nonexistent.path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path not registered")
	})
}

// TestResetFunctionality tests reset operations
func TestResetFunctionality(t *testing.T) {
	cfg := New()
	cfg.Register("test1", "default1")
	cfg.Register("test2", "default2")

	// Set values in different sources
	cfg.SetSource(SourceFile, "test1", "file1")
	cfg.SetSource(SourceEnv, "test1", "env1")
	cfg.SetSource(SourceCLI, "test2", "cli2")

	t.Run("ResetSingleSource", func(t *testing.T) {
		cfg.ResetSource(SourceEnv)

		// Env value should be gone
		_, exists := cfg.GetSource("test1", SourceEnv)
		assert.False(t, exists)

		// Other sources should remain
		val, exists := cfg.GetSource("test1", SourceFile)
		assert.True(t, exists)
		assert.Equal(t, "file1", val)
	})

	t.Run("ResetAll", func(t *testing.T) {
		cfg.Reset()

		// All values should revert to defaults
		val1, _ := cfg.Get("test1")
		val2, _ := cfg.Get("test2")
		assert.Equal(t, "default1", val1)
		assert.Equal(t, "default2", val2)

		// Source values should be cleared
		sources := cfg.GetSources("test1")
		assert.Empty(t, sources)
	})
}

// TestValueSizeLimit tests the MaxValueSize constraint
func TestValueSizeLimit(t *testing.T) {
	cfg := New()
	cfg.Register("test", "")

	// Create a value larger than MaxValueSize
	largeValue := make([]byte, MaxValueSize+1)
	for i := range largeValue {
		largeValue[i] = 'x'
	}

	err := cfg.Set("test", string(largeValue))
	assert.Error(t, err)
	assert.Equal(t, ErrValueSize, err)
}

// TestGetRegisteredPaths tests path listing functionality
func TestGetRegisteredPaths(t *testing.T) {
	cfg := New()

	paths := []string{
		"server.host",
		"server.port",
		"server.tls.enabled",
		"database.host",
		"database.port",
		"cache.ttl",
	}

	for _, path := range paths {
		cfg.Register(path, "")
	}

	t.Run("GetAllPaths", func(t *testing.T) {
		all := cfg.GetRegisteredPaths("")
		assert.Len(t, all, len(paths))
		for _, path := range paths {
			assert.True(t, all[path])
		}
	})

	t.Run("GetPathsWithPrefix", func(t *testing.T) {
		serverPaths := cfg.GetRegisteredPaths("server.")
		assert.Len(t, serverPaths, 3)
		assert.True(t, serverPaths["server.host"])
		assert.True(t, serverPaths["server.port"])
		assert.True(t, serverPaths["server.tls.enabled"])
	})

	t.Run("GetPathsWithDefaults", func(t *testing.T) {
		defaults := cfg.GetRegisteredPathsWithDefaults("database.")
		assert.Len(t, defaults, 2)
		assert.Contains(t, defaults, "database.host")
		assert.Contains(t, defaults, "database.port")
	})
}