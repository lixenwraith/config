// FILE: lixenwraith/config/validator.go
package config

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Common validators for configuration values

// Port validates TCP/UDP port range
func Port(p int64) error {
	if p < MinPortNumber || p > MaxPortNumber {
		return wrapError(ErrValidation, fmt.Errorf("must be 1-65535, got %d", p))
	}
	return nil
}

// Positive validates positive numbers
func Positive[T int64 | float64](n T) error {
	if n <= 0 {
		return wrapError(ErrValidation, fmt.Errorf("must be positive, got %v", n))
	}
	return nil
}

// NonNegative validates non-negative numbers
func NonNegative[T int64 | float64](n T) error {
	if n < 0 {
		return wrapError(ErrValidation, fmt.Errorf("must be non-negative, got %v", n))
	}
	return nil
}

// IPAddress validates IP address format
func IPAddress(s string) error {
	if s == "" || s == IPv4Any || s == IPv6Any {
		return nil // Allow common defaults
	}
	if net.ParseIP(s) == nil {
		return wrapError(ErrValidation, fmt.Errorf("invalid IP address: %s", s))
	}
	return nil
}

// IPv4Address validates IPv4 address format
func IPv4Address(s string) error {
	if s == "" || s == IPv4Any {
		return nil // Allow common defaults
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return wrapError(ErrValidation, fmt.Errorf("invalid IPv4 address: %s", s))
	}
	return nil
}

// IPv6Address validates IPv6 address format
func IPv6Address(s string) error {
	if s == "" || s == IPv6Any {
		return nil // Allow common defaults
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return wrapError(ErrValidation, fmt.Errorf("invalid IPv6 address: %s", s))
	}
	// Valid net.ParseIP with nil ip.To4 indicates IPv6
	if ip.To4() != nil {
		return wrapError(ErrValidation, fmt.Errorf("invalid IPv6 address (is an IPv4 address): %s", s))
	}
	return nil
}

// URLPath validates URL path format
func URLPath(s string) error {
	if s != "" && !strings.HasPrefix(s, "/") {
		return wrapError(ErrValidation, fmt.Errorf("must start with /: %s", s))
	}
	return nil
}

// OneOf creates a validator for allowed values
func OneOf[T comparable](allowed ...T) func(T) error {
	return func(val T) error {
		for _, a := range allowed {
			if val == a {
				return nil
			}
		}
		return wrapError(ErrValidation, fmt.Errorf("must be one of %v, got %v", allowed, val))
	}
}

// Range creates a min/max validator
func Range[T int64 | float64](min, max T) func(T) error {
	return func(val T) error {
		if val < min || val > max {
			return wrapError(ErrValidation, fmt.Errorf("must be %v-%v, got %v", min, max, val))
		}
		return nil
	}
}

// Pattern creates a regex validator
func Pattern(pattern string) func(string) error {
	re := regexp.MustCompile(pattern)
	return func(s string) error {
		if !re.MatchString(s) {
			return wrapError(ErrValidation, fmt.Errorf("must match pattern %s", pattern))
		}
		return nil
	}
}

// NonEmpty validates non-empty strings
func NonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return wrapError(ErrValidation, fmt.Errorf("must not be empty"))
	}
	return nil
}