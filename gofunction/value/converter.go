package value

import (
	"fmt"
	"reflect"
)

// DefaultConverter 默认类型转换器
type DefaultConverter struct{}

// Convert 执行类型转换
func (dc *DefaultConverter) Convert(from Value, to reflect.Type) (Value, error) {
	fromValue := from.RValue()
	fromType := fromValue.Type()

	// 如果类型相同，直接返回
	if fromType == to {
		return from, nil
	}

	// 检查是否可以转换
	if !fromType.ConvertibleTo(to) {
		return nil, fmt.Errorf("cannot convert from %s to %s", fromType, to)
	}

	// 执行转换
	convertedValue := fromValue.Convert(to)
	return RValue{Value: convertedValue}, nil
}

// DefaultTypeConverter 全局转换器实例
var DefaultTypeConverter = &DefaultConverter{}
