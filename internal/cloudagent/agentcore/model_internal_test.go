package agentcore

import (
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestMergeToolsPrefersOptions(t *testing.T) {
	bound := []*schema.ToolInfo{{Name: "b"}}
	if mergeTools(bound, nil)[0].Name != "b" {
		t.Fatal("merge bound")
	}
	if mergeTools(bound, []*schema.ToolInfo{{Name: "o"}})[0].Name != "o" {
		t.Fatal("merge opt")
	}
}

func TestSingleMessageStreamYieldsOnceThenEOF(t *testing.T) {
	sr := singleMessageStream(schema.AssistantMessage("one", nil))
	got, err := sr.Recv()
	if err != nil || got.Content != "one" {
		t.Fatalf("single stream %v %v", got, err)
	}
	if _, err = sr.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("eof=%v", err)
	}
}
