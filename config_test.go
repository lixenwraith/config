// FILE: lixenwraith/config/config_test.go
package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestConfigCreation tests various config creation patterns
func TestConfigCreation(t *testing.T) {
	t.Run("NewWithDefaultOptions", func(t *testing.T) {
		cfg := New()
		if got := cfg; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		if got := cfg.items; got == nil {
			t.Errorf("got %v, want non-nil", got)
		}
		checkEqual(t, cfg.options.Sources, []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault})
	})

	t.Run("NewWithCustomOptions", func(t *testing.T) {
		opts := LoadOptions{
			Sources:   []Source{SourceEnv, SourceFile, SourceDefault},
			EnvPrefix: "MYAPP_",
		}
		cfg := NewWithOptions(opts)
		if got := cfg; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, cfg.options.Sources, opts.Sources)
		checkEqual(t, cfg.options.EnvPrefix, "MYAPP_")
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
				checkErrorContains(t, err, tt.errorMsg)
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				val, exists := cfg.Get(tt.path)
				if !exists {
					t.Errorf("exists should be true")
				}
				checkEqual(t, val, tt.defaultVal)
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
		mustNoError(t, err)

		// Verify paths registered with TOML tags
		paths := cfg.GetRegisteredPaths("")
		if !paths["name"] {
			t.Errorf("paths[\"name\"] should be true")
		}
		if !paths["db.host"] {
			t.Errorf("paths[\"db.host\"] should be true")
		}
		if !paths["db.port"] {
			t.Errorf("paths[\"db.port\"] should be true")
		}
		if !paths["db.max_connections"] {
			t.Errorf("paths[\"db.max_connections\"] should be true")
		}
		if !paths["db.timeout"] {
			t.Errorf("paths[\"db.timeout\"] should be true")
		}
		if !paths["db.debug"] {
			t.Errorf("paths[\"db.debug\"] should be true")
		}
		if !paths["tags"] {
			t.Errorf("paths[\"tags\"] should be true")
		}
		if !paths["metadata"] {
			t.Errorf("paths[\"metadata\"] should be true")
		}

		// Verify default values
		val, _ := cfg.Get("db.timeout")
		checkEqual(t, val, 30*time.Second)
	})

	t.Run("RejectJSONTags", func(t *testing.T) {
		cfg := New()
		if err := cfg.RegisterStructWithTags("", defaultConfig, "json"); err == nil {
			t.Fatalf("expected an error")
		}
		if got := len(cfg.GetRegisteredPaths()); got != 0 {
			t.Fatalf("length = %d, want %d", got, 0)
		}
	})

	t.Run("UnsupportedTag", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStructWithTags("", defaultConfig, "xml")
		checkErrorContains(t, err, "unsupported tag name")
	})

	t.Run("WithPrefix", func(t *testing.T) {
		cfg := New()
		err := cfg.RegisterStruct("server", defaultConfig)
		mustNoError(t, err)

		paths := cfg.GetRegisteredPaths("server.")
		if !paths["server.name"] {
			t.Errorf("paths[\"server.name\"] should be true")
		}
		if !paths["server.db.host"] {
			t.Errorf("paths[\"server.db.host\"] should be true")
		}
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
	checkEqual(t, val, "from-cli")

	// Remove CLI value
	cfg.ResetSource(SourceCLI)
	val, _ = cfg.Get("test.value")
	checkEqual(t, val, "from-env")

	// Change precedence
	cfg.SetLoadOptions(LoadOptions{
		Sources: []Source{SourceFile, SourceEnv, SourceCLI, SourceDefault},
	})
	val, _ = cfg.Get("test.value")
	checkEqual(t, val, "from-file")

	// Test GetSources
	sources := cfg.GetSources("test.value")
	checkEqual(t, sources[SourceFile], "from-file")
	checkEqual(t, sources[SourceEnv], "from-env")
}

// TestSetPrecedence tests runtime precedence switching
func TestSetPrecedence(t *testing.T) {
	t.Run("BasicPrecedenceSwitch", func(t *testing.T) {
		cfg := New()
		cfg.Register("test.value", "default")

		// Set different values in each source
		cfg.SetSource(SourceFile, "test.value", "from-file")
		cfg.SetSource(SourceEnv, "test.value", "from-env")
		cfg.SetSource(SourceCLI, "test.value", "from-cli")

		// Default precedence: CLI > Env > File > Default
		val, _ := cfg.Get("test.value")
		checkEqual(t, val, "from-cli")

		// Switch to File > CLI > Env > Default
		err := cfg.SetPrecedence(SourceFile, SourceCLI, SourceEnv, SourceDefault)
		mustNoError(t, err)

		val, _ = cfg.Get("test.value")
		checkEqual(t, val, "from-file")

		// Verify precedence was updated
		precedence := cfg.GetPrecedence()
		checkEqual(t, precedence, []Source{SourceFile, SourceCLI, SourceEnv, SourceDefault})
	})

	t.Run("NoPrecedenceChangeOptimization", func(t *testing.T) {
		cfg := New()
		cfg.Register("test.value", "default")
		cfg.SetSource(SourceFile, "test.value", "from-file")

		// Set same precedence
		initialPrecedence := cfg.GetPrecedence()
		err := cfg.SetPrecedence(initialPrecedence...)
		mustNoError(t, err)

		// Should be no-op, verify by checking version
		version1 := cfg.version.Load()
		err = cfg.SetPrecedence(initialPrecedence...)
		mustNoError(t, err)
		version2 := cfg.version.Load()

		checkEqual(t, version2, version1, "Version should not change on no-op")
	})

	t.Run("AutoAddDefaultSource", func(t *testing.T) {
		cfg := New()

		// Set precedence without SourceDefault
		err := cfg.SetPrecedence(SourceCLI, SourceFile, SourceEnv)
		mustNoError(t, err)

		// SourceDefault should be auto-appended
		precedence := cfg.GetPrecedence()
		checkEqual(t, precedence, []Source{SourceCLI, SourceFile, SourceEnv, SourceDefault})
	})

	t.Run("InvalidSourceError", func(t *testing.T) {
		cfg := New()

		// Try to set invalid source
		err := cfg.SetPrecedence("invalid", SourceFile)
		checkErrorContains(t, err, "invalid source")

		// Precedence should remain unchanged
		precedence := cfg.GetPrecedence()
		checkEqual(t, precedence, []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault})
	})

	t.Run("PrecedenceChangeNotifications", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.toml")
		writeTestFile(t, configFile, []byte(`value = "from-file"`), 0644)

		cfg := New()
		cfg.Register("value", "default")
		cfg.LoadFile(configFile)
		cfg.SetSource(SourceCLI, "value", "from-cli")

		// Enable watching
		opts := WatchOptions{
			PollInterval: 100 * time.Millisecond,
			Debounce:     50 * time.Millisecond,
		}
		cfg.AutoUpdateWithOptions(opts)
		defer cfg.StopAutoUpdate()

		// Start watching for changes
		changes := cfg.Watch()

		// Change precedence - should trigger notification
		mustNoError(t, cfg.SetPrecedence(SourceFile, SourceCLI, SourceEnv, SourceDefault))

		// Wait for precedence change notification
		select {
		case change := <-changes:
			checkEqual(t, change, "precedence:value")

			// Verify value changed
			val, _ := cfg.Get("value")
			checkEqual(t, val, "from-file")
		case <-time.After(500 * time.Millisecond):
			t.Error("Timeout waiting for precedence change notification")
		}
	})

	t.Run("MultipleValuesAffected", func(t *testing.T) {
		cfg := New()
		paths := []string{"app.name", "app.version", "app.debug"}

		for _, path := range paths {
			cfg.Register(path, "default-"+path)
			cfg.SetSource(SourceFile, path, "file-"+path)
			cfg.SetSource(SourceEnv, path, "env-"+path)
		}

		// Initial state: Env wins
		cfg.SetPrecedence(SourceEnv, SourceFile, SourceDefault)
		for _, path := range paths {
			val, _ := cfg.Get(path)
			checkEqual(t, val, "env-"+path)
		}

		// Switch: File wins
		err := cfg.SetPrecedence(SourceFile, SourceEnv, SourceDefault)
		mustNoError(t, err)

		for _, path := range paths {
			val, _ := cfg.Get(path)
			checkEqual(t, val, "file-"+path)
		}
	})

	t.Run("ConcurrentPrecedenceChanges", func(t *testing.T) {
		cfg := New()
		cfg.Register("test", "default")
		cfg.SetSource(SourceFile, "test", "file")
		cfg.SetSource(SourceCLI, "test", "cli")

		var wg sync.WaitGroup
		errors := make(chan error, 20)

		// Multiple goroutines changing precedence
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				var sources []Source
				if id%2 == 0 {
					sources = []Source{SourceFile, SourceCLI, SourceDefault}
				} else {
					sources = []Source{SourceCLI, SourceFile, SourceDefault}
				}

				if err := cfg.SetPrecedence(sources...); err != nil {
					errors <- err
				}
			}(i)
		}

		// Concurrent reads during precedence changes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				val, exists := cfg.Get("test")
				if !exists {
					errors <- fmt.Errorf("value not found during concurrent access")
				}
				// Value should be either "file" or "cli"
				if val != "file" && val != "cli" {
					errors <- fmt.Errorf("unexpected value: %v", val)
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		if got := len(errs); got != 0 {
			t.Errorf("length = %d, want %d: %s", got, 0, "Concurrent precedence changes should not produce errors")
		}
	})
}

// TestPrecedenceWithAutoUpdate verifies no conflicts between precedence and auto-update
func TestPrecedenceWithAutoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.toml")

	// Initial file content
	writeTestFile(t, configFile, []byte(`
	server = "file-server-1"
	port = 8080
	`), 0644)

	cfg := New()
	cfg.Register("server", "default-server")
	cfg.Register("port", 0)

	// Load with CLI override
	cfg.LoadFile(configFile)
	cfg.SetSource(SourceCLI, "server", "cli-server")

	// CLI wins initially
	val, _ := cfg.Get("server")
	checkEqual(t, val, "cli-server")

	// Enable auto-update
	opts := WatchOptions{
		PollInterval: 100 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(opts)
	defer cfg.StopAutoUpdate()

	// Switch precedence to File > CLI
	err := cfg.SetPrecedence(SourceFile, SourceCLI, SourceEnv, SourceDefault)
	mustNoError(t, err)

	// File should now win
	val, _ = cfg.Get("server")
	checkEqual(t, val, "file-server-1")

	changes := cfg.Watch()
	// Update file
	writeTestFile(t, configFile, []byte(`
	server = "file-server-2"
	port = 9090
	`), 0644)

	// The watcher publishes registered-path events after updating the snapshot.
	_ = receiveWatchEvent(t, changes)

	// File still wins with new value
	val, _ = cfg.Get("server")
	checkEqual(t, val, "file-server-2")

	// CLI value is preserved but not active
	cliVal, exists := cfg.GetSource("server", SourceCLI)
	if !exists {
		t.Errorf("exists should be true")
	}
	checkEqual(t, cliVal, "cli-server")

	// Switch back to CLI > File
	err = cfg.SetPrecedence(SourceCLI, SourceFile, SourceEnv, SourceDefault)
	mustNoError(t, err)

	// CLI wins again
	val, _ = cfg.Get("server")
	checkEqual(t, val, "cli-server")
}

// TestTypeConversion tests automatic type conversion through checked decoding
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
	mustNoError(t, err)

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
	mustNoError(t, cfg.SetSource(SourceEnv, "ints", "7,8,9"))

	// Scan into struct
	var result TestConfig
	err = cfg.Scan(&result)
	mustNoError(t, err)

	checkEqual(t, result.IntValue, int64(100))
	checkEqual(t, result.FloatValue, 2.718)
	checkEqual(t, result.BoolValue, false)
	checkEqual(t, result.Duration, 90*time.Second)
	checkEqual(t, result.Time.Format(time.RFC3339), "2024-12-25T10:00:00Z")
	checkEqual(t, result.IP.String(), "192.168.1.1")
	checkEqual(t, result.IPNet.String(), "10.0.0.0/8")
	checkEqual(t, result.URL.String(), "https://example.com:8080/path")
	checkEqual(t, result.StringSlice, []string{"x", "y", "z"})
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
				value := id*100 + j
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
				value := id*100 + j
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
	if got := len(errs); got != 0 {
		t.Errorf("length = %d, want %d: %s", got, 0, "Concurrent access should not produce errors")
	}
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
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		_, exists := cfg.Get("server.port")
		if exists {
			t.Errorf("exists should be false")
		}

		// Other paths should remain
		_, exists = cfg.Get("server.host")
		if !exists {
			t.Errorf("exists should be true")
		}
	})

	t.Run("UnregisterParentPath", func(t *testing.T) {
		err := cfg.Unregister("server.tls")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// All child paths should be removed
		_, exists := cfg.Get("server.tls.enabled")
		if exists {
			t.Errorf("exists should be false")
		}
		_, exists = cfg.Get("server.tls.cert")
		if exists {
			t.Errorf("exists should be false")
		}

		// Sibling paths should remain
		_, exists = cfg.Get("server.host")
		if !exists {
			t.Errorf("exists should be true")
		}
	})

	t.Run("UnregisterNonExistentPath", func(t *testing.T) {
		err := cfg.Unregister("nonexistent.path")
		checkErrorContains(t, err, "path not registered")
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
		if exists {
			t.Errorf("exists should be false")
		}

		// Other sources should remain
		val, exists := cfg.GetSource("test1", SourceFile)
		if !exists {
			t.Errorf("exists should be true")
		}
		checkEqual(t, val, "file1")
	})

	t.Run("ResetAll", func(t *testing.T) {
		cfg.Reset()

		// All values should revert to defaults
		val1, _ := cfg.Get("test1")
		val2, _ := cfg.Get("test2")
		checkEqual(t, val1, "default1")
		checkEqual(t, val2, "default2")

		// Source values should be cleared
		sources := cfg.GetSources("test1")
		if got := len(sources); got != 0 {
			t.Errorf("length = %d, want %d", got, 0)
		}
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
	if err == nil {
		t.Errorf("expected an error")
	}
	checkEqual(t, err, ErrValueSize)
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
		if got := len(all); got != len(paths) {
			t.Errorf("length = %d, want %d", got, len(paths))
		}
		for _, path := range paths {
			if !(all[path]) {
				t.Errorf("all[path] should be true")
			}
		}
	})

	t.Run("GetPathsWithPrefix", func(t *testing.T) {
		serverPaths := cfg.GetRegisteredPaths("server.")
		if got := len(serverPaths); got != 3 {
			t.Errorf("length = %d, want %d", got, 3)
		}
		if !serverPaths["server.host"] {
			t.Errorf("serverPaths[\"server.host\"] should be true")
		}
		if !serverPaths["server.port"] {
			t.Errorf("serverPaths[\"server.port\"] should be true")
		}
		if !serverPaths["server.tls.enabled"] {
			t.Errorf("serverPaths[\"server.tls.enabled\"] should be true")
		}
	})

	t.Run("GetPathsWithDefaults", func(t *testing.T) {
		defaults := cfg.GetRegisteredPathsWithDefaults("database.")
		if got := len(defaults); got != 2 {
			t.Errorf("length = %d, want %d", got, 2)
		}
		if _, ok := defaults["database.host"]; !ok {
			t.Errorf("unexpected map membership for %q", "database.host")
		}
		if _, ok := defaults["database.port"]; !ok {
			t.Errorf("unexpected map membership for %q", "database.port")
		}
	})
}
