// Test program for the config package
package main

import (
	"fmt"
	"github.com/LixenWraith/config"
	"os"
	"path/filepath"
)

// LogConfig represents logging configuration parameters
type LogConfig struct {
	// Basic settings
	Level     int64  `toml:"level"`
	Name      string `toml:"name"`
	Directory string `toml:"directory"`
	Format    string `toml:"format"` // "txt" or "json"
	Extension string `toml:"extension"`
	// Formatting
	ShowTimestamp bool `toml:"show_timestamp"`
	ShowLevel     bool `toml:"show_level"`
	// Buffer and size limits
	BufferSize     int64 `toml:"buffer_size"`       // Channel buffer size
	MaxSizeMB      int64 `toml:"max_size_mb"`       // Max size per log file
	MaxTotalSizeMB int64 `toml:"max_total_size_mb"` // Max total size of all logs in dir
	MinDiskFreeMB  int64 `toml:"min_disk_free_mb"`  // Minimum free disk space required
	// Timers
	FlushIntervalMs    int64   `toml:"flush_interval_ms"`    // Interval for flushing file buffer
	TraceDepth         int64   `toml:"trace_depth"`          // Default trace depth (0-10)
	RetentionPeriodHrs float64 `toml:"retention_period_hrs"` // Hours to keep logs (0=disabled)
	RetentionCheckMins float64 `toml:"retention_check_mins"` // How often to check retention
	// Disk check settings
	DiskCheckIntervalMs    int64 `toml:"disk_check_interval_ms"`   // Base interval for disk checks
	EnableAdaptiveInterval bool  `toml:"enable_adaptive_interval"` // Adjust interval based on log rate
	MinCheckIntervalMs     int64 `toml:"min_check_interval_ms"`    // Minimum adaptive interval
	MaxCheckIntervalMs     int64 `toml:"max_check_interval_ms"`    // Maximum adaptive interval
}

func main() {
	// Create a temporary file path for our test
	tempDir := os.TempDir()
	configPath := filepath.Join(tempDir, "logconfig_test.toml")

	// Clean up any existing file from previous runs
	os.Remove(configPath)

	fmt.Println("=== LogConfig Test Program ===")
	fmt.Printf("Using temporary config file: %s\n\n", configPath)

	// Initialize the Config instance
	cfg := config.New()

	// Register default values for all LogConfig fields
	registerLogConfigDefaults(cfg)

	// Load the configuration (will use defaults since file doesn't exist yet)
	exists, err := cfg.Load(configPath, nil)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config file exists: %v (expected: false)\n", exists)

	// Unmarshal into LogConfig struct
	var logConfig LogConfig
	err = cfg.UnmarshalSubtree("log", &logConfig)
	if err != nil {
		fmt.Printf("Error unmarshaling config: %v\n", err)
		os.Exit(1)
	}

	// Print current values
	fmt.Println("\n=== Default Configuration Values ===")
	printLogConfig(logConfig)

	// Modify some values
	fmt.Println("\n=== Modifying Configuration Values ===")
	fmt.Println("Changing:")
	fmt.Println("  - level: 1 → 2")
	fmt.Println("  - name: default_logger → modified_logger")
	fmt.Println("  - format: txt → json")
	fmt.Println("  - max_size_mb: 10 → 50")
	fmt.Println("  - retention_period_hrs: 24.0 → 72.0")
	fmt.Println("  - enable_adaptive_interval: false → true")

	cfg.Set("log.level", int64(2))
	cfg.Set("log.name", "modified_logger")
	cfg.Set("log.format", "json")
	cfg.Set("log.max_size_mb", int64(50))
	cfg.Set("log.retention_period_hrs", 72.0)
	cfg.Set("log.enable_adaptive_interval", true)

	// Save the configuration
	err = cfg.Save(configPath)
	if err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nSaved configuration to: %s\n", configPath)

	// Read the file to verify it contains the expected values
	fileBytes, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Generated TOML File Contents ===")
	fmt.Println(string(fileBytes))

	// Load the config again to verify it can be read back correctly
	exists, err = cfg.Load(configPath, nil)
	if err != nil {
		fmt.Printf("Error reloading config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nConfig file exists: %v (expected: true)\n", exists)

	// Unmarshal into a new LogConfig to verify loaded values
	var loadedConfig LogConfig
	err = cfg.UnmarshalSubtree("log", &loadedConfig)
	if err != nil {
		fmt.Printf("Error unmarshaling reloaded config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Loaded Configuration Values ===")
	printLogConfig(loadedConfig)

	// Verify specific values were changed correctly
	fmt.Println("\n=== Verification ===")
	verifyConfig(loadedConfig)

	// Clean up
	os.Remove(configPath)
	fmt.Println("\nCleanup: Temporary file removed.")
	fmt.Println("\n=== Test Complete ===")
}

// registerLogConfigDefaults registers all default values for the LogConfig struct
func registerLogConfigDefaults(cfg *config.Config) {
	fmt.Println("Registering default values...")

	// Basic settings
	cfg.Register("log.level", int64(1))
	cfg.Register("log.name", "default_logger")
	cfg.Register("log.directory", "./logs")
	cfg.Register("log.format", "txt")
	cfg.Register("log.extension", ".log")

	// Formatting
	cfg.Register("log.show_timestamp", true)
	cfg.Register("log.show_level", true)

	// Buffer and size limits
	cfg.Register("log.buffer_size", int64(1000))
	cfg.Register("log.max_size_mb", int64(10))
	cfg.Register("log.max_total_size_mb", int64(100))
	cfg.Register("log.min_disk_free_mb", int64(500))

	// Timers
	cfg.Register("log.flush_interval_ms", int64(1000))
	cfg.Register("log.trace_depth", int64(3))
	cfg.Register("log.retention_period_hrs", 24.0)
	cfg.Register("log.retention_check_mins", 15.0)

	// Disk check settings
	cfg.Register("log.disk_check_interval_ms", int64(60000))
	cfg.Register("log.enable_adaptive_interval", false)
	cfg.Register("log.min_check_interval_ms", int64(5000))
	cfg.Register("log.max_check_interval_ms", int64(300000))
}

// printLogConfig prints the values of a LogConfig struct
func printLogConfig(cfg LogConfig) {
	fmt.Println("Basic settings:")
	fmt.Printf("  - Level: %d\n", cfg.Level)
	fmt.Printf("  - Name: %s\n", cfg.Name)
	fmt.Printf("  - Directory: %s\n", cfg.Directory)
	fmt.Printf("  - Format: %s\n", cfg.Format)
	fmt.Printf("  - Extension: %s\n", cfg.Extension)

	fmt.Println("Formatting:")
	fmt.Printf("  - ShowTimestamp: %t\n", cfg.ShowTimestamp)
	fmt.Printf("  - ShowLevel: %t\n", cfg.ShowLevel)

	fmt.Println("Buffer and size limits:")
	fmt.Printf("  - BufferSize: %d\n", cfg.BufferSize)
	fmt.Printf("  - MaxSizeMB: %d\n", cfg.MaxSizeMB)
	fmt.Printf("  - MaxTotalSizeMB: %d\n", cfg.MaxTotalSizeMB)
	fmt.Printf("  - MinDiskFreeMB: %d\n", cfg.MinDiskFreeMB)

	fmt.Println("Timers:")
	fmt.Printf("  - FlushIntervalMs: %d\n", cfg.FlushIntervalMs)
	fmt.Printf("  - TraceDepth: %d\n", cfg.TraceDepth)
	fmt.Printf("  - RetentionPeriodHrs: %.1f\n", cfg.RetentionPeriodHrs)
	fmt.Printf("  - RetentionCheckMins: %.1f\n", cfg.RetentionCheckMins)

	fmt.Println("Disk check settings:")
	fmt.Printf("  - DiskCheckIntervalMs: %d\n", cfg.DiskCheckIntervalMs)
	fmt.Printf("  - EnableAdaptiveInterval: %t\n", cfg.EnableAdaptiveInterval)
	fmt.Printf("  - MinCheckIntervalMs: %d\n", cfg.MinCheckIntervalMs)
	fmt.Printf("  - MaxCheckIntervalMs: %d\n", cfg.MaxCheckIntervalMs)
}

// verifyConfig checks if the modified values were set correctly
func verifyConfig(cfg LogConfig) {
	allCorrect := true

	// Check each modified value
	if cfg.Level != 2 {
		fmt.Printf("ERROR: Level is %d, expected 2\n", cfg.Level)
		allCorrect = false
	}

	if cfg.Name != "modified_logger" {
		fmt.Printf("ERROR: Name is %s, expected 'modified_logger'\n", cfg.Name)
		allCorrect = false
	}

	if cfg.Format != "json" {
		fmt.Printf("ERROR: Format is %s, expected 'json'\n", cfg.Format)
		allCorrect = false
	}

	if cfg.MaxSizeMB != 50 {
		fmt.Printf("ERROR: MaxSizeMB is %d, expected 50\n", cfg.MaxSizeMB)
		allCorrect = false
	}

	if cfg.RetentionPeriodHrs != 72.0 {
		fmt.Printf("ERROR: RetentionPeriodHrs is %.1f, expected 72.0\n", cfg.RetentionPeriodHrs)
		allCorrect = false
	}

	if !cfg.EnableAdaptiveInterval {
		fmt.Printf("ERROR: EnableAdaptiveInterval is %t, expected true\n", cfg.EnableAdaptiveInterval)
		allCorrect = false
	}

	// Check that unmodified values retained their defaults
	if cfg.Directory != "./logs" {
		fmt.Printf("ERROR: Directory changed to %s, expected './logs'\n", cfg.Directory)
		allCorrect = false
	}

	if cfg.BufferSize != 1000 {
		fmt.Printf("ERROR: BufferSize changed to %d, expected 1000\n", cfg.BufferSize)
		allCorrect = false
	}

	if allCorrect {
		fmt.Println("SUCCESS: All configuration values match expected values!")
	} else {
		fmt.Println("FAILURE: Some configuration values don't match expected values!")
	}
}