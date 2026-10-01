package jsonvalue

import "testing"

func TestStringRendersScalars(t *testing.T) {
	cases := map[string]any{"": nil, "x": "x", "4": 4, "1.5": 1.5, "1e+06": float64(1000000), "true": true}
	for want, value := range cases {
		if got := String(value); got != want {
			t.Fatalf("String(%#v) = %q, want %q", value, got, want)
		}
	}
}

func TestTextRendersIntegralNumbersInDecimal(t *testing.T) {
	cases := map[string]any{"": nil, "s": "s", "3": float64(3), "1000000": float64(1000000), "1.5": 1.5, "true": true}
	for want, value := range cases {
		if got := Text(value); got != want {
			t.Fatalf("Text(%#v) = %q, want %q", value, got, want)
		}
	}
}

func TestShapeNameNamesJSONShapes(t *testing.T) {
	cases := map[string]any{"an array": []any{}, "an object": map[string]any{}, "a number": 1.0, "a boolean": false, "int": 3, "<nil>": nil}
	for want, value := range cases {
		if got := ShapeName(value); got != want {
			t.Fatalf("ShapeName(%#v) = %q, want %q", value, got, want)
		}
	}
}
