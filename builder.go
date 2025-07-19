// FILE: lixenwraith/config/builder.go
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

// Builder provides a fluent API for constructing a Config instance. It allows for
// chaining configuration options before final build of the config object.
type Builder struct {
	cfg        *Config
	opts       LoadOptions
	defaults   any
	tagName    string
	prefix     string
	file       string
	args       []string
	err        error
	validators []ValidatorFunc
}

// ValidatorFunc defines the signature for a function that can validate a Config instance.
// It receives the fully loaded *Config object and should return an error if validation fails.
type ValidatorFunc func(c *Config) error

// NewBuilder creates a new configuration builder
func NewBuilder() *Builder {
	return &Builder{
		cfg:        New(),
		opts:       DefaultLoadOptions(),
		args:       os.Args[1:],
		validators: make([]ValidatorFunc, 0),
	}
}

// Build creates the Config instance with all specified options
func (b *Builder) Build() (*Config, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Use tagName if set, default to "toml"
	tagName := b.tagName
	if tagName == "" {
		tagName = "toml"
	}

	// Register defaults if provided
	if b.defaults != nil {
		if err := b.cfg.RegisterStructWithTags(b.prefix, b.defaults, tagName); err != nil {
			return nil, fmt.Errorf("failed to register defaults: %w", err)
		}
	}

	// Explicitly set the file path on the config object so the watcher can find it,
	// even if the initial load fails with a non-fatal error (e.g., file not found).
	b.cfg.configFilePath = b.file

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
		// Ignore ErrConfigNotFound for app to proceed with defaults/env vars
		if !errors.Is(err, ErrConfigNotFound) {
			panic(fmt.Sprintf("config build failed: %v", err))
		}
	}
	return cfg
}

// WithDefaults sets the struct containing default values
func (b *Builder) WithDefaults(defaults any) *Builder {
	b.defaults = defaults
	return b
}

// WithTagName sets the struct tag name to use for field mapping
// Supported values: "toml" (default), "json", "yaml"
func (b *Builder) WithTagName(tagName string) *Builder {
	switch tagName {
	case "toml", "json", "yaml":
		b.tagName = tagName
		if b.cfg != nil { // Ensure cfg exists
			b.cfg.tagName = tagName
		}
	default:
		b.err = fmt.Errorf("unsupported tag name %q, must be one of: toml, json, yaml", tagName)
	}
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

// WithTarget enables type-aware mode for the builder
func (b *Builder) WithTarget(target any) *Builder {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		b.err = fmt.Errorf("WithTarget requires non-nil pointer to struct, got %T", target)
		return b
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		b.err = fmt.Errorf("WithTarget requires pointer to struct, got pointer to %v", elem.Kind())
		return b
	}

	// Initialize struct cache
	if b.cfg.structCache == nil {
		b.cfg.structCache = &structCache{
			target:     target,
			targetType: elem.Type(),
		}
	}

	// Register struct fields automatically
	if b.defaults == nil {
		b.defaults = target
	}

	return b
}

// WithValidator adds a validation function that runs at the end of the build process
// Multiple validators can be added and are executed in the order they are added
// Validation runs after all sources are loaded
// If any validator returns error, build fails without running subsequent validators
func (b *Builder) WithValidator(fn ValidatorFunc) *Builder {
	if fn != nil {
		b.validators = append(b.validators, fn)
	}
	return b
}