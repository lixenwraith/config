// FILE: lixenwraith/config/config.go
// Package config provides thread-safe configuration management for Go applications
// with support for multiple sources: TOML files, environment variables, command-line
// arguments, and default values with configurable precedence.
package config

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// Max config item value size to prevent misuse
const MaxValueSize = 1024 * 1024 // 1MB

// Errors
var (
	// ErrConfigNotFound indicates the specified configuration file was not found.
	ErrConfigNotFound = errors.New("configuration file not found")

	// ErrCLIParse indicates that parsing command-line arguments failed.
	ErrCLIParse = errors.New("failed to parse command-line arguments")

	// ErrEnvParse indicates that parsing environment variables failed.
	// TODO: use in loader:loadEnv or remove
	ErrEnvParse = errors.New("failed to parse environment variables")

	// ErrValueSize indicates a value larger than MaxValueSize
	ErrValueSize = fmt.Errorf("value size exceeds maximum %d bytes", MaxValueSize)
)

// configItem holds configuration values from different sources
type configItem struct {
	defaultValue any
	values       map[Source]any // Values from each source
	currentValue any            // Computed value based on precedence
}

// structCache manages the typed representation of configuration
type structCache struct {
	target     any          // User-provided struct pointer
	targetType reflect.Type // Cached type for validation
	version    int64        // Version for invalidation
	populated  bool         // Whether cache is valid
	mu         sync.RWMutex
}

// SecurityOptions for enhanced file loading security
type SecurityOptions struct {
	PreventPathTraversal bool  // Prevent ../ in paths
	EnforceFileOwnership bool  // Unix only: ensure file owned by current user
	MaxFileSize          int64 // Maximum config file size (0 = no limit)
}

// Config manages application configuration. It can be used in two primary ways:
// 1. As a dynamic key-value store, accessed via methods like Get(), String(), and Int64()
// 2. As a source for a type-safe struct, populated via BuildAndScan() or AsStruct()
type Config struct {
	items        map[string]configItem
	tagName      string
	fileFormat   string // Separate from tagName: "toml", "json", "yaml", or "auto"
	securityOpts *SecurityOptions
	mutex        sync.RWMutex
	options      LoadOptions    // Current load options
	fileData     map[string]any // Cached file data
	envData      map[string]any // Cached env data
	cliData      map[string]any // Cached CLI data
	version      atomic.Int64
	structCache  *structCache

	// File watching support
	watcher        *watcher
	configFilePath string // Track loaded file path
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		items:      make(map[string]configItem),
		tagName:    "toml",
		fileFormat: "auto",
		// securityOpts: &SecurityOptions{
		// 	PreventPathTraversal: false,
		// 	EnforceFileOwnership: false,
		// 	MaxFileSize:          0,
		// },
		options:  DefaultLoadOptions(),
		fileData: make(map[string]any),
		envData:  make(map[string]any),
		cliData:  make(map[string]any),
	}
}

// NewWithOptions creates a new Config instance with custom load options
func NewWithOptions(opts LoadOptions) *Config {
	c := New()
	c.options = opts
	return c
}

// SetLoadOptions updates the load options and recomputes current values
func (c *Config) SetLoadOptions(opts LoadOptions) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.options = opts

	// Recompute all current values based on new precedence
	for path, item := range c.items {
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}

	return nil
}

// SetPrecedence updates source precedence with validation
func (c *Config) SetPrecedence(sources ...Source) error {
	// Validate all required sources present
	required := map[Source]bool{
		SourceDefault: false,
		SourceFile:    false,
		SourceEnv:     false,
		SourceCLI:     false,
	}

	for _, s := range sources {
		if _, valid := required[s]; !valid {
			return fmt.Errorf("invalid source: %s", s)
		}
		required[s] = true
	}

	// Ensure SourceDefault is included
	if !required[SourceDefault] {
		sources = append(sources, SourceDefault)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// FIXED: Check if precedence actually changed
	oldPrecedence := c.options.Sources
	if reflect.DeepEqual(oldPrecedence, sources) {
		return nil // No change needed
	}

	// Track value changes before updating precedence
	oldValues := make(map[string]any)
	for path, item := range c.items {
		oldValues[path] = item.currentValue
	}

	// Update precedence
	c.options.Sources = sources

	// Recompute values and track changes
	changedPaths := make([]string, 0)
	for path, item := range c.items {
		item.currentValue = c.computeValue(item)
		if !reflect.DeepEqual(oldValues[path], item.currentValue) {
			changedPaths = append(changedPaths, path)
		}
		c.items[path] = item
	}

	// Notify watchers of precedence change
	if c.watcher != nil && len(changedPaths) > 0 {
		for _, path := range changedPaths {
			c.watcher.notifyWatchers("precedence:" + path)
		}
	}

	c.invalidateCache()
	return nil
}

// GetPrecedence returns current source precedence
func (c *Config) GetPrecedence() []Source {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]Source, len(c.options.Sources))
	copy(result, c.options.Sources)
	return result
}

// computeValue determines the current value based on precedence
func (c *Config) computeValue(item configItem) any {
	// Check sources in precedence order
	for _, source := range c.options.Sources {
		if val, exists := item.values[source]; exists && val != nil {
			return val
		}
	}

	// No source had a value, use default
	return item.defaultValue
}

// SetFileFormat sets the expected format for configuration files.
// Use "auto" to detect based on file extension.
func (c *Config) SetFileFormat(format string) error {
	switch format {
	case "toml", "json", "yaml", "auto":
		// Valid formats
	default:
		return fmt.Errorf("unsupported file format %q, must be one of: toml, json, yaml, auto", format)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.fileFormat = format
	return nil
}

// SetSecurityOptions configures security checks for file loading
func (c *Config) SetSecurityOptions(opts SecurityOptions) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.securityOpts = &opts
}

// Get retrieves a configuration value using the path and indicator if the path was registered
func (c *Config) Get(path string) (any, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, registered := c.items[path]
	if !registered {
		return nil, false
	}

	return item.currentValue, true
}

// GetSource retrieves a value from a specific source
func (c *Config) GetSource(path string, source Source) (any, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, registered := c.items[path]
	if !registered {
		return nil, false
	}

	val, exists := item.values[source]
	return val, exists
}

// Set updates a configuration value for the given path.
// It sets the value in the highest priority source from the configured Sources.
// By default, this is SourceCLI. Returns an error if the path is not registered.
// To set a value in a specific source, use SetSource instead.
func (c *Config) Set(path string, value any) error {
	return c.SetSource(c.options.Sources[0], path, value)
}

// SetSource sets a value for a specific source
func (c *Config) SetSource(source Source, path string, value any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, registered := c.items[path]
	if !registered {
		return fmt.Errorf("path %s is not registered", path)
	}

	if str, ok := value.(string); ok && len(str) > MaxValueSize {
		return ErrValueSize
	}

	if item.values == nil {
		item.values = make(map[Source]any)
	}

	item.values[source] = value
	item.currentValue = c.computeValue(item)
	c.items[path] = item

	// Update source cache
	switch source {
	case SourceFile:
		c.fileData[path] = value
	case SourceEnv:
		c.envData[path] = value
	case SourceCLI:
		c.cliData[path] = value
	}

	c.invalidateCache() // Invalidate cache after changes
	return nil
}

// GetSources returns all sources that have a value for the given path
func (c *Config) GetSources(path string) map[Source]any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, registered := c.items[path]
	if !registered {
		return nil
	}

	result := make(map[Source]any)
	for source, value := range item.values {
		result[source] = value
	}
	return result
}

// Reset clears all non-default values and resets to defaults
func (c *Config) Reset() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Clear source caches
	c.fileData = make(map[string]any)
	c.envData = make(map[string]any)
	c.cliData = make(map[string]any)

	// Reset all items to default values
	for path, item := range c.items {
		item.values = make(map[Source]any)
		item.currentValue = item.defaultValue
		c.items[path] = item
	}

	c.invalidateCache() // Invalidate cache after changes
}

// ResetSource clears all values from a specific source
func (c *Config) ResetSource(source Source) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Clear source cache
	switch source {
	case SourceFile:
		c.fileData = make(map[string]any)
	case SourceEnv:
		c.envData = make(map[string]any)
	case SourceCLI:
		c.cliData = make(map[string]any)
	}

	// Remove source values from all items
	for path, item := range c.items {
		delete(item.values, source)
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}

	c.invalidateCache() // Invalidate cache after changes
}

// Override Set methods to invalidate cache
func (c *Config) invalidateCache() {
	c.version.Add(1)
}

// AsStruct returns the populated struct if in type-aware mode
func (c *Config) AsStruct() (any, error) {
	if c.structCache == nil || c.structCache.target == nil {
		return nil, fmt.Errorf("no target struct configured")
	}

	c.structCache.mu.RLock()
	currentVersion := c.version.Load()
	needsUpdate := !c.structCache.populated || c.structCache.version != currentVersion
	c.structCache.mu.RUnlock()

	if needsUpdate {
		if err := c.populateStruct(); err != nil {
			return nil, err
		}
	}

	return c.structCache.target, nil
}

// Target populates the provided struct with current configuration
func (c *Config) Target(out any) error {
	return c.Scan(out)
}

// populateStruct updates the cached struct representation using unified unmarshal
func (c *Config) populateStruct() error {
	c.structCache.mu.Lock()
	defer c.structCache.mu.Unlock()

	currentVersion := c.version.Load()
	if c.structCache.populated && c.structCache.version == currentVersion {
		return nil
	}

	if err := c.unmarshal("", c.structCache.target); err != nil {
		return fmt.Errorf("failed to populate struct cache: %w", err)
	}

	c.structCache.version = currentVersion
	c.structCache.populated = true
	return nil
}