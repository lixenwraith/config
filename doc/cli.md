# Command Line Arguments

The config package supports command-line argument parsing with flexible formats and automatic type conversion.

## Argument Formats

### Key-Value Pairs

```bash
# Space-separated
./myapp --server.port 8080 --database.url "postgres://localhost/db"

# Equals-separated
./myapp --server.port=8080 --database.url=postgres://localhost/db

# Mixed formats
./myapp --server.port 8080 --debug=true
```

### Boolean Flags

```bash
# Boolean flags don't require a value (assumed true)
./myapp --debug --verbose

# Explicit boolean values
./myapp --debug=true --verbose=false
```

### Nested Paths

Use dot notation for nested configuration:

```bash
./myapp --server.host=0.0.0.0 --server.port=9090 --server.tls.enabled=true
```

## Type Conversion

Command-line values are automatically converted to match registered types:

```go
type Config struct {
    Port     int64         `toml:"port"`
    Timeout  time.Duration `toml:"timeout"`
    Ratio    float64       `toml:"ratio"`
    Enabled  bool          `toml:"enabled"`
    Tags     []string      `toml:"tags"`
}

// All these are parsed correctly:
// --port=8080              → int64(8080)
// --timeout=30s            → time.Duration(30 * time.Second)
// --ratio=0.95             → float64(0.95)
// --enabled=true           → bool(true)
// --tags=prod,stable       → []string{"prod", "stable"}
```

## Integration with flag Package

### Generate flag.FlagSet

```go
// Generate flags from registered configuration
fs := cfg.GenerateFlags()

// Parse command line
if err := fs.Parse(os.Args[1:]); err != nil {
    log.Fatal(err)
}

// Apply parsed flags to configuration
if err := cfg.BindFlags(fs); err != nil {
    log.Fatal(err)
}
```

### Custom Flag Registration

```go
fs := flag.NewFlagSet("myapp", flag.ContinueOnError)

// Add custom flags
verbose := fs.Bool("v", false, "verbose output")
configFile := fs.String("config", "config.toml", "config file path")

// Parse
fs.Parse(os.Args[1:])

// Use custom flags
if *verbose {
    log.SetLevel(log.DebugLevel)
}

// Load config with custom file path
cfg, _ := config.NewBuilder().
    WithFile(*configFile).
    Build()

// Bind remaining flags
cfg.BindFlags(fs)
```

## Precedence and Overrides

Command-line arguments have the highest precedence by default:

```go
// Default precedence: CLI > Env > File > Default
cfg, _ := config.Quick(defaults, "APP_", "config.toml")

// Even if config.toml sets port=8080 and APP_PORT=9090,
// --port=7070 will win
```

Change precedence if needed:

```go
cfg, _ := config.NewBuilder().
    WithSources(
        config.SourceEnv,     // Env highest
        config.SourceCLI,     // Then CLI
        config.SourceFile,    // Then file
        config.SourceDefault, // Finally defaults
    ).
    Build()
```

## Argument Parsing Details

### Validation

- Paths must use valid identifiers (letters, numbers, underscore, dash)
- No leading/trailing dots in paths
- Empty segments not allowed (no `..` in paths)

### Special Cases

```bash
# Double dash stops flag parsing
./myapp --port=8080 -- --not-a-flag

# Single dash flags are ignored (not GNU-style)
./myapp -p 8080  # Ignored, use --port

# Quoted values preserve spaces
./myapp --message="Hello World" --name='John Doe'

# Escape quotes in values
./myapp --json="{\"key\": \"value\"}"
```

### Value Parsing Rules

1. **Booleans**: `true`, `false` (case-sensitive)
2. **Numbers**: Standard decimal notation
3. **Strings**: Quoted or unquoted (quotes removed if present)
4. **Lists**: Comma-separated (when target type is slice)

## Override Arguments

```go
// Parse custom arguments instead of os.Args
customArgs := []string{"--debug", "--port=9090"}

cfg, _ := config.NewBuilder().
    WithArgs(customArgs).
    Build()
```

## Error Handling

CLI parsing errors are returned from `Build()` or `LoadCLI()`:

```go
cfg, err := config.NewBuilder().
    WithDefaults(&Config{}).
    Build()

if err != nil {
    switch {
    case errors.Is(err, config.ErrCLIParse):
        log.Fatal("Invalid command line arguments:", err)
    default:
        log.Fatal("Configuration error:", err)
    }
}
```

## See Also

- [Environment Variables](env.md) - Environment variable handling
- [Access Patterns](access.md) - Retrieving parsed values