package main

import (
	"fmt"
	"strings"
	"text/template"
)

var opUnList = map[string]opValues{
	"SUB": {
		op:     "-",
		values: templateUnValues,
	},
	"XOR": {
		op:     "^",
		values: templateUnValues,
	},
}

var templateUnValues = map[string]interface{}{
	"types": map[string]string{
		"Int":     "int",
		"Int8":    "int8",
		"Int16":   "int16",
		"Int32":   "int32",
		"Int64":   "int64",
		"Uint":    "uint",
		"Uint8":   "uint8",
		"Uint16":  "uint16",
		"Uint32":  "uint32",
		"Uint64":  "uint64",
		"Uintptr": "uintptr",
	},
}

var templateUnFuncBody = `
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	{{ $op := .op  }}
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst {
				v := {{ $op }}vx.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).({{ $value }})
					vm.setReg(ir, v)
				}
			}
		{{ end }}
		}
	} else {
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst {
				v := xtype.Neg{{ $key }}(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.Neg{{ $key }}(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		{{ end }}
		}
	}
	panic("unreachable")
`

func generateOpUn() {
	var resultStr string
	for name, op := range opUnList {
		templateStr := new2Line + "func makeUnOp" + name + "(pfn *function, instr *ssa.BinOp) func(vm *goVm) {" + templateUnFuncBody + "}"
		tx := template.Must(template.New(name).Parse(templateStr))
		result := new(strings.Builder)
		_ = tx.Execute(result, op.values)
		resultStr += result.String()
	}
	resultStr = generateImport + resultStr
	fmt.Println(resultStr)
}
