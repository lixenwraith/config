# Validation

The config package provides flexible validation through function injection and bundled validators for common scenarios.

## Validation Stages

The package supports two validation stages when using the Builder pattern:

### Pre-Population Validation (`WithValidator`)

Runs on raw configuration values before struct population:

```go
cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    WithValidator(func(c *config.Config) error {
        // Check required paths exist
        return c.Validate("api.key", "database.url")
    }).
    Build()
```

### Post-Population Validation (`WithTypedValidator`)

Type-safe validation after struct is populated:

```go
type AppConfig struct {
    Server struct {
        Port int64 `toml:"port"`
    } `toml:"server"`
}

cfg, _ := config.NewBuilder().
    WithTarget(&AppConfig{}).
    WithTypedValidator(func(cfg *AppConfig) error {
        // No type assertion needed - cfg.Server.Port is int64
        if cfg.Server.Port < 1024 || cfg.Server.Port > 65535 {
            return fmt.Errorf("port %d outside valid range", cfg.Server.Port)
        }
        return nil
    }).
    Build()
```

## Bundled Validators

The package includes common validation functions in the `config` package:

### Numeric Validators

```go
// Port validates TCP/UDP port range (1-65535)
config.Port(8080) // returns nil

// Positive validates positive numbers
config.Positive(42) // returns nil
config.Positive(-1) // returns error

// NonNegative validates non-negative numbers  
config.NonNegative(0)  // returns nil
config.NonNegative(-5) // returns error

// Range creates min/max validators
portValidator := config.Range(1024, 65535)
err := portValidator(8080) // returns nil
```

### String Validators

```go
// NonEmpty validates non-empty strings
config.NonEmpty("hello") // returns nil
config.NonEmpty("")      // returns error

// Pattern creates regex validators
emailPattern := config.Pattern(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
err := emailPattern("user@example.com") // returns nil

// URLPath validates URL path format
config.URLPath("/api/v1") // returns nil
config.URLPath("api/v1")  // returns error (must start with /)
```

### Network Validators

```go
// IPAddress validates any IP address
config.IPAddress("192.168.1.1") // returns nil
config.IPAddress("::1")         // returns nil

// IPv4Address validates IPv4 only
config.IPv4Address("192.168.1.1") // returns nil
config.IPv4Address("::1")         // returns error

// IPv6Address validates IPv6 only
config.IPv6Address("2001:db8::1") // returns nil
config.IPv6Address("192.168.1.1") // returns error
```

### Enum Validators

```go
// OneOf creates allowed values validator
logLevel := config.OneOf("debug", "info", "warn", "error")
err := logLevel("info")  // returns nil
err = logLevel("trace")  // returns error
```

## Integration Example

```go
type ServerConfig struct {
    Host string `toml:"host"`
    Port int64  `toml:"port"`
    Mode string `toml:"mode"`
}

cfg, err := config.NewBuilder().
    WithTarget(&ServerConfig{}).
    WithFile("config.toml").
    WithTypedValidator(func(c *ServerConfig) error {
        // Use bundled validators
        if err := config.IPAddress(c.Host); err != nil {
            return fmt.Errorf("invalid host: %w", err)
        }
        
        if err := config.Port(c.Port); err != nil {
            return fmt.Errorf("invalid port: %w", err)
        }
        
        modeValidator := config.OneOf("development", "production")
        if err := modeValidator(c.Mode); err != nil {
            return fmt.Errorf("invalid mode: %w", err)
        }
        
        return nil
    }).
    Build()
```

## Integration with go-playground/validator

The package works seamlessly with `go-playground/validator`:

```go
import "github.com/go-playground/validator/v10"

type ServerConfig struct {
    Host string `toml:"host" validate:"required,hostname"`
    Port int    `toml:"port" validate:"required,min=1024,max=65535"`
    
    TLS struct {
        Enabled bool   `toml:"enabled"`
        Cert    string `toml:"cert" validate:"required_if=Enabled true"`
        Key     string `toml:"key" validate:"required_if=Enabled true"`
    } `toml:"tls"`
}

// Load configuration
cfg, _ := config.NewBuilder().
    WithDefaults(&ServerConfig{}).
    Build()

// Get populated struct
serverCfg, _ := cfg.AsStruct()

// Validate with go-playground/validator
validate := validator.New()
if err := validate.Struct(serverCfg); err != nil {
    return err // Validation errors
}
```

Tags don't conflict since each package uses different tag names (`toml`/`json`/`yaml` for config, `validate` for validator).