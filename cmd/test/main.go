// File: lixenwraith/cmd/test/main.go
// Test program for the config package
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/lixenwraith/config"
)

// AppConfig represents a simple application configuration
type AppConfig struct {
	Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	} `toml:"server"`
	Database struct {
		URL      string `toml:"url"`
		MaxConns int    `toml:"max_conns"`
	} `toml:"database"`
	API struct {
		Key     string `toml:"key" env:"CUSTOM_API_KEY"` // Custom env mapping
		Timeout int    `toml:"timeout"`
	} `toml:"api"`
	Debug   bool   `toml:"debug"`
	LogFile string `toml:"log_file"`
}

func main() {
	fmt.Println("=== Config Package Feature Test ===\n")

	// Test directories
	tempDir := os.TempDir()
	configPath := filepath.Join(tempDir, "test_config.toml")
	defer os.Remove(configPath)

	// Set up test environment variables
	setupEnvironment()
	defer cleanupEnvironment()

	// Run feature tests
	testQuickStart()
	testBuilder()
	testSourceTracking()
	testEnvironmentFeatures()
	testValidation()
	testUtilities()

	fmt.Println("\n=== All Tests Complete ===")
}

func testQuickStart() {
	fmt.Println("=== Test 1: Quick Start ===")

	// Define defaults
	defaults := AppConfig{}
	defaults.Server.Host = "localhost"
	defaults.Server.Port = 8080
	defaults.Database.URL = "postgres://localhost/testdb"
	defaults.Database.MaxConns = 10
	defaults.Debug = false

	// Quick initialization
	cfg, err := config.Quick(defaults, "TEST_", "")
	if err != nil {
		log.Fatalf("Quick init failed: %v", err)
	}

	// Access values
	host, _ := cfg.String("server.host")
	port, _ := cfg.Int64("server.port")
	fmt.Printf("Quick config - Host: %s, Port: %d\n", host, port)

	// Verify env override (TEST_DEBUG=true was set)
	debug, _ := cfg.Bool("debug")
	fmt.Printf("Debug from env: %v (should be true)\n", debug)
}

func testBuilder() {
	fmt.Println("\n=== Test 2: Builder Pattern ===")

	defaults := AppConfig{}
	defaults.Server.Port = 8080
	defaults.API.Timeout = 30

	// Custom precedence: Env > File > CLI > Default
	cfg, err := config.NewBuilder().
		WithDefaults(defaults).
		WithEnvPrefix("APP_").
		WithSources(
			config.SourceEnv,
			config.SourceFile,
			config.SourceCLI,
			config.SourceDefault,
		).
		WithArgs([]string{"--server.port=9999"}).
		Build()

	if err != nil {
		log.Fatalf("Builder failed: %v", err)
	}

	// ENV should win over CLI due to custom precedence
	port, _ := cfg.Int64("server.port")
	fmt.Printf("Port with Env > CLI precedence: %d (should be 7070 from env)\n", port)
}

func testSourceTracking() {
	fmt.Println("\n=== Test 3: Source Tracking ===")

	cfg := config.New()
	cfg.Register("test.value", "default")

	// Set from multiple sources
	cfg.SetSource("test.value", config.SourceFile, "from-file")
	cfg.SetSource("test.value", config.SourceEnv, "from-env")
	cfg.SetSource("test.value", config.SourceCLI, "from-cli")

	// Show all sources
	sources := cfg.GetSources("test.value")
	fmt.Println("All sources for test.value:")
	for source, value := range sources {
		fmt.Printf("  %s: %v\n", source, value)
	}

	// Get from specific source
	envVal, exists := cfg.GetSource("test.value", config.SourceEnv)
	fmt.Printf("Value from env source: %v (exists: %v)\n", envVal, exists)

	// Current value (default precedence)
	current, _ := cfg.String("test.value")
	fmt.Printf("Current value: %s (should be from-cli)\n", current)
}

func testEnvironmentFeatures() {
	fmt.Println("\n=== Test 4: Environment Features ===")

	cfg := config.New()
	cfg.Register("api.key", "")
	cfg.Register("api.secret", "")
	cfg.Register("database.host", "localhost")

	// Test 4a: Custom env transform
	fmt.Println("\n4a. Custom Environment Transform:")
	opts := config.LoadOptions{
		Sources: []config.Source{config.SourceEnv, config.SourceDefault},
		EnvTransform: func(path string) string {
			switch path {
			case "api.key":
				return "CUSTOM_API_KEY"
			case "database.host":
				return "DB_HOST"
			default:
				return ""
			}
		},
	}
	cfg.LoadWithOptions("", nil, opts)

	apiKey, _ := cfg.String("api.key")
	fmt.Printf("API Key from CUSTOM_API_KEY: %s\n", apiKey)

	// Test 4b: Discover environment variables
	fmt.Println("\n4b. Environment Discovery:")
	cfg2 := config.New()
	cfg2.Register("server.port", 8080)
	cfg2.Register("debug", false)
	cfg2.Register("api.timeout", 30)

	discovered := cfg2.DiscoverEnv("TEST_")
	fmt.Println("Discovered env vars with TEST_ prefix:")
	for path, envVar := range discovered {
		fmt.Printf("  %s -> %s\n", path, envVar)
	}

	// Test 4c: Export configuration as env vars
	fmt.Println("\n4c. Export as Environment:")
	cfg2.Set("server.port", 3000)
	cfg2.Set("debug", true)

	exports := cfg2.ExportEnv("EXPORT_")
	fmt.Println("Non-default values exported:")
	for env, value := range exports {
		fmt.Printf("  export %s=%s\n", env, value)
	}

	// Test 4d: RegisterWithEnv
	fmt.Println("\n4d. RegisterWithEnv:")
	cfg3 := config.New()
	err := cfg3.RegisterWithEnv("special.value", "default", "SPECIAL_ENV_VAR")
	if err != nil {
		fmt.Printf("RegisterWithEnv error: %v\n", err)
	}
	special, _ := cfg3.String("special.value")
	fmt.Printf("Value from SPECIAL_ENV_VAR: %s\n", special)
}

func testValidation() {
	fmt.Println("\n=== Test 5: Validation ===")

	cfg := config.New()
	cfg.RegisterRequired("api.key", "")
	cfg.RegisterRequired("database.url", "")
	cfg.Register("optional.setting", "default")

	// Should fail validation
	err := cfg.Validate("api.key", "database.url")
	if err != nil {
		fmt.Printf("Validation failed as expected: %v\n", err)
	}

	// Set required values
	cfg.Set("api.key", "secret-key")
	cfg.Set("database.url", "postgres://localhost/db")

	// Should pass validation
	err = cfg.Validate("api.key", "database.url")
	if err == nil {
		fmt.Println("Validation passed after setting required values")
	}
}

func testUtilities() {
	fmt.Println("\n=== Test 6: Utility Features ===")

	// Create config with some data
	cfg := config.New()
	cfg.Register("app.name", "testapp")
	cfg.Register("app.version", "1.0.0")
	cfg.Register("server.port", 8080)

	cfg.SetSource("app.version", config.SourceFile, "1.1.0")
	cfg.SetSource("server.port", config.SourceEnv, 9090)

	// Test 6a: Debug output
	fmt.Println("\n6a. Debug Output:")
	debug := cfg.Debug()
	fmt.Printf("Debug info (first 200 chars): %.200s...\n", debug)

	// Test 6b: Clone
	fmt.Println("\n6b. Clone Configuration:")
	clone := cfg.Clone()
	clone.Set("app.name", "cloned-app")

	original, _ := cfg.String("app.name")
	cloned, _ := clone.String("app.name")
	fmt.Printf("Original app.name: %s, Cloned: %s\n", original, cloned)

	// Test 6c: Reset source
	fmt.Println("\n6c. Reset Sources:")
	sources := cfg.GetSources("server.port")
	fmt.Printf("Sources before reset: %v\n", sources)

	cfg.ResetSource(config.SourceEnv)
	sources = cfg.GetSources("server.port")
	fmt.Printf("Sources after env reset: %v\n", sources)

	// Test 6d: Save and load specific source
	fmt.Println("\n6d. Save/Load Specific Source:")
	tempFile := filepath.Join(os.TempDir(), "source_test.toml")
	defer os.Remove(tempFile)

	err := cfg.SaveSource(tempFile, config.SourceFile)
	if err != nil {
		fmt.Printf("SaveSource error: %v\n", err)
	} else {
		fmt.Println("Saved SourceFile values to temp file")
	}

	// Test 6e: GetRegisteredPaths
	fmt.Println("\n6e. Registered Paths:")
	paths := cfg.GetRegisteredPaths("app.")
	fmt.Printf("Paths with 'app.' prefix: %v\n", paths)

	pathsWithDefaults := cfg.GetRegisteredPathsWithDefaults("app.")
	for path, def := range pathsWithDefaults {
		fmt.Printf("  %s: %v\n", path, def)
	}
}

func setupEnvironment() {
	// Set test environment variables
	os.Setenv("TEST_DEBUG", "true")
	os.Setenv("TEST_SERVER_PORT", "6666")
	os.Setenv("APP_SERVER_PORT", "7070")
	os.Setenv("CUSTOM_API_KEY", "env-api-key")
	os.Setenv("DB_HOST", "env-db-host")
	os.Setenv("SPECIAL_ENV_VAR", "special-value")
}

func cleanupEnvironment() {
	os.Unsetenv("TEST_DEBUG")
	os.Unsetenv("TEST_SERVER_PORT")
	os.Unsetenv("APP_SERVER_PORT")
	os.Unsetenv("CUSTOM_API_KEY")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("SPECIAL_ENV_VAR")
}