// FILE: lixenwraith/config/decode_test.go
package config

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

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
	require.NoError(t, err)

	// Verify conversions
	assert.Equal(t, "192.168.1.100", result.Network.IP.String())
	assert.Equal(t, "192.168.1.0/24", result.Network.IPNet.String())
	assert.Equal(t, "https://api.example.com:8443/v1", result.Network.URL.String())
	assert.Equal(t, 150*time.Second, result.Network.Timeout)
	assert.Equal(t, 5, result.Network.Retry.Count)
	assert.Equal(t, 10*time.Second, result.Network.Retry.Interval)
	assert.Equal(t, []string{"prod", "staging", "test"}, result.Tags)
	assert.Equal(t, []int{80, 443, 8080}, result.Ports)
	assert.Equal(t, "production", result.Labels["env"])
	assert.Equal(t, "1.2.3", result.Labels["version"])
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
	require.NoError(t, err)

	assert.Equal(t, "appserver", server.Host)
	assert.Equal(t, 9000, server.Port)
	assert.Equal(t, true, server.Enabled)

	// Test non-existent base path
	var empty ServerConfig
	err = cfg.Scan(&empty, "app.nonexistent")
	assert.NoError(t, err) // Should not error, just empty
	assert.Equal(t, "", empty.Host)
	assert.Equal(t, 0, empty.Port)
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
		{SourceDefault, ""}, // No value in default source
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			var result Config
			err := cfg.ScanSource(tt.source, &result)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Value)
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
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErr)
		})
	}
}

// TestCustomTypeConversion tests edge cases in type conversion
func TestCustomTypeConversion(t *testing.T) {
	cfg := New()

	t.Run("InvalidIPAddress", func(t *testing.T) {
		type Config struct {
			IP net.IP `toml:"ip"`
		}

		cfg.Register("ip", net.IP{})
		cfg.Set("ip", "not-an-ip")

		var result Config
		err := cfg.Scan(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP address")
	})

	t.Run("InvalidCIDR", func(t *testing.T) {
		type Config struct {
			Network *net.IPNet `toml:"network"`
		}

		cfg.Register("network", (*net.IPNet)(nil))
		cfg.Set("network", "invalid-cidr")

		var result Config
		err := cfg.Scan(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid CIDR")
	})

	t.Run("InvalidURL", func(t *testing.T) {
		type Config struct {
			Endpoint *url.URL `toml:"endpoint"`
		}

		cfg.Register("endpoint", (*url.URL)(nil))
		cfg.Set("endpoint", "://invalid-url")

		var result Config
		err := cfg.Scan(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid URL")
	})

	t.Run("LongIPString", func(t *testing.T) {
		type Config struct {
			IP net.IP `toml:"ip"`
		}

		cfg.Register("ip", net.IP{})
		// String longer than max IPv6 length
		longIP := make([]byte, 50)
		for i := range longIP {
			longIP[i] = 'x'
		}
		cfg.Set("ip", string(longIP))

		var result Config
		err := cfg.Scan(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP length")
	})

	t.Run("LongURL", func(t *testing.T) {
		type Config struct {
			URL *url.URL `toml:"url"`
		}

		cfg.Register("url", (*url.URL)(nil))
		// URL longer than 2048 bytes
		longURL := "https://example.com/"
		for i := 0; i < 2048; i++ {
			longURL += "x"
		}
		cfg.Set("url", longURL)

		var result Config
		err := cfg.Scan(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "URL too long")
	})
}

// TestZeroFields tests that ZeroFields option works correctly
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
	require.NoError(t, err)

	// ZeroFields should reset all fields before decoding
	assert.Equal(t, "newvalue", result.KeepValue)
	assert.Equal(t, "resetdefault", result.ResetValue)
	assert.Equal(t, "initial", result.NestedValue.Field) // Unregistered, so Scan should not touch it
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
	require.NoError(t, err)

	assert.Equal(t, 42, result.IntFromString)
	assert.Equal(t, 3.14159, result.FloatFromString)
	assert.Equal(t, true, result.BoolFromString)
	assert.Equal(t, "12345", result.StringFromInt)
	assert.Equal(t, "1", result.StringFromBool) // mapstructure converts bool(true) to "1" in weak conversion
}