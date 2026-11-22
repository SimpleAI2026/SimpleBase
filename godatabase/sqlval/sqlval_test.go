package sqlval

import (
	"testing"
	"time"
)

func TestBoolInt(t *testing.T) {
	if BoolInt(true) != 1 || BoolInt(false) != 0 {
		t.Fatal("BoolInt")
	}
}

func TestNullTime(t *testing.T) {
	if NullTime(nil) != nil {
		t.Fatal("nil should map to NULL")
	}
	loc := time.FixedZone("UTC+8", 8*3600)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)
	got, ok := NullTime(&ts).(time.Time)
	if !ok || got.Location() != time.UTC || !got.Equal(ts) {
		t.Fatalf("NullTime = %v", got)
	}
}

func TestNullTimeValue(t *testing.T) {
	if NullTimeValue(time.Time{}) != nil {
		t.Fatal("zero should map to NULL")
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	got, ok := NullTimeValue(ts).(time.Time)
	if !ok || got.Location() != time.UTC || !got.Equal(ts) {
		t.Fatalf("NullTimeValue = %v", got)
	}
}

func TestNullString(t *testing.T) {
	if NullString("") != nil || NullString("x") != "x" {
		t.Fatal("NullString")
	}
}
