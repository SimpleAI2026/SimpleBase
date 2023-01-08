package main

import (
	"fmt"
	"strings"
	"text/template"
)

var templateCvtValues = removeDefaultValuesAttributes("String", "Complex64", "Complex128")

var templateCvtFuncBody = `
{{ $types := .types  }}
{{ range $key, $value := $types }}
func cvt{{ $key }}(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = {{ $value }}
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			{{ range $key, $value := $types }}
			case reflect.{{ $key }}:
				v = T(vm.reg(ix).({{ $value }}))
			{{ end }}
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(vm *goVm) {
			var v T
			switch xkind {
			{{ range $key, $value := $types }}
			case reflect.{{ $key }}:
				v = T(vm.{{ $value }}(ix))
			{{ end }}
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
{{ end }}
`

var templateCvtMakeTypeChangeInstrValues = removeDefaultValuesAttributes()

var templateCvtMakeTypeChangeInstrFuncBody = `
{{ $types := .types  }}
func makeTypeChangeInstr(pfn *function, instr *ssa.ChangeType) func(vm *goVm) {
	typ := pfn.Interp.preToType(instr.Type())
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	if kx.isStatic() {
		var v interface{}
		if vx == nil {
			v = reflect.New(typ).Elem().Interface()
		} else {
			v = reflect.ValueOf(vx).Convert(typ).Interface()
		}
		return func(vm *goVm) {
			vm.setReg(ir, v)
		}
	}
	kind := typ.Kind()
	switch kind {
	{{ range $key, $value := $types }}
	case reflect.{{ $key }}:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.{{ $key }}(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Convert{{ $key }}(t, x))
			}
		}
	{{ end }}
	case reflect.Ptr, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertPtr(t, x))
		}
	case reflect.Struct, reflect.Array:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertDirect(t, x))
		}
	case reflect.Interface:
		return func(vm *goVm) {
			x := vm.reg(ix)
			if x == nil {
				vm.setReg(ir, reflect.New(typ).Elem().Interface())
			} else {
				vm.setReg(ir, reflect.ValueOf(x).Convert(typ).Interface())
			}
		}
	case reflect.Bool:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Bool(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertBool(t, x))
			}
		}
	case reflect.UnsafePointer:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertPtr(t, x))
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", kind))
}
`

func generateOpCvt() {
	var resultStr string
	{
		tx := template.Must(template.New("cvt").Parse(templateCvtFuncBody))
		result := new(strings.Builder)
		if err := tx.Execute(result, templateCvtValues); err != nil {
			panic(err)
		} else {
			resultStr += result.String()
		}
	}
	{
		tx := template.Must(template.New("cvtInstr").Parse(templateCvtMakeTypeChangeInstrFuncBody))
		result := new(strings.Builder)
		if err := tx.Execute(result, templateCvtMakeTypeChangeInstrValues); err != nil {
			panic(err)
		} else {
			resultStr += result.String()
		}
	}
	resultStr = generateImportWithSSA + resultStr
	fmt.Println(resultStr)
}
