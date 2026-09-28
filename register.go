// FILE: lixenwraith/config/register.go
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// Register makes a configuration path known to the Config instance
// The path should be dot-separated (e.g., "server.port", "debug")
// Each segment of the path must be a valid TOML key identifier
// defaultValue is the value returned by Get if no specific value has been set
func (c *Config) Register(path string, defaultValue any) error {
	if err := validatePath(path); err != nil {
		return err
	}
	owned, err := copyValue(defaultValue)
	if err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.registerLocked(path, owned)
}

func validatePath(path string) error {
	if path == "" {
		return wrapError(ErrInvalidPath, fmt.Errorf("registration path cannot be empty"))
	}
	parts := strings.Split(path, ".")
	if len(parts) > maxValueDepth {
		return wrapError(ErrInvalidPath, fmt.Errorf("path nesting exceeds %d", maxValueDepth))
	}
	for _, part := range parts {
		if !isValidKeySegment(part) {
			return wrapError(ErrInvalidPath, fmt.Errorf("invalid path segment %q", part))
		}
	}
	return nil
}

func (c *Config) registerLocked(path string, value any) error {
	for existing := range c.items {
		if existing != path && (strings.HasPrefix(existing, path+".") || strings.HasPrefix(path, existing+".")) {
			return wrapError(ErrInvalidPath, fmt.Errorf("overlapping registration %q and %q", path, existing))
		}
	}
	item, exists := c.items[path]
	if !exists {
		item.values = make(map[Source]any)
	}
	item.defaultValue = value
	for _, v := range item.values {
		if err := validateValue(item, v); err != nil {
			return err
		}
	}
	item.currentValue = c.computeValue(item)
	c.items[path] = item
	c.invalidateCache()
	return nil
}

func validateValue(item configItem, value any) error {
	if item.defaultValue == nil || value == nil {
		return nil
	}
	target := reflect.New(reflect.TypeOf(item.defaultValue)).Interface()
	if err := decodeConfig(value, target); err != nil {
		return wrapError(ErrTypeMismatch, err)
	}
	return nil
}

func validSource(source Source) bool {
	return source == SourceDefault || source == SourceFile || source == SourceEnv || source == SourceCLI
}

// RegisterWithEnv registers a path with an explicit environment variable mapping
func (c *Config) RegisterWithEnv(path string, defaultValue any, envVar string) error {
	if err := c.Register(path, defaultValue); err != nil {
		return err
	}

	// Check if the environment variable exists and load it
	if value, exists := os.LookupEnv(envVar); exists {
		return c.SetSource(SourceEnv, path, value) // Already wrapped with error category in SetSource
	}

	return nil
}

// RegisterRequired registers a path and marks it as required
// The configuration will fail validation if this value is not provided
func (c *Config) RegisterRequired(path string, defaultValue any) error {
	if err := validatePath(path); err != nil {
		return err
	}
	owned, err := copyValue(defaultValue)
	if err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.registerLocked(path, owned); err != nil {
		return err
	}
	item := c.items[path]
	item.required = true
	c.items[path] = item
	return nil
}

// Unregister removes a configuration path and all its children
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
			return wrapError(ErrPathNotRegistered, fmt.Errorf("path not registered: %s", path))
		}
	}

	// Remove the path itself if it exists
	delete(c.items, path)
	delete(c.fileData, path)
	delete(c.envData, path)
	delete(c.cliData, path)

	c.invalidateCache()

	// Remove any child paths
	prefix := path + "."
	for childPath := range c.items {
		if strings.HasPrefix(childPath, prefix) {
			delete(c.items, childPath)
			delete(c.fileData, childPath)
			delete(c.envData, childPath)
			delete(c.cliData, childPath)
		}
	}

	return nil
}

// RegisterStruct registers configuration values derived from a struct
// It uses struct tags (`toml:"..."`) to determine the configuration paths
// The prefix is prepended to all paths (e.g., "log."). An empty prefix is allowed
func (c *Config) RegisterStruct(prefix string, structWithDefaults any) error {
	return c.RegisterStructWithTags(prefix, structWithDefaults, FormatTOML)
}

// RegisterStructWithTags is like RegisterStruct but allows custom tag names
func (c *Config) RegisterStructWithTags(prefix string, structWithDefaults any, tagName string) error {
	v := reflect.ValueOf(structWithDefaults)

	// Handle pointer or direct struct value
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return wrapError(ErrTypeMismatch, fmt.Errorf("RegisterStructWithTags requires a non-nil struct pointer or value"))
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return wrapError(ErrTypeMismatch, fmt.Errorf("RegisterStructWithTags requires a struct or struct pointer, got %T", structWithDefaults))
	}

	// Validate tag name
	switch tagName {
	case FormatTOML:
		// Supported tags
	default:
		return wrapError(ErrTypeMismatch, fmt.Errorf("unsupported tag name %q, must be one of: toml", tagName))
	}

	var errors []string

	// Use helper function for recursive registration with specified tag
	// Stage registrations and explicit env values so failures do not leave a partial schema.
	staged := New()
	staged.registerFields(v, prefix, "", &errors, tagName)

	if len(errors) > 0 {
		return wrapError(ErrTypeMismatch, fmt.Errorf("failed to register %d field(s): %s", len(errors), strings.Join(errors, "; ")))
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	for path, item := range staged.items {
		for existing := range c.items {
			if existing != path && (strings.HasPrefix(existing, path+".") || strings.HasPrefix(path, existing+".")) {
				return wrapError(ErrInvalidPath, fmt.Errorf("overlapping registration %q and %q", path, existing))
			}
		}
		if old, ok := c.items[path]; ok {
			for _, value := range old.values {
				if err := validateValue(item, value); err != nil {
					return err
				}
			}
		}
	}
	for path, item := range staged.items {
		if old, ok := c.items[path]; ok {
			for source, value := range old.values {
				if _, provided := item.values[source]; !provided {
					item.values[source] = value
				}
			}
		}
		item.currentValue = c.computeValue(item)
		c.items[path] = item
	}
	c.invalidateCache()
	return nil
}

// registerFields is a helper function that handles the recursive field registration
func (c *Config) registerFields(v reflect.Value, pathPrefix, fieldPath string, errors *[]string, tagName string) {
	if strings.Count(pathPrefix, ".") > maxValueDepth {
		*errors = append(*errors, "recursive struct exceeds maximum depth")
		return
	}
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if !field.IsExported() {
			continue
		}

		// Get tag value based on tagName parameter
		tag := field.Tag.Get(tagName)
		if strings.Split(tag, ",")[0] == "-" {
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

		// Handle nested structs recursively
		fieldType := fieldValue.Type()
		isStruct := fieldValue.Kind() == reflect.Struct
		isPtrToStruct := fieldValue.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct

		if isStruct || isPtrToStruct {
			// Check if the field's TYPE is one that should be treated as a single value,
			// even though it's a struct. These types have custom decode hooks
			isAtomicStruct := atomicType(fieldValue.Type())

			// Only recurse if it's a "normal" struct, not an atomic one
			if !isAtomicStruct {
				nestedValue := fieldValue
				if isPtrToStruct {
					if fieldValue.IsNil() {
						nestedValue = reflect.New(fieldType.Elem()).Elem()
					}
					if !fieldValue.IsNil() {
						nestedValue = fieldValue.Elem()
					}
				}

				nestedPrefix := currentPath + "."
				c.registerFields(nestedValue, nestedPrefix, fieldPath+field.Name+".", errors, tagName)
				continue
			}
			// If it is an atomic struct, we fall through and register it as a single value
		}

		// Register non-struct fields
		if _, exists := c.items[currentPath]; exists {
			*errors = append(*errors, fmt.Sprintf("duplicate TOML path %q", currentPath))
			continue
		}
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
				if setErr := c.SetSource(SourceEnv, currentPath, value); setErr != nil {
					*errors = append(*errors, fmt.Sprintf("field %s%s env %s: %v", fieldPath, field.Name, envTag, setErr))
				}
			}
		}
	}
}

// GetRegisteredPaths returns all registered configuration paths with the specified prefix
func (c *Config) GetRegisteredPaths(prefix ...string) map[string]bool {
	p := ""
	if len(prefix) > 0 {
		p = prefix[0]
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]bool)
	for path := range c.items {
		if strings.HasPrefix(path, p) {
			result[path] = true
		}
	}

	return result
}

// GetRegisteredPathsWithDefaults returns paths with their default values
func (c *Config) GetRegisteredPathsWithDefaults(prefix ...string) map[string]any {
	p := ""
	if len(prefix) > 0 {
		p = prefix[0]
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]any)
	for path, item := range c.items {
		if strings.HasPrefix(path, p) {
			result[path] = cloneOwned(item.defaultValue)
		}
	}

	return result
}

// Scan decodes configuration into target using the unified unmarshal function
func (c *Config) Scan(target any, basePath ...string) error {
	return c.unmarshal("", target, basePath...)
}

// ScanSource decodes configuration from specific source using unified unmarshal
func (c *Config) ScanSource(source Source, target any, basePath ...string) error {
	return c.unmarshal(source, target, basePath...)
}
