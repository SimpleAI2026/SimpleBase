package loader

import (
	"errors"
	"fmt"
)

var (
	ErrNoPackage        = errors.New("no package")
	ErrPackage          = errors.New("package contain errors")
	ErrNotFoundMain     = errors.New("not found main package")
	ErrTestFailed       = errors.New("test failed")
	ErrNotFoundPackage  = errors.New("not found package")
	ErrNotFoundImporter = errors.New("not found provider for types.Importer")
	ErrGoexitDeadlock   = errors.New("fatal error: no goroutines (main called runtime.Goexit) - deadlock!")
	ErrNoFunction       = errors.New("no function")
	ErrNoCustomBuiltin  = errors.New("no custom builtin")
	ErrNoTestFiles      = errors.New("no test files")
	ErrGoList           = errors.New("error go list")
)

type ExitError int

func (r ExitError) Error() string {
	return fmt.Sprintf("exit %v", int(r))
}
