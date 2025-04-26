package config

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"reflect"
	"strings"
)

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

	// Check if the exact path exists before proceeding
	if _, exists := c.items[path]; !exists {
		// Check if it's a prefix for other registered paths
		hasChildren := false
		prefix := path + "."
		for childPath := range c.items {
			if strings.HasPrefix(childPath, prefix) {
				hasChildren = true
				break
			}
		}
		// If neither the path nor any children exist, return error
		if !hasChildren {
			return fmt.Errorf("path not registered: %s", path)
		}
	}

	// Remove the path itself if it exists
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
// It uses struct tags (`toml:"..."`) to determine the configuration paths.
// The prefix is prepended to all paths (e.g., "log."). An empty prefix is allowed.
func (c *Config) RegisterStruct(prefix string, structWithDefaults interface{}) error {
	v := reflect.ValueOf(structWithDefaults)

	// Handle pointer or direct struct value
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return fmt.Errorf("RegisterStruct requires a non-nil struct pointer or value")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return fmt.Errorf("RegisterStruct requires a struct or struct pointer, got %T", structWithDefaults)
	}

	var errors []string

	// Use a helper function for recursive registration
	c.registerFields(v, prefix, "", &errors) // Pass receiver `c`

	if len(errors) > 0 {
		return fmt.Errorf("failed to register %d field(s): %s", len(errors), strings.Join(errors, "; "))
	}

	return nil
}

// registerFields is a helper function that handles the recursive field registration.
// It's now a method on *Config to simplify calling c.Register.
func (c *Config) registerFields(v reflect.Value, pathPrefix, fieldPath string, errors *[]string) {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if !field.IsExported() {
			continue
		}

		// Get tag value or use field name
		tag := field.Tag.Get("toml")
		if tag == "-" {
			continue // Skip this field
		}

		key := field.Name
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
			// Note: We are ignoring other tag options like 'omitempty' here,
			// as RegisterStruct is about setting defaults.
		}

		// Build full path
		currentPath := key
		if pathPrefix != "" {
			// Ensure trailing dot on prefix if needed
			if !strings.HasSuffix(pathPrefix, ".") {
				pathPrefix += "."
			}
			currentPath = pathPrefix + key
		}

		// Handle nested structs recursively
		// Check for pointer to struct as well
		fieldType := fieldValue.Type()
		isStruct := fieldValue.Kind() == reflect.Struct
		isPtrToStruct := fieldValue.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct

		if isStruct || isPtrToStruct {
			// Dereference pointer if necessary
			nestedValue := fieldValue
			if isPtrToStruct {
				if fieldValue.IsNil() {
					// Skip nil pointers, as their paths aren't well-defined defaults.
					continue
				}
				nestedValue = fieldValue.Elem()
			}

			// For nested structs, append a dot and continue recursion
			nestedPrefix := currentPath + "."
			c.registerFields(nestedValue, nestedPrefix, fieldPath+field.Name+".", errors) // Call recursively on `c`
			continue
		}

		// Register non-struct fields
		// Use fieldValue.Interface() to get the actual default value
		if err := c.Register(currentPath, fieldValue.Interface()); err != nil {
			*errors = append(*errors, fmt.Sprintf("field %s%s (path %s): %v", fieldPath, field.Name, currentPath, err))
		}
	}
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

// Scan decodes the configuration data under a specific base path
// into the target struct or map. It operates on the current, merged configuration state.
// The target must be a non-nil pointer to a struct or map.
// It uses the "toml" struct tag for mapping fields.
func (c *Config) Scan(basePath string, target any) error {
	// Validate target
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("target of Scan must be a non-nil pointer, got %T", target)
	}

	c.mutex.RLock() // Read lock is sufficient

	// Build the full nested map from the current state of registered items
	fullNestedMap := make(map[string]any)
	for path, item := range c.items {
		setNestedValue(fullNestedMap, path, item.currentValue)
	}

	c.mutex.RUnlock() // Unlock before decoding

	var sectionData any = fullNestedMap

	// Navigate to the specific section if basePath is provided
	if basePath != "" {
		// Allow trailing dot for convenience
		basePath = strings.TrimSuffix(basePath, ".")
		if basePath == "" { // Handle case where input was just "."
			// Use the full map
		} else {
			segments := strings.Split(basePath, ".")
			current := any(fullNestedMap)
			found := true

			for _, segment := range segments {
				currentMap, ok := current.(map[string]any)
				if !ok {
					// Path segment does not lead to a map/table
					found = false
					break
				}

				value, exists := currentMap[segment]
				if !exists {
					// The requested path segment does not exist in the current config
					found = false
					break
				}
				current = value
			}

			if !found {
				// If the path doesn't fully exist, decode an empty map into the target.
				sectionData = make(map[string]any)
			} else {
				sectionData = current
			}
		}
	}

	// Ensure the final data we are decoding from is actually a map
	sectionMap, ok := sectionData.(map[string]any)
	if !ok {
		// This can happen if the basePath points to a non-map value (e.g., a string, int)
		return fmt.Errorf("configuration path %q does not refer to a scannable section (map), but to type %T", basePath, sectionData) // Updated error message
	}

	// Use mapstructure to decode the relevant section map into the target
	decoderConfig := &mapstructure.DecoderConfig{
		Result:           target,
		TagName:          "toml", // Use the same tag name for consistency
		WeaklyTypedInput: true,   // Allow conversions (e.g., int to string if needed by target)
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	err = decoder.Decode(sectionMap) // Use sectionMap
	if err != nil {
		return fmt.Errorf("failed to scan section %q into %T: %w", basePath, target, err) // Updated error message
	}

	return nil
}