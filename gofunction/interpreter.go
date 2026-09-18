package gofunction

import (
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	"github.com/linkxzhou/SimpleBase/gofunction/value"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Program 代码编译后生成的结构体
type Program struct {
	mainPkg   *ssa.Package
	globals   map[ssa.Value]*value.Value // 全局变量地址（不可变映射）
	importPkg []string                   // 导入的包信息
	importer  *importer.Importer         // 包导入器（解析外部对象）
}

// ParseFuncList 解析源码并返回函数名列表。
// exportedAll 为 true 时导出所有函数（含方法），否则仅导出包级导出函数。
func ParseFuncList(sourceCode string, exportedAll bool) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", sourceCode, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	var flist []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// fn.Recv != nil 表示 struct 成员方法
		if fn.Recv != nil && !exportedAll {
			continue
		}
		if exportedAll {
			flist = append(flist, fn.Name.Name)
			continue
		}
		if fn.Name.IsExported() && fn.Name.Name != "init" {
			flist = append(flist, fn.Name.Name)
		}
	}
	return flist, nil
}

// Run 编译并执行代码中的指定函数
func Run(seqid, sourceCode string, funcName string, params ...interface{}) (interface{}, error) {
	program, err := BuildProgram(seqid, "main", sourceCode)
	if err != nil {
		return nil, err
	}
	return program.Run(seqid, funcName, params...)
}

// autoImport 自动添加import语句
func autoImport(f *ast.File) []string {
	var importPkg []string
	imported := make(map[string]bool)
	for _, i := range f.Imports {
		imported[i.Path.Value] = true
		importPkg = append(importPkg, i.Path.Value)
	}
	for _, unresolved := range f.Unresolved {
		if doc.IsPredeclared(unresolved.Name) {
			continue
		}
		if importSpec := importer.GetPackageByName(unresolved.Name); importSpec != nil {
			if imported[importSpec.Path.Value] {
				continue
			}
			imported[importSpec.Path.Value] = true
			f.Imports = append(f.Imports, importSpec)
			f.Decls = append(f.Decls, &ast.GenDecl{
				Specs: []ast.Spec{importSpec},
			})
		}
	}
	return importPkg
}

// BuildProgram 编译代码。packages 为可选的已编译包（供脚本 import）
func BuildProgram(seqid, fname, sourceCode string, packages ...*ssa.Package) (*Program, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fname+".go", sourceCode, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	files := []*ast.File{f}

	importPkg := autoImport(f)
	pkg := types.NewPackage(f.Name.Name, f.Name.Name)

	packageImporter := importer.NewImporter(packages...)
	mode := ssa.SanityCheckFunctions | ssa.BareInits
	mainPkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: packageImporter}, fset, pkg, files, mode)
	if err != nil {
		return nil, err
	}
	context := newCallContext()
	program := &Program{
		mainPkg:   mainPkg,
		globals:   make(map[ssa.Value]*value.Value),
		importPkg: importPkg,
		importer:  packageImporter,
	}
	value.ExternalValueWrap(packageImporter, mainPkg)
	program.initGlobal()
	fr := &frame{
		program: program,
		context: context,
		seqid:   seqid,
	}
	if init := mainPkg.Func("init"); init != nil {
		for _, pkg := range packages {
			if dependInit := pkg.Func("init"); dependInit != nil {
				callSSA(fr, dependInit, nil, nil)
			}
		}
		// init初始化函数
		callSSA(fr, init, nil, nil)
	}
	context.cancelFunc()
	return program, nil
}

// Run 执行函数
func (p *Program) Run(seqid, funcName string, params ...interface{}) (interface{}, error) {
	val, _, err := p.RunWithContext(seqid, funcName, params...)
	return val, err
}

// RunWithContext 执行函数并返回上下文
func (p *Program) RunWithContext(seqid, funcName string, params ...interface{}) (result interface{}, ctx *Context, err error) {
	defer func() {
		if re := recover(); re != nil {
			err = fmt.Errorf("recover: %v", re)
		}
	}()

	ctx = newCallContext()
	mainFn := p.mainPkg.Func(funcName)
	if mainFn == nil {
		return nil, nil, errors.New("function not found")
	}
	if debugging {
		_, _ = mainFn.WriteTo(stderrWriter)
	}
	args := make([]value.Value, len(params))
	for i := range args {
		args[i] = value.ValueOf(params[i])
	}
	fr := &frame{
		program: p,
		context: ctx,
		seqid:   seqid,
	}
	ret := callSSA(fr, mainFn, args, nil)
	if fr.panic != nil {
		err = fmt.Errorf("err: %v", fr.panic)
	}
	ctx.waitGoroutines(defaultTimeout)
	if ret != nil {
		result = ret.Interface()
	}
	ctx.cancelFunc()
	return result, ctx, err
}

func (p *Program) initGlobal() {
	for _, v := range p.mainPkg.Members {
		if g, ok := v.(*ssa.Global); ok {
			// 与 SSA 一致：全局符号类型为 *T，存储指向单元的指针
			cell := zero(g.Type().(*types.Pointer).Elem())
			p.globals[g] = &cell
		}
	}
}

// SetGlobalValue 修改全局变量的值
func (p *Program) SetGlobalValue(name string, val interface{}) error {
	v := p.mainPkg.Members[name]
	g, ok := v.(*ssa.Global)
	if !ok {
		return fmt.Errorf("global Value %s not found", name)
	}
	src := reflect.ValueOf(val)
	if cell, exists := p.globals[g]; exists && cell != nil {
		rv := (*cell).RValue()
		if rv.Kind() == reflect.Ptr && rv.Elem().IsValid() {
			dst := rv.Elem()
			if src.Type().AssignableTo(dst.Type()) {
				dst.Set(src)
			} else if src.CanConvert(dst.Type()) {
				dst.Set(src.Convert(dst.Type()))
			} else {
				return fmt.Errorf("cannot assign %s to global %s (%s)", src.Type(), name, dst.Type())
			}
			return nil
		}
	}
	nv := value.ValueOf(val)
	p.globals[g] = &nv
	return nil
}

// GetGlobalValue 获取全局变量的值
func (p *Program) GetGlobalValue(name string) (interface{}, error) {
	v := p.mainPkg.Members[name]
	g, ok := v.(*ssa.Global)
	if !ok {
		return nil, fmt.Errorf("global Value %s not found", name)
	}
	if gv, ok := p.globals[g]; ok && gv != nil && *gv != nil {
		rv := (*gv).RValue()
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil, nil
			}
			return rv.Elem().Interface(), nil
		}
		return (*gv).Interface(), nil
	}
	// 未初始化的全局变量返回其零值
	return zero(g.Type().(*types.Pointer).Elem()).Elem().Interface(), nil
}

// Package 获取package，用于导入到其他package
func (p *Program) Package() *ssa.Package {
	return p.mainPkg
}

// externalValue 根据外部全局变量解析其宿主对象。
// 通过 Global 所属 ssa.Package 的 types.Package 路径 + 变量名，
// 从 importer 的外部对象映射中找到 reflect.Value。
func (p *Program) externalValue(g *ssa.Global) value.Value {
	pkgPath := ""
	if g.Pkg != nil {
		pkgPath = g.Pkg.Pkg.Path()
	}
	name := g.Name()
	imp := p.importer
	if imp == nil {
		panic(fmt.Sprintf("get: external global %s.%s but no importer", pkgPath, name))
	}
	key := fmt.Sprintf("%s.%s", pkgPath, name)
	obj, ok := imp.ExternalObjects()[key]
	if !ok || !obj.Value.IsValid() {
		panic(fmt.Sprintf("get: external object %s not registered", key))
	}
	// 函数直接可调用；变量返回包装以便读写
	return value.NewExternalValue(obj.Value)
}

// externalFunction 根据外部 SSA 函数（无函数体）解析其宿主实现。
// fn.String() 形如 "fmt.Sprint"，与注册表的 "pkgPath.Name" 键一致。
func (p *Program) externalFunction(fn *ssa.Function) *reflect.Value {
	if p.importer == nil {
		return nil
	}
	obj, ok := p.importer.ExternalObjects()[fn.String()]
	if !ok || !obj.Value.IsValid() || obj.Value.Kind() != reflect.Func {
		return nil
	}
	return &obj.Value
}

// importedFunction 从已导入的 SSA 包中查找同名函数（跨 Program 调用）
func (p *Program) importedFunction(fn *ssa.Function) *ssa.Function {
	if p.importer == nil || fn == nil {
		return nil
	}
	keys := make([]string, 0, 2)
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		keys = append(keys, fn.Pkg.Pkg.Path(), fn.Pkg.Pkg.Name())
	}
	for _, key := range keys {
		if pkg := p.importer.SsaPackage(key); pkg != nil {
			if found := pkg.Func(fn.Name()); found != nil && len(found.Blocks) > 0 {
				return found
			}
		}
	}
	return nil
}
