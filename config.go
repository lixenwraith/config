package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/LixenWraith/tinytoml"
)

type cliArg struct {
	key   string
	value string
}

func Load(path string, config interface{}, args []string) (bool, error) {
	if config == nil {
		return false, fmt.Errorf("config cannot be nil")
	}

	configExists := false
	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		configExists = true
		data, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := tinytoml.Unmarshal(data, config); err != nil {
			return false, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	if len(args) > 0 {
		overrides, err := parseArgs(args)
		if err != nil {
			return configExists, fmt.Errorf("failed to parse CLI args: %w", err)
		}

		if err := mergeConfig(config, overrides); err != nil {
			return configExists, fmt.Errorf("failed to merge CLI args: %w", err)
		}
	}

	return configExists, nil
}

func Save(path string, config interface{}) error {
	v := reflect.ValueOf(config)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return fmt.Errorf("config must be a struct or pointer to struct")
	}

	data, err := tinytoml.Marshal(v.Interface())
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to save config file: %w", err)
	}

	return nil
}

func parseArgs(args []string) (map[string]interface{}, error) {
	parsed := make([]cliArg, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}

		key := strings.TrimPrefix(arg, "--")
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			parsed = append(parsed, cliArg{key: key, value: "true"})
			continue
		}

		parsed = append(parsed, cliArg{key: key, value: args[i+1]})
		i++
	}

	result := make(map[string]interface{})
	for _, arg := range parsed {
		keys := strings.Split(arg.key, ".")
		current := result
		for i, k := range keys[:len(keys)-1] {
			if _, exists := current[k]; !exists {
				current[k] = make(map[string]interface{})
			}
			if nested, ok := current[k].(map[string]interface{}); ok {
				current = nested
			} else {
				return nil, fmt.Errorf("invalid nested key at %s", strings.Join(keys[:i+1], "."))
			}
		}

		lastKey := keys[len(keys)-1]
		if val, err := strconv.ParseBool(arg.value); err == nil {
			current[lastKey] = val
		} else if val, err := strconv.ParseInt(arg.value, 10, 64); err == nil {
			current[lastKey] = val
		} else if val, err := strconv.ParseFloat(arg.value, 64); err == nil {
			current[lastKey] = val
		} else {
			current[lastKey] = arg.value
		}
	}

	return result, nil
}

func mergeConfig(base interface{}, override map[string]interface{}) error {
	baseValue := reflect.ValueOf(base)
	if baseValue.Kind() != reflect.Ptr || baseValue.IsNil() {
		return fmt.Errorf("base config must be a non-nil pointer")
	}

	data, err := json.Marshal(override)
	if err != nil {
		return fmt.Errorf("failed to marshal override values: %w", err)
	}

	if err := json.Unmarshal(data, base); err != nil {
		return fmt.Errorf("failed to merge override values: %w", err)
	}

	return nil
}