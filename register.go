// File: lixenwraith/config/register.go
package config

import (
	"fmt"
	"os"
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
func (c *Config) RegisterStruct(prefix string, structWithDefaults any) error {
	return c.RegisterStructWithTags(prefix, structWithDefaults, "toml")
}

// RegisterStructWithTags is like RegisterStruct but allows custom tag names
func (c *Config) RegisterStructWithTags(prefix string, structWithDefaults any, tagName string) error {
	v := reflect.ValueOf(structWithDefaults)

	// Handle pointer or direct struct value
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return fmt.Errorf("RegisterStructWithTags requires a non-nil struct pointer or value")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return fmt.Errorf("RegisterStructWithTags requires a struct or struct pointer, got %T", structWithDefaults)
	}

	// Validate tag name
	switch tagName {
	case "toml", "json", "yaml":
		// Supported tags
	default:
		return fmt.Errorf("unsupported tag name %q, must be one of: toml, json, yaml", tagName)
	}

	var errors []string

	// Use helper function for recursive registration with specified tag
	c.registerFields(v, prefix, "", &errors, tagName)

	if len(errors) > 0 {
		return fmt.Errorf("failed to register %d field(s): %s", len(errors), strings.Join(errors, "; "))
	}

	return nil
}

// registerFields is a helper function that handles the recursive field registration.
func (c *Config) registerFields(v reflect.Value, pathPrefix, fieldPath string, errors *[]string, tagName string) {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if !field.IsExported() {
			continue
		}

		// Get tag value based on tagName parameter
		tag := field.Tag.Get(tagName)
		if tag == "-" {
			continue
		}

		// Fall back to field name if no tag
		key := field.Name
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}

		// Check for additional tags
		envTag := field.Tag.Get("env") // Explicit env var name
		required := field.Tag.Get("required") == "true"

		// Build full path
		currentPath := key
		if pathPrefix != "" {
			if !strings.HasSuffix(pathPrefix, ".") {
				pathPrefix += "."
			}
			currentPath = pathPrefix + key
		}

		// TODO: use mapstructure instead of logic with reflection
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
			c.registerFields(nestedValue, nestedPrefix, fieldPath+field.Name+".", errors, tagName)
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

// Scan decodes configuration into target using the unified unmarshal function
func (c *Config) Scan(basePath string, target any) error {
	return c.unmarshal(basePath, "", target) // Empty source means use merged state
}

// ScanSource decodes configuration from specific source using unified unmarshal
func (c *Config) ScanSource(basePath string, source Source, target any) error {
	return c.unmarshal(basePath, source, target)
}