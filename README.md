# Config

A simple configuration management package for Go applications that supports TOML files and CLI arguments.

## Features

- TOML configuration with [tinytoml](https://github.com/LixenWraith/tinytoml)
- Command line argument overrides with dot notation
- Default config handling
- Atomic file operations
- No external dependencies beyond tinytoml

## Installation

```bash
go get github.com/example/config
```

## Usage

```go
type AppConfig struct {
    Server struct {
        Host string `toml:"host"`
        Port int    `toml:"port"`
    } `toml:"server"`
}

func main() {
    cfg := AppConfig{
        Host: "localhost",
        Port: 8080,
    } // default config
    exists, err := config.LoadConfig("config.toml", &cfg, os.Args[1:])
    if err != nil {
        log.Fatal(err)
    }

    if !exists {
        if err := config.SaveConfig("config.toml", &cfg); err != nil {
            log.Fatal(err)
        }
    }
}
```

### CLI Arguments

Override config values using dot notation:
```bash
./app --server.host localhost --server.port 8080
```

## API

### `LoadConfig(path string, config interface{}, args []string) (bool, error)`
Loads configuration from TOML file and CLI args. Returns true if config file exists.

### `SaveConfig(path string, config interface{}) error`
Saves configuration to TOML file atomically.

## Limitations

- Supports only basic Go types and structures supported by tinytoml
- CLI arguments must use `--key value` format
- Indirect dependency on [mapstructure](https://github.com/mitchellh/mapstructure) through tinytoml

## License

BSD-3