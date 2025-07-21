// FILE: lixenwraith/config/decode.go
package config

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

// unmarshal is the single authoritative function for decoding configuration
// into target structures. All public decoding methods delegate to this.
func (c *Config) unmarshal(source Source, target any, basePath ...string) error {
	// Parse variadic basePath
	path := ""
	switch len(basePath) {
	case 0:
		// Use default empty path
	case 1:
		path = basePath[0]
	default:
		return fmt.Errorf("too many basePath arguments: expected 0 or 1, got %d", len(basePath))
	}

	// Validate target
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("unmarshal target must be non-nil pointer, got %T", target)
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Build nested map based on source selection
	nestedMap := make(map[string]any)

	if source == "" {
		// Use current merged state
		for path, item := range c.items {
			setNestedValue(nestedMap, path, item.currentValue)
		}
	} else {
		// Use specific source
		for path, item := range c.items {
			if val, exists := item.values[source]; exists {
				setNestedValue(nestedMap, path, val)
			}
		}
	}

	// Navigate to basePath section
	sectionData := navigateToPath(nestedMap, path)

	// Ensure we have a map to decode, normalizing if necessary.
	sectionMap, err := normalizeMap(sectionData)
	if err != nil {
		if sectionData == nil {
			sectionMap = make(map[string]any) // Empty section is valid.
		} else {
			// Path points to a non-map value, which is an error for Scan.
			return fmt.Errorf("path %q refers to non-map value (type %T)", path, sectionData)
		}
	}

	// Create decoder with comprehensive hooks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		TagName:          c.tagName,
		WeaklyTypedInput: true,
		DecodeHook:       c.getDecodeHook(),
		ZeroFields:       true,
		Metadata:         nil,
	})
	if err != nil {
		return fmt.Errorf("decoder creation failed: %w", err)
	}

	if err := decoder.Decode(sectionMap); err != nil {
		return fmt.Errorf("decode failed for path %q: %w", path, err)
	}

	return nil
}

// normalizeMap ensures that the input data is a map[string]any for the decoder.
func normalizeMap(data any) (map[string]any, error) {
	if data == nil {
		return make(map[string]any), nil
	}

	// If it's already the correct type, return it.
	if m, ok := data.(map[string]any); ok {
		return m, nil
	}

	// Use reflection to handle other map types (e.g., map[string]bool)
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Map {
		if v.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map keys must be strings, but got %v", v.Type().Key())
		}

		// Create a new map[string]any and copy the values.
		normalized := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			normalized[iter.Key().String()] = iter.Value().Interface()
		}
		return normalized, nil
	}

	return nil, fmt.Errorf("expected a map but got %T", data)
}

// getDecodeHook returns the composite decode hook for all type conversions
func (c *Config) getDecodeHook() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		// Network types
		stringToNetIPHookFunc(),
		stringToNetIPNetHookFunc(),
		stringToURLHookFunc(),

		// Standard hooks
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToTimeHookFunc(time.RFC3339),
		mapstructure.StringToSliceHookFunc(","),

		// Custom application hooks
		c.customDecodeHook(),
	)
}

// stringToNetIPHookFunc handles net.IP conversion
func stringToNetIPHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		if t != reflect.TypeOf(net.IP{}) {
			return data, nil
		}

		// SECURITY: Validate IP string format to prevent injection
		str := data.(string)
		if len(str) > 45 { // Max IPv6 length
			return nil, fmt.Errorf("invalid IP length: %d", len(str))
		}

		ip := net.ParseIP(str)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", str)
		}

		return ip, nil
	}
}

// stringToNetIPNetHookFunc handles net.IPNet conversion
func stringToNetIPNetHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		isPtr := t.Kind() == reflect.Ptr
		targetType := t
		if isPtr {
			targetType = t.Elem()
		}
		if targetType != reflect.TypeOf(net.IPNet{}) {
			return data, nil
		}

		str := data.(string)
		if len(str) > 49 { // Max IPv6 CIDR length
			return nil, fmt.Errorf("invalid CIDR length: %d", len(str))
		}
		_, ipnet, err := net.ParseCIDR(str)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %w", err)
		}
		if isPtr {
			return ipnet, nil
		}
		return *ipnet, nil
	}
}

// stringToURLHookFunc handles url.URL conversion
func stringToURLHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		isPtr := t.Kind() == reflect.Ptr
		targetType := t
		if isPtr {
			targetType = t.Elem()
		}
		if targetType != reflect.TypeOf(url.URL{}) {
			return data, nil
		}

		str := data.(string)
		if len(str) > 2048 {
			return nil, fmt.Errorf("URL too long: %d bytes", len(str))
		}
		u, err := url.Parse(str)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		if isPtr {
			return u, nil
		}
		return *u, nil
	}
}

// customDecodeHook allows for application-specific type conversions
func (c *Config) customDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		// SECURITY: Add custom validation for application types here
		// Example: Rate limit parsing, permission validation, etc.

		// Pass through by default
		return data, nil
	}
}

// navigateToPath traverses nested map to reach the specified path
func navigateToPath(nested map[string]any, path string) any {
	if path == "" {
		return nested
	}

	path = strings.TrimSuffix(path, ".")
	if path == "" {
		return nested
	}

	segments := strings.Split(path, ".")
	current := any(nested)

	for _, segment := range segments {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}

		value, exists := currentMap[segment]
		if !exists {
			return nil
		}
		current = value
	}

	return current
}