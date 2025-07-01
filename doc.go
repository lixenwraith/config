// File: lixenwraith/config/doc.go

// Package config provides thread-safe configuration management for Go applications
// with support for multiple sources: TOML files, environment variables, command-line
// arguments, and default values with configurable precedence.
//
// Features:
//   - Multiple configuration sources with customizable precedence
//   - Thread-safe operations using sync.RWMutex
//   - Automatic type conversions for common Go types
//   - Struct registration with tag support
//   - Environment variable auto-discovery and mapping
//   - Builder pattern for easy initialization
//   - Source tracking to see where values originated
//   - Configuration validation
//   - Zero dependencies (only stdlib + toml parser + mapstructure)
//
// Quick Start:
//
//	type Config struct {
//	    Server struct {
//	        Host string `toml:"host"`
//	        Port int    `toml:"port"`
//	    } `toml:"server"`
//	}
//
//	defaults := Config{}
//	defaults.Server.Host = "localhost"
//	defaults.Server.Port = 8080
//
//	cfg, err := config.Quick(defaults, "MYAPP_", "config.toml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	host, _ := cfg.String("server.host")
//	port, _ := cfg.Int64("server.port")
//
// Default Precedence (highest to lowest):
//  1. Command-line arguments (--server.port=9090)
//  2. Environment variables (MYAPP_SERVER_PORT=9090)
//  3. Configuration file (config.toml)
//  4. Default values
//
// Custom Precedence:
//
//	cfg, err := config.NewBuilder().
//	    WithDefaults(defaults).
//	    WithSources(
//	        config.SourceEnv,     // Environment the highest priority
//	        config.SourceCLI,
//	        config.SourceFile,
//	        config.SourceDefault,
//	    ).
//	    Build()
//
// Thread Safety:
// All operations are thread-safe. The package uses read-write mutexes to allow
// concurrent reads while protecting writes.
package config