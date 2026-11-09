//go:build ignore

package main

func Exported()      {}
func hidden()        {}
func (s *S) Method() {}

type S struct{}

var x = 1
