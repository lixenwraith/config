// File: lixenwraith/config/type.go
package config

import (
	"fmt"
	"reflect"
	"strconv"
)

// String retrieves a string configuration value using the path.
// Attempts conversion from common types if the stored value isn't already a string.
func (c *Config) String(path string) (string, error) {
	val, found := c.Get(path)
	if !found {
		return "", fmt.Errorf("path not registered: %s", path)
	}
	if val == nil {
		return "", nil // Treat nil as empty string for convenience
	}

	if strVal, ok := val.(string); ok {
		return strVal, nil
	}

	// Attempt conversion for common types
	switch v := val.(type) {
	case fmt.Stringer:
		return v.String(), nil
	case []byte:
		return string(v), nil
	case int, int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(val).Int(), 10), nil
	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(val).Uint(), 10), nil
	case float32, float64:
		return strconv.FormatFloat(reflect.ValueOf(val).Float(), 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case error:
		return v.Error(), nil
	default:
		return "", fmt.Errorf("cannot convert type %T to string for path %s", val, path)
	}
}

// Int64 retrieves an int64 configuration value using the path.
// Attempts conversion from numeric types, parsable strings, and booleans.
func (c *Config) Int64(path string) (int64, error) {
	val, found := c.Get(path)
	if !found {
		return 0, fmt.Errorf("path not registered: %s", path)
	}
	if val == nil {
		return 0, fmt.Errorf("value for path %s is nil, cannot convert to int64", path)
	}

	// Use reflection for broader compatibility with numeric types
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := v.Uint()
		// Check for potential overflow converting uint64 to int64
		maxInt64 := int64(^uint64(0) >> 1)
		if u > uint64(maxInt64) {
			return 0, fmt.Errorf("cannot convert unsigned integer %d (type %T) to int64 for path %s: overflow", u, val, path)
		}
		return int64(u), nil
	case reflect.Float32, reflect.Float64:
		// Truncate float to int
		return int64(v.Float()), nil
	case reflect.String:
		s := v.String()
		if i, err := strconv.ParseInt(s, 0, 64); err == nil { // Use base 0 for auto-detection (e.g., "0xFF")
			return i, nil
		} else {
			if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
				return int64(f), nil // Truncate
			}
			// Return the original integer parsing error if float also fails
			return 0, fmt.Errorf("cannot convert string %q to int64 for path %s: %w", s, path, err)
		}
	case reflect.Bool:
		if v.Bool() {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("cannot convert type %T to int64 for path %s", val, path)
}

// Bool retrieves a boolean configuration value using the path.
// Attempts conversion from numeric types (0=false, non-zero=true) and parsable strings.
func (c *Config) Bool(path string) (bool, error) {
	val, found := c.Get(path)
	if !found {
		return false, fmt.Errorf("path not registered: %s", path)
	}
	if val == nil {
		return false, fmt.Errorf("value for path %s is nil, cannot convert to bool", path)
	}

	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.String:
		s := v.String()
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		} else {
			return false, fmt.Errorf("cannot convert string %q to bool for path %s: %w", s, path, err)
		}
	// Numeric interpretation: 0 is false, non-zero is true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() != 0, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() != 0, nil
	case reflect.Float32, reflect.Float64:
		return v.Float() != 0, nil
	}

	return false, fmt.Errorf("cannot convert type %T to bool for path %s", val, path)
}

// Float64 retrieves a float64 configuration value using the path.
// Attempts conversion from numeric types, parsable strings, and booleans.
func (c *Config) Float64(path string) (float64, error) {
	val, found := c.Get(path)
	if !found {
		return 0.0, fmt.Errorf("path not registered: %s", path)
	}
	if val == nil {
		return 0.0, fmt.Errorf("value for path %s is nil, cannot convert to float64", path)
	}

	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), nil
	case reflect.String:
		s := v.String()
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, nil
		} else {
			return 0.0, fmt.Errorf("cannot convert string %q to float64 for path %s: %w", s, path, err)
		}
	case reflect.Bool:
		if v.Bool() {
			return 1.0, nil
		}
		return 0.0, nil
	}

	return 0.0, fmt.Errorf("cannot convert type %T to float64 for path %s", val, path)
}