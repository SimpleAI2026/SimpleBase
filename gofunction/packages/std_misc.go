package packages

import (
	"errors"
	"strconv"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdErrors errors 包注册清单
func stdErrors() []*importer.Object {
	return []*importer.Object{
		funcObj("New", errors.New),
		funcObj("Is", errors.Is),
		funcObj("As", errors.As),
		funcObj("Unwrap", errors.Unwrap),
	}
}

// stdStrconv strconv 包注册清单
func stdStrconv() []*importer.Object {
	return []*importer.Object{
		funcObj("Itoa", strconv.Itoa),
		funcObj("Atoi", strconv.Atoi),
		funcObj("ParseInt", strconv.ParseInt),
		funcObj("ParseFloat", strconv.ParseFloat),
		funcObj("ParseBool", strconv.ParseBool),
		funcObj("FormatInt", strconv.FormatInt),
		funcObj("FormatFloat", strconv.FormatFloat),
	}
}
