// FILE: lixenwraith/config/utility.go
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// Quick creates a fully configured Config instance with a single call
// This is the recommended way to initialize configuration for most applications
func Quick(structDefaults any, envPrefix, configFile string) (*Config, error) {
	cfg := New()

	// Register defaults from struct if provided
	if structDefaults != nil {
		if err := cfg.RegisterStruct("", structDefaults); err != nil {
			return nil, wrapError(ErrTypeMismatch, fmt.Errorf("failed to register defaults: %w", err))
		}
	}

	// Load with standard precedence: CLI > Env > File > Default
	opts := DefaultLoadOptions()
	opts.EnvPrefix = envPrefix

	err := cfg.loadWithOptions(configFile, os.Args[1:], opts)
	return cfg, err
}

// QuickCustom creates a Config with custom options
func QuickCustom(structDefaults any, opts LoadOptions, configFile string) (*Config, error) {
	cfg := NewWithOptions(opts)

	// Register defaults from struct if provided
	if structDefaults != nil {
		if err := cfg.RegisterStruct("", structDefaults); err != nil {
			return nil, wrapError(ErrTypeMismatch, fmt.Errorf("failed to register defaults: %w", err))
		}
	}

	err := cfg.loadWithOptions(configFile, os.Args[1:], opts)
	return cfg, err
}

// MustQuick is like Quick but panics on error
func MustQuick(structDefaults any, envPrefix, configFile string) *Config {
	cfg, err := Quick(structDefaults, envPrefix, configFile)
	if err != nil && !errors.Is(err, ErrConfigNotFound) {
		panic(fmt.Sprintf("config initialization failed: %v", err))
	}
	return cfg
}

// GenerateFlags creates flag.FlagSet entries for all registered paths
func (c *Config) GenerateFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for path, item := range c.items {
		// Create flag based on default value type
		switch v := item.defaultValue.(type) {
		case bool:
			fs.Bool(path, v, fmt.Sprintf("Config: %s", path))
		case int64:
			fs.Int64(path, v, fmt.Sprintf("Config: %s", path))
		case int:
			fs.Int(path, v, fmt.Sprintf("Config: %s", path))
		case float64:
			fs.Float64(path, v, fmt.Sprintf("Config: %s", path))
		case string:
			fs.String(path, v, fmt.Sprintf("Config: %s", path))
		default:
			// For other types, use string flag
			fs.String(path, fmt.Sprintf("%v", v), fmt.Sprintf("Config: %s", path))
		}
	}

	return fs
}

// BindFlags updates configuration from parsed flag.FlagSet
func (c *Config) BindFlags(fs *flag.FlagSet) error {
	if fs == nil {
		return wrapError(ErrCLIParse, fmt.Errorf("nil flag set"))
	}
	values := make(map[string]any)
	fs.Visit(func(f *flag.Flag) { values[f.Name] = f.Value.String() })
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for path := range values {
		if _, ok := c.items[path]; !ok {
			return wrapError(ErrCLIParse, fmt.Errorf("failed to bind unregistered flag %q: %w", path, ErrPathNotRegistered))
		}
	}
	if err := c.replaceSourceLocked(SourceCLI, values); err != nil {
		return wrapError(ErrCLIParse, err)
	}
	return nil
}

// Validate checks that all required configuration values are set
// A value is considered "set" if it differs from its default value
func (c *Config) Validate(required ...string) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var missing []string
	if len(required) == 0 {
		for path, item := range c.items {
			if item.required {
				required = append(required, path)
			}
		}
	}

	for _, path := range required {
		item, exists := c.items[path]
		if !exists {
			missing = append(missing, path+" (not registered)")
			continue
		}

		// Check if value equals default (indicating not set)
		if reflect.DeepEqual(item.currentValue, item.defaultValue) {
			// Check if any source provided a value
			hasValue := false
			for _, val := range item.values {
				if val != nil {
					hasValue = true
					break
				}
			}
			if !hasValue {
				missing = append(missing, path)
			}
		}
	}

	if len(missing) > 0 {
		return wrapError(ErrValidation, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", ")))
	}

	return nil
}

// Debug returns a formatted string showing all configuration values and their sources
func (c *Config) Debug() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var b strings.Builder
	b.WriteString("Configuration Debug Info:\n")
	b.WriteString(fmt.Sprintf("Precedence: %v\n", c.options.Sources))
	b.WriteString("Current values:\n")

	for path, item := range c.items {
		b.WriteString(fmt.Sprintf("  %s:\n", path))
		b.WriteString(fmt.Sprintf("    Current: %v\n", item.currentValue))
		b.WriteString(fmt.Sprintf("    Default: %v\n", item.defaultValue))

		for source, value := range item.values {
			b.WriteString(fmt.Sprintf("    %s: %v\n", source, value))
		}
	}

	return b.String()
}

// Dump writes the current configuration to stdout in TOML format
func (c *Config) Dump() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	nestedData := make(map[string]any)
	for path, item := range c.items {
		setNestedValue(nestedData, path, item.currentValue)
	}

	data, err := marshalConfig(nestedData)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	clone := NewWithOptions(c.options)
	clone.tagName, clone.fileFormat = c.tagName, c.fileFormat
	clone.configFilePath = c.configFilePath
	clone.fileComments = append([]string(nil), c.fileComments...)
	clone.unknownCLIKeys = append([]string(nil), c.unknownCLIKeys...)
	if c.securityOpts != nil {
		opts := *c.securityOpts
		clone.securityOpts = &opts
	}
	for path, item := range c.items {
		next := configItem{defaultValue: cloneOwned(item.defaultValue), currentValue: cloneOwned(item.currentValue), values: make(map[Source]any)}
		for source, value := range item.values {
			next.values[source] = cloneOwned(value)
		}
		next.required = item.required
		clone.items[path] = next
	}
	for k, v := range c.fileData {
		clone.fileData[k] = cloneOwned(v)
	}
	for k, v := range c.envData {
		clone.envData[k] = cloneOwned(v)
	}
	for k, v := range c.cliData {
		clone.cliData[k] = cloneOwned(v)
	}
	if c.structCache != nil {
		clone.structCache = &structCache{targetType: c.structCache.targetType, prefix: c.structCache.prefix}
	}
	return clone
}

// QuickTyped creates a fully configured Config with a typed target
func QuickTyped[T any](target *T, envPrefix, configFile string) (*Config, error) {
	return NewBuilder().
		WithTarget(target).
		WithEnvPrefix(envPrefix).
		WithFile(configFile).
		Build()
}

// GetTyped retrieves a configuration value and decodes it into the specified type T
// It leverages the same decoding hooks as the Scan and AsStruct methods,
// providing type conversion from strings, numbers, etc.
func GetTyped[T any](c *Config, path string) (T, error) {
	var zero T

	rawValue, exists := c.Get(path)
	if !exists {
		return zero, wrapError(ErrPathNotFound, fmt.Errorf("path %q not found", path))
	}

	var target T
	if err := decodeConfig(rawValue, &target); err != nil {
		return zero, err
	}
	return target, nil
}

// GetTypedWithDefault retrieves a configuration value with a default fallback
// If the path doesn't exist or isn't set, it sets and returns the default value
// For simple cases where explicit defaults aren't pre-registered
func GetTypedWithDefault[T any](c *Config, path string, defaultValue T) (T, error) {
	c.mutex.Lock()
	if item, exists := c.items[path]; exists {
		raw := item.currentValue
		c.mutex.Unlock()
		var result T
		if err := decodeConfig(raw, &result); err != nil {
			return result, err
		}
		return result, nil
	}
	if err := validatePath(path); err != nil {
		c.mutex.Unlock()
		return defaultValue, err
	}
	owned, err := copyValue(defaultValue)
	if err == nil {
		err = c.registerLocked(path, owned)
	}
	c.mutex.Unlock()
	if err != nil {
		return defaultValue, err
	}
	var result T
	err = decodeConfig(owned, &result)
	return result, err
}

// ScanTyped is a generic wrapper around Scan. It allocates a new instance of type T,
// populates it with configuration data from the given base path, and returns a pointer to it
func ScanTyped[T any](c *Config, basePath ...string) (*T, error) {
	var target T
	if err := c.Scan(&target, basePath...); err != nil {
		return nil, err
	}
	return &target, nil
}

// ScanMap decodes a configuration map directly into a target struct
// without requiring a full Config instance. This is useful for plugin
// initialization where config data arrives as a map[string]any.
func ScanMap(configMap map[string]any, target any, tagName ...string) error {
	if len(tagName) > 1 || len(tagName) == 1 && tagName[0] != "" && tagName[0] != FormatTOML {
		return wrapError(ErrTypeMismatch, fmt.Errorf("only toml tags are supported"))
	}
	return decodeConfig(configMap, target)
}
