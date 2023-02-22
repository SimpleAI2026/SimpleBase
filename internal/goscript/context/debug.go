package context

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type DebugInfo struct {
	*ssa.DebugRef
	FSet    *token.FileSet
	ToValue func() (*types.Var, interface{}, bool) // var object value
}

func (i *DebugInfo) Position() token.Position {
	return i.FSet.Position(i.Pos())
}

func (i *DebugInfo) AsVar() (*types.Var, interface{}, bool) {
	return i.ToValue()
}

func (i *DebugInfo) AsFunc() (*types.Func, bool) {
	v, ok := i.Object().(*types.Func)
	return v, ok
}
