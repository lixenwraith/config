// FILE: example/watch_demo.go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lixenwraith/config"
)

// AppConfig represents our application configuration
type AppConfig struct {
	Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	} `toml:"server"`

	Database struct {
		URL         string        `toml:"url"`
		MaxConns    int           `toml:"max_conns"`
		IdleTimeout time.Duration `toml:"idle_timeout"`
	} `toml:"database"`

	Features struct {
		RateLimit bool `toml:"rate_limit"`
		Caching   bool `toml:"caching"`
	} `toml:"features"`
}

func main() {
	// Create configuration with defaults
	defaults := &AppConfig{}
	defaults.Server.Host = "localhost"
	defaults.Server.Port = 8080
	defaults.Database.MaxConns = 10
	defaults.Database.IdleTimeout = 30 * time.Second

	// Build configuration
	cfg, err := config.NewBuilder().
		WithDefaults(defaults).
		WithEnvPrefix("MYAPP_").
		WithFile("config.toml").
		Build()
	if err != nil && !errors.Is(err, config.ErrConfigNotFound) {
		log.Fatal("Failed to load config:", err)
	}

	// Enable auto-update with custom options
	watchOpts := config.WatchOptions{
		PollInterval:      500 * time.Millisecond, // Check twice per second
		Debounce:          200 * time.Millisecond, // Quick response
		MaxWatchers:       10,
		ReloadTimeout:     2 * time.Second,
		VerifyPermissions: true, // SECURITY: Detect permission changes
	}
	cfg.AutoUpdateWithOptions(watchOpts)
	defer cfg.StopAutoUpdate()

	// Start watching for changes
	changes := cfg.Watch()

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Log initial configuration
	logConfig(cfg)

	// Watch for changes
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case path := <-changes:
				handleConfigChange(cfg, path)
			}
		}
	}()

	// Main loop
	log.Println("Watching for configuration changes. Edit config.toml to see updates.")
	log.Println("Press Ctrl+C to exit.")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			log.Println("Shutting down...")
			return

		case <-ticker.C:
			// Periodic health check
			var port int64
			port, _ = cfg.Get("server.port")
			log.Printf("Server still running on port %d", port)
		}
	}
}

func handleConfigChange(cfg *config.Config, path string) {
	switch path {
	case "__file_deleted__":
		log.Println("⚠️  Config file was deleted!")
	case "__permissions_changed__":
		log.Println("⚠️  SECURITY: Config file permissions changed!")
	case "__reload_error__":
		log.Printf("❌ Failed to reload config: %s", path)
	case "__reload_timeout__":
		log.Println("⚠️  Config reload timed out")
	default:
		// Normal configuration change
		value, _ := cfg.Get(path)
		log.Printf("📝 Config changed: %s = %v", path, value)

		// Handle specific changes
		switch path {
		case "server.port":
			log.Println("Port changed - server restart required")
		case "database.url":
			log.Println("Database URL changed - reconnection required")
		case "features.rate_limit":
			if cfg.Bool("features.rate_limit") {
				log.Println("Rate limiting enabled")
			} else {
				log.Println("Rate limiting disabled")
			}
		}
	}
}

func logConfig(cfg *config.Config) {
	log.Println("Current configuration:")
	log.Printf("  Server: %s:%d", cfg.String("server.host"), cfg.Int("server.port"))
	log.Printf("  Database: %s (max_conns=%d)",
		cfg.String("database.url"),
		cfg.Int("database.max_conns"))
	log.Printf("  Features: rate_limit=%v, caching=%v",
		cfg.Bool("features.rate_limit"),
		cfg.Bool("features.caching"))
}

// Example config.toml file:
/*
[server]
host = "localhost"
port = 8080

[database]
url = "postgres://localhost/myapp"
max_conns = 25
idle_timeout = "30s"

[features]
rate_limit = true
caching = false
*/