// FILE: lixenwraith/config/convenience_test.go
package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuickFunctions tests the convenience Quick* functions
func TestQuickFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "quick.toml")
	os.WriteFile(configFile, []byte(`
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
		defer func() { os.Args = oldArgs }()

		cfg, err := Quick(defaults, "QUICK_", configFile)
		require.NoError(t, err)

		// CLI should override
		port, _ := cfg.Get("port")
		assert.Equal(t, "9999", port)

		// File value
		host, _ := cfg.Get("host")
		assert.Equal(t, "quickhost", host)
	})

	t.Run("QuickCustom", func(t *testing.T) {
		opts := LoadOptions{
			Sources:   []Source{SourceFile, SourceDefault}, // Only file and defaults
			EnvPrefix: "CUSTOM_",
		}

		cfg, err := QuickCustom(defaults, opts, configFile)
		require.NoError(t, err)

		// Should use file value
		port, _ := cfg.Get("port")
		assert.Equal(t, int64(7777), port)
	})

	t.Run("MustQuickPanic", func(t *testing.T) {
		// Valid case - should not panic
		assert.NotPanics(t, func() {
			cfg := MustQuick(defaults, "TEST_", configFile)
			assert.NotNil(t, cfg)
		})

		// Invalid struct - should panic
		assert.Panics(t, func() {
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
		require.NoError(t, err)

		// Should populate from file
		updated, err := cfg.AsStruct()
		require.NoError(t, err)

		typedCfg := updated.(*QuickConfig)
		assert.Equal(t, "quickhost", typedCfg.Host)
		assert.Equal(t, 7777, typedCfg.Port)
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
		require.NotNil(t, fs)

		// Verify flags exist
		hostFlag := fs.Lookup("server.host")
		require.NotNil(t, hostFlag)
		assert.Equal(t, "localhost", hostFlag.DefValue)

		portFlag := fs.Lookup("server.port")
		require.NotNil(t, portFlag)
		assert.Equal(t, "8080", portFlag.DefValue)

		debugFlag := fs.Lookup("debug.enabled")
		require.NotNil(t, debugFlag)
		assert.Equal(t, "false", debugFlag.DefValue)

		timeoutFlag := fs.Lookup("timeout")
		require.NotNil(t, timeoutFlag)
		assert.Equal(t, "30.5", timeoutFlag.DefValue)
	})

	t.Run("BindFlags", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("server.host", "default", "")
		fs.Int("server.port", 8080, "")
		fs.Bool("debug.enabled", false, "")

		// Parse with test values
		err := fs.Parse([]string{"-server.host=flaghost", "-server.port=5555", "-debug.enabled"})
		require.NoError(t, err)

		// Bind to config
		err = cfg.BindFlags(fs)
		require.NoError(t, err)

		// Verify values were set
		host, _ := cfg.Get("server.host")
		assert.Equal(t, "flaghost", host)

		port, _ := cfg.Get("server.port")
		assert.Equal(t, "5555", port)

		debug, _ := cfg.Get("debug.enabled")
		assert.Equal(t, "true", debug)
	})

	t.Run("BindFlagsError", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("unregistered.path", "value", "")
		fs.Parse([]string{"-unregistered.path=test"})

		err := cfg.BindFlags(fs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to bind 1 flags")
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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required configuration")
		assert.Contains(t, err.Error(), "required.host")
		assert.Contains(t, err.Error(), "required.port")
	})

	t.Run("ValidationPasses", func(t *testing.T) {
		cfg.Set("required.host", "localhost")
		cfg.Set("required.port", 8080)

		err := cfg.Validate("required.host", "required.port")
		assert.NoError(t, err)
	})

	t.Run("ValidationUnregisteredPath", func(t *testing.T) {
		err := cfg.Validate("nonexistent.path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent.path (not registered)")
	})

	t.Run("ValidationWithSourceValue", func(t *testing.T) {
		cfg2 := New()
		cfg2.Register("test", "default")

		// Value equals default but from different source
		cfg2.SetSource(SourceEnv, "test", "default")

		err := cfg2.Validate("test")
		assert.NoError(t, err) // Should pass because env provided value
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

		assert.Contains(t, debug, "Configuration Debug Info")
		assert.Contains(t, debug, "Precedence:")
		assert.Contains(t, debug, "server.host:")
		assert.Contains(t, debug, "Current: envhost")
		assert.Contains(t, debug, "Default: localhost")
		assert.Contains(t, debug, "file: filehost")
		assert.Contains(t, debug, "env: envhost")
	})

	t.Run("Dump", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := cfg.Dump()
		assert.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		// Read output
		output := make([]byte, 1024)
		n, _ := r.Read(output)
		outputStr := string(output[:n])

		assert.Contains(t, outputStr, "[server]")
		assert.Contains(t, outputStr, "host = ")
		assert.Contains(t, outputStr, "port = ")
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
	require.NotNil(t, clone)

	// Verify values are copied
	val, exists := clone.Get("original.value")
	assert.True(t, exists)
	assert.Equal(t, "filevalue", val)

	val, exists = clone.Get("shared.value")
	assert.True(t, exists)
	assert.Equal(t, "envvalue", val)

	// Modify clone should not affect original
	clone.Set("original.value", "clonevalue")

	originalVal, _ := cfg.Get("original.value")
	cloneVal, _ := clone.Get("original.value")

	assert.Equal(t, "filevalue", originalVal)
	assert.Equal(t, "clonevalue", cloneVal)

	// Verify source data is copied
	sources := clone.GetSources("shared.value")
	assert.Equal(t, "envvalue", sources[SourceEnv])
}

func TestGenericHelpers(t *testing.T) {
	cfg := New()
	cfg.Register("server.host", "localhost")
	cfg.Register("server.port", "8080") // Note: string value
	cfg.Register("features.dark_mode", true)
	cfg.Register("timeouts.read", "5s")

	t.Run("GetTyped", func(t *testing.T) {
		port, err := GetTyped[int](cfg, "server.port")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)

		host, err := GetTyped[string](cfg, "server.host")
		require.NoError(t, err)
		assert.Equal(t, "localhost", host)

		// Test with custom decode hook type
		readTimeout, err := GetTyped[time.Duration](cfg, "timeouts.read")
		require.NoError(t, err)
		assert.Equal(t, 5*time.Second, readTimeout)

		_, err = GetTyped[int](cfg, "nonexistent.path")
		assert.Error(t, err)
	})

	t.Run("ScanTyped", func(t *testing.T) {
		type ServerConfig struct {
			Host string `toml:"host"`
			Port int    `toml:"port"`
		}

		serverConf, err := ScanTyped[ServerConfig](cfg, "server")
		require.NoError(t, err)
		require.NotNil(t, serverConf)
		assert.Equal(t, "localhost", serverConf.Host)
		assert.Equal(t, 8080, serverConf.Port)
	})
}