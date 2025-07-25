// FILE: lixenwraith/config/example/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/lixenwraith/config"
)

// AppConfig defines a richer configuration structure to showcase more features.
type AppConfig struct {
	Server struct {
		Host     string `toml:"host"`
		Port     int64  `toml:"port"`
		LogLevel string `toml:"log_level"`
	} `toml:"server"`
	FeatureFlags map[string]bool `toml:"feature_flags"`
}

const configFilePath = "config.toml"

func main() {
	// =========================================================================
	// PART 1: INITIAL SETUP
	// Create a clean config.toml file on disk for our program to read.
	// =========================================================================
	log.Println("---")
	log.Println("➡️  PART 1: Creating initial configuration file...")

	// Defer cleanup to run at the very end of the program.
	defer func() {
		log.Println("---")
		log.Println("🧹 Cleaning up...")
		os.Remove(configFilePath)
		// Unset the environment variable we use for testing.
		os.Unsetenv("APP_SERVER_PORT")
		log.Printf("Removed %s and unset APP_SERVER_PORT.", configFilePath)
	}()

	initialData := &AppConfig{}
	initialData.Server.Host = "localhost"
	initialData.Server.Port = 8080
	initialData.Server.LogLevel = "info"
	initialData.FeatureFlags = map[string]bool{"enable_metrics": true}

	if err := createInitialConfigFile(initialData); err != nil {
		log.Fatalf("❌ Failed during initial file creation: %v", err)
	}
	log.Printf("✅ Initial configuration saved to %s.", configFilePath)

	// =========================================================================
	// PART 2: RECOMMENDED CONFIGURATION USING THE BUILDER
	// This demonstrates source precedence, validation, and type-safe targets.
	// =========================================================================
	log.Println("---")
	log.Println("➡️  PART 2: Configuring manager with the Builder...")

	// Set an environment variable to demonstrate source precedence (Env > File).
	os.Setenv("APP_SERVER_PORT", "8888")
	log.Println("   (Set environment variable APP_SERVER_PORT=8888)")

	// Create a "target" struct. The builder will automatically populate this
	// and keep it updated when using `AsStruct()`.
	target := &AppConfig{}

	// Use the builder to chain multiple configuration options.
	builder := config.NewBuilder().
		WithTarget(target).                             // Enables type-safe `AsStruct()` and auto-registration.
		WithDefaults(initialData).                      // Explicitly set the source of defaults.
		WithFile(configFilePath).                       // Specifies the config file to read.
		WithEnvPrefix("APP_").                          // Sets prefix for environment variables (e.g., APP_SERVER_PORT).
		WithTypedValidator(func(cfg *AppConfig) error { // <-- NEW METHOD
			// No type assertion needed! `cfg.Server.Port` is guaranteed to be an int64
			// because the validator runs *after* the target struct is populated.
			if cfg.Server.Port < 1024 || cfg.Server.Port > 65535 {
				return fmt.Errorf("port %d is outside the recommended range (1024-65535)", cfg.Server.Port)
			}
			return nil
		})

	// Build the final config object.
	cfg, err := builder.Build()
	if err != nil {
		log.Fatalf("❌ Builder failed: %v", err)
	}

	log.Println("✅ Builder finished successfully. Initial values loaded.")
	initialTarget, _ := cfg.AsStruct()
	printCurrentState(initialTarget.(*AppConfig), "Initial State (Env overrides File)")

	// =========================================================================
	// PART 3: DYNAMIC RELOADING WITH THE WATCHER
	// We'll now modify the file and verify the watcher updates the config.
	// =========================================================================
	log.Println("---")
	log.Println("➡️  PART 3: Testing the file watcher...")

	// Use WithOptions to demonstrate customizing the watcher.
	watchOpts := config.WatchOptions{
		PollInterval: 250 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
	}
	cfg.AutoUpdateWithOptions(watchOpts)
	changes := cfg.Watch()
	log.Println("✅ Watcher is now active with custom options.")

	// Start a goroutine to modify the file after a short delay.
	var wg sync.WaitGroup
	wg.Add(1)
	go modifyFileOnDiskStructurally(&wg)
	log.Println("   (Modifier goroutine dispatched to change file in 1 second...)")

	log.Println("   (Waiting for watcher notification...)")
	select {
	case path := <-changes:
		log.Printf("✅ Watcher detected a change for path: '%s'", path)
		log.Println("   Verifying in-memory config using AsStruct()...")

		// Retrieve the updated, type-safe struct.
		updatedTarget, err := cfg.AsStruct()
		if err != nil {
			log.Fatalf("❌ AsStruct() failed after update: %v", err)
		}

		// Type-assert and verify the new values.
		typedCfg := updatedTarget.(*AppConfig)
		expectedLevel := "debug"
		if typedCfg.Server.LogLevel != expectedLevel {
			log.Fatalf("❌ VERIFICATION FAILED: Expected log_level '%s', but got '%s'.", expectedLevel, typedCfg.Server.LogLevel)
		}

		log.Println("✅ VERIFICATION SUCCESSFUL: In-memory config was updated by the watcher.")
		printCurrentState(typedCfg, "Final State (Updated by Watcher)")

	case <-time.After(5 * time.Second):
		log.Fatalf("❌ TEST FAILED: Timed out waiting for watcher notification.")
	}

	wg.Wait()
}

// createInitialConfigFile is a helper to set up the initial file state.
func createInitialConfigFile(data *AppConfig) error {
	cfg := config.New()
	if err := cfg.RegisterStruct("", data); err != nil {
		return err
	}
	return cfg.Save(configFilePath)
}

// modifyFileOnDiskStructurally simulates an external program that changes the config file.
func modifyFileOnDiskStructurally(wg *sync.WaitGroup) {
	defer wg.Done()
	time.Sleep(1 * time.Second)
	log.Println("   (Modifier goroutine: now changing file on disk...)")

	// Create a new, independent config instance to simulate an external process.
	modifierCfg := config.New()
	// Register the struct shape so the loader knows what paths are valid.
	if err := modifierCfg.RegisterStruct("", &AppConfig{}); err != nil {
		log.Fatalf("❌ Modifier failed to register struct: %v", err)
	}
	// Load the current state from disk.
	if err := modifierCfg.LoadFile(configFilePath); err != nil {
		log.Fatalf("❌ Modifier failed to load file: %v", err)
	}

	// Change the log level.
	modifierCfg.Set("server.log_level", "debug")

	// Use the generic GetTyped function. This is safe because modifierCfg has loaded the file.
	featureFlags, err := config.GetTyped[map[string]bool](modifierCfg, "feature_flags")
	if err != nil {
		log.Fatalf("❌ Modifier failed to get typed feature_flags: %v", err)
	}

	// Modify the typed map and set it back.
	featureFlags["enable_metrics"] = false
	modifierCfg.Set("feature_flags", featureFlags)

	// Save the changes back to disk, which will trigger the watcher in the main goroutine.
	if err := modifierCfg.Save(configFilePath); err != nil {
		log.Fatalf("❌ Modifier failed to save file: %v", err)
	}
	log.Println("   (Modifier goroutine: finished.)")
}

// printCurrentState is a helper to display the typed config state.
func printCurrentState(cfg *AppConfig, title string) {
	fmt.Println("   --------------------------------------------------")
	fmt.Printf("             %s\n", title)
	fmt.Println("   --------------------------------------------------")
	fmt.Printf("     Server Host:      %s\n", cfg.Server.Host)
	fmt.Printf("     Server Port:      %d\n", cfg.Server.Port)
	fmt.Printf("     Server Log Level: %s\n", cfg.Server.LogLevel)
	fmt.Printf("     Feature Flags:    %v\n", cfg.FeatureFlags)
	fmt.Println("   --------------------------------------------------")
}
