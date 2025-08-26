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
	cfg             *Config
	opts            LoadOptions
	defaults        any
	tagName         string
	fileFormat      string
	securityOpts    *SecurityOptions
	prefix          string
	file            string
	args            []string
	err             error
	validators      []ValidatorFunc
	typedValidators []any
}

// ValidatorFunc defines the signature for a function that can validate a Config instance.
// It receives the fully loaded *Config object and should return an error if validation fails.
type ValidatorFunc func(c *Config) error

// NewBuilder creates a new configuration builder
func NewBuilder() *Builder {
	return &Builder{
		cfg:             New(),
		opts:            DefaultLoadOptions(),
		args:            os.Args[1:],
		validators:      make([]ValidatorFunc, 0),
		typedValidators: make([]any, 0),
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

	// Set format and security settings
	if b.fileFormat != "" {
		b.cfg.fileFormat = b.fileFormat
	}
	if b.securityOpts != nil {
		b.cfg.securityOpts = b.securityOpts
	}

	// 1. Register defaults
	// If WithDefaults() was called, it takes precedence.
	// If not, but WithTarget() was called, use the target struct for defaults.
	if b.defaults != nil {
		// WithDefaults() was called explicitly.
		if err := b.cfg.RegisterStructWithTags(b.prefix, b.defaults, tagName); err != nil {
			return nil, fmt.Errorf("failed to register defaults: %w", err)
		}
	} else if b.cfg.structCache != nil && b.cfg.structCache.target != nil {
		// No explicit defaults, so use the target struct as the source of defaults.
		// This is the behavior the tests rely on.
		if err := b.cfg.RegisterStructWithTags(b.prefix, b.cfg.structCache.target, tagName); err != nil {
			return nil, fmt.Errorf("failed to register target struct as defaults: %w", err)
		}
	}

	// Explicitly set the file path on the config object so the watcher can find it,
	// even if the initial load fails with a non-fatal error (file not found).
	b.cfg.configFilePath = b.file

	// 2. Load configuration
	loadErr := b.cfg.LoadWithOptions(b.file, b.args, b.opts)
	if loadErr != nil && !errors.Is(loadErr, ErrConfigNotFound) {
		// Return on fatal load errors. ErrConfigNotFound is not fatal.
		return nil, loadErr
	}

	// 3. Run non-typed validators
	for _, validator := range b.validators {
		if err := validator(b.cfg); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	// 4. Populate target and run typed validators
	if b.cfg.structCache != nil && b.cfg.structCache.target != nil && len(b.typedValidators) > 0 {
		// Populate the target struct first. This unifies all types (e.g., string "8888" -> int64 8888).
		populatedTarget, err := b.cfg.AsStruct()
		if err != nil {
			return nil, fmt.Errorf("failed to populate target struct for validation: %w", err)
		}

		// Run the typed validators against the populated, type-safe struct.
		for _, validator := range b.typedValidators {
			validatorFunc := reflect.ValueOf(validator)
			validatorType := validatorFunc.Type()

			// Check if the validator's input type matches the target's type.
			if validatorType.In(0) != reflect.TypeOf(populatedTarget) {
				return nil, fmt.Errorf("typed validator signature %v does not match target type %T", validatorType, populatedTarget)
			}

			// Call the validator.
			results := validatorFunc.Call([]reflect.Value{reflect.ValueOf(populatedTarget)})
			if !results[0].IsNil() {
				err := results[0].Interface().(error)
				return nil, fmt.Errorf("typed configuration validation failed: %w", err)
			}
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

// WithFileFormat sets the expected file format
func (b *Builder) WithFileFormat(format string) *Builder {
	switch format {
	case "toml", "json", "yaml", "auto":
		b.fileFormat = format
	default:
		b.err = fmt.Errorf("unsupported file format %q", format)
	}
	return b
}

// WithSecurityOptions sets security options for file loading
func (b *Builder) WithSecurityOptions(opts SecurityOptions) *Builder {
	b.securityOpts = &opts
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

// WithTypedValidator adds a type-safe validation function that runs at the end of the build process,
// after the target struct has been populated. The provided function must accept a single argument
// that is a pointer to the same type as the one provided to WithTarget, and must return an error.
func (b *Builder) WithTypedValidator(fn any) *Builder {
	if fn == nil {
		return b
	}

	// Basic reflection check to ensure it's a function that takes one argument and returns an error.
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func || t.NumIn() != 1 || t.NumOut() != 1 || t.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		b.err = fmt.Errorf("WithTypedValidator requires a function with signature func(*T) error")
		return b
	}

	b.typedValidators = append(b.typedValidators, fn)
	return b
}