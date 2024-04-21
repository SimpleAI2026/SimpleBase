package vfsextend

import (
	"github.com/linkxzhou/LessDB/internal/s3"

	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var invalidContentRangeErr = errors.New("invalid Content-Range response")
var missCacheErr = errors.New("miss Cache")

type Option interface {
	set(*RangeReader)
}

type roundTripperOption struct {
	r http.RoundTripper
}

func (o *roundTripperOption) set(rr *RangeReader) {
	rr.roundTripper = o.r
}

func WithRoundTripper(r http.RoundTripper) Option {
	return &roundTripperOption{
		r: r,
	}
}

type cacheHandlerOption struct {
	h CacheHandler
}

func (o *cacheHandlerOption) set(rr *RangeReader) {
	rr.cacheHandler = o.h
}

func WithCacheHandler(c CacheHandler) Option {
	return &cacheHandlerOption{
		h: c,
	}
}

type uriHandlerOption struct {
	h s3.URIHandler
}

func (o *uriHandlerOption) set(rr *RangeReader) {
	rr.uriHandler = o.h
}

func WithURIHandler(u s3.URIHandler) Option {
	return &uriHandlerOption{
		h: u,
	}
}

type CacheHandler interface {
	Get(p []byte, off int64, fetcher VFSReadAt) (int, error)
	Size(fetcher VFSReadAt) (int64, error)
	Set(fileName string)
}

type RangeReader struct {
	name         string
	roundTripper http.RoundTripper
	cacheHandler CacheHandler
	uriHandler   s3.URIHandler
	lastEtag     string // Etag

	fetcher readerAt
}

func New(name string, opts ...Option) *RangeReader {
	rr := RangeReader{
		name: name,
	}

	rr.fetcher = readerAt{
		readAt:   rr.rawReadAt,
		readSize: rr.rawSize,
		readEtag: rr.rawEtag,
	}

	for _, opt := range opts {
		opt.set(&rr)
	}

	return &rr
}

func (rr *RangeReader) CacheHandler() CacheHandler {
	cacheHandler := rr.cacheHandler
	if cacheHandler == nil {
		cacheHandler = &nopCacheHandler{}
	}

	cacheHandler.Set(rr.name)
	return cacheHandler
}

func (rr *RangeReader) ReadAt(p []byte, off int64) (n int, err error) {
	if n, err = rr.CacheHandler().Get(p, off, rr.fetcher); err == missCacheErr {
		return rr.CacheHandler().Get(p, off, rr.fetcher)
	}
	return
}

func (rr *RangeReader) Size() (n int64, err error) {
	return rr.CacheHandler().Size(rr.fetcher)
}

func (rr *RangeReader) rawEtag() string {
	return rr.lastEtag
}

func (rr *RangeReader) rawReadAt(p []byte, off int64) (n int, err error) {
	url, err := rr.uriHandler.URI(rr.name)
	if err != nil {
		return 0, err
	}

	fetchSize := len(p)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(fetchSize-1)))

	resp, err := rr.client().Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	n, err = io.ReadFull(resp.Body, p)
	if err == io.ErrUnexpectedEOF {
		return n, io.EOF
	} else if err != nil {
		return n, err
	} else {
		// pass
	}

	etag := strings.ReplaceAll(resp.Header.Get("Etag"), `"`, "")
	if etag != "" {
		rr.lastEtag = etag
	}

	return n, nil
}

func (rr *RangeReader) client() *http.Client {
	if rr.roundTripper == nil {
		return http.DefaultClient
	}

	return &http.Client{Transport: rr.roundTripper}
}

func (rr *RangeReader) rawSize() (n int64, err error) {
	url, err := rr.uriHandler.URI(rr.name)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Range", "bytes=0-0")
	resp, err := rr.client().Do(req)
	if err != nil {
		return 0, err
	}

	io.Copy(io.Discard, resp.Body)
	defer resp.Body.Close()

	rangeHeader := resp.Header.Get("Content-Range")
	rangeFields := strings.Fields(rangeHeader)
	if len(rangeFields) != 2 {
		return 0, invalidContentRangeErr
	}

	if strings.ToLower(rangeFields[0]) != "bytes" {
		return 0, invalidContentRangeErr
	}

	amts := strings.Split(rangeFields[1], "/")

	if len(amts) != 2 {
		return 0, invalidContentRangeErr
	}

	if amts[1] == "*" {
		return 0, invalidContentRangeErr
	}

	n, err = strconv.ParseInt(amts[1], 10, 64)
	if err != nil {
		return 0, invalidContentRangeErr
	}

	etag := strings.ReplaceAll(resp.Header.Get("Etag"), `"`, "")
	if etag != "" {
		rr.lastEtag = etag
	}

	return n, nil
}

type VFSReadAt interface {
	io.ReaderAt
	Size() (int64, error)
	Etag() string
}

type readerAt struct {
	readAt   func([]byte, int64) (int, error)
	readSize func() (int64, error)
	readEtag func() string
}

func (r readerAt) ReadAt(p []byte, off int64) (n int, err error) {
	return r.readAt(p, off)
}

func (r readerAt) Size() (n int64, err error) {
	return r.readSize()
}

func (r readerAt) Etag() string {
	if r.readEtag == nil {
		return "default"
	}

	return r.readEtag()
}
