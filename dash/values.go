package dash

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Values is a decoded configuration document addressed by dotted key path,
// so a panel reads "weather.latitude" without anyone declaring a struct for
// it.
//
// It keeps the raw decoded object, so a key this build does not recognise
// survives a load and save unchanged. That is deliberately better than a
// typed struct, which silently drops anything it has no field for - and it is
// what lets panels be added and removed without rewriting a user's file.
type Values struct {
	raw map[string]any
}

// NewValues returns an empty document.
func NewValues() Values { return Values{raw: map[string]any{}} }

// LoadValues decodes a configuration document.
func LoadValues(data []byte) (Values, error) {
	values := NewValues()
	if len(data) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(data, &values.raw); err != nil {
		return NewValues(), err
	}
	return values, nil
}

// Bytes encodes the document in the same shape it was read in: two-space
// indent with a trailing newline, matching what the app writes today.
func (v Values) Bytes() ([]byte, error) {
	data, err := json.MarshalIndent(v.raw, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Clone returns a deep copy, so a settings form can be edited and discarded.
func (v Values) Clone() Values {
	if v.raw == nil {
		return NewValues()
	}
	data, err := json.Marshal(v.raw)
	if err != nil {
		return NewValues()
	}
	clone, err := LoadValues(data)
	if err != nil {
		return NewValues()
	}
	return clone
}

// Keys lists every key path present in the document, dotted, in no order.
// Used to check that every stored key is claimed by some panel's schema.
func (v Values) Keys() []string {
	return appendKeys(nil, "", v.raw)
}

func appendKeys(out []string, prefix string, node map[string]any) []string {
	for key, value := range node {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok {
			out = appendKeys(out, path, nested)
			continue
		}
		out = append(out, path)
	}
	return out
}

// lookup walks a dotted path, returning the value and whether it was present.
func (v Values) lookup(path string) (any, bool) {
	node := v.raw
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if i == len(parts)-1 {
			value, ok := node[part]
			return value, ok
		}
		next, ok := node[part].(map[string]any)
		if !ok {
			return nil, false
		}
		node = next
	}
	return nil, false
}

// String returns a string value, or "" when absent or of another type.
func (v Values) String(path string) string {
	value, ok := v.lookup(path)
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

// Has reports whether a key is present. A panel needs this to tell "absent"
// from "false": a boolean setting whose default is true cannot be read with
// Bool alone, because an unset key and a deliberate false look identical.
func (v Values) Has(path string) bool {
	_, ok := v.lookup(path)
	return ok
}

// Bool returns a boolean value, or false when absent.
func (v Values) Bool(path string) bool {
	value, _ := v.lookup(path)
	typed, _ := value.(bool)
	return typed
}

// Float returns a number, or 0 when absent. JSON has one number type, so a
// value written as an integer reads back here unchanged.
func (v Values) Float(path string) float64 {
	value, ok := v.lookup(path)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// List splits a comma-separated value, trimming blanks. It is the shape every
// multi-entry setting in the file already uses.
func (v Values) List(path string) []string {
	var out []string
	for _, part := range strings.Split(v.String(path), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Delete removes a key. Removing a key that is not there is not an error.
func (v *Values) Delete(path string) {
	if v.raw == nil {
		return
	}
	node := v.raw
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if i == len(parts)-1 {
			delete(node, part)
			return
		}
		next, ok := node[part].(map[string]any)
		if !ok {
			return
		}
		node = next
	}
}

// Merge overlays another document onto this one, with the other winning.
// Objects present in both are merged key by key rather than replaced, so
// overlaying one nested field does not drop its siblings.
func (v *Values) Merge(other Values) {
	if v.raw == nil {
		v.raw = map[string]any{}
	}
	mergeInto(v.raw, other.raw)
}

func mergeInto(dst, src map[string]any) {
	for key, value := range src {
		nested, isObject := value.(map[string]any)
		existing, wasObject := dst[key].(map[string]any)
		if isObject && wasObject {
			mergeInto(existing, nested)
			continue
		}
		dst[key] = value
	}
}

// Set writes a value, creating intermediate objects as needed.
func (v *Values) Set(path string, value any) {
	if v.raw == nil {
		v.raw = map[string]any{}
	}
	node := v.raw
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if i == len(parts)-1 {
			node[part] = value
			return
		}
		next, ok := node[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[part] = next
		}
		node = next
	}
}
