package loader

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"reflect"
	"sort"
)

type Importer struct {
	ctx         *Context
	pkgs        map[string]*types.Package
	importing   map[string]bool
	defaultImpl types.Importer
}

func NewImporter(ctx *Context) *Importer {
	return &Importer{
		ctx:         ctx,
		pkgs:        make(map[string]*types.Package),
		importing:   make(map[string]bool),
		defaultImpl: importer.Default(),
	}
}

func (i *Importer) Import(path string) (*types.Package, error) {
	if pkg, ok := i.pkgs[path]; ok {
		return pkg, nil
	}
	if i.importing[path] {
		return nil, fmt.Errorf("cycle importing package %q", path)
	}
	i.importing[path] = true
	defer func() {
		i.importing[path] = false
	}()
	if pkg, err := i.ctx.Loader.Import(path); err == nil && pkg.Complete() {
		i.pkgs[path] = pkg
		return pkg, nil
	}
	if pkg, ok := i.ctx.pkgs[path]; ok {
		if !pkg.Package.Complete() {
			if err := pkg.Load(); err != nil {
				return nil, err
			}
		}
		i.pkgs[path] = pkg.Package
		return pkg.Package, nil
	}
	return nil, ErrNotFoundPackage
}

// TODO:
var (
	builtinPkg = &Package{
		Name:       "fmt",
		Path:       "fmt",
		Deps:       make(map[string]string),
		Interfaces: map[string]reflect.Type{},
		NamedTypes: map[string]reflect.Type{},
		AliasTypes: map[string]reflect.Type{},
		Vars:       map[string]reflect.Value{},
		Funcs: map[string]reflect.Value{
			"Println": reflect.ValueOf(fmt.Println),
		},
		TypedConsts:   map[string]TypedConst{},
		UntypedConsts: map[string]UntypedConst{},
	}

	importPkgs = map[string][]string{
		"bytes":           []string{},
		"container/heap":  []string{},
		"container/list":  []string{},
		"container/ring":  []string{},
		"crypto/md5":      []string{},
		"encoding/base64": []string{},
		"encoding/hex":    []string{},
		"encoding/xml":    []string{},
		"errors":          []string{},
		"fmt":             []string{},
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
	}

	registerPkgs = make(map[string]*Package)
)

// PackageList return all register packages
func PackageList() (list []string) {
	for pkg := range registerPkgs {
		list = append(list, pkg)
	}
	sort.Strings(list)
	return
}

// LookupPackage lookup register pkgs
func LookupPackage(name string) (pkg *Package, ok bool) {
	pkg, ok = registerPkgs[name]
	return
}

// RegisterPackage register pkg
func RegisterPackage(pkg *Package) {
	if p, ok := registerPkgs[pkg.Path]; ok {
		p.merge(pkg)
		return
	}
	registerPkgs[pkg.Path] = pkg
}

func builtinImportPkgs() error {
	for path, _ := range importPkgs {
		pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(path)
		if err != nil {
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

func init() {
	// builtinImportPkgs() // TODO:
	RegisterPackage(builtinPkg)
	fmt.Println("RegisterPackage: ", builtinPkg)
}
