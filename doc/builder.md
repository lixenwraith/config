# Builder Pattern

The builder pattern provides fine-grained control over configuration initialization and loading behavior.

## Basic Builder Usage

```go
cfg, err := config.NewBuilder().
    WithDefaults(defaultStruct).
    WithEnvPrefix("MYAPP_").
    WithFile("config.toml").
    Build()
```

## Builder Methods

### WithDefaults

Register a struct containing default values:

```go
type Config struct {
    Host string `toml:"host"`
    Port int    `toml:"port"`
}

defaults := &Config{
    Host: "localhost",
    Port: 8080,
}

cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    Build()
```

### WithTarget

Enable type-aware mode with automatic struct population:

```go
var appConfig Config

cfg, _ := config.NewBuilder().
    WithTarget(&appConfig).  // Registers struct and enables AsStruct()
    WithFile("config.toml").
    Build()

// Access populated struct
populated, _ := cfg.AsStruct()
config := populated.(*Config)
```

### WithTagName

Use different struct tags for field mapping:

```go
type Config struct {
    Server struct {
        Host string `json:"host"`      // Using JSON tags
        Port int    `json:"port"`
    } `json:"server"`
}

cfg, _ := config.NewBuilder().
    WithDefaults(&Config{}).
    WithTagName("json").  // Use json tags instead of toml
    Build()
```

Supported tag names: `toml` (default), `json`, `yaml`

### WithPrefix

Add a prefix to all registered paths:

```go
cfg, _ := config.NewBuilder().
    WithDefaults(serverConfig).
    WithPrefix("server").  // All paths prefixed with "server."
    Build()

// Access as "server.host" instead of just "host"
host, _ := cfg.Get("server.host")
```

### WithEnvPrefix

Set environment variable prefix:

```go
cfg, err := config.NewBuilder().
    WithEnvPrefix("MYAPP_").
    Build()

// Reads from MYAPP_SERVER_PORT for "server.port"
```

### WithSources

Configure source precedence order:

```go
// Environment variables take highest priority
cfg, _ := config.NewBuilder().
    WithSources(
        config.SourceEnv,
        config.SourceFile,
        config.SourceCLI,
        config.SourceDefault,
    ).
    Build()
```

### WithEnvTransform

Custom environment variable name mapping:

```go
cfg, _ := config.NewBuilder().
    WithEnvTransform(func(path string) string {
        // Custom mapping logic
        switch path {
        case "server.port":
            return "PORT"  // Use $PORT instead of $MYAPP_SERVER_PORT
        case "database.url":
            return "DATABASE_URL"
        default:
            // Default transformation
            return "MYAPP_" + strings.ToUpper(
                strings.ReplaceAll(path, ".", "_"),
            )
        }
    }).
    Build()
```

### WithEnvWhitelist

Limit which configuration paths check environment variables:

```go
cfg, _ := config.NewBuilder().
    WithEnvWhitelist(
        "server.port",
        "database.url",
        "api.key",
    ).  // Only these paths read from env
    Build()
```

### WithValidator

Add validation functions that run *before* the target struct is populated. These validators operate on the raw `*config.Config` object and are suitable for checking required paths or formats before type conversion.

```go
// Validator runs on raw, pre-decoded values.
cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    WithValidator(func(c *config.Config) error {
        // Validate port range
        port, _ := c.Get("server.port")
        if p := port.(int64); p < 1024 || p > 65535 {
            return fmt.Errorf("port must be between 1024-65535")
        }
        return nil
    }).
    WithValidator(func(c *config.Config) error {
        // Validate required fields
        return c.Validate("api.key", "database.url")
    }).
    Build()
```

For type-safe validation, see `WithTypedValidator`.

### WithTypedValidator

Add a type-safe validation function that runs *after* the configuration has been fully loaded and decoded into the target struct (set by `WithTarget`). This is the recommended approach for most validation logic.

The validation function must accept a single argument: a pointer to the same struct type that was passed to `WithTarget`.
```go
type AppConfig struct {
    Server struct {
        Port int64 `toml:"port"`
    } `toml:"server"`
}

var target AppConfig

cfg, err := config.NewBuilder().
    WithTarget(&target).
    WithFile("config.toml").
    WithTypedValidator(func(conf *AppConfig) error {
        if conf.Server.Port < 1024 || conf.Server.Port > 65535 {
            return fmt.Errorf("port %d is outside the valid range", conf.Server.Port)
        }
        return nil
    }).
    Build()
```

### WithFile

Set configuration file path:

```go
cfg, _ := config.NewBuilder().
    WithFile("/etc/myapp/config.toml").
    Build()
```

### WithArgs

Override command-line arguments (default is os.Args[1:]):

```go
cfg, _ := config.NewBuilder().
    WithArgs([]string{"--debug", "--server.port=9090"}).
    Build()
```

### WithFileDiscovery

Enable automatic configuration file discovery:

```go
cfg, _ := config.NewBuilder().
    WithFileDiscovery(config.FileDiscoveryOptions{
        Name:       "myapp",
        Extensions: []string{".toml", ".conf"},
        EnvVar:     "MYAPP_CONFIG",
        CLIFlag:    "--config",
        UseXDG:     true,
    }).
    Build()
```

This searches for configuration files in:
1. Path specified by `--config` flag
2. Path in `$MYAPP_CONFIG` environment variable
3. Current directory
4. XDG config directories (`~/.config/myapp/`, `/etc/myapp/`)

## Method Interaction and Precedence

While most builder methods can be chained in any order, it's important to understand how `WithDefaults` and `WithTarget` interact to define the default configuration values.

### `WithDefaults` Has Precedence

**Rule:** If `WithDefaults()` is used anywhere in the chain, it will **always** be the definitive source for default values.

This is the recommended approach for clarity and explicitness. It cleanly separates the struct that defines the defaults from the struct that will be populated.

**Example (Recommended Pattern):**

```go
// initialData contains the fallback values.
initialData := &AppConfig{
    Server: ServerConfig{Port: 8080},
}

// target is an empty shell for population.
var target AppConfig

// WithDefaults explicitly sets the defaults.
// WithTarget sets up the config for type-safe decoding.
cfg, err := config.NewBuilder().
    WithTarget(&target).
    WithDefaults(initialData).
    WithFile("config.toml").
    Build()
```

In this scenario, the `target` struct is *only* used for type information and `AsStruct()` functionality; its initial (zero) values are not used as defaults as per below.

### Using `WithTarget` for Defaults

**Rule:** If `WithDefaults()` is **not** used, the struct passed to `WithTarget()` will serve as the source of default values.

This provides a convenient shorthand for simpler cases where the initial state of your application's config struct *is* the desired default state. The unit tests for the package rely on this behavior.

**Example (Convenience Pattern):**

```go
// The initial state of this struct will be used as the defaults.
target := &AppConfig{
    Server: ServerConfig{Port: 8080},
}

// Since WithDefaults() is absent, the builder uses `target`
// for both defaults and for type-safe decoding.
cfg, err := config.NewBuilder().
    WithTarget(&target).
    WithFile("config.toml").
    Build()
```

## Usage Patterns

### Type-Safe Configuration Access

```go
type AppConfig struct {
    Server ServerConfig `toml:"server"`
    DB     DBConfig     `toml:"database"`
}

var conf AppConfig

cfg, _ := config.NewBuilder().
    WithTarget(&conf).
    WithFile("config.toml").
    Build()

// Direct struct access after building
fmt.Printf("Port: %d\n", conf.Server.Port)

// Or get updated struct anytime
latest, _ := cfg.AsStruct()
appConf := latest.(*AppConfig)
```

### Multi-Stage Validation

```go
cfg, err := config.NewBuilder().
    WithDefaults(defaults).
    // Stage 1: Validate structure
    WithValidator(validateStructure).
    // Stage 2: Validate values
    WithValidator(validateRanges).
    // Stage 3: Validate relationships
    WithValidator(validateRelationships).
    Build()

func validateStructure(c *config.Config) error {
    required := []string{"server.host", "server.port", "database.url"}
    return c.Validate(required...)
}

func validateRanges(c *config.Config) error {
    port, _ := c.Get("server.port")
    if p := port.(int64); p < 1 || p > 65535 {
        return fmt.Errorf("invalid port: %d", p)
    }
    return nil
}

func validateRelationships(c *config.Config) error {
    // Validate that related values make sense together
    // e.g., if SSL is enabled, ensure cert paths are set
    return nil
}
```

### Error Handling

The builder accumulates errors and returns them on `Build()`:

```go
cfg, err := config.NewBuilder().
    WithTarget(nil).          // Error: nil target
    WithTagName("invalid").   // Error: unsupported tag
    Build()

if err != nil {
    // err contains first error encountered
}
```

For panic on error use `MustBuild()`

## See Also

- [Environment Variables](env.md) - Environment configuration details
- [Live Reconfiguration](reconfiguration.md) - File watching with builder