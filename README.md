# Config

A simple, thread-safe configuration management package for Go applications that supports TOML files, command-line argument overrides, and registered default values.

## Features

- **Thread-Safe Operations:** Uses `sync.RWMutex` to protect concurrent access during all configuration operations.
- **TOML Configuration:** Uses [BurntSushi/toml](https://github.com/BurntSushi/toml) for loading and saving configuration files.
- **Command-Line Overrides:** Allows overriding configuration values using dot notation in CLI arguments (e.g., `--server.port 9090`).
- **Path-Based Access:** Register configuration paths with default values for direct, consistent access with clear error messages.
- **Struct Registration:** Register an entire struct as configuration defaults, using struct tags to determine paths.
- **Atomic File Operations:** Ensures configuration files are written atomically to prevent corruption.
- **Path Validation:** Validates configuration path segments against TOML key requirements.
- **Type Conversions:** Helper methods for converting configuration values to common Go types with detailed error messages.
- **Hierarchical Data Management:** Automatically handles nested structures through dot notation.

## Installation

```bash
go get github.com/LixenWraith/config
```

Dependencies will be automatically fetched:
```
github.com/BurntSushi/toml
github.com/mitchellh/mapstructure
```

## Usage

### Basic Usage Pattern

```go
// 1. Initialize a new Config instance
cfg := config.New()

// 2. Register configuration paths with default values
cfg.Register("server.host", "127.0.0.1")
cfg.Register("server.port", 8080)

// 3. Load configuration from file with CLI argument overrides
err := cfg.Load("app_config.toml", os.Args[1:])
if err != nil {
    if errors.Is(err, config.ErrConfigNotFound) {
        log.Println("Config file not found, using defaults")
    } else {
        log.Fatal(err)
    }
}

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
// Use type-specific accessor methods
port, err := cfg.Int64("server.port")
debug, err := cfg.Bool("debug")
rate, err := cfg.Float64("rate.limit")
name, err := cfg.String("server.name")
```

### Using Scan to Populate Structs

```go
// Define a struct matching your configuration
type AppConfig struct {
    ServerName string `toml:"name"`
    ServerPort int64  `toml:"port"`
    Debug      bool   `toml:"debug"`
}

// Create an instance to receive the configuration
var appConfig AppConfig

// Scan the configuration into the struct
err := cfg.Scan("server", &appConfig)
if err != nil {
    log.Fatal(err)
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

### `(*Config) Scan(basePath string, target any) error`

Decodes a section of the configuration into a struct or map.

- **basePath**: Dot-separated path to the configuration subtree.
- **target**: Pointer to a struct or map where the configuration should be unmarshaled.
- **Returns**: Error if unmarshaling fails.

### `(*Config) Load(filePath string, args []string) error`

Loads configuration from a TOML file and merges overrides from command-line arguments.

- **filePath**: Path to the TOML configuration file.
- **args**: Command-line arguments (e.g., `os.Args[1:]`).
- **Returns**: Error on failure, which can be checked with:
  - `errors.Is(err, config.ErrConfigNotFound)` to detect missing file
  - `errors.Is(err, config.ErrCLIParse)` to detect CLI parsing errors

### `(*Config) Save(filePath string) error`

Saves the current configuration to the specified TOML file path, performing an atomic write.

- **filePath**: Path where the TOML configuration file will be written.
- **Returns**: Error if marshaling or file operations fail, nil on success.

## License

BSD-3-Clause