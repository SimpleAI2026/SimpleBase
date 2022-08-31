package interp

import (
	"fmt"
	"reflect"

	"github.com/linkxzhou/gongx/loader"
	"golang.org/x/tools/go/ssa"
)

func checkPackages(intp *Interp, pkgs []*ssa.Package) (err error) {
	if intp.ctx.Mode&loader.DisableRecover == 0 {
		defer func() {
			if v := recover(); v != nil {
				err = v.(error)
			}
		}()
	}

	visit := visitor{
		intp: intp,
		prog: intp.mainpkg.Prog,
		pkgs: make(map[*ssa.Package]bool),
		seen: make(map[*ssa.Function]bool),
	}

	for _, pkg := range pkgs {
		visit.pkgs[pkg] = true
	}

	visit.program()
	return
}

type visitor struct {
	intp *Interp
	prog *ssa.Program
	pkgs map[*ssa.Package]bool
	seen map[*ssa.Function]bool
}

func (visit *visitor) program() {
	chks := make(map[string]bool)
	chks[""] = true // anonymous struct embed named type
	for pkg := range visit.pkgs {
		chks[pkg.Pkg.Path()] = true
		for _, mem := range pkg.Members {
			if fn, ok := mem.(*ssa.Function); ok {
				visit.function(fn)
			}
		}
	}
	isExtern := func(typ reflect.Type) bool {
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		return !chks[typ.PkgPath()]
	}
	for _, T := range visit.prog.RuntimeTypes() {
		typ := visit.intp.preToType(T)
		// skip extern type
		if isExtern(typ) {
			continue
		}
		mmap := make(map[string]*ssa.Function)
		mset := visit.prog.MethodSets.MethodSet(T)
		for i, n := 0, mset.Len(); i < n; i++ {
			sel := mset.At(i)
			obj := sel.Obj()
			if pkg := obj.Pkg(); pkg != nil && !chks[pkg.Path()] {
				continue
			}
			fn := visit.prog.MethodValue(sel)
			mmap[obj.Name()] = fn
			visit.function(fn)
		}
		visit.intp.msets[typ] = mmap
	}
}

func (visit *visitor) function(fn *ssa.Function) {
	if visit.seen[fn] {
		return
	}
	visit.seen[fn] = true
	fnPath := fn.String()
	if f, ok := visit.intp.ctx.Override[fnPath]; ok &&
		visit.intp.preToType(fn.Type()) == f.Type() {
		fn.Blocks = nil
		return
	}
	if fn.Blocks == nil {
		if _, ok := visit.pkgs[fn.Pkg]; ok {
			if _, ok = externValues[fnPath]; !ok {
				panic(fmt.Errorf("%v: missing function body", visit.intp.ctx.FileSet.Position(fn.Pos())))
			}
		}
		return
	}
	visit.intp.loadType(fn.Type())
	for _, alloc := range fn.Locals {
		visit.intp.loadType(alloc.Type())
		visit.intp.loadType(deref(alloc.Type()))
	}
	pfn := visit.intp.loadFunction(fn)
	for _, p := range fn.Params {
		pfn.regIndex(p)
	}
	for _, p := range fn.FreeVars {
		pfn.regIndex(p)
	}
	var opbuf [32]*ssa.Value // avoid alloc in common case
	for _, b := range fn.Blocks {
		instrs := make([]func(*frame), len(b.Instrs))
		ssaInstrs := make([]ssa.Instruction, len(b.Instrs))
		var index int
		n := len(b.Instrs)
		for i := 0; i < n; i++ {
			instr := b.Instrs[i]
			ops := instr.Operands(opbuf[:0])
			switch instr := instr.(type) {
			case *ssa.Alloc:
				visit.intp.loadType(instr.Type())
				visit.intp.loadType(deref(instr.Type()))
			case *ssa.Next:
				// skip *ssa.opaqueType: iter
				ops = nil
			case *ssa.Extract:
				// skip
				ops = nil
			case *ssa.TypeAssert:
				visit.intp.loadType(instr.AssertedType)
			case *ssa.MakeChan:
				visit.intp.loadType(instr.Type())
			case *ssa.MakeMap:
				visit.intp.loadType(instr.Type())
			case *ssa.MakeSlice:
				visit.intp.loadType(instr.Type())
			case *ssa.SliceToArrayPointer:
				visit.intp.loadType(instr.Type())
			case *ssa.Convert:
				visit.intp.loadType(instr.Type())
			case *ssa.ChangeType:
				visit.intp.loadType(instr.Type())
			case *ssa.MakeInterface:
				visit.intp.loadType(instr.Type())
			}
			for _, op := range ops {
				switch v := (*op).(type) {
				case *ssa.Function:
					visit.function(v)
				case nil:
					// skip
				default:
					visit.intp.loadType(v.Type())
				}
			}
			ifn := makeInstr(visit.intp, pfn, instr)
			if ifn == nil {
				continue
			}
			instrs[index] = ifn
			ssaInstrs[index] = instr
			index++
		}
		instrsSlice := instrs[:index]
		offset := len(pfn.Instrs)
		pfn.Blocks = append(pfn.Blocks, offset)
		pfn.Instrs = append(pfn.Instrs, instrsSlice...)
		pfn.InstrsLen = len(pfn.Instrs)
		pfn.ssaInstrs = append(pfn.ssaInstrs, ssaInstrs...)
		if b == fn.Recover && (visit.intp.ctx.Mode&loader.DisableRecover == 0) {
			pfn.Recover = pfn.Instrs[offset:]
		}
	}
	pfn.initPool()
}
