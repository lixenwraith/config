// Package config provides thread-safe configuration management for Go applications
// with support for TOML files, command-line overrides, and default values.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/LixenWraith/tinytoml"
	"github.com/mitchellh/mapstructure"
)

// configItem holds both the default and current value for a configuration path
type configItem struct {
	defaultValue any
	currentValue any
}

// Config manages application configuration loaded from files and CLI arguments.
type Config struct {
	items map[string]configItem // Maps paths to config items (default and current values)
	mutex sync.RWMutex          // Protects concurrent access
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		items: make(map[string]configItem),
	}
}

// Register makes a configuration path known to the Config instance.
// The path should be dot-separated (e.g., "server.port", "debug").
// Each segment of the path must be a valid TOML key identifier.
// defaultValue is the value returned by Get if no specific value has been set.
func (c *Config) Register(path string, defaultValue any) error {
	if path == "" {
		return fmt.Errorf("registration path cannot be empty")
	}

	// Validate path segments
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		if !isValidKeySegment(segment) {
			return fmt.Errorf("invalid path segment %q in path %q", segment, path)
		}
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items[path] = configItem{
		defaultValue: defaultValue,
		currentValue: defaultValue, // Initially set to default
	}

	return nil
}

// Unregister removes a configuration path and all its children.
func (c *Config) Unregister(path string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.items[path]; !exists {
		return fmt.Errorf("path not registered: %s", path)
	}

	// Remove the path itself
	delete(c.items, path)

	// Remove any child paths
	prefix := path + "."
	for childPath := range c.items {
		if strings.HasPrefix(childPath, prefix) {
			delete(c.items, childPath)
		}
	}

	return nil
}

// RegisterStruct registers configuration values derived from a struct.
// It uses struct tags to determine the configuration paths.
// The prefix is prepended to all paths (e.g., "log.").
func (c *Config) RegisterStruct(prefix string, structWithDefaults interface{}) error {
	v := reflect.ValueOf(structWithDefaults)

	// Handle pointer or direct struct value
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return fmt.Errorf("RegisterStruct requires a struct, got %T", structWithDefaults)
	}

	t := v.Type()
	var firstErr error

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get tag value or use field name
		tag := field.Tag.Get("toml")
		if tag == "-" {
			continue // Skip this field
		}

		// Extract tag name or use field name
		key := field.Name
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}

		// Build full path
		path := prefix + key

		// Register this field
		if err := c.Register(path, fieldValue.Interface()); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// GetRegisteredPaths returns all registered configuration paths with the specified prefix.
func (c *Config) GetRegisteredPaths(prefix string) map[string]bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]bool)
	for path := range c.items {
		if strings.HasPrefix(path, prefix) {
			result[path] = true
		}
	}

	return result
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

// String retrieves a string configuration value using the path.
func (c *Config) String(path string) (string, error) {
	val, found := c.Get(path)
	if !found {
		return "", fmt.Errorf("path not registered: %s", path)
	}

	if strVal, ok := val.(string); ok {
		return strVal, nil
	}

	// Try to convert other types to string
	switch v := val.(type) {
	case fmt.Stringer:
		return v.String(), nil
	case error:
		return v.Error(), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

// Int64 retrieves an int64 configuration value using the path.
func (c *Config) Int64(path string) (int64, error) {
	val, found := c.Get(path)
	if !found {
		return 0, fmt.Errorf("path not registered: %s", path)
	}

	// Type assertion
	if intVal, ok := val.(int64); ok {
		return intVal, nil
	}

	// Try to convert other numeric types
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, nil
		} else {
			return 0, fmt.Errorf("cannot convert string '%s' to int64: %w", v, err)
		}
	}

	return 0, fmt.Errorf("cannot convert %T to int64", val)
}

// Bool retrieves a boolean configuration value using the path.
func (c *Config) Bool(path string) (bool, error) {
	val, found := c.Get(path)
	if !found {
		return false, fmt.Errorf("path not registered: %s", path)
	}

	// Type assertion
	if boolVal, ok := val.(bool); ok {
		return boolVal, nil
	}

	// Try to convert string to bool
	if strVal, ok := val.(string); ok {
		if b, err := strconv.ParseBool(strVal); err == nil {
			return b, nil
		} else {
			return false, fmt.Errorf("cannot convert string '%s' to bool: %w", strVal, err)
		}
	}

	// Try to interpret numbers
	switch v := val.(type) {
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	}

	return false, fmt.Errorf("cannot convert %T to bool", val)
}

// Float64 retrieves a float64 configuration value using the path.
func (c *Config) Float64(path string) (float64, error) {
	val, found := c.Get(path)
	if !found {
		return 0.0, fmt.Errorf("path not registered: %s", path)
	}

	// Type assertion
	if floatVal, ok := val.(float64); ok {
		return floatVal, nil
	}

	// Try to convert other numeric types
	switch v := val.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		} else {
			return 0.0, fmt.Errorf("cannot convert string '%s' to float64: %w", v, err)
		}
	}

	return 0.0, fmt.Errorf("cannot convert %T to float64", val)
}

// Load reads configuration from a TOML file and merges overrides from command-line arguments.
// 'args' should be the command-line arguments (e.g., os.Args[1:]).
// Returns true if the configuration file was found and loaded, false otherwise.
func (c *Config) Load(path string, args []string) (bool, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	configExists := false

	// First, build a nested map for file data (if it exists)
	nestedData := make(map[string]any)

	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		configExists = true
		fileData, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("failed to read config file '%s': %w", path, err)
		}

		if err := tinytoml.Unmarshal(fileData, &nestedData); err != nil {
			return false, fmt.Errorf("failed to parse TOML config file '%s': %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check config file '%s': %w", path, err)
	}

	// Flatten the nested map into path->value pairs
	flattenedData := flattenMap(nestedData, "")

	// Parse CLI arguments if any
	if len(args) > 0 {
		cliOverrides, err := parseArgs(args)
		if err != nil {
			return configExists, fmt.Errorf("failed to parse CLI args: %w", err)
		}

		// Merge CLI overrides into flattened data (CLI takes precedence)
		for path, value := range flattenMap(cliOverrides, "") {
			flattenedData[path] = value
		}
	}

	// Update configItems with loaded values
	for path, value := range flattenedData {
		if item, registered := c.items[path]; registered {
			// Update existing item
			item.currentValue = value
			c.items[path] = item
		} else {
			// Create new item with default = current = loaded value
			c.items[path] = configItem{
				defaultValue: value,
				currentValue: value,
			}
		}
	}

	return configExists, nil
}

// Save writes the current configuration to a TOML file.
// It performs an atomic write using a temporary file.
func (c *Config) Save(path string) error {
	c.mutex.RLock()

	// Build a nested map from our flat structure
	nestedData := make(map[string]any)
	for path, item := range c.items {
		setNestedValue(nestedData, path, item.currentValue)
	}

	c.mutex.RUnlock() // Release lock before I/O operations

	tomlData, err := tinytoml.Marshal(nestedData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Atomic write logic
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory '%s': %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary config file: %w", err)
	}
	defer os.Remove(tempFile.Name()) // Clean up temp file if rename fails

	if _, err := tempFile.Write(tomlData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp config file '%s': %w", tempFile.Name(), err)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp config file '%s': %w", tempFile.Name(), err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file '%s': %w", tempFile.Name(), err)
	}

	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("failed to rename temp file to '%s': %w", path, err)
	}

	if err := os.Chmod(path, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on config file '%s': %w", path, err)
	}

	return nil
}

// UnmarshalSubtree decodes the configuration data under a specific base path into the target struct or map.
func (c *Config) UnmarshalSubtree(basePath string, target any) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Build the nested map from our flat structure
	fullNestedMap := make(map[string]any)
	for path, item := range c.items {
		setNestedValue(fullNestedMap, path, item.currentValue)
	}

	var subtreeData any

	if basePath == "" {
		// Use the entire data structure
		subtreeData = fullNestedMap
	} else {
		// Navigate to the specific subtree
		segments := strings.Split(basePath, ".")
		current := any(fullNestedMap)

		for _, segment := range segments {
			currentMap, ok := current.(map[string]any)
			if !ok {
				// Path segment is not a map
				return fmt.Errorf("configuration path segment %q is not a table (map)", segment)
			}

			value, exists := currentMap[segment]
			if !exists {
				// If the path doesn't exist, return an empty map
				subtreeData = make(map[string]any)
				break
			}

			current = value
		}

		if subtreeData == nil {
			subtreeData = current
		}
	}

	// Ensure we have a map for decoding
	subtreeMap, ok := subtreeData.(map[string]any)
	if !ok {
		return fmt.Errorf("configuration path %q does not refer to a table (map)", basePath)
	}

	// Use mapstructure to decode
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		TagName:          "toml",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	err = decoder.Decode(subtreeMap)
	if err != nil {
		return fmt.Errorf("failed to decode subtree %q: %w", basePath, err)
	}

	return nil
}

// parseArgs processes command-line arguments into a nested map structure.
// Expects arguments in the format "--key.subkey value" or "--booleanflag".
func parseArgs(args []string) (map[string]any, error) {
	result := make(map[string]any)
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			i++ // Skip non-flag arguments
			continue
		}

		keyPath := strings.TrimPrefix(arg, "--")
		if keyPath == "" {
			i++ // Skip "--" argument
			continue
		}

		var valueStr string
		// Check if it's a boolean flag (next arg starts with -- or end of args)
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			valueStr = "true" // Assume boolean flag if no value provided
			i++               // Consume only the flag
		} else {
			valueStr = args[i+1]
			i += 2 // Consume flag and value
		}

		// Try to parse the value into bool, int, float, otherwise keep as string
		var value any
		if v, err := strconv.ParseBool(valueStr); err == nil {
			value = v
		} else if v, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			value = v // Store as int64
		} else if v, err := strconv.ParseFloat(valueStr, 64); err == nil {
			value = v
		} else {
			value = valueStr // Keep as string if parsing fails
		}

		// Set the value in the result map
		setNestedValue(result, keyPath, value)
	}

	return result, nil
}

// flattenMap converts a nested map to a flat map with dot-notation paths.
func flattenMap(nested map[string]any, prefix string) map[string]any {
	flat := make(map[string]any)

	for key, value := range nested {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		if nestedMap, isMap := value.(map[string]any); isMap {
			// Recursively flatten nested maps
			for subPath, subValue := range flattenMap(nestedMap, path) {
				flat[subPath] = subValue
			}
		} else {
			// Add leaf value
			flat[path] = value
		}
	}

	return flat
}

// setNestedValue sets a value in a nested map using a dot-notation path.
func setNestedValue(nested map[string]any, path string, value any) {
	segments := strings.Split(path, ".")

	if len(segments) == 1 {
		// Base case: set the value directly
		nested[segments[0]] = value
		return
	}

	// Ensure parent map exists
	if _, exists := nested[segments[0]]; !exists {
		nested[segments[0]] = make(map[string]any)
	}

	// Ensure the existing value is a map, or replace it
	current := nested[segments[0]]
	currentMap, isMap := current.(map[string]any)
	if !isMap {
		currentMap = make(map[string]any)
		nested[segments[0]] = currentMap
	}

	// Recurse with remaining path
	setNestedValue(currentMap, strings.Join(segments[1:], "."), value)
}

// isValidKeySegment checks if a single path segment is valid.
func isValidKeySegment(s string) bool {
	if len(s) == 0 {
		return false
	}
	firstChar := rune(s[0])
	// Using simplified check: must not contain dots and must be valid TOML key part
	if strings.ContainsRune(s, '.') {
		return false // Segments themselves cannot contain dots
	}
	if !isAlpha(firstChar) && firstChar != '_' {
		return false
	}
	for _, r := range s[1:] {
		if !isAlpha(r) && !isNumeric(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// isAlpha checks if a character is a letter (A-Z, a-z)
func isAlpha(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isNumeric checks if a character is a digit (0-9)
func isNumeric(c rune) bool {
	return c >= '0' && c <= '9'
}