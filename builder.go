// File: lixenwraith/config/builder.go
package config

import (
	"fmt"
	"os"
)

// Builder provides a fluent interface for building configurations
type Builder struct {
	cfg      *Config
	opts     LoadOptions
	defaults interface{}
	prefix   string
	file     string
	args     []string
	err      error
}

// NewBuilder creates a new configuration builder
func NewBuilder() *Builder {
	return &Builder{
		cfg:  New(),
		opts: DefaultLoadOptions(),
		args: os.Args[1:],
	}
}

// WithDefaults sets the struct containing default values
func (b *Builder) WithDefaults(defaults interface{}) *Builder {
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

// WithSources sets the precedence order for configuration sources
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
	if err := b.cfg.LoadWithOptions(b.file, b.args, b.opts); err != nil {
		return nil, err
	}

	return b.cfg, nil
}

// MustBuild is like Build but panics on error
func (b *Builder) MustBuild() *Config {
	cfg, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("config build failed: %v", err))
	}
	return cfg
}