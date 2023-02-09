package utils

import (
	"fmt"
	"reflect"
)

func ToStr(n interface{}) string {
	if reflect.ValueOf(n).Kind() == reflect.String {
		return n.(string)
	}
	return fmt.Sprintf("%v", n)
}
