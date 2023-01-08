package main

const new2Line = "\n\n"

const generateImportWithSSA = `
// This is generate file
package goscript

import (
	"fmt"
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)
`

type opValues struct {
	op         string
	values     map[string]interface{}
	valuesType string
}

func removeDefaultValuesAttributes(removes ...string) map[string]interface{} {
	var defaultValues = map[string]string{
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
		"Complex64":  "complex64",
		"Complex128": "complex128",
		"String":     "string",
	}
	for _, l := range removes {
		delete(defaultValues, l)
	}
	return map[string]interface{}{
		"types": defaultValues,
	}
}
