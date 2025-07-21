# Access Patterns

This guide covers all methods for getting and setting configuration values, type conversions, and working with structured data.

**Always Register First**: Register paths before setting values
**Use Type Assertions**: After struct registration, types are guaranteed

## Getting Values

### Basic Get

```go
// Get returns (value, exists)
value, exists := cfg.Get("server.port")
if !exists {
    log.Fatal("server.port not configured")
}

// Type assertion (safe after registration)
port := value.(int64)
```

### Type-Safe Access

When using struct registration, types are guaranteed:

```go
type Config struct {
    Server struct {
        Port int64  `toml:"port"`
        Host string `toml:"host"`
    } `toml:"server"`
}

cfg.RegisterStruct("", &Config{})

// After registration, type assertions are safe
port, _ := cfg.Get("server.port")
portNum := port.(int64)  // Won't panic - type is enforced
```

### Get from Specific Source

```go
// Get value from specific source
envPort, exists := cfg.GetSource(config.SourceEnv, "server.port")
if exists {
    log.Printf("Port from environment: %v", envPort)
}

// Check all sources
sources := cfg.GetSources("server.port")
for source, value := range sources {
    log.Printf("%s: %v", source, value)
}
```

### Struct Scanning

```go
// Scan into struct
var serverConfig struct {
    Host string `toml:"host"`
    Port int64  `toml:"port"`
    TLS  struct {
        Enabled bool   `toml:"enabled"`
        Cert    string `toml:"cert"`
    } `toml:"tls"`
}

if err := cfg.Scan(&serverConfig, "server"); err != nil {
    log.Fatal(err)
}

// Use structured data
log.Printf("Server: %s:%d", serverConfig.Host, serverConfig.Port)
```

### Target Population

```go
// Populate entire config struct
var config AppConfig
if err := cfg.Target(&config); err != nil {
    log.Fatal(err)
}

// Or with builder pattern
var config AppConfig
cfg, _ := config.NewBuilder().
    WithTarget(&config).
    Build()

// Access directly
fmt.Println(config.Server.Port)
```

### GetTyped

Retrieves a single configuration value and decodes it to the specified type.

```go
import "time"

// Returns an int, converting from string "9090" if necessary.
port, err := config.GetTyped[int](cfg, "server.port")

// Returns a time.Duration, converting from string "5m30s".
timeout, err := config.GetTyped[time.Duration](cfg, "server.timeout")
```

### ScanTyped

A generic wrapper around `Scan` that allocates, populates, and returns a pointer to a struct of the specified type.

```go
// Instead of:
// var dbConf DBConfig
// if err := cfg.Scan("database", &dbConf); err != nil { ... }

// You can write:
dbConf, err := config.ScanTyped[DBConfig](cfg, "database")
if err != nil {
    // ...
}
// dbConf is a *DBConfig```
```

### Type-Aware Mode

```go
var conf AppConfig

cfg, _ := config.NewBuilder().
    WithTarget(&conf).
    Build()

// Get updated struct anytime
latest, err := cfg.AsStruct()
if err != nil {
    log.Fatal(err)
}
appConfig := latest.(*AppConfig)
```

## Setting Values

### Basic Set

```go
// Set updates the highest priority source (default: CLI)
if err := cfg.Set("server.port", int64(9090)); err != nil {
    log.Fatal(err)  // Error if path not registered
}
```

### Set in Specific Source

```go
// Set value in specific source
cfg.SetSource(config.SourceEnv, "server.port", "8080")
cfg.SetSource(config.SourceCLI, "debug", true)

// File source typically set via LoadFile, but can be manual
cfg.SetSource(config.SourceFile, "feature.enabled", true)
```

### Batch Updates

```go
// Multiple updates
updates := map[string]any{
    "server.port":     int64(9090),
    "server.host":     "0.0.0.0",
    "database.maxconns": int64(50),
}

for path, value := range updates {
    if err := cfg.Set(path, value); err != nil {
        log.Printf("Failed to set %s: %v", path, err)
    }
}
```

## Type Conversions

The package uses mapstructure for flexible type conversion:

```go
// These all work for a string field
cfg.Set("name", "value")           // Direct string
cfg.Set("name", 123)               // Number → "123"
cfg.Set("name", true)              // Boolean → "true"

// For int64 fields
cfg.Set("port", int64(8080))       // Direct
cfg.Set("port", "8080")            // String → int64
cfg.Set("port", 8080.0)            // Float → int64
cfg.Set("port", int(8080))         // int → int64
```

### Duration Handling

```go
type Config struct {
    Timeout time.Duration `toml:"timeout"`
}

// All these work
cfg.Set("timeout", 30*time.Second)  // Direct duration
cfg.Set("timeout", "30s")           // String parsing
cfg.Set("timeout", "5m30s")         // Complex duration
```

### Network Types

```go
type Config struct {
    IP      net.IP     `toml:"ip"`
    CIDR    net.IPNet  `toml:"cidr"`
    URL     url.URL    `toml:"url"`
}

// Automatic parsing
cfg.Set("ip", "192.168.1.1")
cfg.Set("cidr", "10.0.0.0/8")
cfg.Set("url", "https://example.com:8080/path")
```

### Slice Handling

```go
type Config struct {
    Tags []string `toml:"tags"`
    Ports []int   `toml:"ports"`
}

// Direct slice
cfg.Set("tags", []string{"prod", "stable"})

// Comma-separated string (from env/CLI)
cfg.Set("tags", "prod,stable,v2")

// Number arrays
cfg.Set("ports", []int{8080, 8081, 8082})
```

## Checking Configuration

### Path Registration

```go
// Check if path is registered
if _, exists := cfg.Get("server.port"); !exists {
    log.Fatal("server.port not registered")
}

// Get all registered paths
paths := cfg.GetRegisteredPaths("server.")
for path := range paths {
    log.Printf("Registered: %s", path)
}

// With default values
defaults := cfg.GetRegisteredPathsWithDefaults("")
for path, defaultVal := range defaults {
    log.Printf("%s = %v (default)", path, defaultVal)
}
```

### Validation

```go
// Check required fields
if err := cfg.Validate("api.key", "database.url"); err != nil {
    log.Fatal("Missing required config:", err)
}

// Custom validation
requiredPorts := []string{"server.port", "metrics.port"}
for _, path := range requiredPorts {
    if val, exists := cfg.Get(path); exists {
        if port := val.(int64); port < 1024 {
            log.Fatalf("%s must be >= 1024", path)
        }
    }
}
```

### Source Inspection

```go
// Debug specific value
path := "server.port"
log.Printf("=== %s ===", path)
log.Printf("Current: %v", cfg.Get(path))

sources := cfg.GetSources(path)
for source, value := range sources {
    log.Printf("  %s: %v", source, value)
}
```

## Advanced Patterns

### Dynamic Configuration

```go
// Change configuration at runtime
func updatePort(cfg *config.Config, port int64) error {
    if port < 1 || port > 65535 {
        return fmt.Errorf("invalid port: %d", port)
    }
    return cfg.Set("server.port", port)
}
```

### Configuration Facade

```go
type ConfigFacade struct {
    cfg *config.Config
}

func (f *ConfigFacade) ServerPort() int64 {
    val, _ := f.cfg.Get("server.port")
    return val.(int64)
}

func (f *ConfigFacade) SetServerPort(port int64) error {
    return f.cfg.Set("server.port", port)
}

func (f *ConfigFacade) DatabaseURL() string {
    val, _ := f.cfg.Get("database.url")
    return val.(string)
}
```

### Default Fallbacks

```go
// Helper for optional configuration
func getOrDefault(cfg *config.Config, path string, defaultVal any) any {
    if val, exists := cfg.Get(path); exists {
        return val
    }
    return defaultVal
}

// Usage
timeout := getOrDefault(cfg, "timeout", 30*time.Second).(time.Duration)
```

## Thread Safety

All access methods are thread-safe:

```go
// Safe concurrent access
var wg sync.WaitGroup

// Multiple readers
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        port, _ := cfg.Get("server.port")
        log.Printf("Port: %v", port)
    }()
}

// Concurrent writes are safe too
wg.Add(1)
go func() {
    defer wg.Done()
    cfg.Set("counter", atomic.AddInt64(&counter, 1))
}()

wg.Wait()
```

## Debugging

### View All Configuration

```go
// Debug output
fmt.Println(cfg.Debug())

// Dump as TOML
cfg.Dump()  // Writes to stdout
```

### Clone for Testing

```go
// Create isolated copy for testing
testCfg := cfg.Clone()
testCfg.Set("server.port", int64(0))  // Random port for tests
```

## See Also

- [Live Reconfiguration](reconfiguration.md) - Reacting to changes
- [Builder Pattern](builder.md) - Type-aware configuration
- [Environment Variables](env.md) - Environment value access