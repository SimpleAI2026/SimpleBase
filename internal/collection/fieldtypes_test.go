package collection

import (
	"errors"
	"testing"
)

func TestValidateDocument(t *testing.T) {
	max := 5.0
	schema := &Collection{Fields: []Field{
		{Name: "title", Type: "text", Required: true, Options: FieldOptions{Max: &max}},
		{Name: "state", Type: "select", Options: FieldOptions{Values: []string{"open", "closed"}}},
	}}
	if _, err := ValidateDocument(schema, map[string]any{"title": "too long", "state": "bad"}, true); err == nil || err.Code != "field_too_long" {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ValidateDocument(schema, map[string]any{"title": "ok", "extra": 1}, true); err == nil || err.Code != "unknown_field" {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, err := ValidateDocument(schema, map[string]any{"title": "ok", "state": "open"}, true); err != nil || got["title"] != "ok" {
		t.Fatalf("valid document rejected: %v", err)
	}
}

func TestMergePatch(t *testing.T) {
	got := MergePatch(map[string]any{"a": 1.0, "nested": map[string]any{"x": 1.0}}, map[string]any{"a": nil, "nested": map[string]any{"y": 2.0}})
	if _, ok := got["a"]; ok || got["nested"].(map[string]any)["x"] != 1.0 {
		t.Fatalf("unexpected patch: %#v", got)
	}
}

func TestValidationError(t *testing.T) {
	var target *ValidationError
	if !errors.As(&ValidationError{Code: "x"}, &target) {
		t.Fatal("error type mismatch")
	}
}
