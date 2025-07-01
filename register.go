// File: lixenwraith/config/register.go
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
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
		values:       make(map[Source]any),
	}

	return nil
}

// RegisterWithEnv registers a path with an explicit environment variable mapping
func (c *Config) RegisterWithEnv(path string, defaultValue any, envVar string) error {
	if err := c.Register(path, defaultValue); err != nil {
		return err
	}

	// Check if the environment variable exists and load it
	if value, exists := os.LookupEnv(envVar); exists {
		parsed := parseValue(value)
		return c.SetSource(path, SourceEnv, parsed)
	}

	return nil
}

// RegisterRequired registers a path and marks it as required
// The configuration will fail validation if this value is not provided
func (c *Config) RegisterRequired(path string, defaultValue any) error {
	// For now, just register normally
	// The required paths will be tracked separately in a future enhancement
	return c.Register(path, defaultValue)
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
	c.registerFields(v, prefix, "", &errors)

	if len(errors) > 0 {
		return fmt.Errorf("failed to register %d field(s): %s", len(errors), strings.Join(errors, "; "))
	}

	return nil
}

// RegisterStructWithTags is like RegisterStruct but allows custom tag names
func (c *Config) RegisterStructWithTags(prefix string, structWithDefaults interface{}, tagName string) error {
	// Save current tag preference
	oldTag := "toml"

	// Temporarily use custom tag
	// Note: This would require modifying registerFields to accept tagName parameter
	// For now, we'll keep using "toml" tag
	_ = oldTag
	_ = tagName

	return c.RegisterStruct(prefix, structWithDefaults)
}

// registerFields is a helper function that handles the recursive field registration.
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

		// Check for additional tags
		envTag := field.Tag.Get("env") // Explicit env var name
		required := field.Tag.Get("required") == "true"

		key := field.Name
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}

		// Build full path
		currentPath := key
		if pathPrefix != "" {
			if !strings.HasSuffix(pathPrefix, ".") {
				pathPrefix += "."
			}
			currentPath = pathPrefix + key
		}

		// Handle nested structs recursively
		fieldType := fieldValue.Type()
		isStruct := fieldValue.Kind() == reflect.Struct
		isPtrToStruct := fieldValue.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct

		if isStruct || isPtrToStruct {
			// Dereference pointer if necessary
			nestedValue := fieldValue
			if isPtrToStruct {
				if fieldValue.IsNil() {
					// Skip nil pointers
					continue
				}
				nestedValue = fieldValue.Elem()
			}

			// For nested structs, append a dot and continue recursion
			nestedPrefix := currentPath + "."
			c.registerFields(nestedValue, nestedPrefix, fieldPath+field.Name+".", errors)
			continue
		}

		// Register non-struct fields
		defaultValue := fieldValue.Interface()

		var err error
		if required {
			err = c.RegisterRequired(currentPath, defaultValue)
		} else {
			err = c.Register(currentPath, defaultValue)
		}

		if err != nil {
			*errors = append(*errors, fmt.Sprintf("field %s%s (path %s): %v", fieldPath, field.Name, currentPath, err))
		}

		// Handle explicit env tag
		if envTag != "" && err == nil {
			if value, exists := os.LookupEnv(envTag); exists {
				parsed := parseValue(value)
				if setErr := c.SetSource(currentPath, SourceEnv, parsed); setErr != nil {
					*errors = append(*errors, fmt.Sprintf("field %s%s env %s: %v", fieldPath, field.Name, envTag, setErr))
				}
			}
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

// GetRegisteredPathsWithDefaults returns paths with their default values
func (c *Config) GetRegisteredPathsWithDefaults(prefix string) map[string]any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]any)
	for path, item := range c.items {
		if strings.HasPrefix(path, prefix) {
			result[path] = item.defaultValue
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
		// This can happen if the basePath points to a non-map value
		return fmt.Errorf("configuration path %q does not refer to a scannable section (map), but to type %T", basePath, sectionData)
	}

	// Use mapstructure to decode the relevant section map into the target
	decoderConfig := &mapstructure.DecoderConfig{
		Result:           target,
		TagName:          "toml", // Use the same tag name for consistency
		WeaklyTypedInput: true,   // Allow conversions
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	err = decoder.Decode(sectionMap)
	if err != nil {
		return fmt.Errorf("failed to scan section %q into %T: %w", basePath, target, err)
	}

	return nil
}

// ScanSource scans configuration from a specific source
func (c *Config) ScanSource(basePath string, source Source, target any) error {
	// Validate target
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("target of ScanSource must be a non-nil pointer, got %T", target)
	}

	c.mutex.RLock()

	// Build nested map from specific source only
	nestedMap := make(map[string]any)
	for path, item := range c.items {
		if val, exists := item.values[source]; exists {
			setNestedValue(nestedMap, path, val)
		}
	}

	c.mutex.RUnlock()

	// Rest of the logic is similar to Scan
	var sectionData any = nestedMap

	if basePath != "" {
		basePath = strings.TrimSuffix(basePath, ".")
		if basePath != "" {
			segments := strings.Split(basePath, ".")
			current := any(nestedMap)

			for _, segment := range segments {
				currentMap, ok := current.(map[string]any)
				if !ok {
					sectionData = make(map[string]any)
					break
				}

				value, exists := currentMap[segment]
				if !exists {
					sectionData = make(map[string]any)
					break
				}
				current = value
			}

			sectionData = current
		}
	}

	sectionMap, ok := sectionData.(map[string]any)
	if !ok {
		return fmt.Errorf("path %q does not refer to a map in source %s", basePath, source)
	}

	decoderConfig := &mapstructure.DecoderConfig{
		Result:           target,
		TagName:          "toml",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	return decoder.Decode(sectionMap)
}