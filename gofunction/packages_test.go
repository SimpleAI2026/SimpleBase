package gofunction

import (
	"testing"
)

func TestAtomic(t *testing.T) {
	sources := `
package main

import "sync/atomic"

func test() int32 {
	var val int32 = 0
	atomic.AddInt32(&val, 1)
	return val
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != int32(1) {
		t.Errorf("Expected 1 got %v", result)
	}
}

func TestBase64(t *testing.T) {
	sources := `
package main

import "encoding/base64"

func test() string {
	return base64.StdEncoding.EncodeToString([]byte("hello"))
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != "aGVsbG8=" {
		t.Errorf("Expected aGVsbG8= got %v", result)
	}
}

func TestBytes(t *testing.T) {
	sources := `
package main

import "bytes"

func test() bool {
	return bytes.Equal([]byte("hello"), []byte("hello"))
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != true {
		t.Errorf("Expected true got %v", result)
	}
}

func TestFmt(t *testing.T) {
	sources := `
package main

import "fmt"

func test() string {
	return fmt.Sprintf("Hello %s", "World")
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != "Hello World" {
		t.Errorf("Expected Hello World got %v", result)
	}
}

func TestHttp(t *testing.T) {
	sources := `
package main

import "net/http"

func test() string {
	return http.MethodGet
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != "GET" {
		t.Errorf("Expected GET got %v", result)
	}
}

func TestHttpRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping external HTTP in short mode")
	}
	sources := `
package main

import (
	"net/http"
	"io/ioutil"
)

func test() (int, string) {
	resp, err := http.Get("http://www.qq.com")
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	
	body, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
	`
	_, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestHttpsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping external HTTPS in short mode")
	}
	sources := `
package main

import (
	"net/http"
	"io/ioutil"
)

func test() (int, string) {
	resp, err := http.Get("https://www.qq.com")
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	
	body, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
	`
	_, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestJson(t *testing.T) {
	sources := `
package main

import "encoding/json"

func test() string {
	data := map[string]string{"name": "John"}
	b, _ := json.Marshal(data)
	return string(b)
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != `{"name":"John"}` {
		t.Errorf("Expected {\"name\":\"John\"} got %v", result)
	}
}

func TestMath(t *testing.T) {
	sources := `
package main

import "math"

func test() float64 {
	return math.Sqrt(16)
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != 4.0 {
		t.Errorf("Expected 4.0 got %v", result)
	}
}

func TestStrings(t *testing.T) {
	sources := `
package main

import "strings"

func test() string {
	return strings.ToUpper("hello")
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != "HELLO" {
		t.Errorf("Expected HELLO got %v", result)
	}
}

func TestTime(t *testing.T) {
	sources := `
package main

import "time"

func test() bool {
	return time.Second == 1000*time.Millisecond
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != true {
		t.Errorf("Expected true got %v", result)
	}
}

func TestRegexp(t *testing.T) {
	sources := `
package main

import "regexp"

func test() bool {
	matched, _ := regexp.MatchString("^[a-z]+$", "hello")
	return matched
}
	`
	result, err := Run("", sources, "test")
	if err != nil {
		t.Error(err)
		return
	}
	if result != true {
		t.Errorf("Expected true got %v", result)
	}
}
