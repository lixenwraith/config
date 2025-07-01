// File: lixenwraith/config/config.go
// Package config provides thread-safe configuration management for Go applications
// with support for multiple sources: TOML files, environment variables, command-line
// arguments, and default values with configurable precedence.
package config

import (
	"errors"
	"fmt"
	"sync"
)

// Errors
var (
	// ErrConfigNotFound indicates the specified configuration file was not found.
	ErrConfigNotFound = errors.New("configuration file not found")

	// ErrCLIParse indicates that parsing command-line arguments failed.
	ErrCLIParse = errors.New("failed to parse command-line arguments")

	// ErrEnvParse indicates that parsing environment variables failed.
	ErrEnvParse = errors.New("failed to parse environment variables")
)

// Source represents a configuration source
type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceEnv     Source = "env"
	SourceCLI     Source = "cli"
)

// LoadMode defines how configuration sources are processed
type LoadMode int

const (
	// LoadModeReplace completely replaces values (default behavior)
	LoadModeReplace LoadMode = iota

	// LoadModeMerge merges maps/structs instead of replacing
	LoadModeMerge
)

// EnvTransformFunc converts a configuration path to an environment variable name
type EnvTransformFunc func(path string) string

// LoadOptions configures how configuration is loaded from multiple sources
type LoadOptions struct {
	// Sources defines the precedence order (first = highest priority)
	// Default: [SourceCLI, SourceEnv, SourceFile, SourceDefault]
	Sources []Source

	// EnvPrefix is prepended to environment variable names
	// Example: "MYAPP_" transforms "server.port" to "MYAPP_SERVER_PORT"
	EnvPrefix string

	// EnvTransform customizes how paths map to environment variables
	// If nil, uses default transformation (dots to underscores, uppercase)
	EnvTransform EnvTransformFunc

	// LoadMode determines how values are merged
	LoadMode LoadMode

	// EnvWhitelist limits which paths are checked for env vars (nil = all)
	EnvWhitelist map[string]bool

	// SkipValidation skips path validation during load
	SkipValidation bool
}

// DefaultLoadOptions returns the standard load options
func DefaultLoadOptions() LoadOptions {
	return LoadOptions{
		Sources:  []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault},
		LoadMode: LoadModeReplace,
	}
}

// configItem holds configuration values from different sources
type configItem struct {
	defaultValue any
	values       map[Source]any // Values from each source
	currentValue any            // Computed value based on precedence
}

// Config manages application configuration loaded from multiple sources.
type Config struct {
	items    map[string]configItem
	mutex    sync.RWMutex
	options  LoadOptions    // Current load options
	fileData map[string]any // Cached file data
	envData  map[string]any // Cached env data
	cliData  map[string]any // Cached CLI data
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		items:    make(map[string]configItem),
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
		item.currentValue = c.computeValue(path, item)
		c.items[path] = item
	}

	return nil
}

// computeValue determines the current value based on precedence
func (c *Config) computeValue(path string, item configItem) any {
	// Check sources in precedence order
	for _, source := range c.options.Sources {
		if val, exists := item.values[source]; exists && val != nil {
			return val
		}
	}

	// No source had a value, use default
	return item.defaultValue
}

// Get retrieves a configuration value using the path.
// It returns the current value based on configured precedence.
// The second return value indicates if the path was registered.
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
// It sets the value in the highest priority source (typically CLI).
// Returns an error if the path is not registered.
func (c *Config) Set(path string, value any) error {
	return c.SetSource(path, c.options.Sources[0], value)
}

// SetSource sets a value for a specific source
func (c *Config) SetSource(path string, source Source, value any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, registered := c.items[path]
	if !registered {
		return fmt.Errorf("path %s is not registered", path)
	}

	if item.values == nil {
		item.values = make(map[Source]any)
	}

	item.values[source] = value
	item.currentValue = c.computeValue(path, item)
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
		item.currentValue = c.computeValue(path, item)
		c.items[path] = item
	}
}