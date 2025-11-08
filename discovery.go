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
		Extensions:    DefaultConfigExtensions,
		EnvVar:        strings.ToUpper(appName) + "_CONFIG",
		CLIFlag:       "--config",
		UseXDG:        true,
		UseCurrentDir: true,
	}
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
		for _, dir := range XDGSystemPaths {
			paths = append(paths, filepath.Join(dir, appName))
		}
	}

	return paths
}