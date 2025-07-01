// File: lixenwraith/config/source_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lixenwraith/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiSourceConfiguration(t *testing.T) {
	t.Run("Source Precedence", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("test.value", "default")

		// Set values in different sources
		cfg.SetSource("test.value", config.SourceFile, "from-file")
		cfg.SetSource("test.value", config.SourceEnv, "from-env")
		cfg.SetSource("test.value", config.SourceCLI, "from-cli")

		// Default precedence: CLI > Env > File > Default
		val, _ := cfg.String("test.value")
		assert.Equal(t, "from-cli", val)

		// Change precedence
		opts := config.LoadOptions{
			Sources: []config.Source{
				config.SourceEnv,
				config.SourceCLI,
				config.SourceFile,
				config.SourceDefault,
			},
		}
		cfg.SetLoadOptions(opts)

		// Now env should win
		val, _ = cfg.String("test.value")
		assert.Equal(t, "from-env", val)
	})

	t.Run("Source Tracking", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("server.port", 8080)

		// Set from multiple sources
		cfg.SetSource("server.port", config.SourceFile, 9090)
		cfg.SetSource("server.port", config.SourceEnv, 7070)

		// Get all sources
		sources := cfg.GetSources("server.port")

		// Should have 2 sources
		assert.Len(t, sources, 2)
		assert.Equal(t, 9090, sources[config.SourceFile])
		assert.Equal(t, 7070, sources[config.SourceEnv])
	})

	t.Run("GetSource", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("api.key", "default-key")

		cfg.SetSource("api.key", config.SourceEnv, "env-key")

		// Get from specific source
		envVal, exists := cfg.GetSource("api.key", config.SourceEnv)
		assert.True(t, exists)
		assert.Equal(t, "env-key", envVal)

		// Get from missing source
		_, exists = cfg.GetSource("api.key", config.SourceFile)
		assert.False(t, exists)
	})

	t.Run("Reset Sources", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("test1", "default1")
		cfg.Register("test2", "default2")

		// Set values
		cfg.SetSource("test1", config.SourceFile, "file1")
		cfg.SetSource("test1", config.SourceEnv, "env1")
		cfg.SetSource("test2", config.SourceCLI, "cli2")

		// Reset specific source
		cfg.ResetSource(config.SourceEnv)

		// Env value should be gone
		_, exists := cfg.GetSource("test1", config.SourceEnv)
		assert.False(t, exists)

		// Other sources remain
		fileVal, _ := cfg.GetSource("test1", config.SourceFile)
		assert.Equal(t, "file1", fileVal)

		// Reset all
		cfg.Reset()

		// All values should be defaults
		val1, _ := cfg.String("test1")
		val2, _ := cfg.String("test2")
		assert.Equal(t, "default1", val1)
		assert.Equal(t, "default2", val2)
	})

	t.Run("LoadWithOptions Integration", func(t *testing.T) {
		// Create temp config file
		tmpdir := t.TempDir()
		configFile := filepath.Join(tmpdir, "test.toml")

		configContent := `
[server]
host = "file-host"
port = 8080

[feature]
enabled = true
`
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

		// Set environment
		os.Setenv("TEST_SERVER_PORT", "9090")
		os.Setenv("TEST_FEATURE_ENABLED", "false")
		t.Cleanup(func() {
			os.Unsetenv("TEST_SERVER_PORT")
			os.Unsetenv("TEST_FEATURE_ENABLED")
		})

		cfg := config.New()
		cfg.Register("server.host", "default-host")
		cfg.Register("server.port", 7070)
		cfg.Register("feature.enabled", false)

		// Load with custom precedence (File highest)
		opts := config.LoadOptions{
			Sources: []config.Source{
				config.SourceFile,
				config.SourceEnv,
				config.SourceCLI,
				config.SourceDefault,
			},
			EnvPrefix: "TEST_",
		}

		err := cfg.LoadWithOptions(configFile, []string{"--server.host=cli-host"}, opts)
		require.NoError(t, err)

		// File should win for all values
		host, _ := cfg.String("server.host")
		assert.Equal(t, "file-host", host)

		port, _ := cfg.Int64("server.port")
		assert.Equal(t, int64(8080), port)

		enabled, _ := cfg.Bool("feature.enabled")
		assert.True(t, enabled)
	})

	t.Run("ScanSource", func(t *testing.T) {
		type ServerConfig struct {
			Host string `toml:"host"`
			Port int    `toml:"port"`
		}

		cfg := config.New()
		cfg.Register("server.host", "default")
		cfg.Register("server.port", 8080)

		// Set different values in different sources
		cfg.SetSource("server.host", config.SourceFile, "file-host")
		cfg.SetSource("server.port", config.SourceFile, 8080)
		cfg.SetSource("server.host", config.SourceEnv, "env-host")
		cfg.SetSource("server.port", config.SourceEnv, 9090)

		// Scan from specific source
		var fileConfig ServerConfig
		err := cfg.ScanSource("server", config.SourceFile, &fileConfig)
		require.NoError(t, err)
		assert.Equal(t, "file-host", fileConfig.Host)
		assert.Equal(t, 8080, fileConfig.Port)

		var envConfig ServerConfig
		err = cfg.ScanSource("server", config.SourceEnv, &envConfig)
		require.NoError(t, err)
		assert.Equal(t, "env-host", envConfig.Host)
		assert.Equal(t, 9090, envConfig.Port)
	})

	t.Run("SaveSource", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("app.name", "myapp")
		cfg.Register("app.version", "1.0.0")
		cfg.Register("server.port", 8080)

		// Set values in different sources
		cfg.SetSource("app.name", config.SourceFile, "fileapp")
		cfg.SetSource("app.version", config.SourceEnv, "2.0.0")
		cfg.SetSource("server.port", config.SourceCLI, 9090)

		// Save only env source
		tmpfile := filepath.Join(t.TempDir(), "config-source.toml")
		err := cfg.SaveSource(tmpfile, config.SourceEnv)
		require.NoError(t, err)

		// Load saved file and verify
		newCfg := config.New()
		newCfg.Register("app.version", "")
		newCfg.LoadFile(tmpfile)

		version, _ := newCfg.String("app.version")
		assert.Equal(t, "2.0.0", version)

		// Should not have other source values
		name, _ := newCfg.String("app.name")
		assert.Empty(t, name)
	})
}