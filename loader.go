// FILE: lixenwraith/config/loader.go
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
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
	// Security: Path traversal check
	if c.securityOpts != nil && c.securityOpts.PreventPathTraversal {
		// Clean the path and check for traversal attempts
		cleanPath := filepath.Clean(path)

		// Check if cleaned path tries to go outside current directory
		if strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." {
			return fmt.Errorf("potential path traversal detected in config path: %s", path)
		}

		// Also check for absolute paths that might escape jail
		if filepath.IsAbs(cleanPath) && filepath.IsAbs(path) {
			// Absolute paths are OK if that's what was provided
		} else if filepath.IsAbs(cleanPath) && !filepath.IsAbs(path) {
			// Relative path became absolute after cleaning - suspicious
			return fmt.Errorf("potential path traversal detected in config path: %s", path)
		}
	}

	// Read file with size limit
	fileInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConfigNotFound
		}
		return fmt.Errorf("failed to stat config file '%s': %w", path, err)
	}

	// Security: File size check
	if c.securityOpts != nil && c.securityOpts.MaxFileSize > 0 {
		if fileInfo.Size() > c.securityOpts.MaxFileSize {
			return fmt.Errorf("config file '%s' exceeds maximum size %d bytes", path, c.securityOpts.MaxFileSize)
		}
	}

	// Security: File ownership check (Unix only)
	if c.securityOpts != nil && c.securityOpts.EnforceFileOwnership && runtime.GOOS != "windows" {
		if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
			if stat.Uid != uint32(os.Geteuid()) {
				return fmt.Errorf("config file '%s' is not owned by current user (file UID: %d, process UID: %d)",
					path, stat.Uid, os.Geteuid())
			}
		}
	}

	// 1. Read and parse file data
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file '%s': %w", path, err)
	}
	defer file.Close()

	// Use LimitedReader for additional safety
	var reader io.Reader = file
	if c.securityOpts != nil && c.securityOpts.MaxFileSize > 0 {
		reader = io.LimitReader(file, c.securityOpts.MaxFileSize)
	}

	fileData, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	// Determine format
	format := c.fileFormat
	if format == "" || format == "auto" {
		// Try extension first
		format = detectFileFormat(path)
		if format == "" {
			// Fall back to content detection
			format = detectFormatFromContent(fileData)
			if format == "" {
				// Last resort: use tagName as hint
				format = c.tagName
			}
		}
	}

	// Parse based on detected/specified format
	fileConfig := make(map[string]any)
	switch format {
	case "toml":
		if err := toml.Unmarshal(fileData, &fileConfig); err != nil {
			return fmt.Errorf("failed to parse TOML config file '%s': %w", path, err)
		}
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(fileData))
		decoder.UseNumber() // Preserve number precision
		if err := decoder.Decode(&fileConfig); err != nil {
			return fmt.Errorf("failed to parse JSON config file '%s': %w", path, err)
		}
	case "yaml":
		if err := yaml.Unmarshal(fileData, &fileConfig); err != nil {
			return fmt.Errorf("failed to parse YAML config file '%s': %w", path, err)
		}
	default:
		return fmt.Errorf("unable to determine config format for file '%s'", path)
	}

	// 2. Prepare New State (Read-Lock Only)
	newFileData := make(map[string]any)

	// Briefly acquire a read-lock to safely get the list of registered paths.
	c.mutex.RLock()
	registeredPaths := make(map[string]bool, len(c.items))
	for p := range c.items {
		registeredPaths[p] = true
	}
	c.mutex.RUnlock()

	// Define a recursive function to populate newFileData. This runs without any lock.
	var apply func(prefix string, data map[string]any)
	apply = func(prefix string, data map[string]any) {
		for key, value := range data {
			fullPath := key
			if prefix != "" {
				fullPath = prefix + "." + key
			}
			if registeredPaths[fullPath] {
				newFileData[fullPath] = value
			} else if subMap, isMap := value.(map[string]any); isMap {
				apply(fullPath, subMap)
			}
		}
	}
	apply("", fileConfig)

	// 3. Atomically Update Config (Write-Lock)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.configFilePath = path
	c.fileData = newFileData

	// Apply the new state to the main config items.
	for path, item := range c.items {
		if value, exists := newFileData[path]; exists {
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			item.values[SourceFile] = value
		} else {
			// Key was not in the new file, so remove its old file-sourced value.
			delete(item.values, SourceFile)
		}
		// Recompute the current value based on new source precedence.
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}

	c.invalidateCache()
	return nil
}

// loadEnv loads configuration from environment variables
func (c *Config) loadEnv(opts LoadOptions) error {
	transform := opts.EnvTransform
	if transform == nil {
		transform = defaultEnvTransform(opts.EnvPrefix)
	}

	// -- 1. Prepare data (Read-Lock to get paths)
	c.mutex.RLock()
	paths := make([]string, 0, len(c.items))
	for p := range c.items {
		paths = append(paths, p)
	}
	c.mutex.RUnlock()

	// -- 2. Process env vars (No Lock)
	foundEnvVars := make(map[string]string)
	for _, path := range paths {
		if opts.EnvWhitelist != nil && !opts.EnvWhitelist[path] {
			continue
		}

		envVar := transform(path)
		if value, exists := os.LookupEnv(envVar); exists {
			if len(value) > MaxValueSize {
				return ErrValueSize
			}
			foundEnvVars[path] = value
		}
	}

	// If no relevant env vars were found, we are done.
	if len(foundEnvVars) == 0 {
		return nil
	}

	// -- 3. Atomically update config (Write-Lock)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.envData = make(map[string]any, len(foundEnvVars))

	for path, value := range foundEnvVars {
		// Store raw string value - mapstructure will handle conversion later.
		if item, exists := c.items[path]; exists {
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			item.values[SourceEnv] = value // Store as string
			item.currentValue = c.computeValue(item)
			c.items[path] = item
			c.envData[path] = value
		}
	}

	c.invalidateCache()
	return nil
}

// loadCLI loads configuration from command-line arguments
func (c *Config) loadCLI(args []string) error {
	// -- 1. Prepare data (No Lock)
	parsedCLI, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCLIParse, err)
	}

	flattenedCLI := flattenMap(parsedCLI, "")
	if len(flattenedCLI) == 0 {
		return nil // No CLI args to process.
	}

	// 2. Atomically update config (Write-Lock)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cliData = flattenedCLI

	for path, value := range flattenedCLI {
		if item, exists := c.items[path]; exists {
			if item.values == nil {
				item.values = make(map[Source]any)
			}
			item.values[SourceCLI] = value
			item.currentValue = c.computeValue(item)
			c.items[path] = item
		}
	}

	c.invalidateCache()
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

// detectFileFormat determines format from file extension
func detectFileFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml", ".tml":
		return "toml"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".conf", ".config":
		// Try to detect from content
		return ""
	default:
		return ""
	}
}

// detectFormatFromContent attempts to detect format by parsing
func detectFormatFromContent(data []byte) string {
	// Try JSON first (strict format)
	var jsonTest any
	if err := json.Unmarshal(data, &jsonTest); err == nil {
		return "json"
	}

	// Try YAML (superset of JSON, so check after JSON)
	var yamlTest any
	if err := yaml.Unmarshal(data, &yamlTest); err == nil {
		return "yaml"
	}

	// Try TOML last
	var tomlTest any
	if err := toml.Unmarshal(data, &tomlTest); err == nil {
		return "toml"
	}

	return ""
}