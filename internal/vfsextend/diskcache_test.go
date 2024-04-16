package vfsextend

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	absTestDir, _ := os.Getwd()
	backingFile, err := os.CreateTemp(absTestDir, "diskcache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backingFile.Name())

	// make sure filesize isn't a pagesize multiple
	fileSize := int64(1<<17 + 37)
	_, err = io.Copy(backingFile, io.LimitReader(rand.Reader, int64(fileSize)))
	if err != nil {
		t.Fatal(err)
	}

	cache := NewDiskCache(backingFile, fileSize)
	// 1<<17 / 1<<13 == 16 pages
	cache.PageSize = 1 << 13

	readAndCheck := func(p []byte, off int64) (int, error) {
		fromBacking := make([]byte, len(p))
		cn, cacheErr := cache.Get(p, off, readerAt{
			readAt: backingFile.ReadAt,
		})
		bn, backingErr := backingFile.ReadAt(fromBacking, off)
		if cacheErr != backingErr {
			t.Fatalf("Fetch error mismatch: cache_err=%s vs backing_err=%s", cacheErr, backingErr)
		}

		if cn != bn {
			t.Fatalf("fetch count mismatch cache_n=%d backing_n=%d", cn, bn)
		}

		if !bytes.Equal(p[:cn], fromBacking[:bn]) {
			t.Fatalf("fetch mismatch %+v vs %+v", p[:cn], fromBacking[:bn])
		}
		return cn, cacheErr
	}

	checks := []struct {
		start    int64
		size     int
		cacheHit bool
	}{
		{0, 100, false},
		// read within a single page
		{1<<13 + 6, 100, false},
		// read again within that page, see that we cache hit
		{1<<13 + 6, 100, true},
		// read again within that page in a different part of the page, see that we cache hit
		{1<<13 + 106, 100, true},
		// read again exactly on the page boundry, see that we hit
		{1 << 13, 1<<13 - 1, true},
		// read across multiple pages
		{1<<13 + 5, 2<<13 + 5, false},
		// read again see that we cache hit
		{1<<13 + 5, 2<<13 + 5, true},
		// read up to last byte
		{fileSize - 7, 7, false},
	}

	for i, check := range checks {
		p := make([]byte, check.size)
		beforeHit := cache.cacheHit
		beforeMiss := cache.cacheMiss
		_, err := readAndCheck(p, int64(check.start))
		if err != nil {
			t.Fatalf("[%d] read err: %s", i, err)
		}
		if check.cacheHit {
			if cache.cacheHit != beforeHit+1 {
				t.Fatalf("[%d] expected cache hit but was not start:%d size:%d", i, check.start, check.size)
			}
		} else {
			if cache.cacheMiss != beforeMiss+1 {
				t.Fatalf("[%d] expected cache miss but was not start:%d size:%d", i, check.start, check.size)
			}
		}
	}
}
