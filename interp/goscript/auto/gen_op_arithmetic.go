package main

import (
	"fmt"
	"strings"
	"text/template"
)

var opArithmeticList = map[string]opValues{
	"ADD": {
		op:     "+",
		values: removeDefaultValuesAttributes(),
	},
	"SUB": {
		op:     "-",
		values: removeDefaultValuesAttributes("String"),
	},
	"MUL": {
		op:     "*",
		values: removeDefaultValuesAttributes("String"),
	},
	"QUO": {
		op:         "/",
		values:     removeDefaultValuesAttributes("String"),
		valuesType: "quo",
	},
	"REM": {
		op:     "%",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
	"AND": {
		op:     "&",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
	"OR": {
		op:     "|",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
	"XOR": {
		op:     "^",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
	"ANDNOT": {
		op:     "&^",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
	"LSS": {
		op:         "<",
		values:     removeDefaultValuesAttributes("Complex64", "Complex128"),
		valuesType: "compare",
	},
	"LEQ": {
		op:         "<=",
		values:     removeDefaultValuesAttributes("Complex64", "Complex128"),
		valuesType: "compare",
	},
	"GTR": {
		op:         ">",
		values:     removeDefaultValuesAttributes("Complex64", "Complex128"),
		valuesType: "compare",
	},
	"GEQ": {
		op:         ">=",
		values:     removeDefaultValuesAttributes("Complex64", "Complex128"),
		valuesType: "compare",
	},
}

var templateArithmeticFuncBody = `
	{{ $op := .op  }}
	{{ $valuesType := .valuesType }}
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	{{ if eq $valuesType "compare" }}
	typ := pfn.Interp.preToType(instr.X.Type())
	{{ else }}
	typ := pfn.Interp.preToType(instr.Type())
	{{ end }}
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst && ky == kindConst {
				{{ if eq $valuesType "quo" }}
				x := xtype.{{ $key }}(vx)
				y := xtype.{{ $key }}(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x {{ $op }} y) }
				}
				{{ end }}
				v := vx.({{ $value }}) {{ $op }} vy.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, x {{ $op }} vm.reg(iy).({{ $value }})) }
			} else if ky == kindConst {
				y := vy.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).({{ $value }}) {{ $op }} y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).({{ $value }}) {{ $op }} vm.reg(iy).({{ $value }})) }
			}
		{{ end }}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst && ky == kindConst {
				{{ if eq $valuesType "quo" }}
				x := xtype.{{ $key }}(vx)
				y := xtype.{{ $key }}(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x {{ $op }} y)) }
				}
				v := xtype.Make(t, xtype.{{ $key }}(vx) {{ $op }} xtype.{{ $key }}(vy))
				{{ else if eq $valuesType "compare" }}
				v := vx.({{ $value }}) {{ $op }} vy.({{ $value }})
				{{ else }}
				v := xtype.Make(t, xtype.{{ $key }}(vx) {{ $op }} xtype.{{ $key }}(vy))
				{{ end }}
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.{{ $key }}(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x {{ $op }} vm.{{ $value }}(iy))) }
			} else if ky == kindConst {
				y := xtype.{{ $key }}(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.{{ $value }}(ix) {{ $op }} y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.{{ $value }}(ix) {{ $op }} vm.{{ $value }}(iy))) }
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
		op.values["valuesType"] = op.valuesType
		tx := template.Must(template.New(name).Parse(templateStr))
		result := new(strings.Builder)
		if err := tx.Execute(result, op.values); err != nil {
			panic(err)
		} else {
			resultStr += result.String()
		}
	}
	resultStr = generateImportWithSSA + resultStr
	fmt.Println(resultStr)
}
