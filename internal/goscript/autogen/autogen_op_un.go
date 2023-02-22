package main

import (
	"fmt"
	"strings"
	"text/template"
)

var opUnSUBList = map[string]opValues{
	"SUB": {
		op:     "-",
		values: removeDefaultValuesAttributes("String"),
	},
}

var templateUnSUBFuncBody = `
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
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
`

var opUnXORList = map[string]opValues{
	"XOR": {
		op:     "^",
		values: removeDefaultValuesAttributes("String", "Float32", "Float64", "Complex64", "Complex128"),
	},
}

var templateUnXORFuncBody = `
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	{{ $op := .op  }}
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		{{ range $key, $value := .types }}
		case reflect.{{ $key }}:
			if kx == kindConst {
				v := ^vx.({{ $value }})
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).({{ $value }})
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
				v := xtype.Xor{{ $key }}(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.Xor{{ $key }}(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		{{ end }}
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
`

func generateOpUn() {
	var resultStr string
	for name, op := range opUnSUBList {
		templateStr := new2Line + "func makeUnOp" + name + "(pfn *function, instr *ssa.UnOp) func(vm *goVm) {" + templateUnSUBFuncBody + "}"
		op.values["op"] = op.op
		tx := template.Must(template.New(name).Parse(templateStr))
		result := new(strings.Builder)
		if err := tx.Execute(result, op.values); err != nil {
			panic(err)
		} else {
			resultStr += result.String()
		}
	}
	for name, op := range opUnXORList {
		templateStr := new2Line + "func makeUnOp" + name + "(pfn *function, instr *ssa.UnOp) func(vm *goVm) {" + templateUnXORFuncBody + "}"
		op.values["op"] = op.op
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
