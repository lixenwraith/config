# Config

Thread-safe configuration management for Go applications with support for multiple sources (files, environment variables, command-line arguments, defaults) and configurable precedence.

## Features

- **Multiple Sources**: Load configuration from defaults, files, environment variables, and CLI arguments
- **Configurable Precedence**: Control which sources override others
- **Type Safety**: Struct-based configuration with automatic validation
- **Thread-Safe**: Concurrent access with read-write locking
- **File Watching**: Automatic reloading on configuration changes
- **Source Tracking**: Know exactly where each value came from
- **Tag Support**: Use `toml`, `json`, or `yaml` struct tags

## Installation

```bash
go get github.com/lixenwraith/config
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
    Debug bool `toml:"debug"`
}

func main() {
    defaults := &AppConfig{}
    defaults.Server.Host = "localhost"
    defaults.Server.Port = 8080

    cfg, err := config.Quick(defaults, "MYAPP_", "config.toml")
    if err != nil {
        log.Fatal(err)
    }

    port, _ := cfg.Get("server.port")
    log.Printf("Server port: %d", port.(int64))
}
```

## Documentation

- [Quick Start Guide](doc/quick-start.md) - Get up and running quickly
- [Builder Pattern](doc/builder.md) - Advanced configuration with the builder
- [Command Line](doc/cli.md) - CLI argument handling
- [Environment Variables](doc/env.md) - Environment variable configuration
- [Configuration Files](doc/file.md) - File loading and formats
- [Access Patterns](doc/access.md) - Getting and setting values
- [Live Reconfiguration](doc/reconfiguration.md) - File watching and updates

## License

BSD-3-Clause