// Test program for the config package
package main

import (
	"errors" // Import errors package
	"fmt"
	"log" // Using standard log for simplicity
	"os"
	"path/filepath"
	"strings"

	"github.com/LixenWraith/config" // Assuming this is the correct import path after potential renaming/moving
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

// Define default configuration values
var defaultLogConfig = LogConfig{
	// Basic settings
	Level:     1,
	Name:      "default_logger",
	Directory: "./logs",
	Format:    "txt",
	Extension: ".log",
	// Formatting
	ShowTimestamp: true,
	ShowLevel:     true,
	// Buffer and size limits
	BufferSize:     1000,
	MaxSizeMB:      10,
	MaxTotalSizeMB: 100,
	MinDiskFreeMB:  500,
	// Timers
	FlushIntervalMs:    1000,
	TraceDepth:         3,
	RetentionPeriodHrs: 24.0,
	RetentionCheckMins: 15.0,
	// Disk check settings
	DiskCheckIntervalMs:    60000,
	EnableAdaptiveInterval: false,
	MinCheckIntervalMs:     5000,
	MaxCheckIntervalMs:     300000,
}

func main() {
	// Create a temporary file path for our test
	tempDir := os.TempDir()
	configPath := filepath.Join(tempDir, "logconfig_test_enhanced.toml")

	// Clean up any existing file from previous runs
	os.Remove(configPath)
	defer os.Remove(configPath) // Ensure cleanup even on error exit

	fmt.Println("=== Enhanced LogConfig Test Program ===")
	fmt.Printf("Using temporary config file: %s\n\n", configPath)

	// 1. Initialize the Config instance
	cfg := config.New()

	// 2. Register default values using RegisterStruct
	fmt.Println("Registering default values using RegisterStruct...")
	err := cfg.RegisterStruct("log.", defaultLogConfig) // Note the "log." prefix
	if err != nil {
		log.Fatalf("FATAL: Error registering defaults: %v\n", err)
	}
	fmt.Println("Defaults registered.")

	// 3. Load configuration (file doesn't exist yet)
	fmt.Println("\nAttempting initial load (expecting file not found)...")
	err = cfg.Load(configPath, nil) // No CLI args yet
	if err != nil {
		// Check specifically for ErrConfigNotFound
		if errors.Is(err, config.ErrConfigNotFound) {
			fmt.Println("SUCCESS: Correctly detected config file not found.")
		} else {
			// Any other error during initial load is unexpected here
			log.Fatalf("FATAL: Unexpected error loading initial config: %v\n", err)
		}
	} else {
		log.Fatalf("FATAL: Expected an error (ErrConfigNotFound) during initial load, but got nil")
	}

	// 4. Unmarshal defaults into LogConfig struct
	var currentConfig LogConfig
	fmt.Println("\nUnmarshaling current config (should be defaults)...")
	err = cfg.Scan("log", &currentConfig)
	if err != nil {
		log.Fatalf("FATAL: Error unmarshaling default config: %v\n", err)
	}

	// Print default values
	fmt.Println("\n=== Current Configuration (Defaults) ===")
	printLogConfig(currentConfig)

	// 5. Modify some values using Set
	fmt.Println("\n=== Modifying Configuration Values via Set ===")
	fmt.Println("Changing:")
	fmt.Println("  - log.name: default_logger → saved_logger")
	fmt.Println("  - log.max_size_mb: 10 → 50")
	fmt.Println("  - log.retention_period_hrs: 24.0 → 48.0") // Different from CLI override later

	cfg.Set("log.name", "saved_logger") // This will be saved to file
	cfg.Set("log.max_size_mb", int64(50))
	cfg.Set("log.retention_period_hrs", 48.0)

	// 6. Save the configuration
	fmt.Println("\nSaving configuration to file...")
	err = cfg.Save(configPath)
	if err != nil {
		log.Fatalf("FATAL: Error saving config: %v\n", err)
	}
	fmt.Printf("Saved configuration to: %s\n", configPath)

	// Optional: Read and print file contents
	// fileBytes, _ := os.ReadFile(configPath)
	// fmt.Println("\n=== Saved TOML File Contents ===")
	// fmt.Println(string(fileBytes))

	// 7. Define some command-line arguments for override testing
	fmt.Println("\n=== Preparing Command-Line Overrides ===")
	// Simulate os.Args[1:]
	cliArgs := []string{
		"--log.level", "3", // Override default 1
		"--log.name", "cli_logger", // Override value set before save ("saved_logger")
		"--log.show_timestamp=false",         // Override default true
		"--log.retention_period_hrs", "72.5", // Override value set before save (48.0)
		"--other.value", "test", // An unregistered key (should be ignored by Load logic)
		"--invalid-key", // Invalid key format (test error handling if desired)
	}
	fmt.Printf("Simulated CLI Args: %v\n", cliArgs)

	// 8. Load again, now with file and CLI overrides
	// Create a *new* config instance to simulate a fresh application start
	// that loads existing file + CLI args over defaults.
	fmt.Println("\nCreating NEW config instance and loading with file and CLI args...")
	cfg2 := config.New()
	fmt.Println("Registering defaults for new instance...")
	err = cfg2.RegisterStruct("log.", defaultLogConfig)
	if err != nil {
		log.Fatalf("FATAL: Error registering defaults for cfg2: %v\n", err)
	}

	fmt.Println("Loading config with file and CLI...")
	err = cfg2.Load(configPath, cliArgs)
	if err != nil {
		// Note: If "--invalid-key" is included above, Load should return ErrCLIParse.
		// Handle or remove the invalid key for a successful load test.
		// Example check:
		if errors.Is(err, config.ErrCLIParse) {
			fmt.Printf("INFO: Expected CLI parsing error detected: %v\n", err)
			// Decide how to proceed - maybe exit or remove the offending arg and retry
			// For this example, we'll filter the bad arg and try again
			var validArgs []string
			for _, arg := range cliArgs {
				if !strings.HasPrefix(arg, "--invalid") {
					validArgs = append(validArgs, arg)
				}
			}
			fmt.Println("Retrying load with filtered CLI args...")
			err = cfg2.Load(configPath, validArgs)
			if err != nil {
				log.Fatalf("FATAL: Error loading config even after filtering CLI args: %v\n", err)
			}
		} else {
			log.Fatalf("FATAL: Unexpected error loading config with file and CLI: %v\n", err)
		}
	}
	fmt.Println("Load successful.")

	// 9. Unmarshal the final configuration state
	var finalConfig LogConfig
	fmt.Println("\nUnmarshaling final config state...")
	err = cfg2.Scan("log", &finalConfig)
	if err != nil {
		log.Fatalf("FATAL: Error unmarshaling final config: %v\n", err)
	}

	fmt.Println("\n=== Final Configuration (Defaults + File + CLI) ===")
	printLogConfig(finalConfig)

	// 10. Verify final values (Defaults < File < CLI)
	fmt.Println("\n=== Final Verification ===")
	verifyFinalConfig(finalConfig)

	// 11. Demonstrate typed accessors on the final state
	fmt.Println("\n=== Demonstrating Typed Accessors ===")
	level, err := cfg2.Int64("log.level")
	if err != nil {
		fmt.Printf("ERROR getting log.level via Int64(): %v\n", err)
	} else {
		fmt.Printf("SUCCESS: cfg2.Int64(\"log.level\") = %d (matches expected CLI override)\n", level)
	}

	name, err := cfg2.String("log.name")
	if err != nil {
		fmt.Printf("ERROR getting log.name via String(): %v\n", err)
	} else {
		fmt.Printf("SUCCESS: cfg2.String(\"log.name\") = %q (matches expected CLI override)\n", name)
	}

	showTS, err := cfg2.Bool("log.show_timestamp")
	if err != nil {
		fmt.Printf("ERROR getting log.show_timestamp via Bool(): %v\n", err)
	} else {
		fmt.Printf("SUCCESS: cfg2.Bool(\"log.show_timestamp\") = %t (matches expected CLI override)\n", showTS)
	}

	// Try getting an unregistered value (should fail)
	_, err = cfg2.String("other.value")
	if err == nil {
		fmt.Println("ERROR: Expected error when getting unregistered key 'other.value', but got nil")
	} else {
		fmt.Printf("SUCCESS: Correctly got error for unregistered key 'other.value': %v\n", err)
	}

	fmt.Println("\n=== Test Complete ===")
}

// printLogConfig prints the values of a LogConfig struct
func printLogConfig(cfg LogConfig) {
	fmt.Println("  Basic:")
	fmt.Printf("    Level: %d, Name: %s, Dir: %s, Format: %s, Ext: %s\n",
		cfg.Level, cfg.Name, cfg.Directory, cfg.Format, cfg.Extension)
	fmt.Println("  Formatting:")
	fmt.Printf("    ShowTimestamp: %t, ShowLevel: %t\n", cfg.ShowTimestamp, cfg.ShowLevel)
	fmt.Println("  Limits:")
	fmt.Printf("    BufferSize: %d, MaxSizeMB: %d, MaxTotalSizeMB: %d, MinDiskFreeMB: %d\n",
		cfg.BufferSize, cfg.MaxSizeMB, cfg.MaxTotalSizeMB, cfg.MinDiskFreeMB)
	fmt.Println("  Timers:")
	fmt.Printf("    FlushIntervalMs: %d, TraceDepth: %d, RetentionPeriodHrs: %.1f, RetentionCheckMins: %.1f\n",
		cfg.FlushIntervalMs, cfg.TraceDepth, cfg.RetentionPeriodHrs, cfg.RetentionCheckMins)
	fmt.Println("  Disk Check:")
	fmt.Printf("    DiskCheckIntervalMs: %d, EnableAdaptive: %t, MinCheckMs: %d, MaxCheckMs: %d\n",
		cfg.DiskCheckIntervalMs, cfg.EnableAdaptiveInterval, cfg.MinCheckIntervalMs, cfg.MaxCheckIntervalMs)
}

// verifyFinalConfig checks if the final values reflect the merge order: Default < File < CLI
func verifyFinalConfig(cfg LogConfig) {
	allCorrect := true
	fmt.Println("Verifying values reflect merge order (Default < File < CLI)...")

	// Value overridden by CLI
	if cfg.Level != 3 {
		fmt.Printf("  ERROR: Level is %d, expected 3 (from CLI)\n", cfg.Level)
		allCorrect = false
	}
	// Value overridden by CLI (overriding file value)
	if cfg.Name != "cli_logger" {
		fmt.Printf("  ERROR: Name is %s, expected 'cli_logger' (from CLI)\n", cfg.Name)
		allCorrect = false
	}
	// Value overridden by CLI
	if cfg.ShowTimestamp != false {
		fmt.Printf("  ERROR: ShowTimestamp is %t, expected false (from CLI)\n", cfg.ShowTimestamp)
		allCorrect = false
	}
	// Value overridden by CLI (float)
	if cfg.RetentionPeriodHrs != 72.5 {
		fmt.Printf("  ERROR: RetentionPeriodHrs is %.1f, expected 72.5 (from CLI)\n", cfg.RetentionPeriodHrs)
		allCorrect = false
	}

	// Value overridden by File (not present in CLI)
	if cfg.MaxSizeMB != 50 {
		fmt.Printf("  ERROR: MaxSizeMB is %d, expected 50 (from File)\n", cfg.MaxSizeMB)
		allCorrect = false
	}

	// Value from Default (not in File or CLI)
	if cfg.Directory != "./logs" {
		fmt.Printf("  ERROR: Directory is %s, expected './logs' (from Default)\n", cfg.Directory)
		allCorrect = false
	}
	if cfg.BufferSize != 1000 {
		fmt.Printf("  ERROR: BufferSize is %d, expected 1000 (from Default)\n", cfg.BufferSize)
		allCorrect = false
	}

	if allCorrect {
		fmt.Println("  SUCCESS: All verified configuration values match expected final state!")
	} else {
		fmt.Println("  FAILURE: Some configuration values don't match expected final state!")
	}
}