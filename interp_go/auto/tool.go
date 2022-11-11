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
	"github.com/linkxzhou/gongx/packages/server": []string{},
}

func builtinImportPkgs() error {
	var builtinStr = ""
	var importStr = ""
	for path, _ := range importPkgs {
		pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(path)
		if err != nil {
			fmt.Println("err: ", err)
			return err
		}
		scope := pkg.Scope()
		namedTypes := ""
		vars := ""
		funcs := ""
		importStr += "\n" + `"` + path + `"`
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
					namedTypes = namedTypes + fmt.Sprintf(`"%s": reflect.TypeOf(func(%s){}).In(0),`+"\n", object.Name(), name)
				case *types.Const:
					// TODO: fix by linkxzhou
				case *types.Var:
					switch object.Type().Underlying().(type) {
					case *types.Interface:
						if !strings.Contains(object.Type().String(), "/") {
							vars = vars + fmt.Sprintf(`"%s": reflect.ValueOf(func (%s){}),`+"\n", object.Name(), object.Type().String())
						}
					default:
						vars = vars + fmt.Sprintf(`"%s": reflect.ValueOf(%s),`+"\n", object.Name(), name)
					}
				case *types.Func:
					funcs = funcs + fmt.Sprintf(`"%s": reflect.ValueOf(%s),`+"\n", object.Name(), name)
				}
			}
		}
		builtinStr += `
		RegisterPackage(&Package{
			Name:       "` + pkgName + `",
			Path:       "` + path + `",
			Deps:       make(map[string]string),
			Interfaces: map[string]reflect.Type{},
			NamedTypes: map[string]reflect.Type{` + namedTypes + `},
			AliasTypes: map[string]reflect.Type{},
			Vars:       map[string]reflect.Value{` + vars + `},
			Funcs: map[string]reflect.Value{` + funcs + `},
			TypedConsts:   map[string]TypedConst{},
			UntypedConsts: map[string]UntypedConst{},
		})
		`
	}
	fmt.Println(`
package loader

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

func main() {
	builtinImportPkgs()
}
