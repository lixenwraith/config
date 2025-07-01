# Config

Thread-safe configuration management for Go with support for TOML files, environment variables, command-line arguments, and defaults with configurable precedence.

## Installation

```bash
go get github.com/LixenWraith/config
```

## Quick Start

```go
package main

import (
    "log"
	
    "github.com/lixenwraith/config"
)

type AppConfig struct {
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
    // Define defaults
    defaults := AppConfig{}
    defaults.Server.Host = "localhost"
    defaults.Server.Port = 8080
    defaults.Database.URL = "postgres://localhost/myapp"
    defaults.Database.MaxConns = 10

    // Initialize with environment prefix and config file
    cfg, err := config.Quick(defaults, "MYAPP_", "config.toml")
    if err != nil {
        log.Fatal(err)
    }

    // Access values
    host, _ := cfg.String("server.host")
    port, _ := cfg.Int64("server.port")
    dbURL, _ := cfg.String("database.url")
    debug, _ := cfg.Bool("debug")

    log.Printf("Server: %s:%d, DB: %s, Debug: %v", host, port, dbURL, debug)
}
```

**config.toml:**
```toml
[server]
host = "production.example.com"
port = 9090

[database]
url = "postgres://prod-db/myapp"
max_conns = 50

debug = false
```

**Usage:**
```bash
# Override with environment variables
export MYAPP_SERVER_PORT=8443
export MYAPP_DEBUG=true

# Override with CLI arguments  
./myapp --server.port=9999 --debug
```

## Key Features

- **Multiple Sources**: Defaults → File → Environment → CLI (configurable order)
- **Type Safety**: Automatic conversion with detailed error messages
- **Thread-Safe**: Concurrent reads with protected writes
- **Builder Pattern**: Fluent interface for advanced configuration
- **Source Tracking**: See which source provided each value
- **Zero Dependencies**: Only stdlib + minimal parsers

## Common Patterns

### Custom Precedence
```go
cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    WithSources(
        config.SourceEnv,     // Env vars highest priority
        config.SourceFile,
        config.SourceCLI,
        config.SourceDefault,
    ).
    Build()
```

### Environment Variable Mapping
```go
// Custom env var names
opts := config.LoadOptions{
    EnvTransform: func(path string) string {
        switch path {
        case "server.port": return "PORT"
        case "database.url": return "DATABASE_URL"
        default: return ""
        }
    },
}
cfg.LoadWithOptions("config.toml", os.Args[1:], opts)
```

### Validation
```go
// Register and validate required fields
cfg.RegisterRequired("api.key", "")
cfg.RegisterRequired("database.url", "")

if err := cfg.Validate("api.key", "database.url"); err != nil {
    log.Fatal("Missing required config: ", err)
}
```

### Source Inspection
```go
// See all sources for a value
sources := cfg.GetSources("server.port")
for source, value := range sources {
    fmt.Printf("%s: %v\n", source, value)
}

// Get value from specific source
envPort, exists := cfg.GetSource("server.port", config.SourceEnv)
```

### Struct Scanning
```go
var serverConfig struct {
    Host string `toml:"host"`
    Port int    `toml:"port"`
}
cfg.Scan("server", &serverConfig)
```

### Environment Whitelist
```go
// Only load specific env vars
cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    WithEnvPrefix("MYAPP_").
    WithEnvWhitelist("api.key", "database.password").
    Build()
```

## API Reference

### Core Methods
- `Quick(defaults, envPrefix, configFile)` - Quick initialization
- `Register(path, defaultValue)` - Register configuration path
- `Get/String/Int64/Bool/Float64(path)` - Type-safe accessors
- `Set(path, value)` - Update configuration
- `Validate(paths...)` - Ensure required values are set

### Advanced Methods
- `NewBuilder()` - Create custom configuration
- `GetSource(path, source)` - Get value from specific source
- `GetSources(path)` - Get all source values
- `Scan(basePath, target)` - Unmarshal into struct
- `Clone()` - Deep copy configuration
- `Debug()` - Show all values and sources

## License

BSD-3-Clause