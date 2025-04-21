# Config

A simple, thread-safe configuration management package for Go applications that supports TOML files, command-line argument overrides, and registered default values.

## Features

- **Thread-Safe Operations:** Uses `sync.RWMutex` to protect concurrent access during all configuration operations.
- **TOML Configuration:** Uses [tinytoml](https://github.com/LixenWraith/tinytoml) for loading and saving configuration files.
- **Command-Line Overrides:** Allows overriding configuration values using dot notation in CLI arguments (e.g., `--server.port 9090`).
- **Type-Safe Access:** Register configuration paths with default values and receive unique keys (UUIDs) for consistent access.
- **Atomic File Operations:** Ensures configuration files are written atomically to prevent corruption.
- **Path Validation:** Validates configuration path segments against TOML key requirements.
- **Minimal Dependencies:** Relies only on `tinytoml` and `google/uuid`.

## Installation

```bash
go get github.com/LixenWraith/config
```

Dependencies will be automatically fetched:
```bash
github.com/LixenWraith/tinytoml
github.com/google/uuid
```

## Usage

### Basic Usage Pattern

```go
// 1. Initialize a new Config instance
cfg := config.New()

// 2. Register configuration keys with paths and default values
keyServerHost, err := cfg.Register("server.host", "127.0.0.1")
keyServerPort, err := cfg.Register("server.port", 8080)

// 3. Load configuration from file with CLI argument overrides
fileExists, err := cfg.Load("app_config.toml", os.Args[1:])

// 4. Access configuration values using the registered keys
serverHost, _ := cfg.Get(keyServerHost)
serverPort, _ := cfg.Get(keyServerPort)

// 5. Save configuration (creates the file if it doesn't exist)
err = cfg.Save("app_config.toml")
```

### CLI Arguments

Command-line arguments override file configuration using dot notation:

```bash
# Override server port and enable debug mode
./your_app --server.port 9090 --debug

# Override nested database setting
./your_app --database.connection.pool_size 50
```

Flags without values are treated as boolean `true`.

## API

### `New() *Config`

Creates and returns a new, initialized `*Config` instance ready for use.

### `(*Config) Register(path string, defaultValue any) (string, error)`

Registers a configuration path with a default value and returns a unique UUID key.

- **path**: Dot-separated path corresponding to the TOML structure. Each segment must be a valid TOML key.
- **defaultValue**: The value returned by `Get` if not found in configuration or CLI.
- **Returns**: UUID string (key) for use with `Get` and error (nil on success)

### `(*Config) Get(key string) (any, bool)`

Retrieves a configuration value using the UUID key from `Register`.

- **key**: The UUID string returned by `Register`.
- **Returns**: The configuration value and a boolean indicating if the key was registered.
- **Value precedence**: CLI Argument > Config File Value > Registered Default Value

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
- **UUID-based Access**: Using UUIDs for configuration keys ensures type safety and prevents string typos during runtime access via `Get`. The path remains the persistent identifier in the config file.
- **Map Structure**: Configuration is stored in nested maps of type `map[string]any`.
- **Path Validation**: Configuration paths are validated to ensure they contain only valid TOML key segments.
- **Atomic Saving**: Configuration is written to a temporary file first, then atomically renamed.
- **CLI Argument Types**: Command-line values are automatically parsed into bool, int64, float64, or string. See note below on Type Handling.

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