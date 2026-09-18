package importer

import (
	"go/token"
	"go/types"
	"reflect"
)

func (p *Importer) parseNameType(t reflect.Type) (named *types.Named) {
	// 首先检查缓存
	if cached, exists := p.typeCache[t]; exists {
		if namedType, ok := cached.(*types.Named); ok {
			return namedType
		}
	}

	pkg := p.Package(t.PkgPath())
	name := t.Name()
	if pkg != nil {
		scope := pkg.Scope()
		obj := scope.Lookup(name)
		if obj == nil {
			typeName := types.NewTypeName(token.NoPos, pkg, name, nil)
			named = types.NewNamed(typeName, nil, nil)
			scope.Insert(typeName)
			obj = typeName
		} else {
			named = obj.Type().(*types.Named)
		}
	} else {
		typeName := types.NewTypeName(token.NoPos, pkg, name, nil)
		named = types.NewNamed(typeName, nil, nil)
	}

	// 将结果存储到缓存中
	p.typeCache[t] = named
	return named
}
