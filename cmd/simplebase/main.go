package main

import (
	"os"

	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

func main() {
	os.Exit(sbcli.Run(os.Args[1:]))
}
