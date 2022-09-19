package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type (
	Context interface {
		Request() *http.Request
		SetRequest(r *http.Request)
		SetResponse(r *Response)
		Response() *Response
		IsTLS() bool
		IsWebSocket() bool
		Scheme() string
		RealIP() string
		Path() string
		SetPath(p string)
		Headers(name string) string
		Param(name string) string
		ParamNames() []string
		SetParamNames(names ...string)
		ParamValues() []string
		SetParamValues(values ...string)
		QueryParam(name string) string
		QueryParams() url.Values
		QueryString() string
		FormValue(name string) string
		FormParams() (url.Values, error)
		FormFile(name string) (*multipart.FileHeader, error)
		MultipartForm() (*multipart.Form, error)
		Cookie(name string) (*http.Cookie, error)
		SetCookie(cookie *http.Cookie)
		Cookies() []*http.Cookie
		Get(key string) interface{}
		Set(key string, val interface{})
		Validate(i interface{}) error
		Render(code int, name string, data interface{}) error
		HTML(code int, html string) error
		HTMLBlob(code int, b []byte) error
		String(code int, s string) error
		JSON(code int, i interface{}) error
		JSONPretty(code int, i interface{}, indent string) error
		JSONBlob(code int, b []byte) error
		JSONP(code int, callback string, i interface{}) error
		JSONPBlob(code int, callback string, b []byte) error
		XML(code int, i interface{}) error
		XMLPretty(code int, i interface{}, indent string) error
		XMLBlob(code int, b []byte) error
		Blob(code int, contentType string, b []byte) error
		Stream(code int, contentType string, r io.Reader) error
		File(file string) error
		Attachment(file string, name string) error
		Inline(file string, name string) error
		NoContent(code int) error
		Redirect(code int, url string) error
		Error(err error)
		Handler() HandlerFunc
		SetHandler(h HandlerFunc)
		Echo() *HttpServer
		Reset(r *http.Request, w http.ResponseWriter)
	}

	context struct {
		request    *http.Request
		response   *Response
		path       string
		pnames     []string
		pvalues    []string
		query      url.Values
		handler    HandlerFunc
		store      Map
		httpServer *HttpServer
		lock       sync.RWMutex
	}
)

const (
	defaultMemory = 32 << 20 // 32 MB
	indexPage     = "index.html"
	defaultIndent = "  "
)

func (c *context) writeContentType(value string) {
	header := c.Response().Header()
	if header.Get(HeaderContentType) == "" {
		header.Set(HeaderContentType, value)
	}
}

func (c *context) Request() *http.Request {
	return c.request
}

func (c *context) SetRequest(r *http.Request) {
	c.request = r
}

func (c *context) Response() *Response {
	return c.response
}

func (c *context) SetResponse(r *Response) {
	c.response = r
}

func (c *context) IsTLS() bool {
	return c.request.TLS != nil
}

func (c *context) IsWebSocket() bool {
	upgrade := c.request.Header.Get(HeaderUpgrade)
	return strings.ToLower(upgrade) == "websocket"
}

func (c *context) Scheme() string {
	// Can't use `r.Request.URL.Scheme`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if c.IsTLS() {
		return "https"
	}
	if scheme := c.request.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := c.request.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := c.request.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := c.request.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}

func (c *context) RealIP() string {
	if ip := c.request.Header.Get(HeaderXForwardedFor); ip != "" {
		i := strings.IndexAny(ip, ", ")
		if i > 0 {
			return ip[:i]
		}
		return ip
	}
	if ip := c.request.Header.Get(HeaderXRealIP); ip != "" {
		return ip
	}
	ra, _, _ := net.SplitHostPort(c.request.RemoteAddr)
	return ra
}

func (c *context) Headers(name string) string {
	return c.request.Header.Get(name)
}

func (c *context) Path() string {
	return c.path
}

func (c *context) SetPath(p string) {
	c.path = p
}

func (c *context) Param(name string) string {
	for i, n := range c.pnames {
		if i < len(c.pvalues) {
			if n == name {
				return c.pvalues[i]
			}
		}
	}
	return ""
}

func (c *context) ParamNames() []string {
	return c.pnames
}

func (c *context) SetParamNames(names ...string) {
	c.pnames = names
	*c.httpServer.maxParam = len(names)
}

func (c *context) ParamValues() []string {
	return c.pvalues[:len(c.pnames)]
}

func (c *context) SetParamValues(values ...string) {
	c.pvalues = values
}

func (c *context) QueryParam(name string) string {
	if c.query == nil {
		c.query = c.request.URL.Query()
	}
	return c.query.Get(name)
}

func (c *context) QueryParams() url.Values {
	if c.query == nil {
		c.query = c.request.URL.Query()
	}
	return c.query
}

func (c *context) QueryString() string {
	return c.request.URL.RawQuery
}

func (c *context) FormValue(name string) string {
	return c.request.FormValue(name)
}

func (c *context) FormParams() (url.Values, error) {
	if strings.HasPrefix(c.request.Header.Get(HeaderContentType), MIMEMultipartForm) {
		if err := c.request.ParseMultipartForm(defaultMemory); err != nil {
			return nil, err
		}
	} else {
		if err := c.request.ParseForm(); err != nil {
			return nil, err
		}
	}
	return c.request.Form, nil
}

func (c *context) FormFile(name string) (*multipart.FileHeader, error) {
	f, fh, err := c.request.FormFile(name)
	if err != nil {
		return nil, err
	}
	f.Close()
	return fh, nil
}

func (c *context) MultipartForm() (*multipart.Form, error) {
	err := c.request.ParseMultipartForm(defaultMemory)
	return c.request.MultipartForm, err
}

func (c *context) Cookie(name string) (*http.Cookie, error) {
	return c.request.Cookie(name)
}

func (c *context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Response(), cookie)
}

func (c *context) Cookies() []*http.Cookie {
	return c.request.Cookies()
}

func (c *context) Get(key string) interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.store[key]
}

func (c *context) Set(key string, val interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.store == nil {
		c.store = make(Map)
	}
	c.store[key] = val
}

func (c *context) Render(code int, name string, data interface{}) (err error) {
	if c.httpServer.Renderer == nil {
		return ErrRendererNotRegistered
	}
	buf := new(bytes.Buffer)
	if err = c.httpServer.Renderer.Render(buf, name, data, c); err != nil {
		return
	}
	return c.HTMLBlob(code, buf.Bytes())
}

func (c *context) HTML(code int, html string) (err error) {
	return c.HTMLBlob(code, []byte(html))
}

func (c *context) HTMLBlob(code int, b []byte) (err error) {
	return c.Blob(code, MIMETextHTMLCharsetUTF8, b)
}

func (c *context) String(code int, s string) (err error) {
	return c.Blob(code, MIMETextPlainCharsetUTF8, []byte(s))
}

func (c *context) jsonPBlob(code int, callback string, i interface{}) (err error) {
	enc := json.NewEncoder(c.response)
	_, pretty := c.QueryParams()["pretty"]
	if c.httpServer.Debug || pretty {
		enc.SetIndent("", "  ")
	}
	c.writeContentType(MIMEApplicationJavaScriptCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(callback + "(")); err != nil {
		return
	}
	if err = enc.Encode(i); err != nil {
		return
	}
	if _, err = c.response.Write([]byte(");")); err != nil {
		return
	}
	return
}

func (c *context) json(code int, i interface{}, indent string) error {
	enc := json.NewEncoder(c.response)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	c.writeContentType(MIMEApplicationJSONCharsetUTF8)
	c.response.Status = code
	return enc.Encode(i)
}

func (c *context) JSON(code int, i interface{}) (err error) {
	indent := ""
	_, pretty := c.QueryParams()["pretty"]
	if c.httpServer.Debug || pretty {
		indent = defaultIndent
	}
	return c.json(code, i, indent)
}

func (c *context) JSONPretty(code int, i interface{}, indent string) (err error) {
	return c.json(code, i, indent)
}

func (c *context) JSONBlob(code int, b []byte) (err error) {
	return c.Blob(code, MIMEApplicationJSONCharsetUTF8, b)
}

func (c *context) JSONP(code int, callback string, i interface{}) (err error) {
	return c.jsonPBlob(code, callback, i)
}

func (c *context) JSONPBlob(code int, callback string, b []byte) (err error) {
	c.writeContentType(MIMEApplicationJavaScriptCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(callback + "(")); err != nil {
		return
	}
	if _, err = c.response.Write(b); err != nil {
		return
	}
	_, err = c.response.Write([]byte(");"))
	return
}

func (c *context) xml(code int, i interface{}, indent string) (err error) {
	c.writeContentType(MIMEApplicationXMLCharsetUTF8)
	c.response.WriteHeader(code)
	enc := xml.NewEncoder(c.response)
	if indent != "" {
		enc.Indent("", indent)
	}
	if _, err = c.response.Write([]byte(xml.Header)); err != nil {
		return
	}
	return enc.Encode(i)
}

func (c *context) XML(code int, i interface{}) (err error) {
	indent := ""
	_, pretty := c.QueryParams()["pretty"]
	if c.httpServer.Debug || pretty {
		indent = defaultIndent
	}
	return c.xml(code, i, indent)
}

func (c *context) XMLPretty(code int, i interface{}, indent string) (err error) {
	return c.xml(code, i, indent)
}

func (c *context) XMLBlob(code int, b []byte) (err error) {
	c.writeContentType(MIMEApplicationXMLCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(xml.Header)); err != nil {
		return
	}
	_, err = c.response.Write(b)
	return
}

func (c *context) Blob(code int, contentType string, b []byte) (err error) {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	_, err = c.response.Write(b)
	return
}

func (c *context) Stream(code int, contentType string, r io.Reader) (err error) {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	_, err = io.Copy(c.response, r)
	return
}

func (c *context) File(file string) (err error) {
	f, err := os.Open(file)
	if err != nil {
		return NotFoundHandler(c)
	}
	defer f.Close()

	fi, _ := f.Stat()
	if fi.IsDir() {
		file = filepath.Join(file, indexPage)
		f, err = os.Open(file)
		if err != nil {
			return NotFoundHandler(c)
		}
		defer f.Close()
		if fi, err = f.Stat(); err != nil {
			return
		}
	}
	http.ServeContent(c.Response(), c.Request(), fi.Name(), fi.ModTime(), f)
	return
}

func (c *context) Attachment(file, name string) error {
	return c.contentDisposition(file, name, "attachment")
}

func (c *context) Inline(file, name string) error {
	return c.contentDisposition(file, name, "inline")
}

func (c *context) contentDisposition(file, name, dispositionType string) error {
	c.response.Header().Set(HeaderContentDisposition, fmt.Sprintf("%s; filename=%q", dispositionType, name))
	return c.File(file)
}

func (c *context) NoContent(code int) error {
	c.response.WriteHeader(code)
	return nil
}

func (c *context) Redirect(code int, url string) error {
	if code < 300 || code > 308 {
		return ErrInvalidRedirectCode
	}
	c.response.Header().Set(HeaderLocation, url)
	c.response.WriteHeader(code)
	return nil
}

func (c *context) Error(err error) {
	c.httpServer.HTTPErrorHandler(err, c)
}

func (c *context) Echo() *HttpServer {
	return c.httpServer
}

func (c *context) Handler() HandlerFunc {
	return c.handler
}

func (c *context) SetHandler(h HandlerFunc) {
	c.handler = h
}

func (c *context) Reset(r *http.Request, w http.ResponseWriter) {
	c.request = r
	c.response.reset(w)
	c.query = nil
	c.handler = NotFoundHandler
	c.store = nil
	c.path = ""
	c.pnames = nil
	for i := 0; i < *c.httpServer.maxParam; i++ {
		c.pvalues[i] = ""
	}
}
