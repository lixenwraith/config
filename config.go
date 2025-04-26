// Package config provides thread-safe configuration management for Go applications
// with support for TOML files, command-line overrides, and default values.
package config

import (
	"errors" // Import errors package
	"fmt"
	"sync"
)

// ErrConfigNotFound indicates the specified configuration file was not found.
var ErrConfigNotFound = errors.New("configuration file not found")

// ErrCLIParse indicates that parsing command-line arguments failed.
var ErrCLIParse = errors.New("failed to parse command-line arguments")

// configItem holds both the default and current value for a configuration path
type configItem struct {
	defaultValue any
	currentValue any
}

// Config manages application configuration loaded from files and CLI arguments.
type Config struct {
	items map[string]configItem
	mutex sync.RWMutex
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		items: make(map[string]configItem),
	}
}

// Get retrieves a configuration value using the path.
// It returns the current value (or default if not explicitly set).
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

// Set updates a configuration value for the given path.
// It returns an error if the path is not registered.
// Note: This allows setting a value of a different type than the default.
// Type-specific getters will handle conversion attempts.
func (c *Config) Set(path string, value any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, registered := c.items[path]
	if !registered {
		return fmt.Errorf("path %s is not registered", path)
	}

	item.currentValue = value
	c.items[path] = item
	return nil
}