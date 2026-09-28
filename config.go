// Package config provides thread-safe configuration management for Go applications
// with support for multiple sources: TOML files, environment variables, command-line
// arguments, and default values with configurable precedence.
package config

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

// configItem holds configuration values from different sources
type configItem struct {
	required     bool
	defaultValue any
	values       map[Source]any // Values from each source
	currentValue any            // Computed value based on precedence
}

// structCache manages the typed representation of configuration
type structCache struct {
	target     any          // User-provided struct pointer
	targetType reflect.Type // Cached type for validation
	snapshot   any          // Immutable cached result; callers receive copies
	prefix     string
	version    int64 // Version for invalidation
	populated  bool  // Whether cache is valid
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
	items          map[string]configItem
	tagName        string
	fileFormat     string // Separate from tagName: toml or auto
	securityOpts   *SecurityOptions
	mutex          sync.RWMutex
	options        LoadOptions    // Current load options
	fileData       map[string]any // Cached file data
	fileComments   []string
	fileGeneration uint64         // Invalidates staged watcher reloads
	envData        map[string]any // Cached env data
	cliData        map[string]any // Cached CLI data
	version        atomic.Int64
	structCache    *structCache

	// CLI paths that matched no registered path (reset on each CLI load)
	unknownCLIKeys []string

	// File watching support
	watcher        *watcher
	configFilePath string // Track loaded file path
}

// New creates and initializes a new Config instance
func New() *Config {
	return &Config{
		items:      make(map[string]configItem),
		tagName:    FormatTOML,
		fileFormat: FormatAuto,
		options:    DefaultLoadOptions(),
		fileData:   make(map[string]any),
		envData:    make(map[string]any),
		cliData:    make(map[string]any),
	}
}

// NewWithOptions creates a new Config instance with custom load options
func NewWithOptions(opts LoadOptions) *Config {
	c := New()
	c.options = cloneLoadOptions(opts)
	return c
}

// SetLoadOptions updates the load options and recomputes current values
func (c *Config) SetLoadOptions(opts LoadOptions) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.options = cloneLoadOptions(opts)

	// Recompute all current values based on new precedence
	for path, item := range c.items {
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}
	c.invalidateCache()
}

// SetPrecedence updates source precedence with validation
func (c *Config) SetPrecedence(sources ...Source) error {
	sources = slices.Clone(sources)
	// Validate all required sources present
	required := map[Source]bool{
		SourceDefault: false,
		SourceFile:    false,
		SourceEnv:     false,
		SourceCLI:     false,
	}

	for _, s := range sources {
		if _, valid := required[s]; !valid {
			return wrapError(ErrNotConfigured, fmt.Errorf("invalid source: %s", s))
		}
		required[s] = true
	}

	// Ensure SourceDefault is included
	if !required[SourceDefault] {
		sources = append(sources, SourceDefault)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if precedence actually changed
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
			c.watcher.notifyWatchers(fmt.Sprintf("%s:%s", EventPrecedenceChanged, path))
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

// SetFileFormat sets the expected format for configuration files
// Use "auto" to detect based on file extension
func (c *Config) SetFileFormat(format string) error {
	switch format {
	case FormatTOML, FormatAuto:
		// Valid formats
	default:
		return wrapError(ErrFileFormat, fmt.Errorf("unsupported file format %q, must be one of: toml, auto", format))
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
	c.fileGeneration++
}

// Get retrieves a configuration value using the path and indicator if the path was registered
func (c *Config) Get(path string) (any, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, registered := c.items[path]
	if !registered {
		return nil, false
	}

	return cloneOwned(item.currentValue), true
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
	if source == SourceDefault {
		return cloneOwned(item.defaultValue), true
	}
	return cloneOwned(val), exists
}

// Set updates a configuration value for the given path
// It sets the value in the highest priority source from the configured Sources
// By default, this is SourceCLI. Returns an error if the path is not registered
// To set a value in a specific source, use SetSource instead
func (c *Config) Set(path string, value any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.setSourceLocked(c.options.Sources[0], path, value)
}

// SetSource sets a value for a specific source
func (c *Config) SetSource(source Source, path string, value any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.setSourceLocked(source, path, value)
}

func (c *Config) setSourceLocked(source Source, path string, value any) error {
	item, registered := c.items[path]
	if !registered {
		return wrapError(ErrPathNotRegistered, fmt.Errorf("path %s is not registered", path))
	}

	if str, ok := value.(string); ok && len(str) > MaxValueSize {
		return ErrValueSize
	}

	if item.values == nil {
		item.values = make(map[Source]any)
	}

	if !validSource(source) {
		return wrapError(ErrTypeMismatch, fmt.Errorf("invalid source %q", source))
	}
	if err := validateValue(item, value); err != nil {
		return err
	}
	owned, err := copyValue(value)
	if err != nil {
		return err
	}
	value = owned
	if source == SourceDefault {
		if item.defaultValue != nil {
			target := reflect.New(reflect.TypeOf(item.defaultValue))
			if err := decodeConfig(value, target.Interface()); err != nil {
				return err
			}
			value = target.Elem().Interface()
		}
		item.defaultValue = value
	} else {
		item.values[source] = value
	}
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
		c.unknownCLIKeys = nil
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
		result[source] = cloneOwned(value)
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
	c.unknownCLIKeys = nil
	c.fileGeneration++

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
		c.fileGeneration++
	case SourceEnv:
		c.envData = make(map[string]any)
	case SourceCLI:
		c.cliData = make(map[string]any)
		c.unknownCLIKeys = nil
	}

	// Remove source values from all items
	for path, item := range c.items {
		delete(item.values, source)
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}

	c.invalidateCache() // Invalidate cache after changes
}

// AsStruct returns the populated struct if in type-aware mode
func (c *Config) AsStruct() (any, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if c.structCache == nil {
		return nil, wrapError(ErrNotConfigured, fmt.Errorf("no target struct configured"))
	}
	cache := c.structCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	version := c.version.Load()
	if !cache.populated || cache.version != version {
		next := reflect.New(cache.targetType)
		if err := decodeConfig(navigateToPath(c.nestedLocked(""), cache.prefix), next.Interface()); err != nil {
			return nil, err
		}
		cache.snapshot = next.Interface()
		cache.version, cache.populated = version, true
	}
	return copyValue(cache.snapshot)
}

// UnknownCLIKeys returns CLI paths that matched no registered config path
func (c *Config) UnknownCLIKeys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return slices.Clone(c.unknownCLIKeys)
}

// computeValue determines the current value based on precedence
func (c *Config) computeValue(item configItem) any {
	// Check sources in precedence order
	for _, source := range c.options.Sources {
		if source == SourceDefault {
			return item.defaultValue
		}
		if val, exists := item.values[source]; exists && val != nil {
			return val
		}
	}

	// No source had a value, use default
	return item.defaultValue
}

// invalidateCache override Set methods to invalidate cache
func (c *Config) invalidateCache() {
	c.version.Add(1)
}
