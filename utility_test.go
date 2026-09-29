// FILE: lixenwraith/config/utility_test.go
package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestQuickFunctions tests the utility Quick* functions
func TestQuickFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "quick.toml")
	writeTestFile(t, configFile, []byte(`
host = "quickhost"
port = 7777
`), 0644)

	type QuickConfig struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
		SSL  bool   `toml:"ssl"`
	}

	defaults := &QuickConfig{
		Host: "localhost",
		Port: 8080,
		SSL:  false,
	}

	t.Run("Quick", func(t *testing.T) {
		// Mock os.Args
		oldArgs := os.Args
		os.Args = []string{"cmd", "--port=9999"}
		t.Cleanup(func() { os.Args = oldArgs })

		cfg, err := Quick(defaults, "QUICK_", configFile)
		mustNoError(t, err)

		// CLI should override
		port, _ := cfg.Get("port")
		checkEqual(t, port, "9999")

		// File value
		host, _ := cfg.Get("host")
		checkEqual(t, host, "quickhost")
	})

	t.Run("QuickCustom", func(t *testing.T) {
		opts := LoadOptions{
			Sources:   []Source{SourceFile, SourceDefault}, // Only file and defaults
			EnvPrefix: "CUSTOM_",
		}

		cfg, err := QuickCustom(defaults, opts, configFile)
		mustNoError(t, err)

		// Should use file value
		port, _ := cfg.Get("port")
		checkEqual(t, port, int64(7777))
	})

	t.Run("MustQuickPanic", func(t *testing.T) {
		// Valid case - should not panic
		{
			cfg := MustQuick(defaults, "TEST_", configFile)
			if got := cfg; got == nil {
				t.Errorf("got %v, want non-nil", got)
			}
		}

		// Invalid struct - should panic
		mustPanic(t, func() {
			MustQuick("not-a-struct", "TEST_", configFile)
		})
	})

	t.Run("QuickTyped", func(t *testing.T) {
		target := &QuickConfig{
			Host: "typedhost",
			Port: 6666,
			SSL:  true,
		}

		cfg, err := QuickTyped(target, "TYPED_", configFile)
		mustNoError(t, err)

		// Should populate from file
		updated, err := cfg.AsStruct()
		mustNoError(t, err)

		typedCfg := updated.(*QuickConfig)
		checkEqual(t, typedCfg.Host, "quickhost")
		checkEqual(t, typedCfg.Port, 7777)
	})
}

// TestFlagGeneration tests flag generation and binding
func TestFlagGeneration(t *testing.T) {
	cfg := New()
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", 8080)
	cfg.Register("debug.enabled", false)
	cfg.Register("timeout", 30.5)
	cfg.Register("name", "app")
	cfg.Register("complex", map[string]any{"key": "value"})

	t.Run("GenerateFlags", func(t *testing.T) {
		fs := cfg.GenerateFlags()
		if got := fs; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}

		// Verify flags exist
		hostFlag := fs.Lookup("server.host")
		if got := hostFlag; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, hostFlag.DefValue, "localhost")

		portFlag := fs.Lookup("server.port")
		if got := portFlag; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, portFlag.DefValue, "8080")

		debugFlag := fs.Lookup("debug.enabled")
		if got := debugFlag; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, debugFlag.DefValue, "false")

		timeoutFlag := fs.Lookup("timeout")
		if got := timeoutFlag; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, timeoutFlag.DefValue, "30.5")
	})

	t.Run("BindFlags", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("server.host", "default", "")
		fs.Int("server.port", 8080, "")
		fs.Bool("debug.enabled", false, "")

		// Parse with test values
		err := fs.Parse([]string{"-server.host=flaghost", "-server.port=5555", "-debug.enabled"})
		mustNoError(t, err)

		// Bind to config
		err = cfg.BindFlags(fs)
		mustNoError(t, err)

		// Verify values were set
		host, _ := cfg.Get("server.host")
		checkEqual(t, host, "flaghost")

		port, _ := cfg.Get("server.port")
		checkEqual(t, port, "5555")

		debug, _ := cfg.Get("debug.enabled")
		checkEqual(t, debug, "true")
	})

	t.Run("BindFlagsError", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("unregistered.path", "value", "")
		fs.Parse([]string{"-unregistered.path=test"})

		err := cfg.BindFlags(fs)
		checkErrorContains(t, err, "unregistered flag")
	})
}

// TestValidation tests configuration validation
func TestValidation(t *testing.T) {
	cfg := New()
	cfg.Register("required.host", "")
	cfg.Register("required.port", 0)
	cfg.Register("optional.timeout", 30)

	t.Run("ValidationFails", func(t *testing.T) {
		err := cfg.Validate("required.host", "required.port")
		checkErrorContains(t, err, "missing required configuration")
		checkErrorContains(t, err, "required.host")
		checkErrorContains(t, err, "required.port")
	})

	t.Run("ValidationPasses", func(t *testing.T) {
		cfg.Set("required.host", "localhost")
		cfg.Set("required.port", 8080)

		err := cfg.Validate("required.host", "required.port")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ValidationUnregisteredPath", func(t *testing.T) {
		err := cfg.Validate("nonexistent.path")
		checkErrorContains(t, err, "nonexistent.path (not registered)")
	})

	t.Run("ValidationWithSourceValue", func(t *testing.T) {
		cfg2 := New()
		cfg2.Register("test", "default")

		// Value equals default but from different source
		cfg2.SetSource(SourceEnv, "test", "default")

		err := cfg2.Validate("test")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		} // Should pass because env provided value
	})
}

// TestDebugAndDump tests debug output functions
func TestDebugAndDump(t *testing.T) {
	cfg := New()
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", 8080)

	cfg.SetSource(SourceFile, "server.host", "filehost")
	cfg.SetSource(SourceEnv, "server.host", "envhost")
	cfg.SetSource(SourceCLI, "server.port", "9999")

	t.Run("Debug", func(t *testing.T) {
		debug := cfg.Debug()

		for _, fragment := range []string{"Configuration Debug Info", "Precedence:", "server.host:", "Current: envhost", "Default: localhost", "file: filehost", "env: envhost"} {
			if !strings.Contains(debug, fragment) {
				t.Errorf("debug output %q is missing %q", debug, fragment)
			}
		}
	})

	t.Run("Dump", func(t *testing.T) {
		outputStr := captureStdout(t, func() { mustNoError(t, cfg.Dump()) })

		if got := outputStr; !strings.Contains(got, "[server]") {
			t.Errorf("unexpected substring membership: %q in %q", "[server]", got)
		}
		if got := outputStr; !strings.Contains(got, "host = ") {
			t.Errorf("unexpected substring membership: %q in %q", "host = ", got)
		}
		if got := outputStr; !strings.Contains(got, "port = ") {
			t.Errorf("unexpected substring membership: %q in %q", "port = ", got)
		}
	})
}

// TestClone tests configuration cloning
func TestClone(t *testing.T) {
	cfg := New()
	cfg.Register("original.value", "default")
	cfg.Register("shared.value", "shared")

	cfg.SetSource(SourceFile, "original.value", "filevalue")
	cfg.SetSource(SourceEnv, "shared.value", "envvalue")

	clone := cfg.Clone()
	if got := clone; got == nil {
		t.Fatalf("got %v, want non-nil", got)
	}

	// Verify values are copied
	val, exists := clone.Get("original.value")
	if !exists {
		t.Errorf("exists should be true")
	}
	checkEqual(t, val, "filevalue")

	val, exists = clone.Get("shared.value")
	if !exists {
		t.Errorf("exists should be true")
	}
	checkEqual(t, val, "envvalue")

	// Modify clone should not affect original
	clone.Set("original.value", "clonevalue")

	originalVal, _ := cfg.Get("original.value")
	cloneVal, _ := clone.Get("original.value")

	checkEqual(t, originalVal, "filevalue")
	checkEqual(t, cloneVal, "clonevalue")

	// Verify source data is copied
	sources := clone.GetSources("shared.value")
	checkEqual(t, sources[SourceEnv], "envvalue")
}

// TestGenericHelpers tests generic helper functions
func TestGenericHelpers(t *testing.T) {
	cfg := New()
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", "8080") // Note: string value
	cfg.Register("features.dark_mode", true)
	cfg.Register("timeouts.read", "5s")

	t.Run("GetTyped", func(t *testing.T) {
		port, err := GetTyped[int](cfg, "server.port")
		mustNoError(t, err)
		checkEqual(t, port, 8080)

		host, err := GetTyped[string](cfg, "server.host")
		mustNoError(t, err)
		checkEqual(t, host, "localhost")

		// Test with custom decode hook type
		readTimeout, err := GetTyped[time.Duration](cfg, "timeouts.read")
		mustNoError(t, err)
		checkEqual(t, readTimeout, 5*time.Second)

		_, err = GetTyped[int](cfg, "nonexistent.path")
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("ScanTyped", func(t *testing.T) {
		type ServerConfig struct {
			Host string `toml:"host"`
			Port int    `toml:"port"`
		}

		serverConf, err := ScanTyped[ServerConfig](cfg, "server")
		mustNoError(t, err)
		if got := serverConf; got == nil {
			t.Fatalf("got %v, want non-nil", got)
		}
		checkEqual(t, serverConf.Host, "localhost")
		checkEqual(t, serverConf.Port, 8080)
	})
}

// TestGetTypedWithDefault tests generic helper function with default value fallback
func TestGetTypedWithDefault(t *testing.T) {
	t.Run("PathNotSet", func(t *testing.T) {
		cfg := New()

		// Get with default when path doesn't exist
		port, err := GetTypedWithDefault(cfg, "server.port", int64(8080))
		mustNoError(t, err)
		checkEqual(t, port, int64(8080))

		// Verify it was actually set
		val, exists := cfg.Get("server.port")
		if !exists {
			t.Errorf("exists should be true")
		}
		checkEqual(t, val, int64(8080))
	})

	t.Run("PathAlreadySet", func(t *testing.T) {
		cfg := New()
		cfg.Register("server.host", "localhost")
		cfg.Set("server.host", "example.com")

		// Should return existing value, not default
		host, err := GetTypedWithDefault(cfg, "server.host", "default.com")
		mustNoError(t, err)
		checkEqual(t, host, "example.com")
	})

	t.Run("DifferentTypes", func(t *testing.T) {
		cfg := New()

		// Test with various types
		timeout, err := GetTypedWithDefault(cfg, "timeouts.read", 30*time.Second)
		mustNoError(t, err)
		checkEqual(t, timeout, 30*time.Second)

		enabled, err := GetTypedWithDefault(cfg, "features.enabled", true)
		mustNoError(t, err)
		if !enabled {
			t.Errorf("enabled should be true")
		}

		tags, err := GetTypedWithDefault(cfg, "app.tags", []string{"default", "tag"})
		mustNoError(t, err)
		checkEqual(t, tags, []string{"default", "tag"})
	})
}

// TestScanMap tests the ScanMap utility function
func TestScanMap(t *testing.T) {
	type Config struct {
		Server struct {
			Host    string        `toml:"host" json:"hostname"`
			Port    int           `toml:"port" json:"port"`
			Timeout time.Duration `toml:"timeout" json:"timeout"`
		} `toml:"server" json:"server"`
		LogLevel string `toml:"log_level" json:"logLevel"`
	}

	t.Run("BasicScanWithTOMLTags", func(t *testing.T) {
		configMap := map[string]any{
			"server": map[string]any{
				"host":    "localhost",
				"port":    8080,
				"timeout": "15s",
			},
			"log_level": "info",
		}

		var target Config
		err := ScanMap(configMap, &target)

		mustNoError(t, err)
		checkEqual(t, target.Server.Host, "localhost")
		checkEqual(t, target.Server.Port, 8080)
		checkEqual(t, target.Server.Timeout, 15*time.Second)
		checkEqual(t, target.LogLevel, "info")
	})

	t.Run("RejectJSONTags", func(t *testing.T) {
		var target Config
		if err := ScanMap(map[string]any{}, &target, "json"); err == nil {
			t.Fatalf("expected an error")
		}
	})

	t.Run("NilMapInput", func(t *testing.T) {
		var target Config
		target.LogLevel = "initial"
		target.Server.Port = 1234

		err := ScanMap(nil, &target)
		mustNoError(t, err)

		// Verify that fields are NOT changed when the map is empty,
		// reflecting the observed behavior.
		checkEqual(t, target.LogLevel, "initial")
		checkEqual(t, target.Server.Port, 1234)
		if got := len(target.Server.Host); got != 0 {
			t.Errorf("length = %d, want %d", got, 0)
		}
	})

	t.Run("PartialMapBehavior", func(t *testing.T) {
		configMap := map[string]any{
			"log_level": "warn",
		}
		var target Config
		target.Server.Host = "initial_host"
		target.Server.Port = 1234
		target.LogLevel = "initial_log"

		err := ScanMap(configMap, &target)
		mustNoError(t, err)

		// Mapped field should be updated
		checkEqual(t, target.LogLevel, "warn")
		// Unmapped fields should be untouched
		checkEqual(t, target.Server.Host, "initial_host", "Unmapped field should be untouched")
		checkEqual(t, target.Server.Port, 1234, "Unmapped field should be untouched")
	})

	t.Run("InvalidTarget", func(t *testing.T) {
		configMap := map[string]any{"log_level": "info"}
		var target Config // Not a pointer

		err := ScanMap(configMap, target)
		if err == nil {
			t.Errorf("expected an error")
		}
		// Invalid targets return a categorized type error.
		checkErrorContains(t, err, "must be non-nil pointer")
	})
}
