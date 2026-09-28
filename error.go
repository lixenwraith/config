// FILE: lixenwraith/config/error.go
package config

import (
	"errors"
)

// Error categories - major error types for client code to check with errors.Is()
var (
	// ErrNotConfigured indicates an operation was attempted on a Config instance
	// that was not properly prepared for it
	ErrNotConfigured = errors.New("operation requires additional configuration")

	// ErrConfigNotFound indicates the specified configuration file was not found
	ErrConfigNotFound = errors.New("configuration file not found")

	// ErrCLIParse indicates that parsing command-line arguments failed
	ErrCLIParse = errors.New("failed to parse command-line arguments")

	// ErrEnvParse indicates that parsing environment variables failed
	ErrEnvParse = errors.New("failed to parse environment variables")

	// ErrValueSize indicates a value larger than MaxValueSize
	ErrValueSize = errors.New("value size exceeds maximum")

	// ErrPathNotFound indicates the configuration path doesn't exist.
	ErrPathNotFound = errors.New("configuration path not found")

	// ErrPathNotRegistered indicates trying to operate on unregistered path.
	ErrPathNotRegistered = errors.New("configuration path not registered")

	// ErrInvalidPath indicates malformed configuration path.
	ErrInvalidPath = errors.New("invalid configuration path")

	// ErrTypeMismatch indicates type conversion/assertion failure.
	ErrTypeMismatch = errors.New("type mismatch")

	// ErrValidation indicates configuration validation failure.
	ErrValidation = errors.New("configuration validation failed")

	// ErrFileFormat indicates unsupported or undetectable file format.
	ErrFileFormat = errors.New("unsupported file format")

	// ErrDecode indicates failure during value decoding.
	ErrDecode = errors.New("failed to decode value")

	// ErrFileAccess indicates file permission or access issues.
	ErrFileAccess = errors.New("file access denied")
)

// wrapError joins a base error type with a more specific error,
// allowing errors.Is() to work on both.
func wrapError(base error, detail error) error {
	return errors.Join(base, detail)
}
