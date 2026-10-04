package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/lixenwraith/toml"
)

func (c *Config) unmarshal(source Source, target any, basePath ...string) error {
	if len(basePath) > 1 {
		return wrapError(ErrInvalidPath, fmt.Errorf("expected at most one basePath"))
	}
	path := ""
	if len(basePath) == 1 {
		path = strings.TrimSuffix(basePath[0], ".")
	}
	c.mutex.RLock()
	nested := c.nestedLocked(source)
	c.mutex.RUnlock()
	section := navigateToPath(nested, path)
	if section == nil {
		section = map[string]any{}
	}
	if reflect.TypeOf(section).Kind() != reflect.Map {
		return wrapError(ErrTypeMismatch, fmt.Errorf("path %q refers to non-map value", path))
	}
	return decodeConfig(section, target)
}

func (c *Config) nestedLocked(source Source) map[string]any {
	nested := make(map[string]any)
	for path, item := range c.items {
		var value any
		var ok bool
		switch source {
		case "":
			value, ok = item.currentValue, true
		case SourceDefault:
			value, ok = item.defaultValue, true
		default:
			value, ok = item.values[source]
		}
		if ok {
			setNestedValue(nested, path, value)
		}
	}
	return nested
}

// decodeConfig stages writes and retains missing struct fields. It never mutates
// stored configuration or existing destination containers on a failed conversion.
func decodeConfig(data any, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return wrapError(ErrTypeMismatch, fmt.Errorf("target must be non-nil pointer"))
	}
	next := reflect.New(rv.Elem().Type()).Elem()
	next.Set(rv.Elem())
	if err := decodeInto(data, next, 0); err != nil {
		return wrapError(ErrDecode, err)
	}
	rv.Elem().Set(next)
	return nil
}

func decodeInto(data any, dst reflect.Value, depth int) error {
	if depth > maxValueDepth {
		return fmt.Errorf("decode nesting exceeds %d", maxValueDepth)
	}
	if data == nil {
		dst.SetZero()
		return nil
	}
	src := reflect.ValueOf(data)
	for src.Kind() == reflect.Pointer || src.Kind() == reflect.Interface {
		if src.IsNil() {
			dst.SetZero()
			return nil
		}
		src = src.Elem()
		depth++
		if depth > maxValueDepth {
			return fmt.Errorf("cyclic pointer input")
		}
	}
	data = src.Interface()
	if dst.Kind() == reflect.Pointer {
		next := reflect.New(dst.Type().Elem())
		if !dst.IsNil() {
			next.Elem().Set(dst.Elem())
		}
		if err := decodeInto(data, next.Elem(), depth+1); err != nil {
			return err
		}
		dst.Set(next)
		return nil
	}
	// Native struct defaults can occur inside slices, arrays and map values.
	// They already have the destination type, but still need a checked deep copy.
	if (atomicType(dst.Type()) || dst.Kind() == reflect.Struct) && src.Type() == dst.Type() {
		v, err := copyReflect(src, depth+1)
		if err != nil {
			return err
		}
		dst.Set(v)
		return nil
	}
	if src.Kind() == reflect.String {
		s := src.String()
		if len(s) > MaxValueSize {
			return ErrValueSize
		}
		switch dst.Type() {
		case durationType:
			v, err := time.ParseDuration(s)
			if err != nil {
				return errors.New("invalid duration: want a number and a unit, as 30s")
			}
			dst.SetInt(int64(v))
			return nil
		case timeType:
			v, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				return errors.New("invalid time: want RFC 3339")
			}
			dst.Set(reflect.ValueOf(v))
			return nil
		case ipType:
			if len(s) > MaxIPv6Length {
				return fmt.Errorf("invalid IP length: %d", len(s))
			}
			v := net.ParseIP(s)
			if v == nil {
				return errors.New("invalid IP address")
			}
			dst.Set(reflect.ValueOf(v))
			return nil
		case ipNetType:
			if len(s) > MaxCIDRLength {
				return fmt.Errorf("invalid CIDR length: %d", len(s))
			}
			_, v, err := net.ParseCIDR(s)
			if err != nil {
				return errors.New("invalid CIDR")
			}
			dst.Set(reflect.ValueOf(*v))
			return nil
		case urlType:
			if len(s) > MaxURLLength {
				return fmt.Errorf("URL too long: %d bytes", len(s))
			}
			v, err := url.Parse(s)
			if err != nil {
				return errors.New("invalid URL")
			}
			dst.Set(reflect.ValueOf(*v))
			return nil
		}
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v, err := strconv.ParseInt(s, 10, dst.Type().Bits())
			if err != nil {
				return valueless("integer", err)
			}
			dst.SetInt(v)
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v, err := strconv.ParseUint(s, 10, dst.Type().Bits())
			if err != nil {
				return valueless("unsigned integer", err)
			}
			dst.SetUint(v)
			return nil
		case reflect.Float32, reflect.Float64:
			v, err := strconv.ParseFloat(s, dst.Type().Bits())
			if err != nil {
				return valueless("float", err)
			}
			return toml.Decode(v, dst.Addr().Interface())
		case reflect.Bool:
			v, err := strconv.ParseBool(s)
			if err != nil {
				return valueless("bool", err)
			}
			dst.SetBool(v)
			return nil
		case reflect.Slice, reflect.Array:
			// One entry: files and maps have native arrays, and a pattern or
			// a DN may hold commas. Text sources split earlier (textList).
			parts := []string{s}
			data, src = parts, reflect.ValueOf(parts)
		}
	}
	switch dst.Kind() {
	case reflect.Interface:
		v, err := copyReflect(src, depth+1)
		if err != nil {
			return err
		}
		if !v.Type().AssignableTo(dst.Type()) {
			return fmt.Errorf("cannot assign %s to %s", v.Type(), dst.Type())
		}
		dst.Set(v)
	case reflect.Struct:
		if atomicType(dst.Type()) {
			return fmt.Errorf("cannot decode %T as %s", data, dst.Type())
		}
		m, err := normalizeMap(data)
		if err != nil {
			return err
		}
		seen := make(map[string]bool)
		for i := 0; i < dst.NumField(); i++ {
			f := dst.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			key := fieldKey(f)
			if key == "-" {
				continue
			}
			if seen[key] {
				return fmt.Errorf("duplicate TOML field %q", key)
			}
			seen[key] = true
			if v, ok := m[key]; ok {
				if err := decodeInto(v, dst.Field(i), depth+1); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
			}
		}
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map keys must be strings")
		}
		m, err := normalizeMap(data)
		if err != nil {
			return err
		}
		next := reflect.MakeMapWithSize(dst.Type(), len(m))
		for key, value := range m {
			v := reflect.New(dst.Type().Elem()).Elem()
			if err := decodeInto(value, v, depth+1); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			next.SetMapIndex(reflect.ValueOf(key).Convert(dst.Type().Key()), v)
		}
		dst.Set(next)
	case reflect.Slice, reflect.Array:
		if src.Kind() != reflect.Slice && src.Kind() != reflect.Array {
			return fmt.Errorf("expected array, got %T", data)
		}
		var next reflect.Value
		if dst.Kind() == reflect.Array {
			if dst.Len() != src.Len() {
				return fmt.Errorf("array length %d does not match %d", src.Len(), dst.Len())
			}
			next = reflect.New(dst.Type()).Elem()
		} else {
			next = reflect.MakeSlice(dst.Type(), src.Len(), src.Len())
		}
		for i := 0; i < src.Len(); i++ {
			if err := decodeInto(src.Index(i).Interface(), next.Index(i), depth+1); err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
		}
		dst.Set(next)
	case reflect.String:
		switch src.Kind() {
		case reflect.String:
			dst.SetString(src.String())
		case reflect.Bool:
			dst.SetString(strconv.FormatBool(src.Bool()))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetString(strconv.FormatInt(src.Int(), 10))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dst.SetString(strconv.FormatUint(src.Uint(), 10))
		case reflect.Float32, reflect.Float64:
			if _, err := copyReflect(src, depth+1); err != nil {
				return err
			}
			dst.SetString(strconv.FormatFloat(src.Float(), 'g', -1, src.Type().Bits()))
		default:
			return fmt.Errorf("cannot convert %T to string", data)
		}
	case reflect.Bool:
		if src.Kind() != reflect.Bool {
			return fmt.Errorf("cannot convert %T to bool", data)
		}
		dst.SetBool(src.Bool())
	default:
		return toml.Decode(data, dst.Addr().Interface())
	}
	return nil
}

// valueless reports a failed conversion without the input, which may be a
// secret misplaced into a numeric field and would reach logs and events
func valueless(kind string, err error) error {
	if ne, ok := errors.AsType[*strconv.NumError](err); ok {
		return fmt.Errorf("invalid %s: %w", kind, ne.Err)
	}
	return fmt.Errorf("invalid %s", kind)
}

// textList splits a string for a list path at commas: environment and
// command-line values are text, where a comma is the only list syntax
func textList(defaultValue, value any) any {
	s, ok := value.(string)
	t := reflect.TypeOf(defaultValue)
	if !ok || t == nil || t == ipType || t.Kind() != reflect.Slice && t.Kind() != reflect.Array || t.Elem().Kind() == reflect.Uint8 {
		return value
	}
	parts := []any{}
	if s != "" {
		for p := range strings.SplitSeq(s, ",") {
			parts = append(parts, p)
		}
	}
	return parts
}

func fieldKey(f reflect.StructField) string {
	key, _, _ := strings.Cut(f.Tag.Get(FormatTOML), ",")
	if key == "" {
		key = f.Name
	}
	return key
}

func normalizeMap(data any) (map[string]any, error) {
	if data == nil {
		return map[string]any{}, nil
	}
	if m, ok := data.(map[string]any); ok {
		return m, nil
	}
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("expected string-keyed map, got %T", data)
	}
	m := make(map[string]any, v.Len())
	it := v.MapRange()
	for it.Next() {
		m[it.Key().String()] = it.Value().Interface()
	}
	return m, nil
}

func navigateToPath(nested map[string]any, path string) any {
	if path == "" {
		return nested
	}
	var current any = nested
	for _, key := range strings.Split(strings.TrimSuffix(path, "."), ".") {
		m, err := normalizeMap(current)
		if err != nil {
			return nil
		}
		current = m[key]
	}
	return current
}
