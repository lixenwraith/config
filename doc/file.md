# Configuration Files

The config package supports TOML configuration files with automatic loading, discovery, and atomic saving.

## TOML Format

TOML (Tom's Obvious, Minimal Language) is the supported configuration format:

```toml
# Basic values
host = "localhost"
port = 8080
debug = false

# Nested sections
[server]
host = "0.0.0.0"
port = 9090
timeout = "30s"

[database]
url = "postgres://localhost/mydb"
max_conns = 25
timeout = "5s"

# Arrays
[features]
enabled = ["auth", "api", "metrics"]

# Inline tables
tls = { enabled = true, cert = "/path/to/cert", key = "/path/to/key" }
```

## Loading Configuration Files

### Basic Loading

```go
cfg := config.New()
cfg.RegisterStruct("", &Config{})

if err := cfg.LoadFile("config.toml"); err != nil {
    if errors.Is(err, config.ErrConfigNotFound) {
        log.Println("Config file not found, using defaults")
    } else {
        log.Fatal("Failed to load config:", err)
    }
}
```

### With Builder

```go
cfg, err := config.NewBuilder().
    WithDefaults(&Config{}).
    WithFile("/etc/myapp/config.toml").
    Build()
```

### Multiple File Attempts

```go
// Try multiple locations
locations := []string{
    "./config.toml",
    "~/.config/myapp/config.toml",
    "/etc/myapp/config.toml",
}

var cfg *config.Config
var err error

for _, path := range locations {
    cfg, err = config.NewBuilder().
        WithDefaults(&Config{}).
        WithFile(path).
        Build()
    
    if err == nil || !errors.Is(err, config.ErrConfigNotFound) {
        break
    }
}
```

## Automatic File Discovery

Use file discovery to find configuration automatically:

```go
cfg, _ := config.NewBuilder().
    WithDefaults(&Config{}).
    WithFileDiscovery(config.FileDiscoveryOptions{
        Name:          "myapp",
        Extensions:    []string{".toml", ".conf"},
        EnvVar:        "MYAPP_CONFIG",
        CLIFlag:       "--config",
        UseXDG:        true,
        UseCurrentDir: true,
        Paths:         []string{"/opt/myapp"},
    }).
    Build()
```

Search order:
1. CLI flag: `--config=/path/to/config.toml`
2. Environment variable: `$MYAPP_CONFIG`
3. Current directory: `./myapp.toml`, `./myapp.conf`
4. XDG config: `~/.config/myapp/myapp.toml`
5. System paths: `/etc/myapp/myapp.toml`
6. Custom paths: `/opt/myapp/myapp.toml`

## Saving Configuration

### Save Current State

```go
// Save all current values atomically
if err := cfg.Save("config.toml"); err != nil {
    log.Fatal("Failed to save config:", err)
}
```

The save operation is atomic - it writes to a temporary file then renames it.

### Save Specific Source

```go
// Save only values from environment variables
if err := cfg.SaveSource("env-config.toml", config.SourceEnv); err != nil {
    log.Fatal(err)
}

// Save only file-loaded values
if err := cfg.SaveSource("file-only.toml", config.SourceFile); err != nil {
    log.Fatal(err)
}
```

### Generate Default Configuration

```go
// Create a default config file
defaults := &Config{}
// ... set default values ...

cfg, _ := config.NewBuilder().
    WithDefaults(defaults).
    Build()

// Save defaults as config template
if err := cfg.SaveSource("config.toml.example", config.SourceDefault); err != nil {
    log.Fatal(err)
}
```

## File Structure Mapping

TOML structure maps directly to dot-notation paths:

```toml
# Maps to "debug"
debug = true

[server]
# Maps to "server.host"
host = "localhost"
# Maps to "server.port"  
port = 8080

[server.tls]
# Maps to "server.tls.enabled"
enabled = true
# Maps to "server.tls.cert"
cert = "/path/to/cert"

[[users]]
# Array elements: "users.0.name", "users.0.role"
name = "admin"
role = "administrator"

[[users]]
# Array elements: "users.1.name", "users.1.role"
name = "user"
role = "standard"
```

## Type Handling

TOML types map to Go types:

```toml
# Strings
name = "myapp"
multiline = """
Line one
Line two
"""

# Numbers
port = 8080          # int64
timeout = 30         # int64
ratio = 0.95         # float64
max_size = 1_000_000 # int64 (underscores allowed)

# Booleans
enabled = true
debug = false

# Dates/Times (RFC 3339)
created_at = 2024-01-15T09:30:00Z
expires = 2024-12-31

# Arrays
ports = [8080, 8081, 8082]
tags = ["production", "stable"]

# Tables (objects)
[database]
host = "localhost"
port = 5432

# Array of tables
[[servers]]
name = "web1"
host = "10.0.0.1"

[[servers]]
name = "web2"  
host = "10.0.0.2"
```

## Error Handling

File loading can produce several error types:

```go
err := cfg.LoadFile("config.toml")
if err != nil {
    switch {
    case errors.Is(err, config.ErrConfigNotFound):
        // File doesn't exist - often not fatal
        log.Println("No config file, using defaults")
        
    case strings.Contains(err.Error(), "failed to parse TOML"):
        // TOML syntax error
        log.Fatal("Invalid TOML syntax:", err)
        
    case strings.Contains(err.Error(), "failed to read"):
        // Permission or I/O error
        log.Fatal("Cannot read config file:", err)
        
    default:
        log.Fatal("Config error:", err)
    }
}
```

## Security Considerations

### File Permissions

```go
// After saving, verify permissions
info, err := os.Stat("config.toml")
if err == nil {
    mode := info.Mode()
    if mode&0077 != 0 {
        log.Warn("Config file is world/group readable")
        // Fix permissions
        os.Chmod("config.toml", 0600)
    }
}
```

### Size Limits

Files and values have size limits:
- Maximum file size: ~10MB (10 * MaxValueSize)
- Maximum value size: 1MB

## Partial Loading

Load only specific sections:

```go
var serverCfg ServerConfig
if err := cfg.Scan("server", &serverCfg); err != nil {
    log.Fatal(err)
}

var dbCfg DatabaseConfig  
if err := cfg.Scan("database", &dbCfg); err != nil {
    log.Fatal(err)
}
```

## Best Practices

1. **Use Example Files**: Generate `.example` files with defaults
2. **Check Permissions**: Ensure config files aren't world-readable
3. **Validate After Load**: Add validators to check loaded values
4. **Handle Missing Files**: Missing config files often aren't fatal
5. **Use Atomic Saves**: The built-in Save method is atomic
6. **Document Structure**: Comment your TOML files thoroughly

## See Also

- [Live Reconfiguration](reconfiguration.md) - Automatic file reloading
- [Builder Pattern](builder.md) - File discovery options
- [Access Patterns](access.md) - Working with loaded values