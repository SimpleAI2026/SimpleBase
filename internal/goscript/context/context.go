package context

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"sort"

	"golang.org/x/tools/go/ssa"
)

// Mode is a bitmask of options affecting the interpreter.
type Mode uint

const (
	DisableRecover    Mode = 1 << iota // Disable recover() in target programs; show interpreter crash instead.
	EnableDumpImports                  // print import packages
	EnableDumpInstr                    // Print packages & SSA instruction code
	EnablePrintAny                     // Enable builtin print for any type ( struct/array )
)

// Context ssa context
type Context struct {
	BuildContext build.Context  // build context
	FileSet      *token.FileSet // file set

	DebugFunc    func(*DebugInfo) // debug func
	ExternalFunc func() []*types.Package
	Override     map[string]reflect.Value // override function

	CallForPool int             // least call count for enable function pool
	Mode        Mode            // mode
	ParserMode  parser.Mode     // parser mode
	BuilderMode ssa.BuilderMode // ssa builder mode

	pkgs      map[string]*sourcePackage // imports
	conf      *types.Config             // types check config
	root      string                    // project root
	isLoadPkg map[*types.Package]bool
}

func (ctx *Context) setRoot(root string) {
	ctx.root = root
}

type sourcePackage struct {
	ctx          *Context
	typesPackage *types.Package
	info         *types.Info
	dir          string
	files        []*ast.File
}

func (sp *sourcePackage) Load() error {
	if sp.info == nil {
		sp.info = &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Implicits:  make(map[ast.Node]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		if err := types.NewChecker(sp.ctx.conf, sp.ctx.FileSet, sp.typesPackage, sp.info).Files(sp.files); err != nil {
			return err
		}
	}
	return nil
}

// NewContext create a new Context
func NewContext(mode Mode) *Context {
	ctx := &Context{
		FileSet:      token.NewFileSet(),
		Mode:         mode,
		ParserMode:   parser.AllErrors,
		BuilderMode:  0, // ssa.SanityCheckFunctions
		BuildContext: build.Default,
		Override:     make(map[string]reflect.Value),
		CallForPool:  64,
		pkgs:         make(map[string]*sourcePackage),
		isLoadPkg:    make(map[*types.Package]bool),
	}
	if mode&EnableDumpInstr != 0 {
		ctx.BuilderMode |= ssa.PrintFunctions
	}
	return ctx
}

func (ctx *Context) SetExternalConfig(importer types.Importer, fn func() []*types.Package) {
	ctx.conf = &types.Config{Importer: importer}
	ctx.ExternalFunc = fn
}

func (ctx *Context) SetEvalMode(b bool) {
	ctx.conf.DisableUnusedImportCheck = b
}

func (ctx *Context) SetLeastCallForEnablePool(count int) {
	ctx.CallForPool = count
}

func (ctx *Context) SetDebug(fn func(*DebugInfo)) {
	ctx.BuilderMode |= ssa.GlobalDebug
	ctx.DebugFunc = fn
}

func (ctx *Context) SetOverrideFunction(name string, fn interface{}) {
	ctx.Override[name] = reflect.ValueOf(fn)
}

func (ctx *Context) ClearOverrideFunction(key string) {
	delete(ctx.Override, key)
}

func (ctx *Context) Import(path string) (*types.Package, error) {
	if pkg, ok := ctx.pkgs[path]; ok {
		if !pkg.typesPackage.Complete() {
			if err := pkg.Load(); err != nil {
				return nil, err
			}
		}
		return pkg.typesPackage, nil
	}
	return nil, ErrNoPackage
}

func (ctx *Context) loadPackageFile(pkgPath string, filename string, src interface{}) (*sourcePackage, error) {
	file, err := parser.ParseFile(ctx.FileSet, filename, src, ctx.ParserMode)
	if err != nil {
		return nil, err
	}
	pkg := types.NewPackage(pkgPath, file.Name.Name)
	sp := &sourcePackage{
		ctx:          ctx,
		typesPackage: pkg,
		files:        []*ast.File{file},
	}
	ctx.pkgs[pkgPath] = sp
	return sp, nil
}

func (ctx *Context) parseGoFiles(dir string, filenames []string) ([]*ast.File, error) {
	var err error
	files := make([]*ast.File, len(filenames))
	for i, filename := range filenames {
		if files[i], err = parser.ParseFile(ctx.FileSet, filepath.Join(dir, filename), nil, 0); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (ctx *Context) loadPackage(bp *build.Package, pkgPath, dir string) (*sourcePackage, error) {
	files, err := ctx.parseGoFiles(dir, append(bp.GoFiles, bp.CgoFiles...))
	if err != nil {
		return nil, err
	}
	sp := &sourcePackage{
		typesPackage: types.NewPackage(pkgPath, bp.Name),
		files:        files,
		dir:          dir,
		ctx:          ctx,
	}
	ctx.pkgs[pkgPath] = sp
	return sp, nil
}

func (ctx *Context) loadPackageFunc(pkgs []*types.Package, prog *ssa.Program) {
	for _, p := range pkgs {
		if !ctx.isLoadPkg[p] {
			ctx.isLoadPkg[p] = true
			ctx.loadPackageFunc(p.Imports(), prog)
			var typesInfo *types.Info = nil
			var pkgFiles []*ast.File = nil
			if pkg, ok := ctx.pkgs[p.Path()]; ok {
				if ctx.Mode&EnableDumpImports != 0 {
					if pkg.dir != "" {
						fmt.Println("# package", p.Path(), pkg.dir)
					} else {
						fmt.Println("# package", p.Path(), "<memory>")
					}
				}
				pkgFiles = pkg.files
				typesInfo = pkg.info
			} else {
				var indirect bool
				if !p.Complete() {
					indirect = true
					p.MarkComplete()
				}
				if ctx.Mode&EnableDumpImports != 0 {
					if indirect {
						fmt.Println("# virtual", p.Path())
					} else {
						fmt.Println("# builtin", p.Path())
					}
				}
			}
			prog.CreatePackage(p, pkgFiles, typesInfo, true).Build()
		}
	}
}

func (ctx *Context) LoadFile(filename string, src interface{}) (*ssa.Package, error) {
	if file, err := parser.ParseFile(ctx.FileSet, filename, src, ctx.ParserMode); err != nil {
		return nil, err
	} else {
		root, _ := filepath.Split(filename)
		ctx.setRoot(root)
		return ctx.loadAstFiles("main", file)
	}
}

func (ctx *Context) LoadDir(dir string) (*ssa.Package, error) {
	bp, err := ctx.BuildContext.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	if files, err := ctx.parseGoFiles(dir, append(bp.GoFiles, bp.CgoFiles...)); err != nil {
		return nil, err
	} else {
		return ctx.loadAstFiles("main", files...)
	}
}

func (ctx *Context) loadAstFiles(pkgPath string, files ...*ast.File) (*ssa.Package, error) {
	if len(files) > 0 && files[0] == nil {
		return nil, errors.New("file invalid")
	}
	sp := &sourcePackage{
		ctx:          ctx,
		typesPackage: types.NewPackage(pkgPath, files[0].Name.Name),
		files:        files,
	}
	if err := sp.Load(); err != nil {
		return nil, err
	}
	return ctx.buildPackage(sp)
}

func (ctx *Context) buildPackage(sp *sourcePackage) (pkg *ssa.Package, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("build SSA package error: %v", e)
		}
	}()
	prog := ssa.NewProgram(ctx.FileSet, ctx.BuilderMode)
	var externalPkg []*types.Package
	if ctx.ExternalFunc != nil { // load external packages
		for _, pkg := range ctx.ExternalFunc() {
			if pkg != nil && !pkg.Complete() {
				externalPkg = append(externalPkg, pkg)
			}
		}
	}
	if len(externalPkg) > 0 {
		sort.Slice(externalPkg, func(i, j int) bool {
			return externalPkg[i].Path() < externalPkg[j].Path()
		})
		ctx.loadPackageFunc(externalPkg, prog)
	}
	ctx.loadPackageFunc(sp.typesPackage.Imports(), prog)
	pkg = prog.CreatePackage(sp.typesPackage, sp.files, sp.info, false)
	pkg.Build()
	return
}
