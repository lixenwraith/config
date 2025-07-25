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

// Config manages application configuration. It can be used in two primary ways:
// 1. As a dynamic key-value store, accessed via methods like Get(), String(), and Int64()
// 2. As a source for a type-safe struct, populated via BuildAndScan() or AsStruct()
type Config struct {
	items       map[string]configItem
	tagName     string
	mutex       sync.RWMutex
	options     LoadOptions    // Current load options
	fileData    map[string]any // Cached file data
	envData     map[string]any // Cached env data
	cliData     map[string]any // Cached CLI data
	version     atomic.Int64
	structCache *structCache

	// File watching support
	watcher        *watcher
	configFilePath string // Track loaded file path
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		items:    make(map[string]configItem),
		tagName:  "toml",
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