//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gosandbox-inner is only supported on linux")
	os.Exit(1)
}
