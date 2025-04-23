# Config

A simple, thread-safe configuration management package for Go applications that supports TOML files, command-line argument overrides, and registered default values.

## Features

- **Thread-Safe Operations:** Uses `sync.RWMutex` to protect concurrent access during all configuration operations.
- **TOML Configuration:** Uses [tinytoml](https://github.com/LixenWraith/tinytoml) for loading and saving configuration files.
- **Command-Line Overrides:** Allows overriding configuration values using dot notation in CLI arguments (e.g., `--server.port 9090`).
- **Path-Based Access:** Register configuration paths with default values for direct, consistent access with clear error messages.
- **Struct Registration:** Register an entire struct as configuration defaults, using struct tags to determine paths.
- **Atomic File Operations:** Ensures configuration files are written atomically to prevent corruption.
- **Path Validation:** Validates configuration path segments against TOML key requirements.
- **Minimal Dependencies:** Relies only on `tinytoml` and `mitchellh/mapstructure`.
- **Struct Unmarshaling:** Supports decoding configuration subtrees into Go structs with the `UnmarshalSubtree` method.
- **Type Conversions:** Helper methods for converting configuration values to common Go types with detailed error messages.
- **Hierarchical Data Management:** Automatically handles nested structures through dot notation.

## Installation

```bash
go get github.com/LixenWraith/config
```

Dependencies will be automatically fetched:
```bash
github.com/LixenWraith/tinytoml
github.com/mitchellh/mapstructure
```

## Usage

### Basic Usage Pattern

```go
// 1. Initialize a new Config instance
cfg := config.New()

// 2. Register configuration paths with default values
err := cfg.Register("server.host", "127.0.0.1")
err = cfg.Register("server.port", 8080)

// 3. Load configuration from file with CLI argument overrides
fileExists, err := cfg.Load("app_config.toml", os.Args[1:])

// 4. Access configuration values using the registered paths
serverHost, err := cfg.String("server.host")
if err != nil {
    log.Fatal(err)
}

serverPort, err := cfg.Int64("server.port")
if err != nil {
    log.Fatal(err)
}

// 5. Save configuration (creates the file if it doesn't exist)
err = cfg.Save("app_config.toml")
```

### Struct-Based Registration

```go
// Define a configuration struct with TOML tags
type ServerConfig struct {
    Host    string `toml:"host"`
    Port    int64  `toml:"port"`
    Timeout int64  `toml:"timeout"`
    Debug   bool   `toml:"debug"`
}

// Create default configuration
defaults := ServerConfig{
    Host:    "localhost",
    Port:    8080,
    Timeout: 30,
    Debug:   false,
}

// Register the entire struct at once
err := cfg.RegisterStruct("server.", defaults)
```

### Accessing Typed Values

```go
// Register configuration paths
cfg.Register("server.port", 8080)
cfg.Register("debug", false)
cfg.Register("rate.limit", 1.5)
cfg.Register("server.name", "default-server")

// Use type-specific accessor methods
port, err := cfg.Int64("server.port")
if err != nil {
    log.Fatalf("Error getting port: %v", err)
}

debug, err := cfg.Bool("debug")
if err != nil {
    log.Fatalf("Error getting debug flag: %v", err)
}

rate, err := cfg.Float64("rate.limit")
if err != nil {
    log.Fatalf("Error getting rate limit: %v", err)
}

name, err := cfg.String("server.name")
if err != nil {
    log.Fatalf("Error getting server name: %v", err)
}
```

## API

### `New() *Config`

Creates and returns a new, initialized `*Config` instance ready for use.

### `(*Config) Register(path string, defaultValue any) error`

Registers a configuration path with a default value.

- **path**: Dot-separated path corresponding to the TOML structure. Each segment must be a valid TOML key.
- **defaultValue**: The value returned if no other value has been set through Load or Set.
- **Returns**: Error (nil on success)

### `(*Config) RegisterStruct(prefix string, structWithDefaults interface{}) error`

Registers all fields of a struct as configuration paths, using struct tags to determine the paths.

- **prefix**: Prefix to prepend to all generated paths (e.g., "server.").
- **structWithDefaults**: Struct containing default values. Fields must have `toml` tags.
- **Returns**: Error if registration fails for any field.

### `(*Config) GetRegisteredPaths(prefix string) map[string]bool`

Returns all registered configuration paths that start with the given prefix.

- **prefix**: Path prefix to filter by (e.g., "server.").
- **Returns**: Map where keys are the registered paths that match the prefix.

### `(*Config) Get(path string) (any, bool)`

Retrieves a configuration value using the registered path.

- **path**: The dot-separated path string used during registration.
- **Returns**: The configuration value and a boolean indicating if the path was registered.
- **Value precedence**: CLI Argument > Config File Value > Registered Default Value

### `(*Config) String(path string) (string, error)`
### `(*Config) Int64(path string) (int64, error)`
### `(*Config) Bool(path string) (bool, error)`
### `(*Config) Float64(path string) (float64, error)`

Type-specific accessor methods that retrieve and attempt to convert configuration values to the desired type.

- **path**: The dot-separated path string used during registration.
- **Returns**: The typed value and an error (nil on success).
- **Errors**: Detailed error messages when:
  - The path is not registered
  - The value cannot be converted to the requested type
  - Type conversion fails (with the specific reason)

### `(*Config) Set(path string, value any) error`

Updates a configuration value using the registered path.

- **path**: The dot-separated path string used during registration.
- **value**: The new value to set.
- **Returns**: Error if the path wasn't registered or if setting the value fails.

### `(*Config) Unregister(path string) error`

Removes a configuration path and all its children from the configuration.

- **path**: The dot-separated path string used during registration.
- **Effects**:
  - Removes the specified path
  - Recursively removes all child paths (e.g., unregistering "server" also removes "server.host", "server.port", etc.)
  - Completely removes both registration and data
- **Returns**: Error if the path wasn't registered.

### `(*Config) UnmarshalSubtree(basePath string, target any) error`

Decodes a section of the configuration into a struct or map.

- **basePath**: Dot-separated path to the configuration subtree.
- **target**: Pointer to a struct or map where the configuration should be unmarshaled.
- **Returns**: Error if unmarshaling fails.

### `(*Config) Load(filePath string, args []string) (bool, error)`

Loads configuration from a TOML file and merges overrides from command-line arguments.

- **filePath**: Path to the TOML configuration file.
- **args**: Command-line arguments (e.g., `os.Args[1:]`).
- **Returns**: Boolean indicating if the file existed and a nil error on success.

### `(*Config) Save(filePath string) error`

Saves the current configuration to the specified TOML file path, performing an atomic write.

- **filePath**: Path where the TOML configuration file will be written.
- **Returns**: Error if marshaling or file operations fail, nil on success.

## Implementation Details

### Key Design Choices

- **Thread Safety**: All operations are protected by a `sync.RWMutex` to support concurrent access.
- **Unified Storage Model**: Uses a `configItem` struct to store both default values and current values for each path.
- **Path-Based Access**: Using path strings directly as configuration keys provides a simple, intuitive API while maintaining the path as the persistent identifier in the config file.
- **Hierarchical Management**: Automatically handles conversion between flat storage and nested TOML structure.
- **Path Validation**: Configuration paths are validated to ensure they contain only valid TOML key segments.
- **Atomic Saving**: Configuration is written to a temporary file first, then atomically renamed.
- **CLI Argument Types**: Command-line values are automatically parsed into bool, int64, float64, or string.
- **Struct Unmarshaling**: The `UnmarshalSubtree` method uses `mapstructure` to decode configuration subtrees into Go structs.

### Naming Conventions

- **Paths**: Configuration paths provided to `Register` (e.g., `"server.port"`) are dot-separated strings.
- **Segments**: Each part of the path between dots (a "segment") must adhere to TOML key naming rules:
  - Must start with a letter (a-z, A-Z) or an underscore (`_`).
  - Subsequent characters can be letters, numbers (0-9), underscores (`_`), or hyphens (`-`).
  - Segments *cannot* contain dots (`.`).

### Type Handling Note

- Values loaded from TOML files or parsed from CLI arguments often result in specific types (e.g., `int64` for integers, `float64` for floats) due to the underlying `tinytoml` and `strconv` packages.
- This might differ from the type of a default value provided during `Register` (e.g., default `int(8080)` vs. loaded `int64(8080)`).
- When retrieving values using `Get`, be mindful of this potential difference and use appropriate type assertions or checks. Consider using `int64` or `float64` for default values where applicable to maintain consistency.
- Alternatively, use the type-specific accessors (`Int64`, `Bool`, `Float64`, `String`) which attempt to convert values to the desired type and provide detailed error messages if conversion fails.

### Merge Behavior Note

- The internal merging logic (used during `Load`) performs a deep merge for nested maps.
- However, non-map types like slices are assigned by reference during the merge. If the source map containing the slice is modified after merging, the change might be reflected in the config data. This is generally not an issue with the standard Load workflow but should be noted if using merge logic independently.

### Limitations

- Supports only basic Go types and structures compatible with the tinytoml package.
- CLI arguments must use `--key value` or `--booleanflag` format.
- Path segments must start with letter/underscore, followed by letters/numbers/dashes/underscores.

## Examples

Complete example programs demonstrating the config package are available in the `cmd` directory.

## License

BSD-3-Clause