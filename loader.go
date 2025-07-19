// FILE: lixenwraith/config/loader.go
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Source represents a configuration source, used to define load precedence
type Source string

const (
	// SourceDefault represents use of registered default values
	SourceDefault Source = "default"
	// SourceFile represents values loaded from a configuration file
	SourceFile Source = "file"
	// SourceEnv represents values loaded from environment variables
	SourceEnv Source = "env"
	// SourceCLI represents values loaded from command-line arguments
	SourceCLI Source = "cli"
)

// LoadMode defines how configuration sources are processed
type LoadMode int

const (
	// LoadModeReplace completely replaces values (default behavior)
	LoadModeReplace LoadMode = iota

	// LoadModeMerge merges maps/structs instead of replacing
	// TODO: future implementation
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

// Load reads configuration from a TOML file and merges overrides from command-line arguments.
// This is a convenience method that maintains backward compatibility.
func (c *Config) Load(filePath string, args []string) error {
	return c.LoadWithOptions(filePath, args, c.options)
}

// LoadWithOptions loads configuration from multiple sources with custom options
func (c *Config) LoadWithOptions(filePath string, args []string, opts LoadOptions) error {
	c.mutex.Lock()
	c.options = opts
	c.mutex.Unlock()

	var loadErrors []error

	// Process each source according to precedence (in reverse order for proper layering)
	for i := len(opts.Sources) - 1; i >= 0; i-- {
		source := opts.Sources[i]

		switch source {
		case SourceDefault:
			// Defaults are already in place from Register calls
			continue

		case SourceFile:
			if filePath != "" {
				if err := c.loadFile(filePath); err != nil {
					if errors.Is(err, ErrConfigNotFound) {
						loadErrors = append(loadErrors, err)
					} else {
						return err // Fatal error
					}
				}
			}

		case SourceEnv:
			if err := c.loadEnv(opts); err != nil {
				loadErrors = append(loadErrors, err)
			}

		case SourceCLI:
			if len(args) > 0 {
				if err := c.loadCLI(args); err != nil {
					loadErrors = append(loadErrors, err)
				}
			}
		}
	}

	return errors.Join(loadErrors...)
}

// LoadEnv loads configuration values from environment variables
func (c *Config) LoadEnv(prefix string) error {
	opts := c.options
	opts.EnvPrefix = prefix
	return c.loadEnv(opts)
}

// LoadCLI loads configuration values from command-line arguments
func (c *Config) LoadCLI(args []string) error {
	return c.loadCLI(args)
}

// LoadFile loads configuration values from a TOML file
func (c *Config) LoadFile(filePath string) error {
	return c.loadFile(filePath)
}

// loadFile reads and parses a TOML configuration file
func (c *Config) loadFile(path string) error {
	fileData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConfigNotFound
		}
		return fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	fileConfig := make(map[string]any)
	if err := toml.Unmarshal(fileData, &fileConfig); err != nil {
		return fmt.Errorf("failed to parse TOML config file '%s': %w", path, err)
	}

	// Flatten and apply file data
	flattenedFileConfig := flattenMap(fileConfig, "")

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Track the config file path for watching
	c.configFilePath = path

	defer c.invalidateCache() // Invalidate cache after changes

	// Store in cache
	c.fileData = flattenedFileConfig

	// Apply to registered paths
	for path, value := range flattenedFileConfig {
		if item, exists := c.items[path]; exists {
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			if str, ok := value.(string); ok && len(str) > MaxValueSize {
				return ErrValueSize
			}
			item.values[SourceFile] = value
			item.currentValue = c.computeValue(path, item)
			c.items[path] = item
		}
		// Ignore unregistered paths from file
	}

	return nil
}

// loadEnv loads configuration from environment variables
func (c *Config) loadEnv(opts LoadOptions) error {
	transform := opts.EnvTransform
	if transform == nil {
		transform = defaultEnvTransform(opts.EnvPrefix)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	defer c.invalidateCache() // Invalidate cache after changes

	c.envData = make(map[string]any)

	for path, item := range c.items {
		if opts.EnvWhitelist != nil && !opts.EnvWhitelist[path] {
			continue
		}

		envVar := transform(path)
		if value, exists := os.LookupEnv(envVar); exists {
			// Store raw string value - mapstructure will handle conversion
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			if len(value) > MaxValueSize {
				return ErrValueSize
			}
			item.values[SourceEnv] = value // Store as string
			item.currentValue = c.computeValue(path, item)
			c.items[path] = item
			c.envData[path] = value
		}
	}

	return nil
}

// loadCLI loads configuration from command-line arguments
func (c *Config) loadCLI(args []string) error {
	parsedCLI, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCLIParse, err)
	}

	flattenedCLI := flattenMap(parsedCLI, "")

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cliData = flattenedCLI

	for path, value := range flattenedCLI {
		if item, exists := c.items[path]; exists {
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			item.values[SourceCLI] = value
			item.currentValue = c.computeValue(path, item)
			c.items[path] = item
		}
	}

	c.invalidateCache() // Invalidate cache after changes
	return nil
}

// DiscoverEnv finds all environment variables matching registered paths
// and returns a map of path -> env var name for found variables
func (c *Config) DiscoverEnv(prefix string) map[string]string {
	transform := c.options.EnvTransform
	if transform == nil {
		transform = defaultEnvTransform(prefix)
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	discovered := make(map[string]string)

	for path := range c.items {
		envVar := transform(path)
		if _, exists := os.LookupEnv(envVar); exists {
			discovered[path] = envVar
		}
	}

	return discovered
}

// ExportEnv exports the current configuration as environment variables
// Only exports paths that have non-default values
func (c *Config) ExportEnv(prefix string) map[string]string {
	transform := c.options.EnvTransform
	if transform == nil {
		transform = defaultEnvTransform(prefix)
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	exports := make(map[string]string)

	for path, item := range c.items {
		// Only export if value differs from default
		if item.currentValue != item.defaultValue {
			envVar := transform(path)
			exports[envVar] = fmt.Sprintf("%v", item.currentValue)
		}
	}

	return exports
}

// defaultEnvTransform creates the default environment variable transformer
func defaultEnvTransform(prefix string) EnvTransformFunc {
	return func(path string) string {
		env := strings.ReplaceAll(path, ".", "_")
		env = strings.ToUpper(env)
		if prefix != "" {
			env = prefix + env
		}
		return env
	}
}

// parseValue attempts to parse a string into appropriate types
// Only basic parse, complex parsing is deferred to mapstructure's decode hooks
func parseValue(s string) any {
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Remove quotes if present
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}

	// Return as string - mapstructure will convert as needed
	return s
}

// Save writes the current configuration to a TOML file atomically.
// Only registered paths are saved.
func (c *Config) Save(path string) error {
	c.mutex.RLock()

	nestedData := make(map[string]any)
	for itemPath, item := range c.items {
		setNestedValue(nestedData, itemPath, item.currentValue)
	}

	c.mutex.RUnlock()

	// Marshal using BurntSushi/toml
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(nestedData); err != nil {
		return fmt.Errorf("failed to marshal config data to TOML: %w", err)
	}
	tomlData := buf.Bytes()

	// Atomic write logic
	dir := filepath.Dir(path)
	// Ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory '%s': %w", dir, err)
	}

	// Create a temporary file in the same directory
	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary config file in '%s': %w", dir, err)
	}

	tempFilePath := tempFile.Name()
	removed := false
	defer func() {
		if !removed {
			os.Remove(tempFilePath)
		}
	}()

	// Write data to the temporary file
	if _, err := tempFile.Write(tomlData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp config file '%s': %w", tempFilePath, err)
	}

	// Sync data to disk
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp config file '%s': %w", tempFilePath, err)
	}

	// Close the temporary file
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file '%s': %w", tempFilePath, err)
	}

	// Set permissions on the temporary file
	if err := os.Chmod(tempFilePath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on temporary config file '%s': %w", tempFilePath, err)
	}

	// Atomically replace the original file
	if err := os.Rename(tempFilePath, path); err != nil {
		return fmt.Errorf("failed to rename temp file '%s' to '%s': %w", tempFilePath, path, err)
	}
	removed = true

	return nil
}

// SaveSource writes values from a specific source to a TOML file
func (c *Config) SaveSource(path string, source Source) error {
	c.mutex.RLock()

	nestedData := make(map[string]any)
	for itemPath, item := range c.items {
		if val, exists := item.values[source]; exists {
			setNestedValue(nestedData, itemPath, val)
		}
	}

	c.mutex.RUnlock()

	// Marshal using BurntSushi/toml
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(nestedData); err != nil {
		return fmt.Errorf("failed to marshal %s source data to TOML: %w", source, err)
	}

	return atomicWriteFile(path, buf.Bytes())
}

// atomicWriteFile performs atomic file write
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory '%s': %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath) // Clean up on any error

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err := os.Chmod(tempPath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// parseArgs processes command-line arguments into a nested map structure.
func parseArgs(args []string) (map[string]any, error) {
	result := make(map[string]any)
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			// Skip non-flag arguments
			i++
			continue
		}

		argContent := strings.TrimPrefix(arg, "--")
		if argContent == "" {
			// Skip "--" argument if used as a separator
			i++
			continue
		}

		var keyPath string
		var valueStr string

		// Check for "--key=value" format
		if strings.Contains(argContent, "=") {
			parts := strings.SplitN(argContent, "=", 2)
			keyPath = parts[0]
			valueStr = parts[1]
			i++ // Consume only this argument
		} else {
			// Handle "--key value" or "--booleanflag"
			keyPath = argContent
			// Check if it's a boolean flag (next arg is another flag or end of args)
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				valueStr = "true"
				i++ // Consume only the flag argument
			} else {
				// It's a key-value pair with a space
				valueStr = args[i+1]
				i += 2 // Consume both flag and value arguments
			}
		}

		if keyPath == "" {
			// Skip invalid flags like --=value
			continue
		}

		// Validate keyPath segments
		segments := strings.Split(keyPath, ".")
		for _, segment := range segments {
			if !isValidKeySegment(segment) {
				return nil, fmt.Errorf("invalid command-line key segment %q in path %q", segment, keyPath)
			}
		}

		// Always store as a string. Let Scan handle final type conversion.
		setNestedValue(result, keyPath, valueStr)
	}

	return result, nil
}