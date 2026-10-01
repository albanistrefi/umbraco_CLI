// Package jsonvalue reads and describes values decoded from JSON into `any`
// (nil, string, float64, bool, []any, map[string]any).
package jsonvalue

import "fmt"

// String renders v as text: nil is empty, a string is returned as-is, and
// anything else goes through fmt.Sprint (so 1e6 renders as "1e+06").
func String(v any) string {
	if v == nil {
		return ""
	}
	text, ok := v.(string)
	if ok {
		return text
	}
	return fmt.Sprint(v)
}

// Text renders v like String, except that an integral JSON number prints in
// plain decimal ("16814", "1000000") instead of fmt's shortest form. Use it
// when a decoded number is an identifier or a key to compare against user
// input.
func Text(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		// JSON numbers decode as float64; render integers without trailing .0
		if value == float64(int64(value)) {
			return fmt.Sprintf("%d", int64(value))
		}
		return fmt.Sprintf("%v", value)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ShapeName names the JSON shape of v for error messages ("an array",
// "an object", "a number", "a boolean"), falling back to its Go type.
func ShapeName(v any) string {
	switch v.(type) {
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	default:
		return fmt.Sprintf("%T", v)
	}
}
