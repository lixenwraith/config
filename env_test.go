// File: lixenwraith/config/env_test.go
package config_test

import (
	"os"
	"testing"

	"github.com/lixenwraith/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentVariables(t *testing.T) {
	t.Run("Basic Environment Loading", func(t *testing.T) {
		// Set up environment
		envVars := map[string]string{
			"TEST_SERVER_HOST": "env-host",
			"TEST_SERVER_PORT": "9999",
			"TEST_DEBUG":       "true",
		}
		for k, v := range envVars {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}

		cfg := config.New()
		cfg.Register("server.host", "default-host")
		cfg.Register("server.port", 8080)
		cfg.Register("debug", false)

		// Load environment variables
		err := cfg.LoadEnv("TEST_")
		require.NoError(t, err)

		// Verify values
		host, _ := cfg.String("server.host")
		assert.Equal(t, "env-host", host)

		port, _ := cfg.Int64("server.port")
		assert.Equal(t, int64(9999), port)

		debug, _ := cfg.Bool("debug")
		assert.True(t, debug)
	})

	t.Run("Custom Environment Transform", func(t *testing.T) {
		os.Setenv("PORT", "3000")
		os.Setenv("DATABASE_URL", "postgres://localhost/test")
		defer func() {
			os.Unsetenv("PORT")
			os.Unsetenv("DATABASE_URL")
		}()

		cfg := config.New()
		cfg.Register("server.port", 8080)
		cfg.Register("database.url", "sqlite://memory")

		opts := config.LoadOptions{
			Sources: []config.Source{config.SourceEnv, config.SourceDefault},
			EnvTransform: func(path string) string {
				mapping := map[string]string{
					"server.port":  "PORT",
					"database.url": "DATABASE_URL",
				}
				return mapping[path]
			},
		}

		err := cfg.LoadWithOptions("", nil, opts)
		require.NoError(t, err)

		port, _ := cfg.Int64("server.port")
		assert.Equal(t, int64(3000), port)

		dbURL, _ := cfg.String("database.url")
		assert.Equal(t, "postgres://localhost/test", dbURL)
	})

	t.Run("Environment Discovery", func(t *testing.T) {
		// Set up various env vars
		envVars := map[string]string{
			"APP_SERVER_HOST":  "discovered",
			"APP_SERVER_PORT":  "4444",
			"APP_UNREGISTERED": "ignored",
		}
		for k, v := range envVars {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}

		cfg := config.New()
		cfg.Register("server.host", "default")
		cfg.Register("server.port", 8080)
		cfg.Register("server.timeout", 30)

		// Discover which registered paths have env vars
		discovered := cfg.DiscoverEnv("APP_")

		// Should find 2 env vars
		assert.Len(t, discovered, 2)
		assert.Equal(t, "APP_SERVER_HOST", discovered["server.host"])
		assert.Equal(t, "APP_SERVER_PORT", discovered["server.port"])
		assert.NotContains(t, discovered, "unregistered")
	})

	t.Run("Environment Whitelist", func(t *testing.T) {
		envVars := map[string]string{
			"SECRET_API_KEY":           "secret-value",
			"SECRET_DATABASE_PASSWORD": "db-pass",
			"SECRET_SERVER_PORT":       "5555",
		}
		for k, v := range envVars {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}

		cfg := config.New()
		cfg.Register("api.key", "")
		cfg.Register("database.password", "")
		cfg.Register("server.port", 8080)

		opts := config.LoadOptions{
			Sources:   []config.Source{config.SourceEnv, config.SourceDefault},
			EnvPrefix: "SECRET_",
			EnvWhitelist: map[string]bool{
				"api.key":           true,
				"database.password": true,
				// server.port is NOT whitelisted
			},
		}

		cfg.LoadWithOptions("", nil, opts)

		// Whitelisted values should load
		apiKey, _ := cfg.String("api.key")
		assert.Equal(t, "secret-value", apiKey)

		dbPass, _ := cfg.String("database.password")
		assert.Equal(t, "db-pass", dbPass)

		// Non-whitelisted should use default
		port, _ := cfg.Int64("server.port")
		assert.Equal(t, int64(8080), port)
	})

	t.Run("RegisterWithEnv", func(t *testing.T) {
		os.Setenv("CUSTOM_PORT", "6666")
		defer os.Unsetenv("CUSTOM_PORT")

		cfg := config.New()

		// Register with explicit env mapping
		err := cfg.RegisterWithEnv("server.port", 8080, "CUSTOM_PORT")
		require.NoError(t, err)

		// Should immediately have env value
		port, _ := cfg.Int64("server.port")
		assert.Equal(t, int64(6666), port)
	})

	t.Run("Export Environment", func(t *testing.T) {
		cfg := config.New()
		cfg.Register("app.name", "myapp")
		cfg.Register("app.version", "1.0.0")
		cfg.Register("server.port", 8080)

		// Set some non-default values
		cfg.Set("app.version", "2.0.0")
		cfg.Set("server.port", 9090)

		// Export as env vars
		exports := cfg.ExportEnv("EXPORT_")

		// Should export non-default values
		assert.Equal(t, "2.0.0", exports["EXPORT_APP_VERSION"])
		assert.Equal(t, "9090", exports["EXPORT_SERVER_PORT"])

		// Should not export defaults
		assert.NotContains(t, exports, "EXPORT_APP_NAME")
	})

	t.Run("Type Parsing from Environment", func(t *testing.T) {
		envVars := map[string]string{
			"TYPES_STRING":     "hello world",
			"TYPES_INT":        "42",
			"TYPES_FLOAT":      "3.14159",
			"TYPES_BOOL_TRUE":  "true",
			"TYPES_BOOL_FALSE": "false",
			"TYPES_QUOTED":     `"quoted string"`,
		}
		for k, v := range envVars {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}

		cfg := config.New()
		cfg.Register("string", "")
		cfg.Register("int", 0)
		cfg.Register("float", 0.0)
		cfg.Register("bool.true", false)
		cfg.Register("bool.false", true)
		cfg.Register("quoted", "")

		cfg.LoadEnv("TYPES_")

		// Verify type conversions
		s, _ := cfg.String("string")
		assert.Equal(t, "hello world", s)

		i, _ := cfg.Int64("int")
		assert.Equal(t, int64(42), i)

		f, _ := cfg.Float64("float")
		assert.Equal(t, 3.14159, f)

		bt, _ := cfg.Bool("bool.true")
		assert.True(t, bt)

		bf, _ := cfg.Bool("bool.false")
		assert.False(t, bf)

		q, _ := cfg.String("quoted")
		assert.Equal(t, "quoted string", q)
	})
}