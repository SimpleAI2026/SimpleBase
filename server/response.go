package server

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
)

const (
	defaultWriteBytesSize = 128
)

type (
	Response struct {
		echo        *HttpServer
		beforeFuncs []func()
		afterFuncs  []func()
		Writer      http.ResponseWriter
		Status      int
		Size        int64
		Committed   bool
		WriteBytes  *bytes.Buffer
	}
)

func NewResponse(w http.ResponseWriter, e *HttpServer) (r *Response) {
	return &Response{Writer: w, echo: e}
}

func (r *Response) Header() http.Header {
	return r.Writer.Header()
}

func (r *Response) Before(fn func()) {
	r.beforeFuncs = append(r.beforeFuncs, fn)
}

func (r *Response) After(fn func()) {
	r.afterFuncs = append(r.afterFuncs, fn)
}

func (r *Response) WriteHeader(code int) {
	if r.Committed {
		return
	}
	r.Status = code
	for _, fn := range r.beforeFuncs {
		fn()
	}
	r.Writer.WriteHeader(r.Status)
	r.Committed = true
}

func (r *Response) Write(b []byte) (n int, err error) {
	if !r.Committed {
		if r.Status == 0 {
			r.Status = http.StatusOK
		}
		r.WriteHeader(r.Status)
	}
	n, err = r.Writer.Write(b)
	r.Size += int64(n)
	for _, fn := range r.afterFuncs {
		fn()
	}
	return
}

func (r *Response) SetWriteBytes(b []byte) {
	r.WriteBytes.Write(b)
}

func (r *Response) GetWriteBytes(n int) ([]byte, error) {
	if r.WriteBytes == nil {
		return nil, errors.New("Response init failed")
	}
	if n <= 0 {
		n = r.WriteBytes.Len()
	}
	return r.WriteBytes.Next(n), nil
}

func (r *Response) Flush() {
	r.Writer.(http.Flusher).Flush()
}

func (r *Response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return r.Writer.(http.Hijacker).Hijack()
}

func (r *Response) reset(w http.ResponseWriter) {
	r.beforeFuncs = nil
	r.afterFuncs = nil
	r.Writer = w
	r.Size = 0
	r.Status = http.StatusOK
	r.Committed = false
	if r.WriteBytes != nil {
		r.WriteBytes.Reset() // reset
	} else {
		r.WriteBytes = new(bytes.Buffer)
		r.WriteBytes.Grow(defaultWriteBytesSize)
	}
}
