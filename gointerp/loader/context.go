package loader

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"sort"
	"sync"

	"github.com/linkxzhou/gongx/log"
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
	Loader       Loader         // types loader
	BuildContext build.Context  // build context
	FileSet      *token.FileSet // file set

	DebugFunc func(*DebugInfo)         // debug func
	Override  map[string]reflect.Value // override function

	CallForPool int             // least call count for enable function pool
	Mode        Mode            // mode
	ParserMode  parser.Mode     // parser mode
	BuilderMode ssa.BuilderMode // ssa builder mode

	pkgs map[string]*sourcePackage // imports
	conf *types.Config             // types check config
	root string                    // project root
}

func (ctx *Context) setRoot(root string) {
	ctx.root = root
}

type sourcePackage struct {
	Context *Context
	Package *types.Package
	Info    *types.Info
	Dir     string
	Files   []*ast.File
}

func (sp *sourcePackage) Load() (err error) {
	if sp.Info == nil {
		sp.Info = &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Implicits:  make(map[ast.Node]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		if err := types.NewChecker(sp.Context.conf, sp.Context.FileSet, sp.Package, sp.Info).Files(sp.Files); err != nil {
			return err
		}
	}
	return
}

// NewContext create a new Context
func NewContext(mode Mode) *Context {
	ctx := &Context{
		Loader:       NewTypesLoader(mode),
		FileSet:      token.NewFileSet(),
		Mode:         mode,
		ParserMode:   parser.AllErrors,
		BuilderMode:  0, // ssa.SanityCheckFunctions
		BuildContext: build.Default,
		pkgs:         make(map[string]*sourcePackage),
		Override:     make(map[string]reflect.Value),
		CallForPool:  64,
	}
	if mode&EnableDumpInstr != 0 {
		ctx.BuilderMode |= ssa.PrintFunctions
	}
	ctx.conf = &types.Config{
		Importer: NewImporter(ctx),
	}
	return ctx
}

func (ctx *Context) SetEvalMode(b bool) {
	ctx.conf.DisableUnusedImportCheck = b
}

// SetLeastCallForEnablePool set least call count for enable function pool, default 64
func (ctx *Context) SetLeastCallForEnablePool(count int) {
	ctx.CallForPool = count
}

func (ctx *Context) SetDebug(fn func(*DebugInfo)) {
	ctx.BuilderMode |= ssa.GlobalDebug
	ctx.DebugFunc = fn
}

// SetOverrideFunction register external function to override function.
// match func fullname and signature
func (ctx *Context) SetOverrideFunction(key string, fn interface{}) {
	ctx.Override[key] = reflect.ValueOf(fn)
}

// ClearOverrideFunction reset override function
func (ctx *Context) ClearOverrideFunction(key string) {
	delete(ctx.Override, key)
}

func (ctx *Context) AddImportFile(path string, filename string, src interface{}) (err error) {
	if tp, err := ctx.loadPackageFile(path, filename, src); err != nil {
		return err
	} else {
		ctx.Loader.SetImport(path, tp.Package, tp.Load)
		return nil
	}
}

func (ctx *Context) AddImport(path string, dir string) (err error) {
	bp, err := ctx.BuildContext.ImportDir(dir, 0)
	if err != nil {
		return err
	}
	bp.ImportPath = path
	tp, err := ctx.loadPackage(bp, path, dir)
	if err != nil {
		return err
	}
	ctx.Loader.SetImport(path, tp.Package, tp.Load)
	return nil
}

func (ctx *Context) loadPackageFile(path string, filename string, src interface{}) (*sourcePackage, error) {
	file, err := ctx.ParseFile(filename, src)
	if err != nil {
		return nil, err
	}
	pkg := types.NewPackage(path, file.Name.Name)
	tp := &sourcePackage{
		Context: ctx,
		Package: pkg,
		Files:   []*ast.File{file},
	}
	ctx.pkgs[path] = tp
	return tp, nil
}

func (ctx *Context) loadPackage(bp *build.Package, path string, dir string) (*sourcePackage, error) {
	files, err := ctx.parseGoFiles(dir, append(bp.GoFiles, bp.CgoFiles...))
	if err != nil {
		return nil, err
	}
	tp := &sourcePackage{
		Package: types.NewPackage(path, bp.Name),
		Files:   files,
		Dir:     dir,
		Context: ctx,
	}
	ctx.pkgs[path] = tp
	return tp, nil
}

func (ctx *Context) parseGoFiles(dir string, filenames []string) ([]*ast.File, error) {
	files := make([]*ast.File, len(filenames))
	errors := make([]error, len(filenames))

	var wg sync.WaitGroup
	wg.Add(len(filenames))
	for i, filename := range filenames {
		go func(i int, filepath string) {
			defer wg.Done()
			files[i], errors[i] = parser.ParseFile(ctx.FileSet, filepath, nil, 0)
		}(i, filepath.Join(dir, filename))
	}
	wg.Wait()

	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (ctx *Context) LoadFile(filename string, src interface{}) (*ssa.Package, error) {
	file, err := ctx.ParseFile(filename, src)
	if err != nil {
		log.Error("LoadFile err: ", err, ", filename: ", filename)
		return nil, err
	}
	root, _ := filepath.Split(filename)
	ctx.setRoot(root)
	return ctx.LoadAstFile("main", file)
}

func (ctx *Context) ParseFile(filename string, src interface{}) (*ast.File, error) {
	return parser.ParseFile(ctx.FileSet, filename, src, ctx.ParserMode)
}

func (ctx *Context) LoadAstFile(path string, file *ast.File) (*ssa.Package, error) {
	files := []*ast.File{file}
	sp := &sourcePackage{
		Context: ctx,
		Package: types.NewPackage(path, file.Name.Name),
		Files:   files,
	}
	if err := sp.Load(); err != nil {
		log.Error("LoadAstFile err: ", err, ", path: ", path)
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
	var createAll func(pkgs []*types.Package)
	prog := ssa.NewProgram(ctx.FileSet, ctx.BuilderMode)
	// Create SSA packages for all imports.
	// Order is not significant.
	created := make(map[*types.Package]bool)
	createAll = func(pkgs []*types.Package) {
		for _, p := range pkgs {
			if !created[p] {
				created[p] = true
				createAll(p.Imports())
				if pkg, ok := ctx.pkgs[p.Path()]; ok {
					if ctx.Mode&EnableDumpImports != 0 {
						if pkg.Dir != "" {
							fmt.Println("# package", p.Path(), pkg.Dir)
						} else {
							fmt.Println("# package", p.Path(), "<memory>")
						}
					}
					prog.CreatePackage(p, pkg.Files, pkg.Info, true).Build()
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
					prog.CreatePackage(p, nil, nil, true).Build()
				}
			}
		}
	}
	var addin []*types.Package
	for _, pkg := range ctx.Loader.Packages() {
		if !pkg.Complete() {
			addin = append(addin, pkg)
		}
	}
	if len(addin) > 0 {
		sort.Slice(addin, func(i, j int) bool {
			return addin[i].Path() < addin[j].Path()
		})
		createAll(addin)
	}
	createAll(sp.Package.Imports())
	// Create and build the primary package.
	pkg = prog.CreatePackage(sp.Package, sp.Files, sp.Info, false)
	pkg.Build()
	return
}
