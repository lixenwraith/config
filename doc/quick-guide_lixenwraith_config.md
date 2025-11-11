# lixenwraith/config Quick Reference Guide

This guide details the `lixenwraith/config` package for thread-safe Go configuration.
It supports multiple sources (files, environment, CLI), type-safe struct population, and live reconfiguration.

## Quick Start: Recommended Usage

The recommended pattern uses the **Builder** with a **target struct** providing compile-time type safety and eliminating the need for runtime type assertions

```go
package main

import (
	"fmt"
	"log"
	"os"
    
	"github.com/lixenwraith/config"
)

// 1. Define your application's configuration struct
type AppConfig struct {
	Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	} `toml:"server"`
	Debug bool `toml:"debug"`
}

func main() {
	// 2. Create a target instance with defaults
	// Method A: Direct initialization (cleaner for simple defaults)
	target := &AppConfig{}
	target.Server.Host = "localhost"
	target.Server.Port = 8080

	// Method B: Use WithDefaults() for explicit separation (shown below)

	// 3. Use the builder to configure and load all sources
	cfg, err := config.NewBuilder().
		WithTarget(target).               // Enable type-safe mode and register struct fields
		// WithDefaults(&AppConfig{...}), // Optional: Override defaults from WithTarget
		WithFile("config.toml").          // Load from file (supports .toml, .json, .yaml)
		WithEnvPrefix("APP_").            // Load from environment (e.g., APP_SERVER_PORT)
		WithArgs(os.Args[1:]).            // Load from command-line flags (e.g., --server.port=9090)
		Build()                           // Build the final config object
	if err != nil {
		log.Fatalf("Config build failed: %v", err)
	}

	// 4. Access the fully populated, type-safe struct
	// The `target` variable is now populated with the final merged values
	fmt.Printf("Running on %s:%d\n", target.Server.Host, target.Server.Port)

    // Or, retrieve the updated struct at any time (e.g., after a live reload)
    latest, _ := cfg.AsStruct()
    latestConfig := latest.(*AppConfig)
    fmt.Printf("Debug mode: %v\n", latestConfig.Debug)
}
```

## Supported Formats: TOML, JSON, YAML

The package supports multiple file formats. The format is auto-detected from the file extension (`.toml`, `.json`, `.yaml`, `.yml`) or file content

*   To specify a format explicitly, use `Builder.WithFileFormat("json")`
*   The default struct tag for field mapping is `toml`, but can be changed with `Builder.WithTagName("json")`

## Builder Pattern

The `Builder` is the primary way to construct a `Config` instance

```go
// NewBuilder creates a new configuration builder
func NewBuilder() *Builder
// Build finalizes configuration; returns the first of any accumulated errors
func (b *Builder) Build() (*Config, error)
// MustBuild is like Build but panics on fatal errors
func (b *Builder) MustBuild() *Config
```

### Builder Methods

*   `WithTarget(target any)`: **(Recommended)** Registers fields from the target struct and allows access via `AsStruct()`
*   `WithDefaults(defaults any)`: Explicitly sets a struct containing default values, overriding any default values already in struct from `WithTarget`
*   `WithFile(path string)`: Sets the configuration file path
*   `WithFileDiscovery(opts FileDiscoveryOptions)`: Enables automatic config file discovery (searches CLI flags, env vars, XDG paths, and current directory)
*   `WithArgs(args []string)`: Sets the command-line arguments to parse (e.g., `--server.port=9090`)
*   `WithEnvPrefix(prefix string)`: Sets the global environment variable prefix (e.g., `MYAPP_`)
*   `WithSources(sources ...Source)`: Overrides the default source precedence order
*   `WithTypedValidator(fn any)`: **(Recommended for validation)** Adds a type-safe validation function with signature `func(c *YourConfigType) error` that runs *after* the target struct is populated
*   `WithValidator(fn ValidatorFunc)`: Adds a validation function that runs *before* type-safe population, operating on the raw `*Config` object
*   `WithTagName(tagName string)`: Sets the primary struct tag for field mapping (`"toml"`, `"json"`, `"yaml"`)
*   `WithPrefix(prefix string)`: Adds a prefix to all registered paths from a struct
*   `WithFileFormat(format string)`: Explicitly sets the file format (`"toml"`, `"json"`, `"yaml"`, `"auto"`)
*   `WithSecurityOptions(opts SecurityOptions)`: Sets security options for file loading (path traversal, file size limits)
*   `WithEnvTransform(fn EnvTransformFunc)`: Sets a custom environment variable mapping function
*   `WithEnvWhitelist(paths ...string)`: Limits environment variable loading to a specific set of paths

## Type-Safe Access & Population

These are the **preferred methods** for accessing configuration data

*   `AsStruct() (any, error)`: After using `Builder.WithTarget()`, this method returns the populated, type-safe target struct - This is the primary way to access config after initialization or live reload
*   `Scan(basePath string, target any)`: Populates a struct with values from a specific config path (e.g., `cfg.Scan("server", &serverConf)`)
*   `GetTyped[T](c *Config, path string) (T, error)`: Retrieves a single value and decodes it to type `T`, handling type conversion automatically
*   `ScanTyped[T](c *Config, basePath ...string) (*T, error)`: A generic wrapper around `Scan` that allocates, populates, and returns a pointer to a struct of type `T`

## Live Reconfiguration

Enable automatic reloading of configuration when the source file changes

*   `AutoUpdate()`: Enables file watching and automatic reloading with default options
*   `AutoUpdateWithOptions(opts WatchOptions)`: Enables reloading with custom options (e.g., poll interval, debounce)
*   `StopAutoUpdate()`: Stops the file watcher
*   `Watch() <-chan string`: Returns a channel that receives the paths of changed values
*   `WatchFile(filePath string, formatHint ...string)`: Switches the watcher to a new file at runtime
*   `IsWatching() bool`: Returns `true` if the file watcher is active

The watch channel also receives special notifications: `"file_deleted"`, `"permissions_changed"`, `"reload_error:..."`

## Source Precedence

The default order of precedence (highest to lowest) is:
1.  **CLI**: Command-line arguments (`--server.port=9090`)
2.  **Env**: Environment variables (`MYAPP_SERVER_PORT=8888`)
3.  **File**: Configuration file (`config.toml`)
4.  **Default**: Values registered from a struct

This order can be changed via `Builder.WithSources()` or `Config.SetPrecedence()`

## Dynamic / Legacy Value Access

These methods are for dynamic key-value access that require type assertion and should be **avoided when a type-safe struct can be used**

*   `Get(path string) (any, bool)`: Retrieves the final merged value. The `bool` indicates if the path was registered. Requires a type assertion, e.g., `port := val.(int64)`
*   `Set(path string, value any)`: Updates a value in the highest priority source. The path must be registered first
*   `GetSource(path string, source Source) (any, bool)`: Retrieves a value from a specific source layer
*   `SetSource(path string, source Source, value any)`: Sets a value for a specific source layer

## API Reference Summary

### Core Types

*   `Config`: The primary thread-safe configuration manager
*   `Source`: A configuration source (`SourceCLI`, `SourceEnv`, `SourceFile`, `SourceDefault`)
*   `LoadOptions`: Options for loading configuration from multiple sources
*   `Builder`: Fluent API for constructing a `Config` instance

### Core Methods

*   `New() *Config`: Creates a new `Config` instance.
*   `Register(path string, defaultValue any)`: Registers a path with a default value
*   `RegisterStruct(prefix string, structWithDefaults any)`: Recursively registers fields from a struct using `toml` tags
*   `Validate(required ...string)`: Checks that all specified required paths have been set from a non-default source
*   `Save(path string)`: Atomically saves the current merged configuration state to a file
*   `Clone() *Config`: Creates a deep copy of the configuration state
*   `Debug() string`: Returns a formatted string of all values for debugging

### Utility Functions

*   `FlattenMap(nested map[string]any, prefix string) map[string]any`: Converts a nested map to a flat map with dot-notation keys
*   `SetNestedValue(nested map[string]any, path string, value any)`: Sets a value in a nested map using a dot-notation path
*   `IsValidKeySegment(s string) bool`: Checks if a string is a valid path segment