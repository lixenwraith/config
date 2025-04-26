package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load reads configuration from a TOML file and merges overrides from command-line arguments.
// 'args' should be the command-line arguments (e.g., os.Args[1:]).
// It returns an error if loading or parsing fails.
// Specific errors ErrConfigNotFound and ErrCLIParse can be checked using errors.Is.
func (c *Config) Load(path string, args []string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var errNotFound error
	var errCLI error

	fileConfig := make(map[string]any) // Holds only file data

	// --- Load from file ---
	fileData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			errNotFound = ErrConfigNotFound
			// fileData is nil, proceed to CLI args
		} else {
			return fmt.Errorf("failed to read config file '%s': %w", path, err)
		}
	} else if err := toml.Unmarshal(fileData, &fileConfig); err != nil {
		return fmt.Errorf("failed to parse TOML config file '%s': %w", path, err)
	}

	// --- Flatten file data ---
	flattenedFileConfig := flattenMap(fileConfig, "")

	// --- Parse CLI arguments ---
	cliOverrides := make(map[string]any) // Holds only CLI args data
	if len(args) > 0 {
		parsedCliMap, parseErr := parseArgs(args) // parseArgs returns a nested map
		if parseErr != nil {
			// Wrap the CLI parsing error with our specific error type
			errCLI = fmt.Errorf("%w: %w", ErrCLIParse, parseErr)
			// Do not return yet, proceed to merge what we have
		} else {
			// Flatten the nested map from CLI args only if parsing succeeded
			cliOverrides = flattenMap(parsedCliMap, "")
		}
	}

	// --- Merge and Update Internal State ---
	// Iterate through registered paths to apply loaded/default values correctly.
	// The order of precedence is: CLI > File > Registered Default
	for regPath, item := range c.items {
		// 1. Check CLI overrides (only if CLI parsing succeeded)
		if errCLI == nil {
			if cliVal, cliExists := cliOverrides[regPath]; cliExists {
				item.currentValue = cliVal
				c.items[regPath] = item
				continue
			}
		}

		// 2. Check File config (if no CLI override or CLI parsing failed)
		if fileVal, fileExists := flattenedFileConfig[regPath]; fileExists {
			item.currentValue = fileVal
		} else {
			// 3. Use Default (if not in CLI or File)
			item.currentValue = item.defaultValue
		}
		c.items[regPath] = item
	}

	return errors.Join(errNotFound, errCLI)
}

// Save writes the current configuration to a TOML file atomically.
// Only registered paths are saved.
func (c *Config) Save(path string) error {
	c.mutex.RLock()

	nestedData := make(map[string]any)
	for itemPath, item := range c.items {
		setNestedValue(nestedData, itemPath, item.currentValue)
	}

	c.mutex.RUnlock()

	// --- Marshal using BurntSushi/toml ---
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	// encoder.Indent = "  " // Optional use of 2 spaces for indentation
	if err := encoder.Encode(nestedData); err != nil {
		return fmt.Errorf("failed to marshal config data to TOML: %w", err)
	}
	tomlData := buf.Bytes()
	// --- End Marshal ---

	// --- Atomic write logic ---
	dir := filepath.Dir(path)
	// Ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil { // 0755 allows owner rwx, group rx, other rx
		return fmt.Errorf("failed to create config directory '%s': %w", dir, err)
	}

	// Create a temporary file in the same directory
	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary config file in '%s': %w", dir, err)
	}
	// Defer cleanup in case of errors during write/rename
	tempFilePath := tempFile.Name()
	removed := false
	defer func() {
		if !removed {
			os.Remove(tempFilePath) // Clean up temp file if rename fails or we panic
		}
	}()

	// Write data to the temporary file
	if _, err := tempFile.Write(tomlData); err != nil {
		tempFile.Close() // Close file before returning error
		return fmt.Errorf("failed to write temp config file '%s': %w", tempFilePath, err)
	}
	// Sync data to disk
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp config file '%s': %w", tempFilePath, err)
	}
	// Close the temporary file
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file '%s': %w", tempFilePath, err)
	}

	// Set permissions on the temporary file *before* renaming (safer)
	// Use 0644: owner rw, group r, other r
	if err := os.Chmod(tempFilePath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on temporary config file '%s': %w", tempFilePath, err)
	}

	// Atomically replace the original file with the temporary file
	if err := os.Rename(tempFilePath, path); err != nil {
		return fmt.Errorf("failed to rename temp file '%s' to '%s': %w", tempFilePath, path, err)
	}
	removed = true // Mark temp file as successfully renamed

	return nil
}

// parseArgs processes command-line arguments into a nested map structure.
// Expects arguments in the format:
//
//	--key.subkey=value
//	--key.subkey value
//	--booleanflag        (implicitly true)
//	--booleanflag=true
//	--booleanflag=false
//
// Values are parsed into bool, int64, float64, or string.
// Returns an error if a key segment is invalid.
func parseArgs(args []string) (map[string]any, error) {
	result := make(map[string]any)
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			// Skip non-flag arguments
			i++
			continue
		}

		// Remove the leading "--"
		argContent := strings.TrimPrefix(arg, "--")
		if argContent == "" {
			// Skip "--" argument if used as a separator
			i++
			continue
		}

		var keyPath string
		var valueStr string

		// Check for "--key=value" format
		if strings.Contains(argContent, "=") {
			parts := strings.SplitN(argContent, "=", 2)
			keyPath = parts[0]
			valueStr = parts[1]
			i++ // Consume only this argument
		} else {
			// Handle "--key value" or "--booleanflag"
			keyPath = argContent
			// Check if it's potentially a boolean flag (next arg starts with -- or end of args)
			isBoolFlag := i+1 >= len(args) || strings.HasPrefix(args[i+1], "--")

			if isBoolFlag {
				// Assume boolean flag is true if no value follows
				valueStr = "true"
				i++ // Consume only the flag argument
			} else {
				// Potential key-value pair with space separation
				valueStr = args[i+1]
				i += 2 // Consume flag and value arguments
			}
		}

		// Validate keyPath segments *after* extracting the key
		segments := strings.Split(keyPath, ".")
		for _, segment := range segments {
			if !isValidKeySegment(segment) {
				// Return a specific error indicating the problem
				return nil, fmt.Errorf("invalid command-line key segment %q in path %q", segment, keyPath)
			}
		}

		// Attempt to parse the value string into richer types
		var value any
		if v, err := strconv.ParseBool(valueStr); err == nil {
			value = v
		} else if v, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			value = v
		} else if v, err := strconv.ParseFloat(valueStr, 64); err == nil {
			value = v
		} else {
			// Keep as string if no other parsing succeeded
			// Remove surrounding quotes if present
			if len(valueStr) >= 2 && valueStr[0] == '"' && valueStr[len(valueStr)-1] == '"' {
				value = valueStr[1 : len(valueStr)-1]
			} else {
				value = valueStr
			}
		}

		setNestedValue(result, keyPath, value)
	}

	return result, nil
}