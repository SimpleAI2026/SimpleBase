package main

import (
	"os"
)

func main() {
	args := os.Args
	if args == nil || len(args) < 2 {
		panic("Support ImportPkgs, OpArithmetic, OpShift, OpCvt, OpUn")
	}
	switch args[1] {
	case "ImportPkgs":
		generateImportPkgs()
	case "OpArithmetic":
		generateOpArithmetic()
	case "OpShift":
		generateOpShift()
	case "OpCvt":
		generateOpCvt()
	case "OpUn":
		generateOpUn()
	default:
		panic("Support ImportPkgs, OpArithmetic, OpShift, OpCvt, OpUn")
	}
}
