// FILE: lixenwraith/config/validator_test.go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPortValidator tests the Port validator
func TestPortValidator(t *testing.T) {
	tests := []struct {
		name    string
		port    int64
		wantErr bool
	}{
		{"ValidLowPort", 1, false},
		{"ValidCommonPort", 8080, false},
		{"ValidHighPort", 65535, false},
		{"InvalidZeroPort", 0, true},
		{"InvalidNegativePort", -1, true},
		{"InvalidTooHighPort", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Port(tt.port)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPositiveValidator tests the Positive validator
func TestPositiveValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		assert.NoError(t, Positive(int64(1)))
		assert.Error(t, Positive(int64(0)))
		assert.Error(t, Positive(int64(-1)))
	})

	t.Run("Float64", func(t *testing.T) {
		assert.NoError(t, Positive(0.001))
		assert.Error(t, Positive(0.0))
		assert.Error(t, Positive(-0.001))
	})
}

// TestNonNegativeValidator tests the NonNegative validator
func TestNonNegativeValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		assert.NoError(t, NonNegative(int64(1)))
		assert.NoError(t, NonNegative(int64(0)))
		assert.Error(t, NonNegative(int64(-1)))
	})

	t.Run("Float64", func(t *testing.T) {
		assert.NoError(t, NonNegative(0.001))
		assert.NoError(t, NonNegative(0.0))
		assert.Error(t, NonNegative(-0.001))
	})
}

// TestIPAddressValidators tests all IP-related validators
func TestIPAddressValidators(t *testing.T) {
	t.Run("IPAddress", func(t *testing.T) {
		assert.NoError(t, IPAddress("192.168.1.1"))
		assert.NoError(t, IPAddress("2001:0db8:85a3:0000:0000:8a2e:0370:7334"))
		assert.NoError(t, IPAddress(""))
		assert.NoError(t, IPAddress("0.0.0.0"))
		assert.NoError(t, IPAddress("::"))
		assert.Error(t, IPAddress("not-an-ip"))
		assert.Error(t, IPAddress("192.168.1.256"))
	})

	t.Run("IPv4Address", func(t *testing.T) {
		assert.NoError(t, IPv4Address("192.168.1.1"))
		assert.NoError(t, IPv4Address(""))
		assert.NoError(t, IPv4Address("0.0.0.0"))
		assert.Error(t, IPv4Address("::1")) // Is not IPv4
		assert.Error(t, IPv4Address("not-an-ip"))
	})

	t.Run("IPv6Address", func(t *testing.T) {
		assert.NoError(t, IPv6Address("2001:db8::1"))
		assert.NoError(t, IPv6Address(""))
		assert.NoError(t, IPv6Address("::"))
		assert.Error(t, IPv6Address("127.0.0.1")) // Is not IPv6
		assert.Error(t, IPv6Address("not-an-ip"))
	})
}

// TestURLPathValidator tests the URLPath validator
func TestURLPathValidator(t *testing.T) {
	assert.NoError(t, URLPath("/api/v1"))
	assert.NoError(t, URLPath("/"))
	assert.NoError(t, URLPath(""))
	assert.Error(t, URLPath("api/v1"))
	assert.Error(t, URLPath("no-slash"))
}

// TestOneOfValidator tests the OneOf validator
func TestOneOfValidator(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		validator := OneOf("prod", "dev", "staging")
		assert.NoError(t, validator("prod"))
		assert.NoError(t, validator("dev"))
		err := validator("test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be one of")
	})

	t.Run("Int", func(t *testing.T) {
		validator := OneOf(200, 404, 500)
		assert.NoError(t, validator(404))
		err := validator(302)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be one of")
	})
}

// TestRangeValidator tests the Range validator
func TestRangeValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		validator := Range[int64](10, 100)
		assert.NoError(t, validator(10))
		assert.NoError(t, validator(50))
		assert.NoError(t, validator(100))
		assert.Error(t, validator(9))
		assert.Error(t, validator(101))
	})

	t.Run("Float64", func(t *testing.T) {
		validator := Range[float64](-1.5, 1.5)
		assert.NoError(t, validator(-1.5))
		assert.NoError(t, validator(0.0))
		assert.NoError(t, validator(1.5))
		assert.Error(t, validator(-1.51))
		assert.Error(t, validator(1.51))
	})
}

// TestPatternValidator tests the Pattern validator
func TestPatternValidator(t *testing.T) {
	// Simple email regex
	validator := Pattern(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	assert.NoError(t, validator("test@example.com"))
	assert.NoError(t, validator("user.name+alias@domain.co.uk"))
	assert.Error(t, validator("not-an-email"))
	assert.Error(t, validator("test@example"))
}

// TestNonEmptyValidator tests the NonEmpty validator
func TestNonEmptyValidator(t *testing.T) {
	assert.NoError(t, NonEmpty("hello"))
	assert.NoError(t, NonEmpty(" a "))
	assert.Error(t, NonEmpty(""))
	assert.Error(t, NonEmpty(" "))
	assert.Error(t, NonEmpty("  \t\n  "))
}
