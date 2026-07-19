package utils

import (
	"errors"
	"os"
	"strings"
)

func split(s string) (string, string, error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", errors.New("environ invalid")
	}
	if len(parts) == 1 {
		return parts[0], "", nil
	}
	return parts[0], parts[1], nil
}

// GetEnvironInfo get all environment map
func GetEnvironInfo() map[string]string {
	env := make(map[string]string, 4)
	environ := os.Environ()
	for _, kv := range environ {
		if k1, v1, err := split(kv); err == nil {
			env[k1] = v1
		}
	}

	return env
}

var environInfo = GetEnvironInfo()

// GetEnviron only to read environment
func GetEnviron(name string) string {
	if v, ok := environInfo[name]; ok {
		return v
	}

	return EmptyNil
}
