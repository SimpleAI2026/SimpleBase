package main

const new2Line = "\n\n"

const generateImport = `
// This is generate file
package goscript

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)
`

type opValues struct {
	op     string
	values map[string]interface{}
}
