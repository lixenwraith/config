// File: lixenwraith/config/builder.go
package config

import (
	"errors"
	"fmt"
	"os"
)

// ValidatorFunc defines the signature for a function that can validate a Config instance.
// It receives the fully loaded *Config object and should return an error if validation fails.
type ValidatorFunc func(c *Config) error

// Builder provides a fluent interface for building configurations
type Builder struct {
	cfg        *Config
	opts       LoadOptions
	defaults   any
	prefix     string
	file       string
	args       []string
	err        error
	validators []ValidatorFunc
}

// NewBuilder creates a new configuration builder
func NewBuilder() *Builder {
	return &Builder{
		cfg:        New(),
		opts:       DefaultLoadOptions(),
		args:       os.Args[1:],
		validators: make([]ValidatorFunc, 0),
	}
}

// WithDefaults sets the struct containing default values
func (b *Builder) WithDefaults(defaults any) *Builder {
	b.defaults = defaults
	return b
}

// WithPrefix sets the prefix for struct registration
func (b *Builder) WithPrefix(prefix string) *Builder {
	b.prefix = prefix
	return b
}

// WithEnvPrefix sets the environment variable prefix
func (b *Builder) WithEnvPrefix(prefix string) *Builder {
	b.opts.EnvPrefix = prefix
	return b
}

// WithFile sets the configuration file path
func (b *Builder) WithFile(path string) *Builder {
	b.file = path
	return b
}

// WithArgs sets the command-line arguments
func (b *Builder) WithArgs(args []string) *Builder {
	b.args = args
	return b
}

// WithSources sets the precedent order for configuration sources
func (b *Builder) WithSources(sources ...Source) *Builder {
	b.opts.Sources = sources
	return b
}

// WithEnvTransform sets a custom environment variable transformer
func (b *Builder) WithEnvTransform(fn EnvTransformFunc) *Builder {
	b.opts.EnvTransform = fn
	return b
}

// WithEnvWhitelist limits which paths are checked for env vars
func (b *Builder) WithEnvWhitelist(paths ...string) *Builder {
	if b.opts.EnvWhitelist == nil {
		b.opts.EnvWhitelist = make(map[string]bool)
	}
	for _, path := range paths {
		b.opts.EnvWhitelist[path] = true
	}
	return b
}

// WithValidator adds a validation function that runs at the end of the build process
// Multiple validators can be added and are executed in the order they are added
func (b *Builder) WithValidator(fn ValidatorFunc) *Builder {
	if fn != nil {
		b.validators = append(b.validators, fn)
	}
	return b
}

// Build creates the Config instance with all specified options
func (b *Builder) Build() (*Config, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Register defaults if provided
	if b.defaults != nil {
		if err := b.cfg.RegisterStruct(b.prefix, b.defaults); err != nil {
			return nil, fmt.Errorf("failed to register defaults: %w", err)
		}
	}

	// Load configuration
	loadErr := b.cfg.LoadWithOptions(b.file, b.args, b.opts)
	if loadErr != nil && !errors.Is(loadErr, ErrConfigNotFound) {
		// Return on fatal load errors. ErrConfigNotFound is not fatal.
		return nil, loadErr
	}

	// Run validators
	for _, validator := range b.validators {
		if err := validator(b.cfg); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	// ErrConfigNotFound or nil
	return b.cfg, loadErr
}

// MustBuild is like Build but panics on error
func (b *Builder) MustBuild() *Config {
	cfg, err := b.Build()
	if err != nil {
		// Ignore ErrConfigNotFound as it is not a fatal error for MustBuild.
		// The application can proceed with defaults/env vars.
		if !errors.Is(err, ErrConfigNotFound) {
			panic(fmt.Sprintf("config build failed: %v", err))
		}
	}
	return cfg
}

// BuildAndScan builds and unmarshals the final configuration into the provided target struct pointer
func (b *Builder) BuildAndScan(target any) error {
	cfg, err := b.Build()
	if err != nil && !errors.Is(err, ErrConfigNotFound) {
		return err
	}

	// Use Scan to populate the target struct.
	// The prefix used during registration is the base path for scanning.
	if err := cfg.Scan(b.prefix, target); err != nil {
		return fmt.Errorf("failed to scan final config into target: %w", err)
	}

	// ErrConfigNotFound or nil
	return err
}