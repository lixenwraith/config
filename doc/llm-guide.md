# lixenwraith/config LLM Usage Guide

Thread-safe configuration management for Go applications with multi-source support, type safety, and live reconfiguration.
Use default configuration and behavior if applicable, unless explicitly required.

## Core Types

### Config
```go
// Primary configuration manager. All operations are thread-safe.
type Config struct {
    // Internal fields - thread-safe configuration store
}
```

### Source
```go
// Represents a configuration source, used to define load precedence.
type Source string

const (
    SourceDefault Source = "default"
    SourceFile    Source = "file"
    SourceEnv     Source = "env"
    SourceCLI     Source = "cli"
)
```

### LoadOptions
```go
type LoadOptions struct {
    Sources      []Source          // Precedence order (first = highest)
    EnvPrefix    string            // Prepended to env var names
    EnvTransform EnvTransformFunc  // Custom path→env mapping
    LoadMode     LoadMode          // Uses default behavior, do not configure
    EnvWhitelist map[string]bool   // Limit env paths (nil = all)
    SkipValidation bool            // Skip path validation
}

type EnvTransformFunc func(path string) string
type LoadMode int // LoadModeReplace (default) or LoadModeMerge
```

## Error Types
```go
var (
ErrConfigNotFound = errors.New("configuration file not found")
ErrCLIParse      = errors.New("failed to parse command-line arguments")
ErrEnvParse      = errors.New("failed to parse environment variables")
ErrValueSize     = fmt.Errorf("value size exceeds maximum %d bytes", MaxValueSize)
)

const MaxValueSize = 1024 * 1024 // 1MB
```

## Core Methods

### Creation
```go
// New creates a new Config instance with default options.
func New() *Config
// NewWithOptions creates a new Config instance with custom load options.
func NewWithOptions(opts LoadOptions) *Config
func DefaultLoadOptions() LoadOptions
```

### Registration
```go
// Register makes a configuration path known with a default value; required before use.
func (c *Config) Register(path string, defaultValue any) error
// RegisterStruct recursively registers fields from a struct using `toml` tags by default.
func (c *Config) RegisterStruct(prefix string, structWithDefaults any) error
// RegisterStructWithTags is like RegisterStruct but allows custom tag names ("json", "yaml").
func (c *Config) RegisterStructWithTags(prefix string, structWithDefaults any, tagName string) error
// RegisterWithEnv registers a path with an explicit environment variable mapping.
func (c *Config) RegisterWithEnv(path string, defaultValue any, envVar string) error
// Unregister removes a configuration path and all its children.
func (c *Config) Unregister(path string) error
```
Only default `toml` tags must be used unless support of other types are explicitly requested.
Path registration is required before setting values. Paths use dot notation (e.g., "server.port").

### Value Access
```go
// Get retrieves the final merged value; the bool indicates if the path was registered.
func (c *Config) Get(path string) (any, bool)
// GetSource retrieves a value from a specific source layer.
func (c *Config) GetSource(path string, source Source) (any, bool)
// GetSources returns all sources that have a value for the given path.
func (c *Config) GetSources(path string) map[Source]any
```
The returned `any` type requires type assertion, e.g., `port := val.(int64)`.

### Value Modification
```go
// Set updates a value in the highest priority source (default: CLI). Path must be registered.
func (c *Config) Set(path string, value any) error
// SetSource sets a value for a specific source layer.
func (c *Config) SetSource(path string, source Source, value any) error
// SetLoadOptions updates the load options, recomputing all current values.
func (c *Config) SetLoadOptions(opts LoadOptions) error
```

### Loading
```go
// Load reads configuration from a TOML file and merges overrides from command-line arguments.
func (c *Config) Load(filePath string, args []string) error
// LoadWithOptions loads configuration from multiple sources with custom options.
func (c *Config) LoadWithOptions(filePath string, args []string, opts LoadOptions) error
// LoadFile loads configuration values from a TOML file into the File source.
func (c *Config) LoadFile(path string) error
// LoadEnv loads values from environment variables into the Env source.
func (c *Config) LoadEnv(prefix string) error
// LoadCLI loads values from command-line arguments into the CLI source.
func (c *Config) LoadCLI(args []string) error
```

### Scanning & Population
```go
// Scan populates a struct from a specific config path (e.g., "server").
func (c *Config) Scan(basePath string, target any) error
// ScanSource decodes configuration from specific source
func (c *Config) ScanSource(basePath string, source Source, target any) error
// Target populates a struct from the root of the config; alias for Scan("", target).
func (c *Config) Target(out any) error
// AsStruct retrieves the pre-configured target struct (see Builder.WithTarget).
func (c *Config) AsStruct() (any, error)
```
Populates structs using mapstructure with automatic type conversion.

### Persistence
```go
// Save atomically saves the current merged configuration state to a TOML file.
func (c *Config) Save(path string) error
// SaveSource atomically saves values from only a specific source to a TOML file.
func (c *Config) SaveSource(path string, source Source) error
```
Atomic file writes in TOML format.

### State Management
```go
// Reset clears all non-default values from all sources.
func (c *Config) Reset()
// ResetSource clears all values from a specific source.
func (c *Config) ResetSource(source Source)
// Clone creates a deep copy of the configuration state.
func (c *Config) Clone() *Config
```

### Inspection
```go
// GetRegisteredPaths returns all registered paths matching a prefix.
func (c *Config) GetRegisteredPaths(prefix string) map[string]bool
// Validate checks that all specified required paths have been set.
func (c *Config) Validate(required ...string) error
// Debug returns a formatted string of all values and their sources for debugging.
func (c *Config) Debug() string
```

### Environment
```go
// DiscoverEnv discovers environment variables matching a prefix.
func (c *Config) DiscoverEnv(prefix string) map[string]string
// ExportEnv exports the current configuration as environment variables
func (c *Config) ExportEnv(prefix string) map[string]string
```

## Builder Pattern

### Builder
```go
type Builder struct {
    // Internal builder state
}

type ValidatorFunc func(c *Config) error
```

### Builder Methods
```go
// NewBuilder creates a new configuration builder.
func NewBuilder() *Builder
// Build finalizes configuration; returns the first of any accumulated errors.
func (b *Builder) Build() (*Config, error)
// WithDefaults sets the struct containing default values.
func (b *Builder) WithDefaults(defaults any) *Builder
// WithTarget enables type-aware mode for AsStruct() and registers struct fields.
func (b *Builder) WithTarget(target any) *Builder
// WithTagName sets the primary struct tag for field mapping: "toml", "json", "yaml".
func (b *Builder) WithTagName(tagName string) *Builder
// WithSources sets the precedence order for configuration sources.
func (b *Builder) WithSources(sources ...Source) *Builder
// WithPrefix adds a prefix to all registered paths from a struct.
func (b *Builder) WithPrefix(prefix string) *Builder
// WithEnvPrefix sets the global environment variable prefix.
func (b *Builder) WithEnvPrefix(prefix string) *Builder
// WithFile sets the configuration file path to be loaded.
func (b *Builder) WithFile(path string) *Builder
// WithArgs sets the command-line arguments to be parsed.
func (b *Builder) WithArgs(args []string) *Builder
// WithValidator adds a validation function that runs after loading.
func (b *Builder) WithValidator(fn ValidatorFunc) *Builder
// WithEnvTransform sets a custom environment variable mapping function.
func (b *Builder) WithSources(sources ...Source) *Builder
// WithEnvTransform sets a custom environment variable mapping function.
func (b *Builder) WithEnvTransform(fn EnvTransformFunc) *Builder
// WithFileDiscovery enables automatic config file discovery
func (b *Builder) WithFileDiscovery(opts FileDiscoveryOptions) *Builder
```

### FileDiscoveryOptions
```go
type FileDiscoveryOptions struct {
    Name          string    // Base name without extension
    Extensions    []string  // Extensions to try in order
    Paths         []string  // Custom search paths
    EnvVar        string    // Environment variable for path
    CLIFlag       string    // CLI flag for path
    UseXDG        bool      // Search XDG directories
    UseCurrentDir bool      // Search current directory
}

func DefaultDiscoveryOptions(appName string) FileDiscoveryOptions
```

## Live Reconfiguration

### AutoUpdate
```go
// AutoUpdate enables automatic configuration reloading on file changes with default options.
func (c *Config) AutoUpdate()
// AutoUpdateWithOptions enables reloading with custom options.
func (c *Config) AutoUpdateWithOptions(opts WatchOptions)
// StopAutoUpdate stops the file watcher and cleans up resources.
func (c *Config) StopAutoUpdate()
// IsWatching returns true if the file watcher is active.
func (c *Config) IsWatching() bool
```

### Watch
```go
// Watch returns a channel that receives paths of changed values.
func (c *Config) Watch() <-chan string
// WatcherCount returns the number of active watch subscribers.
func (c *Config) WatcherCount() int
```
Channel receives paths of changed values or special notifications: `"file_deleted"`, `"permissions_changed"`, `"reload_error:*"`.

### WatchOptions
```go
type WatchOptions struct {
    PollInterval      time.Duration  // File check interval (min 100ms)
    Debounce          time.Duration  // Delay after changes
    MaxWatchers       int            // Concurrent watch limit
    ReloadTimeout     time.Duration  // Reload operation timeout
    VerifyPermissions bool           // Check permission changes
}

func DefaultWatchOptions() WatchOptions
```

## Type System

### Supported Types
- Basic: `bool`, `int64`, `float64`, `string`
- Time: `time.Duration`, `time.Time`
- Network: `net.IP`, `net.IPNet`, `url.URL`
- Slices: Any slice type with comma-separated parsing
- Complex: Any type via mapstructure decode hooks

### Type Conversion
All integer types are stored as `int64`, and floats as `float64`. String inputs from sources like environment variables or CLI arguments are automatically parsed to the target registered type. Custom types supported via decode hooks.

### Struct Tags
The `WithTagName` builder method sets the primary tag used for mapping paths.
```go
type Config struct {
    // Uses the tag set by WithTagName (default "toml") for path name.
    // The `env` tag provides an explicit environment variable override.
    Port     int64         `toml:"port" env:"PORT"`
    Timeout  time.Duration `toml:"timeout"`
    // Slices are populated from comma-separated strings (env/CLI) or arrays (file).
    Tags     []string      `toml:"tags"`
}
```

## Thread Safety
All methods are thread-safe. Concurrent reads and writes are synchronized internally.

## Path Validation
- Paths use dot notation: "server.port", "database.connections.max"
- Segments must be valid identifiers: `[A-Za-z0-9_-]+`
- No leading/trailing dots or empty segments

## Source Precedence
Default order (highest to lowest):
1.  CLI arguments
2.  Environment variables
3.  Configuration file
4.  Default values

Precedence is configurable via `Builder.WithSources()` or `LoadOptions.Sources`.