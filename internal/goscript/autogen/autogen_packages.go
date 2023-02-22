package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"strings"
)

var importPkgs = map[string][]string{
	"bytes":           nil,
	"container/heap":  nil,
	"container/list":  nil,
	"container/ring":  nil,
	"crypto/md5":      nil,
	"encoding/base64": nil,
	"encoding/hex":    nil,
	"encoding/xml":    nil,
	"errors":          nil,
	"html":            nil,
	"math":            nil,
	"math/rand":       nil,
	"net/http":        nil,
	"net/url":         nil,
	"regexp":          nil,
	"sort":            nil,
	"strconv":         nil,
	"strings":         nil,
	"time":            nil,
	"unicode":         nil,
	"unicode/utf8":    nil,
	"unicode/utf16":   nil,
	"sync":            nil,
	"sync/atomic":     nil,
	"crypto/sha1":     nil,
	"encoding/json":   nil,
	"encoding/binary": nil,
	"io/ioutil":       []string{"io"},
	"io":              nil,
	"html/template":   nil,
	"path":            nil,
	"mime/multipart":  nil,
	"crypto/des":      nil,
	"crypto/cipher":   nil,
	"crypto/tls":      nil,
	"fmt":             nil,
	"github.com/linkxzhou/goedge/packages/server": nil,
}

const (
	tab1Str = `	`
	tab3Str = `			`
)

func generateImportPkgs() error {
	var builtinStr = ""
	var importStr = ""
	for path := range importPkgs {
		pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(path)
		if err != nil {
			fmt.Println("err: ", err)
			return err
		}
		scope := pkg.Scope()
		namedTypes := ""
		vars := ""
		funcs := ""
		importStr += "\n" + tab1Str + `"` + path + `"`
		pkgName := path
		if arr := strings.Split(pkgName, "/"); len(arr) > 0 {
			pkgName = arr[len(arr)-1]
		}
		for _, declName := range pkg.Scope().Names() {
			if ast.IsExported(declName) {
				object := scope.Lookup(declName)
				name := fmt.Sprintf("%s.%s", object.Pkg().Name(), object.Name())
				switch object.(type) {
				case *types.TypeName:
					namedTypes = namedTypes + fmt.Sprintf(tab3Str+`"%s": reflect.TypeOf(func(%s){}).In(0),`+"\n", object.Name(), name)
				case *types.Const:
					// TODO: fix by linkxzhou
				case *types.Var:
					switch object.Type().Underlying().(type) {
					case *types.Interface:
						if !strings.Contains(object.Type().String(), "/") {
							vars = vars + fmt.Sprintf(tab3Str+`"%s": reflect.ValueOf(func (%s){}),`+"\n", object.Name(), object.Type().String())
						}
					default:
						vars = vars + fmt.Sprintf(tab3Str+`"%s": reflect.ValueOf(%s),`+"\n", object.Name(), name)
					}
				case *types.Func:
					funcs = funcs + fmt.Sprintf(tab3Str+`"%s": reflect.ValueOf(%s),`+"\n", object.Name(), name)
				}
			}
		}
		builtinStr += `
	registerPackage(&Package{
		Name:       "` + pkgName + `",
		Path:       "` + path + `",
		Deps:       make(map[string]string),
		Interfaces: map[string]reflect.Type{},
		NamedTypes: map[string]reflect.Type{
` + namedTypes + `
		},
		AliasTypes: map[string]reflect.Type{},
		Vars:       map[string]reflect.Value{
` + vars + `
		},
		Funcs: map[string]reflect.Value{
` + funcs + `
		},
		TypedConsts:   map[string]TypedConst{},
		UntypedConsts: map[string]UntypedConst{},
	})
		`
	}

	fmt.Println(`
// This is generate file
package importer

import (
	"reflect"
	` + importStr + `
)

func init() {
	` + builtinStr + `
}
	`)

	return nil
}
