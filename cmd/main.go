package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/LixenWraith/config"
)

const (
	initialConfigFile = "config_initial.toml"
	finalConfigFile   = "config_final.toml"
)

// Sample TOML content for the initial configuration file
var initialTomlContent = `
debug = true
log_level = "info" # This will be overridden by default

[server]
host = "localhost"
port = 8080 # This will be overridden by CLI

[smtp]
host = "mail.example.com" # This will be overridden by CLI
port = 587
auth_user = "file_user"
# auth_pass is missing, will use default
`

func main() {
	// --- Setup: Create a temporary directory for config files ---
	tempDir, err := os.MkdirTemp("", "config_example")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up temp directory

	initialPath := filepath.Join(tempDir, initialConfigFile)
	finalPath := filepath.Join(tempDir, finalConfigFile)

	// Write the initial TOML config file
	err = os.WriteFile(initialPath, []byte(initialTomlContent), 0644)
	if err != nil {
		log.Fatalf("Failed to write initial config file: %v", err)
	}
	fmt.Printf("Wrote initial config to: %s\n", initialPath)

	// --- Step 1: Create and Register Configuration ---
	fmt.Println("\n--- Step 1: Initialize Config and Register Keys ---")
	c := config.New()

	// Register keys and store their UUIDs
	// Provide default values that might be overridden by file or CLI
	keyDebug, err := c.Register("debug", false) // Default false, overridden by file
	handleErr(err)
	keyLogLevel, err := c.Register("log_level", "warn") // Default warn, file has "info"
	handleErr(err)
	keyServerHost, err := c.Register("server.host", "127.0.0.1") // Default 127.0.0.1, overridden by file
	handleErr(err)
	keyServerPort, err := c.Register("server.port", 9090) // Default 9090, file 8080, CLI 9999
	handleErr(err)
	keySmtpHost, err := c.Register("smtp.host", "default.mail.com") // Default, file mail.example.com, CLI override.mail.com
	handleErr(err)
	keySmtpPort, err := c.Register("smtp.port", 25) // Default 25, overridden by file
	handleErr(err)
	keySmtpUser, err := c.Register("smtp.auth_user", "default_user") // Default, overridden by file
	handleErr(err)
	keySmtpPass, err := c.Register("smtp.auth_pass", "default_pass") // Default, not in file or CLI
	handleErr(err)
	keyNewCliFlag, err := c.Register("new_cli_flag", false) // Default false, set true by CLI
	handleErr(err)
	keyOnlyDefault, err := c.Register("only_default", "this_is_the_default") // Only has a default value
	handleErr(err)

	fmt.Println("Registered configuration keys with defaults.")
	fmt.Printf("  - debug (default: false): %s\n", keyDebug)
	fmt.Printf("  - server.port (default: 9090): %s\n", keyServerPort)
	fmt.Printf("  - smtp.auth_pass (default: 'default_pass'): %s\n", keySmtpPass)
	fmt.Printf("  - only_default (default: 'this_is_the_default'): %s\n", keyOnlyDefault)

	// --- Step 2: Load Configuration from File and CLI Args ---
	fmt.Println("\n--- Step 2: Load from File and CLI ---")
	// Simulate command-line arguments
	cliArgs := []string{
		"--server.port", "9999", // Override file value
		"--smtp.host", "override.mail.com", // Override file value
		"--new_cli_flag",                       // Set boolean flag to true
		"--unregistered.cli.arg", "some_value", // This will be loaded but not accessible via registered Get
	}
	fmt.Printf("Simulated CLI Args: %v\n", cliArgs)

	foundFile, err := c.Load(initialPath, cliArgs)
	handleErr(err)
	fmt.Printf("Config file loaded: %t\n", foundFile)

	// --- Step 3: Access Merged Configuration Values ---
	fmt.Println("\n--- Step 3: Access Merged Values via Get() ---")

	// Retrieve values using the UUID keys. Observe the precedence: Default -> File -> CLI
	debugVal, _ := c.Get(keyDebug)             // Expect: true (from file)
	logLevelVal, _ := c.Get(keyLogLevel)       // Expect: "info" (from file)
	serverHostVal, _ := c.Get(keyServerHost)   // Expect: "localhost" (from file)
	serverPortVal, _ := c.Get(keyServerPort)   // Expect: 9999 (int64 from CLI)
	smtpHostVal, _ := c.Get(keySmtpHost)       // Expect: "override.mail.com" (from CLI)
	smtpPortVal, _ := c.Get(keySmtpPort)       // Expect: 587 (int64 from file - parseArgs converts numbers)
	smtpUserVal, _ := c.Get(keySmtpUser)       // Expect: "file_user" (from file)
	smtpPassVal, _ := c.Get(keySmtpPass)       // Expect: "default_pass" (only default exists)
	newCliFlagVal, _ := c.Get(keyNewCliFlag)   // Expect: true (from CLI flag)
	onlyDefaultVal, _ := c.Get(keyOnlyDefault) // Expect: "this_is_the_default" (only default exists)
	_, registered := c.Get("nonexistent-uuid") // Expect: registered = false

	fmt.Printf("Debug          (File): %v (%T)\n", debugVal, debugVal)
	fmt.Printf("LogLevel       (File): %v (%T)\n", logLevelVal, logLevelVal)
	fmt.Printf("Server Host    (File): %v (%T)\n", serverHostVal, serverHostVal)
	fmt.Printf("Server Port    (CLI) : %v (%T)\n", serverPortVal, serverPortVal)
	fmt.Printf("SMTP Host      (CLI) : %v (%T)\n", smtpHostVal, smtpHostVal)
	fmt.Printf("SMTP Port      (File): %v (%T)\n", smtpPortVal, smtpPortVal) // Note: parseArgs converts to int64
	fmt.Printf("SMTP User      (File): %v (%T)\n", smtpUserVal, smtpUserVal)
	fmt.Printf("SMTP Pass    (Default): %v (%T)\n", smtpPassVal, smtpPassVal)
	fmt.Printf("New CLI Flag   (CLI) : %v (%T)\n", newCliFlagVal, newCliFlagVal)
	fmt.Printf("Only Default (Default): %v (%T)\n", onlyDefaultVal, onlyDefaultVal)
	fmt.Printf("Unregistered UUID     : Found=%t\n", registered)

	// --- Step 4: Save the Final Configuration ---
	fmt.Println("\n--- Step 4: Save Final Configuration ---")
	err = c.Save(finalPath)
	handleErr(err)
	fmt.Printf("Saved final merged configuration to: %s\n", finalPath)

	// Optional: Print the content of the saved file
	savedContent, _ := os.ReadFile(finalPath)
	fmt.Println("--- Content of saved file (config_final.toml): ---")
	fmt.Println(string(savedContent))
	fmt.Println("-------------------------------------------------")

	// --- Step 5: Load Saved Config into a New Instance and Compare ---
	fmt.Println("\n--- Step 5: Reload Saved Config and Verify ---")
	c2 := config.New()

	// NOTE: For c2 to use Get(), keys would need to be registered again.
	// For simple verification here, we load and access the underlying map.
	// In a real app, you'd likely register keys consistently at startup.
	foundSavedFile, err := c2.Load(finalPath, nil) // Load without CLI args this time
	handleErr(err)
	if !foundSavedFile {
		log.Fatalf("Failed to find the saved config file '%s' for reloading", finalPath)
	}
	fmt.Println("Reloaded final config into a new instance.")

	// Directly compare some values from the internal data map for verification
	// This requires accessing the unexported 'data' field via a helper or reflection,
	// OR rely on saving/loading being correct (which is what we test here).
	// Let's assume Save/Load worked and verify the expected final values are present after load.

	// We need to register keys again in c2 to use Get() for comparison
	key2ServerPort, _ := c2.Register("server.port", 0) // Default doesn't matter now
	key2SmtpHost, _ := c2.Register("smtp.host", "")
	key2NewCliFlag, _ := c2.Register("new_cli_flag", false)
	key2Unregistered, _ := c2.Register("unregistered.cli.arg", "") // Register the CLI-only arg

	reloadedPort, _ := c2.Get(key2ServerPort)
	reloadedSmtpHost, _ := c2.Get(key2SmtpHost)
	reloadedCliFlag, _ := c2.Get(key2NewCliFlag)
	reloadedUnregistered, _ := c2.Get(key2Unregistered) // Get the value added only via CLI initially

	fmt.Println("Comparing reloaded values:")
	fmt.Printf("  - Reloaded Server Port: %v (Expected: 9999) - Match: %t\n", reloadedPort, reloadedPort == int64(9999))
	fmt.Printf("  - Reloaded SMTP Host  : %v (Expected: override.mail.com) - Match: %t\n", reloadedSmtpHost, reloadedSmtpHost == "override.mail.com")
	fmt.Printf("  - Reloaded CLI Flag   : %v (Expected: true) - Match: %t\n", reloadedCliFlag, reloadedCliFlag == true)
	fmt.Printf("  - Reloaded Unreg Arg  : %v (Expected: some_value) - Match: %t\n", reloadedUnregistered, reloadedUnregistered == "some_value")

	fmt.Println("\n--- Example Finished ---")
}

// Simple error handler
func handleErr(err error) {
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}