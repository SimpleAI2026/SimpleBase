package packages

import (
	"reflect"
	"regexp"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdTime time 包注册清单
func stdTime() []*importer.Object {
	return []*importer.Object{
		typeObj("Time", reflect.TypeOf(time.Time{})),
		typeObj("Duration", reflect.TypeOf(time.Duration(0))),
		typeObj("Location", reflect.TypeOf((*time.Location)(nil)).Elem()),
		typeObj("Weekday", reflect.TypeOf(time.Sunday)),
		funcObj("Now", time.Now),
		funcObj("LoadLocation", time.LoadLocation),
		funcObj("Date", time.Date),
		funcObj("Parse", time.Parse),
		funcObj("Since", time.Since),
		funcObj("Unix", time.Unix),
		constObj("Second", time.Second),
		constObj("Millisecond", time.Millisecond),
		constObj("Microsecond", time.Microsecond),
		constObj("Nanosecond", time.Nanosecond),
		constObj("Minute", time.Minute),
		constObj("Hour", time.Hour),
		constObj("Saturday", time.Saturday),
		constObj("Sunday", time.Sunday),
		constObj("Monday", time.Monday),
	}
}

// stdRegexp regexp 包注册清单。
// Regexp 类型供 var r *regexp.Regexp 声明解析，
// Compile/MustCompile 返回 *Regexp，其方法通过反射自动可用。
func stdRegexp() []*importer.Object {
	return []*importer.Object{
		typeObj("Regexp", reflect.TypeOf((*regexp.Regexp)(nil)).Elem()),
		funcObj("MatchString", regexp.MatchString),
		funcObj("QuoteMeta", regexp.QuoteMeta),
		funcObj("Compile", regexp.Compile),
		funcObj("MustCompile", regexp.MustCompile),
	}
}
