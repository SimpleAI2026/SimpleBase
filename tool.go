package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
)

var importPkgs = map[string][]string{
	"bytes":           []string{},
	"container/heap":  []string{},
	"container/list":  []string{},
	"container/ring":  []string{},
	"crypto/md5":      []string{},
	"encoding/base64": []string{},
	"encoding/hex":    []string{},
	"encoding/xml":    []string{},
	"errors":          []string{},
	"html":            []string{},
	"math":            []string{},
	"math/rand":       []string{},
	"net/http":        []string{},
	"net/url":         []string{},
	"regexp":          []string{},
	"sort":            []string{},
	"strconv":         []string{},
	"strings":         []string{},
	"time":            []string{},
	"unicode":         []string{},
	"unicode/utf8":    []string{},
	"unicode/utf16":   []string{},
	"sync":            []string{},
	"sync/atomic":     []string{},
	"crypto/sha1":     []string{},
	"encoding/json":   []string{},
	"encoding/binary": []string{},
	"io/ioutil":       []string{"io"},
	"io":              []string{},
	"html/template":   []string{},
	"path":            []string{},
	"mime/multipart":  []string{},
	"crypto/des":      []string{},
	"crypto/cipher":   []string{},
	"crypto/tls":      []string{},
	"fmt":             []string{},
}

func builtinImportPkgs() error {
	for path, _ := range importPkgs {
		fmt.Println("=============> ", path)
		if path != "fmt" {
			continue
		}
		pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(path)
		if err != nil {
			fmt.Println("err: ", err)
			return err
		}
		scope := pkg.Scope()
		for _, declName := range pkg.Scope().Names() {
			if ast.IsExported(declName) {
				object := scope.Lookup(declName)
				name := fmt.Sprintf("%s.%s", object.Pkg().Name(), object.Name())
				switch object.(type) {
				case *types.TypeName:
					fmt.Printf(`register.NewType("%s", reflect.TypeOf(func(%s){}).In(0), "%s")`+"\n", object.Name(), name, "")
				case *types.Const:
					fmt.Printf(`register.NewConst("%s", %s, "%s")`+"\n", object.Name(), name, "")
				case *types.Var:
					switch object.Type().Underlying().(type) {
					case *types.Interface:
						fmt.Printf(`register.NewVar("%s", &%s, reflect.TypeOf(func (%s){}).In(0), "%s")`+"\n", object.Name(), name, object.Type().String(), "")
					default:
						fmt.Printf(`register.NewVar("%s", &%s, reflect.TypeOf(%s), "%s")`+"\n", object.Name(), name, name, "")
					}
				case *types.Func:
					fmt.Printf(`register.NewFunction("%s", %s, "%s")`+"\n", object.Name(), name, "")
				}
			}
		}
	}
	return nil
}

func main() {
	builtinImportPkgs()
}
