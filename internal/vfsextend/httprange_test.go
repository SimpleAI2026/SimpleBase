package vfsextend

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPRangeRead(t *testing.T) {
	absTestDir, _ := os.Getwd()
	dir, err := os.MkdirTemp(absTestDir, "httprange-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}

	fileSize := 1 << 17
	_, err = io.Copy(f, io.LimitReader(rand.Reader, int64(fileSize)))
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer server.Close()

	rr := New(server.URL + "/file")

	checks := []struct {
		off  int
		size int
	}{
		{0, 0},
		{0, 1 << 17},
		{0, 1<<17 + 1},
		{389, 687},
		{389, 687},
		{217, 548},
	}

	for i, check := range checks {
		rrb := make([]byte, check.size)
		fb := make([]byte, check.size)

		rrn, rrErr := rr.ReadAt(rrb, int64(check.off))
		fn, fErr := f.ReadAt(fb, int64(check.off))

		if rrErr != fErr {
			t.Fatalf("[%d] read err mismatch: rr_err=%s f_err=%s", i, rrErr, fErr)
		}

		if rrn != fn {
			t.Fatalf("[%d] read len mismatch: rrn=%d fn=%d", i, rrn, fn)
		}

		if !bytes.Equal(rrb[:rrn], fb[:fn]) {
			t.Fatalf("[%d] read mismatch %+v vs %+v", i, rrb[:rrn], fb[:fn])
		}
	}
}
