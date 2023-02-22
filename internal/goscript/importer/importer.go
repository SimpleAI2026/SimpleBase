package importer

import (
	"errors"
	"fmt"
	"go/importer"
	"go/types"

	"github.com/linkxzhou/goedge/internal/goscript/context"
)

var (
	ErrNotFoundImporter = errors.New("not found provider for types.Importer")
	ErrNotFoundPackage  = errors.New("not found package")
)

type Importer struct {
	pkgs        map[string]*types.Package
	importing   map[string]bool
	defaultImpl types.Importer
	ctx         *context.Context
	typesLoader *TypesLoader
}

func NewImporter(ctx *context.Context, typesLoader *TypesLoader) *Importer {
	return &Importer{
		pkgs:        make(map[string]*types.Package),
		importing:   make(map[string]bool),
		defaultImpl: importer.Default(),
		ctx:         ctx,
		typesLoader: typesLoader,
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
	if pkg, err := i.typesLoader.Import(path); err == nil && pkg.Complete() {
		i.pkgs[path] = pkg
		return pkg, nil
	}
	if pkg, _ := i.ctx.Import(path); pkg != nil {
		i.pkgs[path] = pkg
		return pkg, nil
	}
	return nil, ErrNotFoundPackage
}
