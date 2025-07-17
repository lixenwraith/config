// FILE: lixenwraith/config/discovery.go
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// FileDiscoveryOptions configures automatic config file discovery
type FileDiscoveryOptions struct {
	// Base name of config file (without extension)
	Name string

	// Extensions to try (in order)
	Extensions []string

	// Custom search paths (in addition to defaults)
	Paths []string

	// Environment variable to check for explicit path
	EnvVar string

	// CLI flag to check (e.g., "--config" or "-c")
	CLIFlag string

	// Whether to search in XDG config directories
	UseXDG bool

	// Whether to search in current directory
	UseCurrentDir bool
}

// DefaultDiscoveryOptions returns sensible defaults
func DefaultDiscoveryOptions(appName string) FileDiscoveryOptions {
	return FileDiscoveryOptions{
		Name:          appName,
		Extensions:    []string{".toml", ".conf", ".config"},
		EnvVar:        strings.ToUpper(appName) + "_CONFIG",
		CLIFlag:       "--config",
		UseXDG:        true,
		UseCurrentDir: true,
	}
}

// WithFileDiscovery enables automatic config file discovery
func (b *Builder) WithFileDiscovery(opts FileDiscoveryOptions) *Builder {
	// Check CLI args first (highest priority)
	if opts.CLIFlag != "" && len(b.args) > 0 {
		for i, arg := range b.args {
			if arg == opts.CLIFlag && i+1 < len(b.args) {
				b.file = b.args[i+1]
				return b
			}
			if strings.HasPrefix(arg, opts.CLIFlag+"=") {
				b.file = strings.TrimPrefix(arg, opts.CLIFlag+"=")
				return b
			}
		}
	}

	// Check environment variable
	if opts.EnvVar != "" {
		if path := os.Getenv(opts.EnvVar); path != "" {
			b.file = path
			return b
		}
	}

	// Build search paths
	var searchPaths []string

	// Custom paths first
	searchPaths = append(searchPaths, opts.Paths...)

	// Current directory
	if opts.UseCurrentDir {
		if cwd, err := os.Getwd(); err == nil {
			searchPaths = append(searchPaths, cwd)
		}
	}

	// XDG paths
	if opts.UseXDG {
		searchPaths = append(searchPaths, getXDGConfigPaths(opts.Name)...)
	}

	// Search for config file
	for _, dir := range searchPaths {
		for _, ext := range opts.Extensions {
			path := filepath.Join(dir, opts.Name+ext)
			if _, err := os.Stat(path); err == nil {
				b.file = path
				return b
			}
		}
	}

	// No file found is not an error - app can run with defaults/env
	return b
}

// getXDGConfigPaths returns XDG-compliant config search paths
func getXDGConfigPaths(appName string) []string {
	var paths []string

	// XDG_CONFIG_HOME
	if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" {
		paths = append(paths, filepath.Join(xdgHome, appName))
	} else if home := os.Getenv("HOME"); home != "" {
		paths = append(paths, filepath.Join(home, ".config", appName))
	}

	// XDG_CONFIG_DIRS
	if xdgDirs := os.Getenv("XDG_CONFIG_DIRS"); xdgDirs != "" {
		for _, dir := range filepath.SplitList(xdgDirs) {
			paths = append(paths, filepath.Join(dir, appName))
		}
	} else {
		// Default system paths
		paths = append(paths,
			filepath.Join("/etc/xdg", appName),
			filepath.Join("/etc", appName),
		)
	}

	return paths
}