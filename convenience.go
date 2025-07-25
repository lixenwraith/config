// FILE: lixenwraith/config/convenience.go
package config

import (
	"flag"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"os"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// Quick creates a fully configured Config instance with a single call
// This is the recommended way to initialize configuration for most applications
func Quick(structDefaults any, envPrefix, configFile string) (*Config, error) {
	cfg := New()

	// Register defaults from struct if provided
	if structDefaults != nil {
		if err := cfg.RegisterStruct("", structDefaults); err != nil {
			return nil, fmt.Errorf("failed to register defaults: %w", err)
		}
	}

	// Load with standard precedence: CLI > Env > File > Default
	opts := DefaultLoadOptions()
	opts.EnvPrefix = envPrefix

	err := cfg.LoadWithOptions(configFile, os.Args[1:], opts)
	return cfg, err
}

// QuickCustom creates a Config with custom options
func QuickCustom(structDefaults any, opts LoadOptions, configFile string) (*Config, error) {
	cfg := NewWithOptions(opts)

	// Register defaults from struct if provided
	if structDefaults != nil {
		if err := cfg.RegisterStruct("", structDefaults); err != nil {
			return nil, fmt.Errorf("failed to register defaults: %w", err)
		}
	}

	err := cfg.LoadWithOptions(configFile, os.Args[1:], opts)
	return cfg, err
}

// MustQuick is like Quick but panics on error
func MustQuick(structDefaults any, envPrefix, configFile string) *Config {
	cfg, err := Quick(structDefaults, envPrefix, configFile)
	if err != nil {
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
	var errors []error
	needsInvalidation := false

	fs.Visit(func(f *flag.Flag) {
		value := f.Value.String()
		// Let mapstructure handle type conversion
		if err := c.SetSource(SourceCLI, f.Name, value); err != nil {
			errors = append(errors, fmt.Errorf("flag %s: %w", f.Name, err))
		} else {
			needsInvalidation = true
		}
	})

	if needsInvalidation {
		c.invalidateCache() // Batch invalidation after all flags
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to bind %d flags: %w", len(errors), errors[0])
	}

	return nil
}

// Validate checks that all required configuration values are set
// A value is considered "set" if it differs from its default value
func (c *Config) Validate(required ...string) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var missing []string

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
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
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

	encoder := toml.NewEncoder(os.Stdout)
	return encoder.Encode(nestedData)
}

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	clone := &Config{
		items:    make(map[string]configItem),
		options:  c.options,
		fileData: make(map[string]any),
		envData:  make(map[string]any),
		cliData:  make(map[string]any),
	}

	// Deep copy items
	for path, item := range c.items {
		newItem := configItem{
			defaultValue: item.defaultValue,
			currentValue: item.currentValue,
			values:       make(map[Source]any),
		}

		for source, value := range item.values {
			newItem.values[source] = value
		}

		clone.items[path] = newItem
	}

	// Copy cache data
	for k, v := range c.fileData {
		clone.fileData[k] = v
	}
	for k, v := range c.envData {
		clone.envData[k] = v
	}
	for k, v := range c.cliData {
		clone.cliData[k] = v
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

// GetTyped retrieves a configuration value and decodes it into the specified type T.
// It leverages the same decoding hooks as the Scan and AsStruct methods,
// providing type conversion from strings, numbers, etc.
func GetTyped[T any](c *Config, path string) (T, error) {
	var zero T

	rawValue, exists := c.Get(path)
	if !exists {
		return zero, fmt.Errorf("path %q not found", path)
	}

	// Prepare the input map and target struct for the decoder.
	inputMap := map[string]any{"value": rawValue}
	var target struct {
		Value T `mapstructure:"value"`
	}

	// Create a new decoder configured with the same hooks as the main config.
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &target,
		TagName:          c.tagName,
		WeaklyTypedInput: true,
		DecodeHook:       c.getDecodeHook(),
		Metadata:         nil,
	})
	if err != nil {
		return zero, fmt.Errorf("failed to create decoder for path %q: %w", path, err)
	}

	// Decode the single value.
	if err := decoder.Decode(inputMap); err != nil {
		return zero, fmt.Errorf("failed to decode value for path %q into type %T: %w", path, zero, err)
	}

	return target.Value, nil
}

// ScanTyped is a generic wrapper around Scan. It allocates a new instance of type T,
// populates it with configuration data from the given base path, and returns a pointer to it.
func ScanTyped[T any](c *Config, basePath ...string) (*T, error) {
	var target T
	if err := c.Scan(&target, basePath...); err != nil {
		return nil, err
	}
	return &target, nil
}