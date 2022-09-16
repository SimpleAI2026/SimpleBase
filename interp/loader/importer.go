package loader

import (
	"fmt"
	"go/importer"
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
			"Errorf":   reflect.ValueOf(fmt.Errorf),
			"Fprint":   reflect.ValueOf(fmt.Fprint),
			"Fprintf":  reflect.ValueOf(fmt.Fprintf),
			"Fprintln": reflect.ValueOf(fmt.Fprintln),
			"Fscan":    reflect.ValueOf(fmt.Fscan),
			"Fscanf":   reflect.ValueOf(fmt.Fscanf),
			"Fscanln":  reflect.ValueOf(fmt.Fscanln),
			"Print":    reflect.ValueOf(fmt.Print),
			"Printf":   reflect.ValueOf(fmt.Printf),
			"Println":  reflect.ValueOf(fmt.Println),
			"Scan":     reflect.ValueOf(fmt.Scan),
			"Scanln":   reflect.ValueOf(fmt.Scanln),
			"Sprint":   reflect.ValueOf(fmt.Sprint),
			"Sprintf":  reflect.ValueOf(fmt.Sprintf),
			"Sprintln": reflect.ValueOf(fmt.Sprintln),
			"Sscan":    reflect.ValueOf(fmt.Sscan),
			"Sscanf":   reflect.ValueOf(fmt.Sscanf),
			"Sscanln":  reflect.ValueOf(fmt.Sscanln),
		},
		TypedConsts:   map[string]TypedConst{},
		UntypedConsts: map[string]UntypedConst{},
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

func init() {
	RegisterPackage(builtinPkg)
}
