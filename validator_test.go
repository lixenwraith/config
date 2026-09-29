package config

import (
	"fmt"
	"testing"
)

// Each value is a subtest, so one failure does not hide another boundary case.
func checkValidator[T any](t *testing.T, validate func(T) error, valid, invalid []T, errorText ...string) {
	t.Helper()
	for _, group := range []struct {
		name      string
		values    []T
		wantError bool
	}{{"valid", valid, false}, {"invalid", invalid, true}} {
		t.Run(group.name, func(t *testing.T) {
			for _, value := range group.values {
				t.Run(fmt.Sprintf("%v", value), func(t *testing.T) {
					err := validate(value)
					if (err != nil) != group.wantError {
						t.Fatalf("validate(%#v) = %v, want error: %v", value, err, group.wantError)
					}
					if group.wantError {
						for _, fragment := range errorText {
							checkErrorContains(t, err, fragment)
						}
					}
				})
			}
		})
	}
}

func TestPortValidator(t *testing.T) {
	checkValidator(t, Port, []int64{1, 8080, 65535}, []int64{0, -1, 65536})
}

func TestPositiveValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		checkValidator(t, Positive[int64], []int64{1}, []int64{0, -1})
	})
	t.Run("Float64", func(t *testing.T) {
		checkValidator(t, Positive[float64], []float64{0.001}, []float64{0, -0.001})
	})
}

func TestNonNegativeValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		checkValidator(t, NonNegative[int64], []int64{1, 0}, []int64{-1})
	})
	t.Run("Float64", func(t *testing.T) {
		checkValidator(t, NonNegative[float64], []float64{0.001, 0}, []float64{-0.001})
	})
}

func TestIPAddressValidators(t *testing.T) {
	for _, tc := range []struct {
		name           string
		validate       func(string) error
		valid, invalid []string
	}{
		{"IPAddress", IPAddress, []string{"192.168.1.1", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", "", "0.0.0.0", "::"}, []string{"not-an-ip", "192.168.1.256"}},
		{"IPv4Address", IPv4Address, []string{"192.168.1.1", "", "0.0.0.0"}, []string{"::1", "not-an-ip"}},
		{"IPv6Address", IPv6Address, []string{"2001:db8::1", "", "::"}, []string{"127.0.0.1", "not-an-ip"}},
	} {
		t.Run(tc.name, func(t *testing.T) { checkValidator(t, tc.validate, tc.valid, tc.invalid) })
	}
}

func TestURLPathValidator(t *testing.T) {
	checkValidator(t, URLPath, []string{"/api/v1", "/", ""}, []string{"api/v1", "no-slash"})
}

func TestOneOfValidator(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		checkValidator(t, OneOf("prod", "dev", "staging"), []string{"prod", "dev"}, []string{"test"}, "must be one of")
	})
	t.Run("Int", func(t *testing.T) {
		checkValidator(t, OneOf(200, 404, 500), []int{404}, []int{302}, "must be one of")
	})
}

func TestRangeValidator(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		checkValidator(t, Range[int64](10, 100), []int64{10, 50, 100}, []int64{9, 101})
	})
	t.Run("Float64", func(t *testing.T) {
		checkValidator(t, Range[float64](-1.5, 1.5), []float64{-1.5, 0, 1.5}, []float64{-1.51, 1.51})
	})
}

func TestPatternValidator(t *testing.T) {
	checkValidator(t, Pattern(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`),
		[]string{"test@example.com", "user.name+alias@domain.co.uk"}, []string{"not-an-email", "test@example"})
}

func TestNonEmptyValidator(t *testing.T) {
	checkValidator(t, NonEmpty, []string{"hello", " a "}, []string{"", " ", "  \t\n  "})
}
