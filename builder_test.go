// FILE: lixenwraith/config/builder_test.go
package config

import (
	"fmt"
	"path/filepath"
	"testing"
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

		mustNoError(t, err)
		if got := cfg; got == nil {
			t.Errorf("got %v, want non-nil", got)
		}

		val, exists := cfg.Get("host")
		if !exists {
			t.Errorf("exists should be true")
		}
		checkEqual(t, val, "localhost")
	})

	t.Run("BuilderWithAllOptions", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.toml")
		writeTestFile(t, configFile, []byte(`host = "filehost"`), 0644)

		type Config struct {
			Host string `toml:"hostname"`
			Port int    `toml:"port"`
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
			WithTagName("toml").
			WithPrefix("server").
			WithEnvPrefix("APP_").
			WithFile(configFile).
			WithArgs([]string{"--server.hostname=clihost"}).
			WithSources(SourceCLI, SourceFile, SourceEnv, SourceDefault).
			WithEnvTransform(envTransform).
			WithEnvWhitelist("server.hostname").
			Build()

		mustNoError(t, err)

		// CLI should take precedence
		val, _ := cfg.Get("server.hostname")
		checkEqual(t, val, "clihost")
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

		mustNoError(t, err)

		// Verify paths were registered
		paths := cfg.GetRegisteredPaths()
		if !paths["db.host"] {
			t.Errorf("paths[\"db.host\"] should be true")
		}
		if !paths["db.port"] {
			t.Errorf("paths[\"db.port\"] should be true")
		}
		if !paths["cache.ttl"] {
			t.Errorf("paths[\"cache.ttl\"] should be true")
		}

		// Test AsStruct
		result, err := cfg.AsStruct()
		mustNoError(t, err)
		checkEqual(t, result, target)
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

		mustNoError(t, err)
		if got := cfg; got == nil {
			t.Errorf("got %v, want non-nil", got)
		}
		if !validatorCalled {
			t.Errorf("validatorCalled should be true")
		}

		// Invalid case
		validatorCalled = false
		cfg2, err := NewBuilder().
			WithDefaults(&UserConfig{Port: 80}).
			WithValidator(validator).
			Build()

		if got := cfg2; got != nil {
			t.Errorf("got %v, want nil", got)
		}
		checkErrorContains(t, err, "configuration validation failed")
		if !validatorCalled {
			t.Errorf("validatorCalled should be true")
		}
	})

	t.Run("BuilderErrorAccumulation", func(t *testing.T) {
		// Unsupported tag name
		_, err := NewBuilder().
			WithTagName("xml").
			WithDefaults(struct{}{}).
			Build()

		checkErrorContains(t, err, "unsupported tag name")

		// Invalid target
		_, err = NewBuilder().
			WithTarget("not-a-pointer").
			Build()

		checkErrorContains(t, err, "requires non-nil pointer to struct")
	})

	t.Run("MustBuildPanic", func(t *testing.T) {
		// Should not panic with valid config
		{
			cfg := NewBuilder().
				WithDefaults(struct{ Port int }{Port: 8080}).
				MustBuild()
			if got := cfg; got == nil {
				t.Errorf("got %v, want non-nil", got)
			}
		}

		// Should panic with error
		mustPanic(t, func() {
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
		writeTestFile(t, configFile, []byte(`test = "value"`), 0644)

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithArgs([]string{"--config", configFile}).
			WithFileDiscovery(opts).
			Build()

		mustNoError(t, err)

		// Verify file was loaded
		val, _ := cfg.Get("test")
		checkEqual(t, val, "value")
	})

	t.Run("DiscoveryWithEnvVar", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "env.toml")
		writeTestFile(t, configFile, []byte(`test = "envvalue"`), 0644)

		t.Setenv("MYAPP_CONFIG", configFile)

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithFileDiscovery(opts).
			Build()

		mustNoError(t, err)

		val, _ := cfg.Get("test")
		checkEqual(t, val, "envvalue")
	})

	t.Run("DiscoveryInCurrentDir", func(t *testing.T) {
		// Discovery runs in an isolated directory and never writes into the checkout.
		t.Chdir(t.TempDir())
		configFile := "myapp.toml"
		writeTestFile(t, configFile, []byte(`test = "cwdvalue"`), 0644)

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

		mustNoError(t, err)

		val, _ := cfg.Get("test")
		checkEqual(t, val, "cwdvalue")
	})

	t.Run("DiscoveryPrecedence", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple config files
		cliFile := filepath.Join(tmpDir, "cli.toml")
		envFile := filepath.Join(tmpDir, "env.toml")
		writeTestFile(t, cliFile, []byte(`test = "clifile"`), 0644)
		writeTestFile(t, envFile, []byte(`test = "envfile"`), 0644)

		// CLI should take precedence over env
		t.Setenv("MYAPP_CONFIG", envFile)

		opts := DefaultDiscoveryOptions("myapp")

		cfg, err := NewBuilder().
			WithDefaults(struct {
				Test string `toml:"test"`
			}{Test: "default"}).
			WithArgs([]string{"--config", cliFile}).
			WithFileDiscovery(opts).
			Build()

		mustNoError(t, err)

		val, _ := cfg.Get("test")
		checkEqual(t, val, "clifile")
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

		mustNoError(t, err)
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

		if err == nil {
			t.Fatalf("expected an error")
		}
		checkErrorContains(t, err, "typed configuration validation failed: port too low")
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

		if err == nil {
			t.Fatalf("expected an error")
		}
		checkErrorContains(t, err, "typed validator signature")
	})
}
