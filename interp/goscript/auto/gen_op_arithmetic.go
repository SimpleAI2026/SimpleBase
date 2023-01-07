package main

import (
	"fmt"
	"strings"
	"text/template"
)

var (
	templateArithmetic1Values = map[string]interface{}{
		"types": map[string]string{
			"Int":        "int",
			"Int8":       "int8",
			"Int16":      "int16",
			"Int32":      "int32",
			"Int64":      "int64",
			"Uint":       "uint",
			"Uint8":      "uint8",
			"Uint16":     "uint16",
			"Uint32":     "uint32",
			"Uint64":     "uint64",
			"Uintptr":    "uintptr",
			"Float32":    "float32",
			"Float64":    "float64",
			"Complex128": "complex128",
			"String":     "string",
		},
	}

	templateArithmetic2Values = map[string]interface{}{
		"types": map[string]string{
			"Int":        "int",
			"Int8":       "int8",
			"Int16":      "int16",
			"Int32":      "int32",
			"Int64":      "int64",
			"Uint":       "uint",
			"Uint8":      "uint8",
			"Uint16":     "uint16",
			"Uint32":     "uint32",
			"Uint64":     "uint64",
			"Uintptr":    "uintptr",
			"Float32":    "float32",
			"Float64":    "float64",
			"Complex128": "complex128",
		},
	}

	templateArithmetic3Values = map[string]interface{}{
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

	templateArithmetic4Values = map[string]interface{}{
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
			"Float32": "float32",
			"Float64": "float64",
			"String":  "string",
		},
	}
)

var opArithmeticList = map[string]opValues{
	"ADD": {
		op:     "+",
		values: templateArithmetic1Values,
	},
	"SUB": {
		op:     "-",
		values: templateArithmetic2Values,
	},
	"MUL": {
		op:     "*",
		values: templateArithmetic2Values,
	},
	"QUO": {
		op:     "/",
		values: templateArithmetic2Values,
	},
	"REM": {
		op:     "%",
		values: templateArithmetic3Values,
	},
	"AND": {
		op:     "&",
		values: templateArithmetic3Values,
	},
	"OR": {
		op:     "|",
		values: templateArithmetic3Values,
	},
	"XOR": {
		op:     "^",
		values: templateArithmetic3Values,
	},
	"ANDNOT": {
		op:     "&^",
		values: templateArithmetic3Values,
	},
	"LSS": {
		op:     "<",
		values: templateArithmetic4Values,
	},
	"LEQ": {
		op:     "<=",
		values: templateArithmetic4Values,
	},
	"GTR": {
		op:     ">",
		values: templateArithmetic4Values,
	},
	"GEQ": {
		op:     ">=",
		values: templateArithmetic4Values,
	},
}

var templateArithmeticFuncBody = `
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	{{ $op := .op  }}
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst && ky == kindConst {
				v := vx.({{ $value }}) {{ $op }} vy.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, x{{ $op }}vm.reg(iy).({{ $value }})) }
			} else if ky == kindConst {
				y := vy.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).({{ $value }}){{ $op }}y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).({{ $value }}){{ $op }}vm.reg(iy).({{ $value }})) }
			}
		{{ end }}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.{{ $key }}(vx){{ $op }}xtype.{{ $key }}(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.{{ $key }}(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x{{ $op }}vm.{{ $value }}(iy))) }
			} else if ky == kindConst {
				y := xtype.{{ $key }}(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.{{ $value }}(ix){{ $op }}y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.{{ $value }}(ix){{ $op }}vm.{{ $value }}(iy))) }
			}
		{{ end }}
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
`

func generateOpArithmetic() {
	var resultStr string
	for name, op := range opArithmeticList {
		templateStr := new2Line + "func makeBinOp" + name + "(pfn *function, instr *ssa.BinOp) func(vm *goVm) {" + templateArithmeticFuncBody + "}"
		op.values["op"] = op.op
		tx := template.Must(template.New(name).Parse(templateStr))
		result := new(strings.Builder)
		if err := tx.Execute(result, op.values); err != nil {
			panic(err)
		} else {
			resultStr += result.String()
		}
	}
	resultStr = `
package goscript

import (
	"reflect"
	"fmt"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)
` + resultStr
	fmt.Println(resultStr)
}
