// FILE: lixenwraith/config/builder_test.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuilder tests the builder pattern
func TestBuilder(t *testing.T) {
	t.Run("BasicBuilder", func(t *testing.T) {
		type Config struct {
			Host string `toml:"host"`
			Port int    `toml:"port"`
		}

		defaults := &Config{
			Host: "localhost",
			Port: 8080,
		}

		cfg, err := NewBuilder().
			WithDefaults(defaults).
			WithEnvPrefix("TEST_").
			Build()

		require.NoError(t, err)
		assert.NotNil(t, cfg)

		val, exists := cfg.Get("host")
		assert.True(t, exists)
		assert.Equal(t, "localhost", val)
	})

	t.Run("BuilderWithAllOptions", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.toml")
		os.WriteFile(configFile, []byte(`host = "filehost"`), 0644)

		type Config struct {
			Host string `json:"hostname"`
			Port int    `json:"port"`
		}

		defaults := &Config{
			Host: "defaulthost",
			Port: 3000,
		}

		// Custom env transform
		envTransform := func(path string) string {
			return "CUSTOM_" + path
		}

		cfg, err := NewBuilder().
			WithDefaults(defaults).
			WithTagName("json").
			WithPrefix("server").
			WithEnvPrefix("APP_").
			WithFile(configFile).
			WithArgs([]string{"--server.hostname=clihost"}).
			WithSources(SourceCLI, SourceFile, SourceEnv, SourceDefault).
			WithEnvTransform(envTransform).
			WithEnvWhitelist("server.hostname").
			Build()

		require.NoError(t, err)

		// CLI should take precedence
		val, _ := cfg.Get("server.hostname")
		assert.Equal(t, "clihost", val)
	})

	t.Run("BuilderWithTarget", func(t *testing.T) {
		type Config struct {
			Database struct {
				Host string `toml:"host"`
				Port int    `toml:"port"`
			} `toml:"db"`
			Cache struct {
				TTL int `toml:"ttl"`
			} `toml:"cache"`
		}

		target := &Config{}
		target.Database.Host = "localhost"
		target.Database.Port = 5432
		target.Cache.TTL = 300

		cfg, err := NewBuilder().
			WithTarget(target).
			Build()

		require.NoError(t, err)

		// Verify paths were registered
		paths := cfg.GetRegisteredPaths()
		assert.True(t, paths["db.host"])
		assert.True(t, paths["db.port"])
		assert.True(t, paths["cache.ttl"])

		// Test AsStruct
		result, err := cfg.AsStruct()
		require.NoError(t, err)
		assert.Equal(t, target, result)
	})

	t.Run("BuilderWithValidator", func(t *testing.T) {
		type UserConfig struct {
			Port int `toml:"port"`
		}

		validatorCalled := false
		validator := func(cfg *Config) error {
			validatorCalled = true
			val, exists := cfg.Get("port")
			if !exists {
				return fmt.Errorf("port not found")
			}
			// Convert to int - could be int64 from storage
			var port int
			switch v := val.(type) {
			case int:
				port = v
			case int64:
				port = int(v)
			default:
				return fmt.Errorf("port has unexpected type %T", v)
			}

			if port < 1024 {
				return fmt.Errorf("port %d is below 1024", port)
			}
			return nil
		}

		// Valid case
		cfg, err := NewBuilder().
			WithDefaults(&UserConfig{Port: 8080}).
			WithValidator(validator).
			Build()

		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.True(t, validatorCalled)

		// Invalid case
		validatorCalled = false
		cfg2, err := NewBuilder().
			WithDefaults(&UserConfig{Port: 80}).
			WithValidator(validator).
			Build()

		assert.Nil(t, cfg2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration validation failed")
		assert.True(t, validatorCalled)
	})

	t.Run("BuilderErrorAccumulation", func(t *testing.T) {
		// Unsupported tag name
		_, err := NewBuilder().
			WithTagName("xml").
			WithDefaults(struct{}{}).
			Build()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported tag name")

		// Invalid target
		_, err = NewBuilder().
			WithTarget("not-a-pointer").
			Build()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires non-nil pointer to struct")
	})

	t.Run("MustBuildPanic", func(t *testing.T) {
		// Should not panic with valid config
		assert.NotPanics(t, func() {
			cfg := NewBuilder().
				WithDefaults(struct{ Port int }{Port: 8080}).
				MustBuild()
			assert.NotNil(t, cfg)
		})

		// Should panic with error
		assert.Panics(t, func() {
			NewBuilder().
				WithTagName("invalid").
				MustBuild()
		})
	})
}

// TestFileDiscovery tests automatic config file discovery
func TestFileDiscovery(t *testing.T) {
	t.Run("DiscoveryWithCLIFlag", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Use .toml extension for TOML content
		configFile := filepath.Join(tmpDir, "custom.toml")
		os.WriteFile(configFile, []byte(`test = "value"`), 0644)

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithArgs([]string{"--config", configFile}).
			WithFileDiscovery(opts).
			Build()

		require.NoError(t, err)

		// Verify file was loaded
		val, _ := cfg.Get("test")
		assert.Equal(t, "value", val)
	})

	// Rest of test cases remain the same...
	t.Run("DiscoveryWithEnvVar", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "env.toml")
		os.WriteFile(configFile, []byte(`test = "envvalue"`), 0644)

		os.Setenv("MYAPP_CONFIG", configFile)
		defer os.Unsetenv("MYAPP_CONFIG")

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithFileDiscovery(opts).
			Build()

		require.NoError(t, err)

		val, _ := cfg.Get("test")
		assert.Equal(t, "envvalue", val)
	})

	t.Run("DiscoveryInCurrentDir", func(t *testing.T) {
		// Create config in current directory
		cwd, _ := os.Getwd()
		configFile := filepath.Join(cwd, "myapp.toml")
		os.WriteFile(configFile, []byte(`test = "cwdvalue"`), 0644)
		defer os.Remove(configFile)

		opts := FileDiscoveryOptions{
			Name:          "myapp",
			Extensions:    []string{".toml"},
			UseCurrentDir: true,
		}

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithFileDiscovery(opts).
			Build()

		require.NoError(t, err)

		val, _ := cfg.Get("test")
		assert.Equal(t, "cwdvalue", val)
	})

	t.Run("DiscoveryPrecedence", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple config files
		cliFile := filepath.Join(tmpDir, "cli.toml")
		envFile := filepath.Join(tmpDir, "env.toml")
		os.WriteFile(cliFile, []byte(`test = "clifile"`), 0644)
		os.WriteFile(envFile, []byte(`test = "envfile"`), 0644)

		// CLI should take precedence over env
		os.Setenv("MYAPP_CONFIG", envFile)
		defer os.Unsetenv("MYAPP_CONFIG")

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithArgs([]string{"--config", cliFile}).
			WithFileDiscovery(opts).
			Build()

		require.NoError(t, err)

		val, _ := cfg.Get("test")
		assert.Equal(t, "clifile", val)
	})
}

func TestBuilderWithTypedValidator(t *testing.T) {
	type Cfg struct {
		Port int `toml:"port"`
	}

	// Case 1: Valid configuration
	t.Run("ValidTyped", func(t *testing.T) {
		target := &Cfg{Port: 8080}
		validator := func(c *Cfg) error {
			if c.Port < 1024 {
				return fmt.Errorf("port too low")
			}
			return nil
		}

		_, err := NewBuilder().
			WithTarget(target).
			WithTypedValidator(validator).
			Build()

		require.NoError(t, err)
	})

	// Case 2: Invalid configuration
	t.Run("InvalidTyped", func(t *testing.T) {
		target := &Cfg{Port: 80}
		validator := func(c *Cfg) error {
			if c.Port < 1024 {
				return fmt.Errorf("port too low")
			}
			return nil
		}

		_, err := NewBuilder().
			WithTarget(target).
			WithTypedValidator(validator).
			Build()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "typed configuration validation failed: port too low")
	})

	// Case 3: Mismatched validator signature
	t.Run("MismatchedSignature", func(t *testing.T) {
		target := &Cfg{}
		validator := func(c *struct{ Name string }) error { // Different type
			return nil
		}

		_, err := NewBuilder().
			WithTarget(target).
			WithTypedValidator(validator).
			Build()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "typed validator signature")
	})
}