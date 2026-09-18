//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func demo(name string) string {
	upper := strings.ToUpper(name)
	msg := fmt.Sprintf("hello %s", upper)
	b, err := json.Marshal(map[string]string{"msg": msg})
	if err != nil {
		return err.Error()
	}
	return string(b)
}
