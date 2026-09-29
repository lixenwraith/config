// FILE: lixenwraith/config/decode_test.go
package config

import (
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestScanWithComplexTypes tests scanning with various complex types
func TestScanWithComplexTypes(t *testing.T) {
	type NetworkConfig struct {
		IP      net.IP        `toml:"ip"`
		IPNet   *net.IPNet    `toml:"subnet"`
		URL     *url.URL      `toml:"endpoint"`
		Timeout time.Duration `toml:"timeout"`
		Retry   struct {
			Count    int           `toml:"count"`
			Interval time.Duration `toml:"interval"`
		} `toml:"retry"`
	}

	type AppConfig struct {
		Network NetworkConfig     `toml:"network"`
		Tags    []string          `toml:"tags"`
		Ports   []int             `toml:"ports"`
		Labels  map[string]string `toml:"labels"`
	}

	cfg := New()

	// Register with defaults
	defaults := &AppConfig{
		Network: NetworkConfig{
			IP:      net.ParseIP("127.0.0.1"),
			Timeout: 30 * time.Second,
		},
		Tags:  []string{"default"},
		Ports: []int{8080},
		Labels: map[string]string{
			"env": "dev",
		},
	}

	err := cfg.RegisterStruct("", defaults)
	mustNoError(t, err)

	// Set values from different sources
	cfg.SetSource(SourceEnv, "network.ip", "192.168.1.100")
	cfg.SetSource(SourceEnv, "network.subnet", "192.168.1.0/24")
	cfg.SetSource(SourceEnv, "network.endpoint", "https://api.example.com:8443/v1")
	cfg.SetSource(SourceFile, "network.timeout", "2m30s")
	cfg.SetSource(SourceFile, "network.retry.count", int64(5))
	cfg.SetSource(SourceFile, "network.retry.interval", "10s")
	cfg.SetSource(SourceCLI, "tags", "prod,staging,test")
	cfg.SetSource(SourceFile, "ports", []any{int64(80), int64(443), int64(8080)})
	cfg.SetSource(SourceFile, "labels", map[string]any{
		"env":     "production",
		"version": "1.2.3",
	})

	// Scan into struct
	var result AppConfig
	err = cfg.Scan(&result)
	mustNoError(t, err)

	// Verify conversions
	checkEqual(t, result.Network.IP.String(), "192.168.1.100")
	checkEqual(t, result.Network.IPNet.String(), "192.168.1.0/24")
	checkEqual(t, result.Network.URL.String(), "https://api.example.com:8443/v1")
	checkEqual(t, result.Network.Timeout, 150*time.Second)
	checkEqual(t, result.Network.Retry.Count, 5)
	checkEqual(t, result.Network.Retry.Interval, 10*time.Second)
	checkEqual(t, result.Tags, []string{"prod", "staging", "test"})
	checkEqual(t, result.Ports, []int{80, 443, 8080})
	checkEqual(t, result.Labels["env"], "production")
	checkEqual(t, result.Labels["version"], "1.2.3")
}

// TestScanWithBasePath tests scanning from nested paths
func TestScanWithBasePath(t *testing.T) {
	type ServerConfig struct {
		Host    string `toml:"host"`
		Port    int    `toml:"port"`
		Enabled bool   `toml:"enabled"`
	}

	cfg := New()
	cfg.Register("app.server.host", "localhost")
	cfg.Register("app.server.port", 8080)
	cfg.Register("app.server.enabled", true)
	cfg.Register("app.database.host", "dbhost")

	cfg.Set("app.server.host", "appserver")
	cfg.Set("app.server.port", 9000)

	// Scan only the server section
	var server ServerConfig
	err := cfg.Scan(&server, "app.server")
	mustNoError(t, err)

	checkEqual(t, server.Host, "appserver")
	checkEqual(t, server.Port, 9000)
	checkEqual(t, server.Enabled, true)

	// Test non-existent base path
	var empty ServerConfig
	err = cfg.Scan(&empty, "app.nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} // Should not error, just empty
	checkEqual(t, empty.Host, "")
	checkEqual(t, empty.Port, 0)
}

// TestScanFromSource tests scanning from specific sources
func TestScanFromSource(t *testing.T) {
	type Config struct {
		Value string `toml:"value"`
	}

	cfg := New()
	cfg.Register("value", "default")

	cfg.SetSource(SourceFile, "value", "fromfile")
	cfg.SetSource(SourceEnv, "value", "fromenv")
	cfg.SetSource(SourceCLI, "value", "fromcli")

	tests := []struct {
		source   Source
		expected string
	}{
		{SourceFile, "fromfile"},
		{SourceEnv, "fromenv"},
		{SourceCLI, "fromcli"},
		{SourceDefault, "default"}, // Registered defaults are a readable source
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			var result Config
			err := cfg.ScanSource(tt.source, &result)
			mustNoError(t, err)
			checkEqual(t, result.Value, tt.expected)
		})
	}
}

// TestInvalidScanTargets tests error cases for scanning
func TestInvalidScanTargets(t *testing.T) {
	cfg := New()
	cfg.Register("test", "value")

	tests := []struct {
		name      string
		target    any
		expectErr string
	}{
		{"NilPointer", nil, "must be non-nil pointer"},
		{"NonPointer", "not-a-pointer", "must be non-nil pointer"},
		{"NilStructPointer", (*struct{})(nil), "must be non-nil pointer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cfg.Scan(tt.target)
			checkErrorContains(t, err, tt.expectErr)
		})
	}
}

// TestCustomTypeConversion tests edge cases in type conversion
func TestCustomTypeConversion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		initial  any
		input    string
		fragment string
	}{
		{"IP", net.IP{}, "not-an-ip", "invalid IP address"},
		{"CIDR", (*net.IPNet)(nil), "invalid-cidr", "invalid CIDR"},
		{"URL", (*url.URL)(nil), "://invalid-url", "invalid URL"},
		{"LongIP", net.IP{}, strings.Repeat("x", 50), "invalid IP length"},
		{"LongURL", (*url.URL)(nil), strings.Repeat("x", MaxURLLength+1), "URL too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := New()
			mustNoError(t, cfg.Register("value", tc.initial))
			err := cfg.Set("value", tc.input)
			if err == nil {
				t.Fatalf("expected an error")
			}
			checkErrorContains(t, err, tc.fragment)
			got, _ := cfg.Get("value")
			checkEqual(t, got, tc.initial)
		})
	}
}

func TestZeroFields(t *testing.T) {
	type Config struct {
		KeepValue   string `toml:"keep"`
		ResetValue  string `toml:"reset"`
		NestedValue struct {
			Field string `toml:"field"`
		} `toml:"nested"`
	}

	cfg := New()

	// Register only some fields
	cfg.Register("keep", "keepdefault")
	cfg.Register("reset", "resetdefault")
	// Don't register nested.field

	cfg.Set("keep", "newvalue")
	// Don't set reset, so it uses default

	// Start with non-zero struct
	result := Config{
		KeepValue:  "initial",
		ResetValue: "initial",
		NestedValue: struct {
			Field string `toml:"field"`
		}{Field: "initial"},
	}

	err := cfg.Scan(&result)
	mustNoError(t, err)

	// ZeroFields should reset all fields before decoding
	checkEqual(t, result.KeepValue, "newvalue")
	checkEqual(t, result.ResetValue, "resetdefault")
	checkEqual(t, result.NestedValue.Field, "initial") // Unregistered, so Scan should not touch it
}

// TestWeaklyTypedInput tests weak type conversion
func TestWeaklyTypedInput(t *testing.T) {
	type Config struct {
		IntFromString   int     `toml:"int_from_string"`
		FloatFromString float64 `toml:"float_from_string"`
		BoolFromString  bool    `toml:"bool_from_string"`
		StringFromInt   string  `toml:"string_from_int"`
		StringFromBool  string  `toml:"string_from_bool"`
	}

	cfg := New()
	defaults := &Config{}
	cfg.RegisterStruct("", defaults)

	// Set string values that should convert
	cfg.Set("int_from_string", "42")
	cfg.Set("float_from_string", "3.14159")
	cfg.Set("bool_from_string", "true")
	cfg.Set("string_from_int", 12345)
	cfg.Set("string_from_bool", true)

	var result Config
	err := cfg.Scan(&result)
	mustNoError(t, err)

	checkEqual(t, result.IntFromString, 42)
	checkEqual(t, result.FloatFromString, 3.14159)
	checkEqual(t, result.BoolFromString, true)
	checkEqual(t, result.StringFromInt, "12345")
	checkEqual(t, result.StringFromBool, "true") // Canonical boolean text
}
