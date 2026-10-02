package main

import "testing"

func TestPostIdentifier(t *testing.T) {
	value := uuid()
	if len(value) != 32 {
		t.Fatalf("expected 32-char post identifier, got %q", value)
	}
	if value == uuid() {
		t.Fatal("random identifiers repeated")
	}
}
