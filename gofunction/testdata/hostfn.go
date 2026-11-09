//go:build ignore

package main

import "example.com/host"

func demo() int {
	_ = host.Label
	return host.Add(20, 22)
}
