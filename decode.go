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
func (c *Config) unmarshal(basePath string, source Source, target any) error {
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
	sectionData := navigateToPath(nestedMap, basePath)

	// Ensure we have a map to decode
	sectionMap, ok := sectionData.(map[string]any)
	if !ok {
		if sectionData == nil {
			sectionMap = make(map[string]any) // Empty section
		} else {
			return fmt.Errorf("path %q refers to non-map value (type %T)", basePath, sectionData)
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
		return fmt.Errorf("decode failed for path %q: %w", basePath, err)
	}

	return nil
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
