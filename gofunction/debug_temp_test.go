package gofunction

import (
	"fmt"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestDebugTemp(t *testing.T) {
	debugging = true
	defer func() { debugging = false }()

	src := `package test
func test() int { sum := 0; for i := 0; i < 3; i++ { sum += i }; return sum }`
	p, err := BuildProgram("dbg", "t", src)
	if err != nil {
		t.Fatal(err)
	}
	mainFn := p.mainPkg.Func("test")
	fmt.Printf("fn blocks: %d\n", len(mainFn.Blocks))

	fr := &frame{program: p, context: newCallContext(), seqid: "dbg"}
	defer fr.context.cancelFunc()
	ret := callSSA(fr, mainFn, nil, nil)
	fmt.Printf("RESULT ret=%#v\n", ret)
}
