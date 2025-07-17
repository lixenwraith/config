# Environment Variables

The config package provides flexible environment variable support with automatic name transformation, custom mappings, and whitelist capabilities.

## Basic Usage

Environment variables are automatically mapped from configuration paths:

```go
cfg, _ := config.Quick(defaults, "MYAPP_", "config.toml")

// These environment variables are automatically loaded:
// MYAPP_SERVER_PORT      → server.port
// MYAPP_DATABASE_URL     → database.url
// MYAPP_LOG_LEVEL        → log.level
// MYAPP_FEATURES_ENABLED → features.enabled
```

## Name Transformation

### Default Transformation

By default, paths are transformed as follows:
1. Dots (`.`) become underscores (`_`)
2. Converted to uppercase
3. Prefix is prepended

```go
// Path transformations:
// server.port        → MYAPP_SERVER_PORT
// database.url       → MYAPP_DATABASE_URL
// tls.cert.path      → MYAPP_TLS_CERT_PATH
// maxRetries         → MYAPP_MAXRETRIES
```

### Custom Transformation

Define custom environment variable mappings:

```go
cfg, _ := config.NewBuilder().
    WithEnvTransform(func(path string) string {
        switch path {
        case "server.port":
            return "PORT"  // Use $PORT directly
        case "database.url":
            return "DATABASE_URL"
        case "api.key":
            return "API_KEY"
        default:
            // Fallback to default transformation
            return "MYAPP_" + strings.ToUpper(
                strings.ReplaceAll(path, ".", "_"),
            )
        }
    }).
    Build()
```

### No Transformation

Return empty string to skip environment lookup:

```go
cfg, _ := config.NewBuilder().
    WithEnvTransform(func(path string) string {
        // Only allow specific env vars
        allowed := map[string]string{
            "port":     "PORT",
            "database": "DATABASE_URL",
        }
        return allowed[path] // Empty string if not in map
    }).
    Build()
```

## Explicit Environment Variable Mapping

Use the `env` struct tag for explicit mappings:

```go
type Config struct {
    Port     int    `toml:"port" env:"PORT"`
    Database string `toml:"database" env:"DATABASE_URL"`
    APIKey   string `toml:"api_key" env:"API_KEY"`
}

// These use the explicit env tag names, ignoring prefix
cfg, _ := config.NewBuilder().
    WithDefaults(&Config{}).
    WithEnvPrefix("MYAPP_").  // Not used for tagged fields
    Build()
```

Or register with explicit environment variable:

```go
cfg.RegisterWithEnv("server.port", 8080, "PORT")
cfg.RegisterWithEnv("database.url", "localhost", "DATABASE_URL")
```

## Environment Variable Whitelist

Limit which paths can be set via environment:

```go
cfg, _ := config.NewBuilder().
    WithEnvWhitelist(
        "server.port",
        "database.url",
        "api.key",
        "log.level",
    ).  // Only these paths read from environment
    Build()
```

## Type Conversion

Environment variables (strings) are automatically converted to the registered type:

```bash
# Booleans
export MYAPP_DEBUG=true
export MYAPP_VERBOSE=false

# Numbers
export MYAPP_PORT=8080
export MYAPP_TIMEOUT=30
export MYAPP_RATIO=0.95

# Durations
export MYAPP_TIMEOUT=30s
export MYAPP_INTERVAL=5m

# Lists (comma-separated)
export MYAPP_TAGS=prod,stable,v2
```

## Manual Environment Loading

Load environment variables at any time:

```go
cfg := config.New()
cfg.RegisterStruct("", &Config{})

// Load with prefix
if err := cfg.LoadEnv("MYAPP_"); err != nil {
    log.Fatal(err)
}

// Or use existing options
if err := cfg.LoadWithOptions("", nil, config.LoadOptions{
    EnvPrefix: "MYAPP_",
    Sources:   []config.Source{config.SourceEnv},
}); err != nil {
    log.Fatal(err)
}
```

## Discovering Environment Variables

Find which environment variables are set:

```go
// Discover all env vars matching registered paths
discovered := cfg.DiscoverEnv("MYAPP_")

for path, envVar := range discovered {
    log.Printf("%s is set via %s", path, envVar)
}
```

## Precedence Examples

Default precedence: CLI > Env > File > Default

Custom precedence (Env > File > CLI > Default):

```go
cfg, _ := config.NewBuilder().
    WithSources(
        config.SourceEnv,
        config.SourceFile,
        config.SourceCLI,
        config.SourceDefault,
    ).
    Build()
```

## See Also

- [Command Line](cli.md) - CLI argument handling
- [File Configuration](file.md) - Configuration file formats
- [Access Patterns](access.md) - Retrieving values