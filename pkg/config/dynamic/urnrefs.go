package dynamic

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"
	"strconv"
	"strings"
)

// RFC 6901 escapes "~" as "~0" and "/" as "~1".
// A replacer works in a single pass, so it never reads its own output: "~01" decodes into "~1" and not into "/".
var (
	jsonPointerEscaper   = strings.NewReplacer("~", "~0", "/", "~1")
	jsonPointerUnescaper = strings.NewReplacer("~1", "/", "~0", "~")
)

// JSONPointer returns the JSON Pointer (RFC 6901) made of the given reference tokens.
func JSONPointer(tokens ...string) string {
	var b strings.Builder
	for _, token := range tokens {
		b.WriteByte('/')
		b.WriteString(jsonPointerEscaper.Replace(token))
	}

	return b.String()
}

var middlewareOriginalURNs = make(map[*Middleware]map[string]string)
var middlewareOriginalURNsMu sync.Mutex

func (m *Middleware) ResolveURNRefs() (*Middleware, error) {
	if len(m.URNRefs) == 0 {
		return m, nil
	}

	middlewareOriginalURNsMu.Lock()
	defer middlewareOriginalURNsMu.Unlock()

	orig, ok := middlewareOriginalURNs[m]
	if !ok {
		orig = make(map[string]string)
		middlewareOriginalURNs[m] = orig
	}

	resolved := m
	for _, pointer := range slices.Sorted(maps.Keys(m.URNRefs)) {
		val := m.URNRefs[pointer]
		if urn, err := resolved.swapURNRef(pointer, val); err != nil {
			return nil, fmt.Errorf("resolving URN reference %q: %w", pointer, err)
		} else if strings.HasPrefix(urn, "urn:k8s:secret:") {
			orig[pointer] = urn
		}
	}

	return resolved, nil
}

func (m *Middleware) RedactURNRefs(err error) error {
	if err == nil || len(m.URNRefs) == 0 {
		return err
	}

	middlewareOriginalURNsMu.Lock()
	defer middlewareOriginalURNsMu.Unlock()

	orig := middlewareOriginalURNs[m]
	var urns []string
	if orig != nil {
		for pointer := range m.URNRefs {
			if urn, ok := orig[pointer]; ok && urn != "" {
				urns = append(urns, urn)
			}
		}
	}

	if len(urns) == 0 {
		return err
	}

	slices.Sort(urns)
	urns = slices.Compact(urns)

	return fmt.Errorf("plugin configuration using %s is invalid: configuration decoding failed",
		strings.Join(urns, ", "))
}

// swapURNRef sets the value at the JSON Pointer, and returns the URN that it replaced.
func (m *Middleware) swapURNRef(pointer, value string) (urn string, err error) {
	path, err := parseJSONPointer(pointer)
	if err != nil {
		return "", err
	}

	return swapStringAt(reflect.ValueOf(m).Elem(), path, value)
}

// swapStringAt follows the path from v, sets the string at its end to value, and returns the old string.
// v must be settable, so the caller gives a copy of the configuration.
func swapStringAt(v reflect.Value, path []string, value string) (old string, err error) {
	switch v.Kind() {
	case reflect.String:
		if len(path) > 0 {
			return "", fmt.Errorf("path continues after a string value: %q", path)
		}

		old = v.String()
		v.SetString(value)

		return old, nil

	case reflect.Pointer:
		if v.IsNil() {
			return "", errors.New("nil value on the path")
		}

		return swapStringAt(v.Elem(), path, value)

	case reflect.Interface:
		if v.IsNil() {
			return "", errors.New("nil value on the path")
		}

		// Go cannot change the value held by an interface in place, so swapStringAt changes a copy and stores it back.
		elem := reflect.New(v.Elem().Type()).Elem()
		elem.Set(v.Elem())
		old, err = swapStringAt(elem, path, value)
		v.Set(elem)

		return old, err
	}

	// Pointers and interfaces lead to the next value without a token, so the path can only end on a string.
	if len(path) == 0 {
		return "", fmt.Errorf("value of type %s is not a string", v.Type())
	}

	switch v.Kind() {
	case reflect.Struct:
		// The pointers use the JSON names of the fields, so a field without such a name never matches.
		for i := range v.NumField() {
			field := v.Type().Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if field.IsExported() && name != "" && name != "-" && name == path[0] {
				return swapStringAt(v.Field(i), path[1:], value)
			}
		}

		return "", fmt.Errorf("unknown field %q", path[0])

	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return "", fmt.Errorf("unsupported map key of type %s on the path", v.Type().Key())
		}

		key := reflect.ValueOf(path[0]).Convert(v.Type().Key())
		if !v.MapIndex(key).IsValid() {
			return "", fmt.Errorf("unknown key %q", path[0])
		}

		// Go cannot change a map value in place, so swapStringAt changes a copy and stores it back.
		elem := reflect.New(v.Type().Elem()).Elem()
		elem.Set(v.MapIndex(key))
		old, err = swapStringAt(elem, path[1:], value)
		v.SetMapIndex(key, elem)

		return old, err

	case reflect.Slice:
		index, err := strconv.Atoi(path[0])
		// RFC 6901 only allows "0" or digits without a leading zero, so the index must be written as Itoa writes it.
		if err != nil || strconv.Itoa(index) != path[0] || index < 0 || index >= v.Len() {
			return "", fmt.Errorf("invalid array index %q", path[0])
		}

		return swapStringAt(v.Index(index), path[1:], value)

	default:
		return "", fmt.Errorf("unsupported value of type %s on the path", v.Type())
	}
}

func parseJSONPointer(pointer string) ([]string, error) {
	rest, ok := strings.CutPrefix(pointer, "/")
	if !ok {
		return nil, errors.New("JSON Pointer must start with a slash")
	}

	path := strings.Split(rest, "/")
	for i, token := range path {
		path[i] = jsonPointerUnescaper.Replace(token)
	}

	return path, nil
}
