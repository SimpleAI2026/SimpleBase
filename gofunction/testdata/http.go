//go:build ignore

package main

import (
	"io"
	"net/http"
)

func fetch(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
