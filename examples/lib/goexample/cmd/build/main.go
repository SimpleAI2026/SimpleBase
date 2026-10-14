package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./examples/lib/goexample/cmd/build <case-directory>")
	}
	dir := os.Args[1]
	dist := filepath.Join(dir, "dist")
	if err := os.RemoveAll(dist); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dist, 0755); err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"index.html", "style.css", "app.js"} {
		in, err := os.Open(filepath.Join(dir, "web", name))
		if err != nil {
			log.Fatal(err)
		}
		out, err := os.Create(filepath.Join(dist, name))
		if err != nil {
			_ = in.Close()
			log.Fatal(err)
		}
		_, err = io.Copy(out, in)
		_ = in.Close()
		closeErr := out.Close()
		if err != nil {
			log.Fatal(err)
		}
		if closeErr != nil {
			log.Fatal(closeErr)
		}
	}
	fmt.Printf("Built %s\n", dist)
}
