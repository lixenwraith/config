// File: lixenwraith/config/helper.go
package config

import "strings"

// flattenMap converts a nested map[string]any to a flat map[string]any with dot-notation paths.
func flattenMap(nested map[string]any, prefix string) map[string]any {
	flat := make(map[string]any)

	for key, value := range nested {
		newPath := key
		if prefix != "" {
			newPath = prefix + "." + key
		}

		// Check if the value is a map that can be further flattened
		if nestedMap, isMap := value.(map[string]any); isMap {
			// Recursively flatten the nested map
			flattenedSubMap := flattenMap(nestedMap, newPath)
			// Merge the flattened sub-map into the main flat map
			for subPath, subValue := range flattenedSubMap {
				flat[subPath] = subValue
			}
		} else {
			// If it's not a map, add the value directly to the flat map
			flat[newPath] = value
		}
	}

	return flat
}

// setNestedValue sets a value in a nested map using a dot-notation path.
// It creates intermediate maps if they don't exist.
// If a segment exists but is not a map, it will be overwritten by a new map.
func setNestedValue(nested map[string]any, path string, value any) {
	segments := strings.Split(path, ".")
	current := nested

	// Iterate through segments up to the second-to-last one
	for i := 0; i < len(segments)-1; i++ {
		segment := segments[i]

		// Check if the next level exists
		next, exists := current[segment]

		if !exists {
			newMap := make(map[string]any)
			current[segment] = newMap
			current = newMap
		} else {
			// If the segment exists, check if it's already a map
			if nextMap, isMap := next.(map[string]any); isMap {
				current = nextMap
			} else {
				newMap := make(map[string]any)
				current[segment] = newMap
				current = newMap
			}
		}
	}

	lastSegment := segments[len(segments)-1]
	current[lastSegment] = value
}

// isValidKeySegment checks if a single path segment is a valid TOML key part.
func isValidKeySegment(s string) bool {
	if len(s) == 0 {
		return false
	}
	// TOML bare keys are sequences of ASCII letters, ASCII digits, underscores, and dashes (A-Za-z0-9_-).
	if strings.ContainsRune(s, '.') {
		return false // Segments themselves cannot contain dots
	}

	for _, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		isUnderscore := r == '_'
		isDash := r == '-'

		if !(isLetter || isDigit || isUnderscore || isDash) {
			return false
		}
	}
	return true
}