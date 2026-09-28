// FILE: lixenwraith/config/loader.go
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
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

	// EnvWhitelist limits which paths are checked for env vars (nil = all)
	EnvWhitelist map[string]bool

	// Deprecated: SkipValidation is retained for source compatibility and ignored.
	// Path and value checks are always enforced.
	SkipValidation bool
}

// DefaultLoadOptions returns the standard load options
func DefaultLoadOptions() LoadOptions {
	return LoadOptions{
		Sources: []Source{SourceCLI, SourceEnv, SourceFile, SourceDefault},
	}
}

func cloneLoadOptions(opts LoadOptions) LoadOptions {
	opts.Sources = slices.Clone(opts.Sources)
	if len(opts.Sources) == 0 {
		opts.Sources = DefaultLoadOptions().Sources
	}
	opts.EnvWhitelist = maps.Clone(opts.EnvWhitelist)
	return opts
}

// loadWithOptions loads configuration from multiple sources with custom options
func (c *Config) loadWithOptions(filePath string, args []string, opts LoadOptions) error {
	opts = cloneLoadOptions(opts)
	c.SetLoadOptions(opts)

	var loadErrors []error
	var missingFile error

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
						missingFile = err
					} else {
						return wrapError(ErrFileAccess, err) // Fatal error
					}
				}
			}

		case SourceEnv:
			if err := c.loadEnv(opts); err != nil {
				loadErrors = append(loadErrors, wrapError(ErrEnvParse, err))
			}

		case SourceCLI:
			if err := c.loadCLI(args); err != nil {
				loadErrors = append(loadErrors, wrapError(ErrCLIParse, err))
			}
		}
	}

	if len(loadErrors) != 0 {
		return errors.Join(loadErrors...)
	}
	return missingFile
}

// LoadEnv loads configuration values from environment variables
func (c *Config) LoadEnv(prefix string) error {
	c.mutex.RLock()
	opts := cloneLoadOptions(c.options)
	c.mutex.RUnlock()
	opts.EnvPrefix = prefix
	return c.loadEnv(opts)
}

// LoadCLI loads configuration values from command-line arguments
func (c *Config) LoadCLI(args []string) error {
	if err := c.loadCLI(args); err != nil {
		return wrapError(ErrCLIParse, err)
	}
	return nil
}

// LoadFile loads configuration values from a TOML file
func (c *Config) LoadFile(filePath string) error {
	if err := c.loadFile(filePath); err != nil {
		return wrapError(ErrFileAccess, err)
	}
	return nil
}

// DiscoverEnv finds all environment variables matching registered paths
// and returns a map of path -> env var name for found variables
func (c *Config) DiscoverEnv(prefix string) map[string]string {
	c.mutex.RLock()
	transform := c.options.EnvTransform
	paths := make([]string, 0, len(c.items))
	for path := range c.items {
		paths = append(paths, path)
	}
	c.mutex.RUnlock()
	if transform == nil {
		transform = defaultEnvTransform(prefix)
	}

	discovered := make(map[string]string)

	for _, path := range paths {
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
	c.mutex.RLock()
	transform := c.options.EnvTransform
	changed := make(map[string]any)
	for path, item := range c.items {
		if !reflect.DeepEqual(item.currentValue, item.defaultValue) {
			changed[path] = item.currentValue
		}
	}
	c.mutex.RUnlock()
	if transform == nil {
		transform = defaultEnvTransform(prefix)
	}

	exports := make(map[string]string)

	for path, value := range changed {
		exports[transform(path)] = fmt.Sprintf("%v", value)
	}

	return exports
}

// loadEnv replaces the complete environment source, including removed variables.
func (c *Config) loadEnv(opts LoadOptions) error {
	transform := opts.EnvTransform
	if transform == nil {
		transform = defaultEnvTransform(opts.EnvPrefix)
	}
	c.mutex.RLock()
	paths := make([]string, 0, len(c.items))
	for path := range c.items {
		paths = append(paths, path)
	}
	c.mutex.RUnlock()
	values := make(map[string]any)
	for _, path := range paths {
		if opts.EnvWhitelist != nil && !opts.EnvWhitelist[path] {
			continue
		}
		if value, ok := os.LookupEnv(transform(path)); ok {
			values[path] = value
		}
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.replaceSourceLocked(SourceEnv, values)
}

func (c *Config) loadCLI(args []string) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.replaceSourceLocked(SourceCLI, flattenMap(parsed, ""))
}

// Validate the entire replacement before any mutation. Caller holds c.mutex.
func (c *Config) replaceSourceLocked(source Source, values map[string]any, guard ...func() error) error {
	accepted := make(map[string]any, len(values))
	var unknown []string
	for path, value := range values {
		item, ok := c.items[path]
		if !ok {
			if source == SourceCLI {
				unknown = append(unknown, path)
			}
			continue
		}
		if err := validateValue(item, value); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		owned, err := copyValue(value)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		accepted[path] = owned
	}
	if len(guard) > 0 {
		if err := guard[0](); err != nil {
			return err
		}
	}
	for path, item := range c.items {
		if value, ok := accepted[path]; ok {
			item.values[source] = value
		} else {
			delete(item.values, source)
		}
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}
	switch source {
	case SourceFile:
		c.fileData = accepted
	case SourceEnv:
		c.envData = accepted
	case SourceCLI:
		c.cliData = accepted
		slices.Sort(unknown)
		c.unknownCLIKeys = unknown
	}
	c.invalidateCache()
	return nil
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
// Only basic parse, complex parsing is deferred to typed decoding
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

	// Return as string - typed decoding converts as needed
	return s
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
			// Arguments after the terminator are positional.
			break
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
				return nil, wrapError(ErrInvalidPath, fmt.Errorf("invalid command-line key segment %q in path %q", segment, keyPath))
			}
		}

		// Always store as a string. Let Scan handle final type conversion.
		if len(valueStr) > MaxValueSize {
			return nil, ErrValueSize
		}
		setNestedValue(result, keyPath, valueStr)
	}

	return result, nil
}
