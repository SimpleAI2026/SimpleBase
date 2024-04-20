package utils

import (
	"errors"
	"os"
	"strings"
)

func split(s string) (string, string, error) {
	arr := strings.Split(s, "=")
	if len(arr) <= 0 {
		return "", "", errors.New("environ invalid")
	} else if len(arr) == 1 {
		return arr[0], "", nil
	} else {
		return arr[0], arr[1], nil
	}
}

// GetEnvironInfo get all environment map
func GetEnvironInfo() map[string]string {
	env := make(map[string]string, 4)
	environ := os.Environ()
	for k := range environ {
		if k1, v1, err := split(environ[k]); err == nil {
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
