package main

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]string{
		"a:80:2": "a 80 2 <nil>", "a:80:0": "a 80 4 <nil>", "a:0:1": " 0 0 bad port 0",
		"a:x:1": " 0 0 strconv.Atoi: parsing \"x\": invalid syntax", "a:80:y": " 0 0 strconv.Atoi: parsing \"y\": invalid syntax",
		"a:80": " 0 0 want 3 parts, got 2", "": " 0 0 empty config", ":65535:-1": " 65535 -1 <nil>",
	}
	for in, want := range cases {
		h, p, w, err := ParseConfig(in)
		if got := fmt.Sprint(h, " ", p, " ", w, " ", err); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if _, _, _, err := ParseConfig(""); !errors.Is(err, ErrEmpty) { t.Error("ErrEmpty identity") }
	_, _, _, err := ParseConfig("a:x:1")
	var ne *strconv.NumError
	if !errors.As(err, &ne) { t.Error("NumError must be unwrappable") }
}
