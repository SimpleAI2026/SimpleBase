package testdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/json-iterator/go"
)

// Basic 基本运算
func (__testSet) Basic() interface{} {
	a := 3
	b := 4
	c := "hello"
	d := "world"
	p := &a
	*p = 5
	return []interface{}{
		a + 1,
		a + b - a*b,
		c + " " + d,
		int8(a) + 1,
		a < b && b > 7 && a == 3 && b >= 2 && b <= 5 || c != d,
	}
}

var S1 int
var S2 = 2
var S3 int
var r *regexp.Regexp

// Global 全局变量测试
func (__testSet) Global() interface{} {
	S2++
	S3 = 5
	r = regexp.MustCompile(`\w`)
	return []interface{}{S1, S2, S3, r.MatchString("abc")}
}

// Import 包导入测试
func (__testSet) Import() interface{} {
	s := "test"
	log := math.Log
	if len(s) > 0 {
		log = math.Log10
	}
	now := time.Now()
	r, _ := regexp.Compile("ab")
	loc, _ := time.LoadLocation("Local")
	v := jsoniter.Get([]byte(`{"k": "v"}`), "k", "a").ToString()

	return []interface{}{
		fmt.Sprintf("%s %s", "hello", s),
		now.Add(-10 * time.Hour).Before(now),
		log(math.Ln10),
		errors.New("test"),
		strconv.Itoa(10),
		r.MatchString("abc"),
		loc.String(),
		v,
	}
}

// NamedType 类型测试
func (__testSet) NamedType() interface{} {
	duration := time.Minute + time.Time{}.Add(time.Minute).Sub(time.Time{})
	return []interface{}{duration, time.Minute}
}

// NilInterface nil接口判断逻辑测试
func (__testSet) NilInterface() interface{} {
	req, _ := http.NewRequest("GET", "http://www.qq.com", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	return res.Status
}

// Recover recover测试
func (__testSet) Recover() (ret interface{}) {
	defer func() {
		_ = recover()
		ret = 10
	}()
	var nilTime *time.Time
	return nilTime.Hour()
}

func producer(ch chan int) {
	for i := 0; i < 5; i++ {
		ch <- i
	}
	close(ch)
}

// Channel channel测试
func (__testSet) Channel() (ret interface{}) {
	result := make([]interface{}, 0)
	ch := make(chan int, 10)
	go producer(ch)
	for i := range ch {
		result = append(result, i)
	}
	return result
}

// TypeAssert 类型断言测试
func (__testSet) TypeAssert() (ret interface{}) {
	f := func() interface{} {
		return int(10)
	}

	s, ok1 := f().(string)
	i, ok2 := f().(int)
	return []interface{}{s, ok1, i, ok2, f().(int) + 1}
}

// Closure 闭包测试
func (__testSet) Closure() (ret interface{}) {
	s := 1
	inc := func() {
		s++
	}
	for s := 0; s < 10; s++ {
		inc()
	}
	return s
}

// Map map测试
func (__testSet) Map() (ret interface{}) {
	m := map[string]interface{}{"a": -1, "b": "s", "c": nil}
	m["d"] = m["a"]
	delete(m, "a")
	v1, ok1 := m["c"]
	v2, ok2 := m["a"]
	return []interface{}{len(m), v1, ok1, v2, ok2, m}
}

// ForRange for range测试
func (__testSet) ForRange() (ret interface{}) {
	sum := 0
	for i, j := range []int{1, 2, 3, 4} {
		sum += i + j
	}
	for k, v := range map[int]int{1: 1, 2: 2, 3: 3, 4: 4} {
		sum += k + v
	}
	return sum
}

func fib(i int) int {
	if i <= 1 {
		return i
	}
	return fib(i-1) + fib(i-2)
}

func call(f func(...int) (int, int), e ...int) (int, int) {
	return f(e...)
}

// Call 函数调用测试
func (__testSet) Call() (ret interface{}) {
	sum := func(e ...int) (s int, n int) {
		for _, i := range e {
			s += i
		}
		return s, len(e)
	}
	s, n := call(sum, 1, 2, 3, 4)
	return []interface{}{fib(10), s, n}
}

type S struct {
	A int
	B int
}

func (s *S) Elems() (int, int) {
	return s.A, s.B
}

// Struct 结构体测试
func (__testSet) Struct() interface{} {
	s1 := &S{3, 5}
	s2 := &S{
		B: 1,
	}
	s2.A = 5
	c, d := s1.Elems()
	return c + d + s2.A + s2.B
}

// String 字符串切片测试
func (__testSet) String() interface{} {
	s := "hello world"
	return []interface{}{s, s[:1], s[1:], s[5:6]}
}

var m = map[string]string{"1": "2"}

// GlobalMap 全局Map测试
func (__testSet) GlobalMap() interface{} {
	return m
}

// MapNilCheck map nil判断测试
func (__testSet) MapNilCheck() interface{} {
	var m1 map[string]interface{}
	var m2 = map[string]interface{}{}
	m3 := map[string]interface{}{"a": "b"}

	return []interface{}{m1 == nil, m2 == nil, m3 == nil}
}

// StructFieldPtr 结构体字段指针测试
func (__testSet) StructFieldPtr() interface{} {
	blob := []byte{103, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 95, 155, 121, 238, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 95, 155, 121, 238}
	vBuf := bytes.NewReader(blob[2:])

	st := struct {
		A uint64
		B uint16
		C uint64
	}{}
	binary.Read(vBuf, binary.BigEndian, &st.A)
	binary.Read(vBuf, binary.BigEndian, &st.B)
	binary.Read(vBuf, binary.BigEndian, &st.C)
	return st
}
