package vm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateEnv(t *testing.T) {
	valid := map[string]string{"PATH": "/bin", "_EMPTY": "", "A1": "value"}
	if err := ValidateEnv(valid); err != nil {
		t.Fatal(err)
	}
	for name, env := range map[string]map[string]string{
		"lowercase key": {"lower": "value"},
		"empty key":     {"": "value"},
		"reserved":      {"PLATFORM_TOKEN": "secret"},
		"long value":    {"KEY": strings.Repeat("x", 4097)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateEnv(env, "PLATFORM_"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	tooMany := make(map[string]string, 33)
	for i := range 33 {
		tooMany[fmt.Sprintf("K%d", i)] = ""
	}
	if err := ValidateEnv(tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many entries: %v", err)
	}
}
