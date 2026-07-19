package database

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestSerializeValue_Nil(t *testing.T) {
	v, err := SerializeValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestSerializeValue_Int64(t *testing.T) {
	v, err := SerializeValue(int64(42))
	if err != nil {
		t.Fatal(err)
	}
	if v.(int64) != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestSerializeValue_Float64(t *testing.T) {
	v, err := SerializeValue(float64(3.14))
	if err != nil {
		t.Fatal(err)
	}
	if v.(float64) != 3.14 {
		t.Errorf("expected 3.14, got %v", v)
	}
}

func TestSerializeValue_Bool(t *testing.T) {
	v, err := SerializeValue(true)
	if err != nil {
		t.Fatal(err)
	}
	if v.(bool) != true {
		t.Errorf("expected true, got %v", v)
	}
}

func TestSerializeValue_String(t *testing.T) {
	v, err := SerializeValue("hello")
	if err != nil {
		t.Fatal(err)
	}
	if v.(string) != "hello" {
		t.Errorf("expected hello, got %v", v)
	}
}

func TestSerializeValue_Bytes(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0xFF}
	v, err := SerializeValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	bv, ok := v.(BlobValue)
	if !ok {
		t.Fatalf("expected BlobValue, got %T", v)
	}
	if bv.Type != "blob" {
		t.Errorf("expected type blob, got %s", bv.Type)
	}
	decoded, err := base64.StdEncoding.DecodeString(bv.Base64)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(raw) {
		t.Fatalf("decoded length mismatch: %d vs %d", len(decoded), len(raw))
	}
	for i := range raw {
		if decoded[i] != raw[i] {
			t.Fatalf("byte %d mismatch", i)
		}
	}
}

func TestSerializeValue_Time(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 678901000, time.UTC)
	v, err := SerializeValue(now)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T", v)
	}
	want := now.Format(time.RFC3339Nano)
	if s != want {
		t.Errorf("expected %s, got %s", want, s)
	}
}

func TestSerializeValue_Unsupported(t *testing.T) {
	_, err := SerializeValue(complex(1, 2))
	if !errors.Is(err, ErrUnsupportedValue) {
		t.Errorf("expected ErrUnsupportedValue, got %v", err)
	}
}

func TestSerializeRow_MixedTypes(t *testing.T) {
	row := []any{int64(1), "abc", nil, []byte{0x01, 0x02}, true}
	out, err := SerializeRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(row) {
		t.Fatalf("length mismatch: %d vs %d", len(out), len(row))
	}
}

func TestSerializeRow_UnsupportedStopsAtError(t *testing.T) {
	row := []any{int64(1), complex(1, 2)}
	_, err := SerializeRow(row)
	if !errors.Is(err, ErrUnsupportedValue) {
		t.Errorf("expected ErrUnsupportedValue, got %v", err)
	}
}

func TestSerializeRows_AllValid(t *testing.T) {
	rows := [][]any{
		{int64(1), "a"},
		{int64(2), "b"},
	}
	out, err := SerializeRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
}
