// File: lixenwraith/config/builder_test.go
package config_test

import (
	"os"
	"testing"

	"github.com/lixenwraith/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	t.Run("Basic Builder", func(t *testing.T) {
		type AppConfig struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
			Debug   bool   `toml:"debug"`
		}

		defaults := AppConfig{
			Name:    "testapp",
			Version: "1.0.0",
			Debug:   false,
		}

		cfg, err := config.NewBuilder().
			WithDefaults(defaults).
			WithPrefix("app.").
			Build()

		require.NoError(t, err)

		// Check registered paths
		paths := cfg.GetRegisteredPaths("app.")
		assert.Len(t, paths, 3)

		// Check values
		name, err := cfg.String("app.name")
		require.NoError(t, err)
		assert.Equal(t, "testapp", name)
	})

	t.Run("Builder with All Options", func(t *testing.T) {
		os.Setenv("BUILDER_SERVER_PORT", "5555")
		defer os.Unsetenv("BUILDER_SERVER_PORT")

		type Config struct {
			Server struct {
				Host string `toml:"host"`
				Port int    `toml:"port"`
			} `toml:"server"`
			API struct {
				Key     string `toml:"key"`
				Timeout int    `toml:"timeout"`
			} `toml:"api"`
		}

		defaults := Config{}
		defaults.Server.Host = "localhost"
		defaults.Server.Port = 8080
		defaults.API.Timeout = 30

		cfg, err := config.NewBuilder().
			WithDefaults(defaults).
			WithEnvPrefix("BUILDER_").
			WithArgs([]string{"--api.key=test-key"}).
			WithSources(
				config.SourceCLI,
				config.SourceEnv,
				config.SourceDefault,
			).
			WithEnvWhitelist("server.port", "api.key").
			Build()

		require.NoError(t, err)

		// CLI should provide api.key
		apiKey, err := cfg.String("api.key")
		require.NoError(t, err)
		assert.Equal(t, "test-key", apiKey)

		// Env should provide server.port (whitelisted)
		port, err := cfg.Int64("server.port")
		require.NoError(t, err)
		assert.Equal(t, int64(5555), port)

		// Non-whitelisted env should not load
		os.Setenv("BUILDER_API_TIMEOUT", "99")
		defer os.Unsetenv("BUILDER_API_TIMEOUT")

		cfg2, err := config.NewBuilder().
			WithDefaults(defaults).
			WithEnvPrefix("BUILDER_").
			WithEnvWhitelist("server.port"). // api.timeout NOT whitelisted
			Build()
		require.NoError(t, err)

		timeout, err := cfg2.Int64("api.timeout")
		require.NoError(t, err)
		assert.Equal(t, int64(30), timeout, "non-whitelisted env should not load")
	})

	t.Run("Builder Custom Transform", func(t *testing.T) {
		os.Setenv("PORT", "3333")
		os.Setenv("DB_URL", "postgres://custom")
		defer func() {
			os.Unsetenv("PORT")
			os.Unsetenv("DB_URL")
		}()

		type Config struct {
			Server struct {
				Port int `toml:"port"`
			} `toml:"server"`
			Database struct {
				URL string `toml:"url"`
			} `toml:"database"`
		}

		cfg, err := config.NewBuilder().
			WithDefaults(Config{}).
			WithEnvTransform(func(path string) string {
				switch path {
				case "server.port":
					return "PORT"
				case "database.url":
					return "DB_URL"
				default:
					return ""
				}
			}).
			Build()

		require.NoError(t, err)

		port, err := cfg.Int64("server.port")
		require.NoError(t, err)
		assert.Equal(t, int64(3333), port)

		dbURL, err := cfg.String("database.url")
		require.NoError(t, err)
		assert.Equal(t, "postgres://custom", dbURL)
	})

	t.Run("MustBuild Panic", func(t *testing.T) {
		assert.Panics(t, func() {
			config.NewBuilder().
				WithDefaults("not a struct").
				MustBuild()
		})
	})
}

func TestQuickFunctions(t *testing.T) {
	t.Run("Quick Success", func(t *testing.T) {
		type Config struct {
			App struct {
				Name string `toml:"name"`
			} `toml:"app"`
		}

		defaults := Config{}
		defaults.App.Name = "quicktest"

		cfg, err := config.Quick(defaults, "QUICK_", "")
		require.NoError(t, err)

		name, err := cfg.String("app.name")
		require.NoError(t, err)
		assert.Equal(t, "quicktest", name)
	})

	t.Run("QuickCustom", func(t *testing.T) {
		opts := config.LoadOptions{
			Sources: []config.Source{
				config.SourceDefault,
				config.SourceEnv,
			},
			EnvPrefix: "CUSTOM_",
		}

		cfg, err := config.QuickCustom(nil, opts, "")
		require.NoError(t, err)
		assert.NotNil(t, cfg)
	})

	t.Run("MustQuick Panic", func(t *testing.T) {
		assert.Panics(t, func() {
			config.MustQuick("invalid", "TEST_", "")
		})
	})
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("required1", "")
		cfg.Register("required2", 0)
		cfg.Register("optional", "has-default")

		// Initial validation should fail
		err := cfg.Validate("required1", "required2")
		assert.Error(t, err, "expected validation to fail for empty values")

		// Set required values
		cfg.Set("required1", "value1")
		cfg.Set("required2", 42)

		// Now should pass
		err = cfg.Validate("required1", "required2")
		assert.NoError(t, err)

		// Validate unregistered path
		err = cfg.Validate("unregistered")
		assert.Error(t, err, "expected error for unregistered path")
	})

	t.Run("Debug Output", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("test.value", "default")
		cfg.SetSource("test.value", config.SourceFile, "from-file")
		cfg.SetSource("test.value", config.SourceEnv, "from-env")

		debug := cfg.Debug()

		// Should contain key information
		assert.NotEmpty(t, debug)

		// Should show sources - checking for actual source string values
		assert.Contains(t, debug, "file")
		assert.Contains(t, debug, "from-file")
		assert.Contains(t, debug, "env")
		assert.Contains(t, debug, "from-env")
	})

	t.Run("Clone", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("original", "value")
		cfg.Set("original", "modified")

		// Clone configuration
		clone := cfg.Clone()

		// Clone should have same values
		val, err := clone.String("original")
		require.NoError(t, err)
		assert.Equal(t, "modified", val)

		// Modifying clone should not affect original
		clone.Set("original", "clone-modified")

		origVal, err := cfg.String("original")
		require.NoError(t, err)
		assert.Equal(t, "modified", origVal, "original should not be affected by clone modification")
	})

	t.Run("GetRegisteredPathsWithDefaults", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("app.name", "myapp")
		cfg.Register("app.version", "1.0.0")
		cfg.Register("server.port", 8080)

		// Get paths with defaults
		paths := cfg.GetRegisteredPathsWithDefaults("app.")

		assert.Len(t, paths, 2)
		assert.Equal(t, "myapp", paths["app.name"])
		assert.Equal(t, "1.0.0", paths["app.version"])
	})
}