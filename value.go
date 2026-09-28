package config

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"reflect"
	"time"
	"unicode/utf8"
)

const maxValueDepth = 128

var (
	timeType     = reflect.TypeFor[time.Time]()
	durationType = reflect.TypeFor[time.Duration]()
	ipType       = reflect.TypeFor[net.IP]()
	ipNetType    = reflect.TypeFor[net.IPNet]()
	urlType      = reflect.TypeFor[url.URL]()
	userInfoType = reflect.TypeFor[url.Userinfo]()
)

func atomicType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == timeType || t == ipType || t == ipNetType || t == urlType
}

// copyValue validates the supported value graph and detaches mutable containers.
// The depth bound rejects cycles without retaining a global identity cache.
func copyValue(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	out, err := copyReflect(reflect.ValueOf(v), 0)
	if err != nil {
		return nil, wrapError(ErrTypeMismatch, err)
	}
	return out.Interface(), nil
}

func copyReflect(v reflect.Value, depth int) (reflect.Value, error) {
	if depth > maxValueDepth {
		return reflect.Value{}, fmt.Errorf("value nesting exceeds %d (possible cycle)", maxValueDepth)
	}
	if v.Type() == ipType && v.Len() != 0 && v.Interface().(net.IP).To16() == nil {
		return reflect.Value{}, fmt.Errorf("invalid IP value")
	}
	if v.Type() == ipNetType {
		n := v.Interface().(net.IPNet)
		_, bits := n.Mask.Size()
		if (len(n.IP) != 0 || len(n.Mask) != 0) && (n.IP.To16() == nil || bits == 0 || bits == 32 && n.IP.To4() == nil) {
			return reflect.Value{}, fmt.Errorf("invalid CIDR value")
		}
	}
	if v.Type() == timeType {
		if _, err := v.Interface().(time.Time).MarshalText(); err != nil {
			return reflect.Value{}, err
		}
	}
	out := reflect.New(v.Type()).Elem()
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return out, nil
		}
		elem, err := copyReflect(v.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		if v.Kind() == reflect.Pointer {
			out.Set(reflect.New(v.Type().Elem()))
			out.Elem().Set(elem)
		} else {
			out.Set(elem)
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, fmt.Errorf("map keys must be strings")
		}
		if v.IsNil() {
			return out, nil
		}
		out.Set(reflect.MakeMapWithSize(v.Type(), v.Len()))
		it := v.MapRange()
		for it.Next() {
			if !utf8.ValidString(it.Key().String()) {
				return reflect.Value{}, fmt.Errorf("invalid UTF-8 map key")
			}
			x, err := copyReflect(it.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.SetMapIndex(it.Key(), x)
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice {
			if v.IsNil() {
				return out, nil
			}
			out.Set(reflect.MakeSlice(v.Type(), v.Len(), v.Len()))
		}
		for i := 0; i < v.Len(); i++ {
			x, err := copyReflect(v.Index(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(x)
		}
	case reflect.Struct:
		// These standard-library values contain immutable private state.
		if v.Type() == timeType || v.Type() == userInfoType {
			out.Set(v)
			return out, nil
		}
		exported := false
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			exported = true
			x, err := copyReflect(v.Field(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Field(i).Set(x)
		}
		if !exported && v.NumField() != 0 {
			return reflect.Value{}, fmt.Errorf("unsupported private state in %s", v.Type())
		}
	case reflect.String:
		if len(v.String()) > MaxValueSize {
			return reflect.Value{}, ErrValueSize
		}
		if !utf8.ValidString(v.String()) {
			return reflect.Value{}, fmt.Errorf("invalid UTF-8 string")
		}
		out.Set(v)
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return reflect.Value{}, fmt.Errorf("non-finite float")
		}
		out.Set(v)
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		out.Set(v)
	default:
		return reflect.Value{}, fmt.Errorf("unsupported value type %s", v.Type())
	}
	return out, nil
}

// cloneOwned is used only on values already accepted by copyValue.
func cloneOwned(v any) any {
	out, err := copyValue(v)
	if err != nil {
		panic("config: invalid internal value: " + err.Error())
	}
	return out
}
