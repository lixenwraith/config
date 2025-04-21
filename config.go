package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync" // Added sync import

	"github.com/LixenWraith/tinytoml"
	"github.com/google/uuid" // Added uuid import
)

// registeredItem holds metadata for a configuration value registered for access.
type registeredItem struct {
	path         string // Dot-separated path (e.g., "server.port")
	defaultValue any
}

// Config manages application configuration loaded from files and CLI arguments.
// It provides thread-safe access to configuration values.
type Config struct {
	data     map[string]any            // Stores the actual configuration data (nested map)
	registry map[string]registeredItem // Maps generated UUIDs to registered items
	mutex    sync.RWMutex              // Protects concurrent access to data and registry
}

// New creates and initializes a new Config instance.
func New() *Config {
	return &Config{
		data:     make(map[string]any),
		registry: make(map[string]registeredItem),
		// mutex is implicitly initialized
	}
}

// Register makes a configuration path known to the Config instance and returns a unique key (UUID) for accessing it.
// The path should be dot-separated (e.g., "server.port", "debug").
// Each segment of the path must be a valid TOML key identifier.
// defaultValue is returned by Get if the value is not found in the loaded configuration.
func (c *Config) Register(path string, defaultValue any) (string, error) {
	if path == "" {
		return "", fmt.Errorf("registration path cannot be empty")
	}

	// Validate path segments
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		// tinytoml.isValidKey doesn't exist, but we can use its logic criteria.
		// Assuming isValidKey checks for alphanumeric, underscore, dash, starting with letter/underscore.
		// We adapt the validation logic here based on tinytoml's description.
		if !isValidKeySegment(segment) {
			return "", fmt.Errorf("invalid path segment %q in path %q", segment, path)
		}
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	newUUID := uuid.NewString()
	item := registeredItem{
		path:         path,
		defaultValue: defaultValue,
	}
	c.registry[newUUID] = item

	return newUUID, nil
}

// Unregister removes a configuration key from the registry.
// Subsequent calls to Get with this key will return (nil, false).
// This does not remove the value from the underlying configuration data map,
// only the ability to access it via this specific registration key.
func (c *Config) Unregister(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.registry, key)
}

// isValidKeySegment checks if a single path segment is valid.
// Adapts the logic described for tinytoml's isValidKey.
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

// Get retrieves a configuration value using the unique key (UUID) obtained from Register.
// It returns the value found in the loaded configuration or the registered default value.
// The second return value (bool) indicates if the key was successfully registered (true) or not (false).
func (c *Config) Get(key string) (any, bool) {
	c.mutex.RLock()
	item, registered := c.registry[key]
	if !registered {
		c.mutex.RUnlock()
		return nil, false
	}

	// Lookup value in the data map using the item's path
	value, found := getValueFromMap(c.data, item.path)
	c.mutex.RUnlock() // Unlock after accessing both registry and data

	if found {
		return value, true
	}
	// Key was registered, but value not found in data, return default
	return item.defaultValue, true
}

// Load reads configuration from a TOML file and merges overrides from command-line arguments.
// It populates the Config instance's internal data map.
// 'args' should be the command-line arguments (e.g., os.Args[1:]).
// Returns true if the configuration file was found and loaded, false otherwise.
func (c *Config) Load(path string, args []string) (bool, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	configExists := false
	loadedData := make(map[string]any) // Load into a temporary map first

	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		configExists = true
		fileData, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("failed to read config file '%s': %w", path, err)
		}

		// Use tinytoml to unmarshal directly into the map
		// Pass a pointer to the map for Unmarshal
		if err := tinytoml.Unmarshal(fileData, &loadedData); err != nil {
			return false, fmt.Errorf("failed to parse config file '%s': %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		// Handle potential errors from os.Stat other than file not existing
		return false, fmt.Errorf("failed to check config file '%s': %w", path, err)
	}

	// Merge loaded data into the main config data
	// This ensures existing data (e.g. from defaults set programmatically before load) isn't wiped out
	mergeMaps(c.data, loadedData)

	// Parse and merge CLI arguments if any
	if len(args) > 0 {
		overrides, err := parseArgs(args)
		if err != nil {
			return configExists, fmt.Errorf("failed to parse CLI args: %w", err)
		}
		// Merge overrides into the potentially file-loaded data
		mergeMaps(c.data, overrides)
	}

	return configExists, nil
}

// Save writes the current configuration stored in the Config instance to a TOML file.
// It performs an atomic write using a temporary file.
func (c *Config) Save(path string) error {
	c.mutex.RLock()
	// Marshal requires the actual value, not a pointer if data is already a map
	dataToMarshal := c.data
	c.mutex.RUnlock() // Unlock before potentially long I/O

	tomlData, err := tinytoml.Marshal(dataToMarshal)
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
		tempFile.Close() // Close file before attempting remove on error path
		return fmt.Errorf("failed to write temp config file '%s': %w", tempFile.Name(), err)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp config file '%s': %w", tempFile.Name(), err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file '%s': %w", tempFile.Name(), err)
	}

	// Use Rename for atomic replace
	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("failed to rename temp file to '%s': %w", path, err)
	}

	// Set permissions after successful rename
	if err := os.Chmod(path, 0644); err != nil {
		// Log or handle this non-critical error? For now, return it.
		return fmt.Errorf("failed to set permissions on config file '%s': %w", path, err)
	}

	return nil
}

// parseArgs processes command-line arguments into a nested map structure.
// Expects arguments in the format "--key.subkey value" or "--booleanflag".
func parseArgs(args []string) (map[string]any, error) {
	overrides := make(map[string]any)
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

		// Build nested map structure based on dots in the keyPath
		keys := strings.Split(keyPath, ".")
		currentMap := overrides
		for j, key := range keys[:len(keys)-1] {
			// Ensure intermediate paths are maps
			if existingVal, ok := currentMap[key]; ok {
				if nestedMap, isMap := existingVal.(map[string]any); isMap {
					currentMap = nestedMap // Navigate deeper
				} else {
					// Error: trying to overwrite a non-map value with a nested structure
					return nil, fmt.Errorf("conflicting CLI key: %q is not a table but has subkey %q", strings.Join(keys[:j+1], "."), keys[j+1])
				}
			} else {
				// Create intermediate map
				newMap := make(map[string]any)
				currentMap[key] = newMap
				currentMap = newMap
			}
		}
		// Set the final value
		lastKey := keys[len(keys)-1]
		currentMap[lastKey] = value
	}

	return overrides, nil
}

// mergeMaps recursively merges the 'override' map into the 'base' map.
// Values in 'override' take precedence. If both values are maps, they are merged recursively.
func mergeMaps(base map[string]any, override map[string]any) {
	if base == nil || override == nil {
		return // Avoid panic on nil maps, though caller should initialize
	}
	for key, overrideVal := range override {
		baseVal, _ := base[key]
		// Check if both values are maps for recursive merge
		baseMap, baseIsMap := baseVal.(map[string]any)
		overrideMap, overrideIsMap := overrideVal.(map[string]any)

		if baseIsMap && overrideIsMap {
			// Recursively merge nested maps
			mergeMaps(baseMap, overrideMap)
		} else {
			// Override value (or add if key doesn't exist in base)
			base[key] = overrideVal
		}
	}
}

// getValueFromMap retrieves a value from a nested map using a dot-separated path.
func getValueFromMap(data map[string]any, path string) (any, bool) {
	keys := strings.Split(path, ".")
	current := any(data) // Start with the top-level map

	for _, key := range keys {
		if currentMap, ok := current.(map[string]any); ok {
			value, exists := currentMap[key]
			if !exists {
				return nil, false // Key not found at this level
			}
			current = value // Move to the next level
		} else {
			return nil, false // Path segment is not a map, cannot traverse further
		}
	}

	// Successfully traversed the entire path
	return current, true
}

// Helper functions adapted from tinytoml internal logic (as it's not exported)
// isAlpha checks if a character is a letter (A-Z, a-z)
func isAlpha(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isNumeric checks if a character is a digit (0-9)
func isNumeric(c rune) bool {
	return c >= '0' && c <= '9'
}