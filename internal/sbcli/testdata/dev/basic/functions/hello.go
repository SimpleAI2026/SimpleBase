//go:build ignore

package main

import "fmt"

type HelloInput struct {
	Name string `json:"name"`
}

type HelloOutput struct {
	Message string `json:"message"`
}

// Hello 是可调用导出：大写开头，恰好一个入参、一个返回值。
func Hello(in HelloInput) HelloOutput {
	name := in.Name
	if name == "" {
		name = "world"
	}
	return HelloOutput{Message: fmt.Sprintf("hello, %s", name)}
}
