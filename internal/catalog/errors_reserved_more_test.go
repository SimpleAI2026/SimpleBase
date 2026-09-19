package catalog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsNotFoundAndAlreadyExists(t *testing.T) {
	if IsNotFound(nil) || IsAlreadyExists(nil) {
		t.Fatal("nil should not match")
	}
	if !IsNotFound(ErrNotFound) || !IsAlreadyExists(ErrAlreadyExists) {
		t.Fatal("sentinel should match")
	}
	if !IsNotFound(fmt.Errorf("%w: db", ErrNotFound)) {
		t.Fatal("wrapped not found")
	}
	if !IsAlreadyExists(fmt.Errorf("%w: name", ErrAlreadyExists)) {
		t.Fatal("wrapped already exists")
	}
	if IsNotFound(ErrAlreadyExists) || IsAlreadyExists(ErrNotFound) {
		t.Fatal("cross match")
	}
	if IsNotFound(errors.New("other")) || IsAlreadyExists(errors.New("other")) {
		t.Fatal("unrelated")
	}
}

func TestStateTransitionError(t *testing.T) {
	e := &StateTransitionError{From: []DatabaseStatus{DatabaseReady, DatabaseClosed}, To: DatabaseCreating}
	msg := e.Error()
	if !strings.Contains(msg, "ready") || !strings.Contains(msg, "closed") || !strings.Contains(msg, "creating") {
		t.Fatalf("error text: %s", msg)
	}
	if !errors.Is(e, ErrInvalidState) {
		t.Fatal("Unwrap should yield ErrInvalidState")
	}
	empty := &StateTransitionError{To: DatabaseReady}
	if empty.Error() == "" {
		t.Fatal("empty from should still format")
	}
}

func TestReservedHelpers(t *testing.T) {
	if !IsSystemProject(ReservedSystemProjectID) {
		t.Fatal("system project")
	}
	if IsSystemProject(DevProjectID) || IsSystemProject("") {
		t.Fatal("non-system project")
	}
	if !IsSystemDatabase(Database{Kind: DatabaseKindSystem}) {
		t.Fatal("system db")
	}
	if IsSystemDatabase(Database{Kind: DatabaseKindUser}) || IsSystemDatabase(Database{}) {
		t.Fatal("user/empty kind is not system")
	}
}

func TestValidateName(t *testing.T) {
	if err := validateName("ok"); err != nil {
		t.Fatal(err)
	}
	if err := validateName(""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("empty: %v", err)
	}
	if err := validateName(strings.Repeat("a", nameMaxLen+1)); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("too long: %v", err)
	}
	if err := validateName("a/b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("slash: %v", err)
	}
	if err := validateName("a\\b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("backslash: %v", err)
	}
	if err := validateName("bad\nname"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("control: %v", err)
	}
}

func TestIsUniqueViolationAndNullTime(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Fatal("nil")
	}
	for _, msg := range []string{"UNIQUE constraint failed: sys_tenants.id", "constraint failed: UNIQUE", "duplicate key value"} {
		if !isUniqueViolation(errors.New(msg)) {
			t.Fatalf("should match %q", msg)
		}
	}
	if isUniqueViolation(errors.New("syntax error")) {
		t.Fatal("unrelated")
	}
	if nullTime(nil) != nil {
		t.Fatal("nil time")
	}
	now := time.Now()
	if nullTime(&now) == nil {
		t.Fatal("non-nil time")
	}
}
