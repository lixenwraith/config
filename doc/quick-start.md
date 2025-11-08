# Quick Start Guide

This guide gets you up and running with the config package in minutes.

## Basic Usage

The simplest way to use the config package is with the `Quick` function:

```go
package main

import (
    "log"
    "github.com/lixenwraith/config"
)

// Define your configuration structure
type Config struct {
    Server struct {
        Host string `toml:"host"`
        Port int    `toml:"port"`
    } `toml:"server"`
    Database struct {
        URL      string `toml:"url"`
        MaxConns int    `toml:"max_conns"`
    } `toml:"database"`
    Debug bool `toml:"debug"`
}

func main() {
    // Create defaults
    defaults := &Config{}
    defaults.Server.Host = "localhost"
    defaults.Server.Port = 8080
    defaults.Database.URL = "postgres://localhost/mydb"
    defaults.Database.MaxConns = 10
    defaults.Debug = false

    // Initialize configuration
    cfg, err := config.Quick(
        defaults,      // Default values from struct
        "MYAPP_",      // Environment variable prefix
        "config.toml", // Configuration file path
    )
    if err != nil {
        log.Fatal(err)
    }

    // Access values
    port, _ := cfg.Get("server.port")
    dbURL, _ := cfg.Get("database.url")
    
    log.Printf("Server running on port %d", port.(int64))
    log.Printf("Database URL: %s", dbURL.(string))
}
```

## Configuration Sources

The package loads configuration from multiple sources in this default order (highest to lowest priority):

1. **Command-line arguments** - Override everything
2. **Environment variables** - Override file and defaults
3. **Configuration file** - Override defaults
4. **Default values** - Base configuration

### Command-Line Arguments

```bash
./myapp --server.port=9090 --debug
```

### Environment Variables

```bash
export MYAPP_SERVER_PORT=9090
export MYAPP_DATABASE_URL="postgres://prod/mydb"
export MYAPP_DEBUG=true
```

### Configuration File (config.toml)

```toml
[server]
host = "0.0.0.0"
port = 8080

[database]
url = "postgres://localhost/mydb"
max_conns = 25

debug = false
```

## Type Safety

The package uses struct tags to ensure type safety. When you register a struct, the types are enforced:

```go
// This struct defines the expected types
type Config struct {
    Port int64  `toml:"port"`    // Must be a number
    Host string `toml:"host"`    // Must be a string
    Debug bool  `toml:"debug"`   // Must be a boolean
}

// Type assertions are safe after registration
port, _ := cfg.Get("port")
portNum := port.(int64)  // Type matches registration
```

## Error Handling

The package validates types during loading:

```go
cfg, err := config.Quick(defaults, "APP_", "config.toml")
if err != nil {
    // Handle errors like:
    // - Invalid TOML syntax
    // - Type mismatches (e.g., string value for int field)
    // - File permissions issues
    log.Fatal(err)
}
```

### Error Categories

The package uses structured error categories for better error handling. Check errors using `errors.Is()`:
```go
if err := cfg.LoadFile("config.toml"); err != nil {
    switch {
    case errors.Is(err, config.ErrConfigNotFound):
        // Config file doesn't exist, use defaults
    case errors.Is(err, config.ErrFileAccess):
        // Permission denied or file access issues
    case errors.Is(err, config.ErrFileFormat):
        // Invalid TOML/JSON/YAML syntax
    case errors.Is(err, config.ErrTypeMismatch):
        // Value type doesn't match registered type
    case errors.Is(err, config.ErrValidation):
        // Validation failed
    default:
        log.Fatal(err)
    }
}
```

See [Errors](./error.go) for the complete list of error categories and description.

## Common Patterns

### Required Fields

```go
// Register required configuration
cfg.RegisterRequired("api.key", "")
cfg.RegisterRequired("database.url", "")

// Validate all required fields are set
if err := cfg.Validate("api.key", "database.url"); err != nil {
    log.Fatal("Missing required configuration:", err)
}
```

### Type-Safe Validation

```go
cfg, _ := config.NewBuilder().
    WithTarget(&Config{}).
    WithTypedValidator(func(c *Config) error {
        if c.Server.Port < 1024 {
            return fmt.Errorf("port must be >= 1024")
        }
        return nil
    }).
    Build()
```

See [Validation](validator.md) for more validation options.

### Using Different Struct Tags

```go
// Use JSON tags instead of default TOML
type Config struct {
    Server struct {
        Host string `json:"host"`
        Port int    `json:"port"`
    } `json:"server"`
}

cfg, _ := config.NewBuilder().
    WithTarget(&Config{}).
    WithTagName("json").
    WithFile("config.json").
    Build()
```

### Checking Value Sources

```go
// See which source provided a value
port, _ := cfg.Get("server.port")
sources := cfg.GetSources("server.port")

for source, value := range sources {
    log.Printf("server.port from %s: %v", source, value)
}
```

## Next Steps

- [Builder Pattern](builder.md) - Advanced configuration options
- [Access Patterns](access.md) - All ways to get and set values