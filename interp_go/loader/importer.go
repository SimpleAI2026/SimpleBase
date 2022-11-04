package loader

import (
	"fmt"
	"go/importer"
	"go/types"
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

// RegisterPackage register pkg
func RegisterPackage(pkg *Package) {
	if p, ok := registerPkgs[pkg.Path]; ok {
		p.merge(pkg)
		return
	}
	registerPkgs[pkg.Path] = pkg
}

var registerPkgs = make(map[string]*Package)
