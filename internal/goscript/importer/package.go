package importer

import (
	"go/constant"
	"reflect"
)

type (
	TypedConst struct {
		Typ   reflect.Type
		Value constant.Value
	}

	UntypedConst struct {
		Typ   string
		Value constant.Value
	}

	Package struct {
		Interfaces    map[string]reflect.Type
		NamedTypes    map[string]reflect.Type
		AliasTypes    map[string]reflect.Type
		Vars          map[string]reflect.Value
		Funcs         map[string]reflect.Value
		TypedConsts   map[string]TypedConst
		UntypedConsts map[string]UntypedConst
		Deps          map[string]string // path -> name
		Name          string
		Path          string
	}
)

func (p *Package) merge(same *Package) {
	for k, v := range same.Interfaces {
		p.Interfaces[k] = v
	}
	for k, v := range same.NamedTypes {
		p.NamedTypes[k] = v
	}
	for k, v := range same.Vars {
		p.Vars[k] = v
	}
	for k, v := range same.Funcs {
		p.Funcs[k] = v
	}
	for k, v := range same.UntypedConsts {
		p.UntypedConsts[k] = v
	}
}

// register package
var registerPkgs = make(map[string]*Package)

func registerPackage(pkg *Package) {
	if p, ok := registerPkgs[pkg.Path]; ok {
		p.merge(pkg)
		return
	}
	registerPkgs[pkg.Path] = pkg
}
