package value

import "reflect"

// MapIter map 迭代器，对应 ssa.Next 指令的迭代状态。
// Next 返回 (ok, key, val) 三元组，与 Go 的 range 语义一致。
// 实现 Value 接口以便作为解释器通用值在 env 中传递。
type MapIter struct {
	I     int             // 当前迭代下标
	Value Value           // 被迭代的 map
	Keys  []reflect.Value // 预取的全部 key
}

// Next 迭代下一个元素。
// 返回值为 []Value{ok, key, val} 三元组；迭代结束时 ok 为 false。
func (iter *MapIter) Next() Value {
	if iter.I >= len(iter.Keys) {
		return RValue{Value: reflect.ValueOf([]Value{
			RValue{Value: reflect.ValueOf(false)},
			RValue{Value: reflect.Zero(interfaceType)},
			RValue{Value: reflect.Zero(interfaceType)},
		})}
	}
	key := iter.Keys[iter.I]
	val := iter.Value.RValue().MapIndex(key)
	iter.I++
	return RValue{Value: reflect.ValueOf([]Value{
		RValue{Value: reflect.ValueOf(true)},
		RValue{Value: key},
		RValue{Value: val},
	})}
}

// interfaceType 空 interface 的 reflect 类型
var interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()

// --- Value 接口实现（迭代器作为不透明值传递，仅支持元信息） ---

func (iter *MapIter) Interface() interface{} { return iter }

func (iter *MapIter) String() string { return "map-iterator" }

func (iter *MapIter) Type() reflect.Type { return reflect.TypeOf(iter) }

func (iter *MapIter) Kind() reflect.Kind { return reflect.Struct }

func (iter *MapIter) IsValid() bool { return true }

func (iter *MapIter) IsNil() bool { return false }

func (iter *MapIter) Int() int64   { panic("MapIter does not support Int") }
func (iter *MapIter) Uint() uint64 { panic("MapIter does not support Uint") }
func (iter *MapIter) Float() float64 {
	panic("MapIter does not support Float")
}
func (iter *MapIter) Bool() bool { panic("MapIter does not support Bool") }

func (iter *MapIter) Len() int   { return len(iter.Keys) - iter.I }
func (iter *MapIter) Cap() int   { return len(iter.Keys) - iter.I }
func (iter *MapIter) Index(_ int) Value {
	panic("MapIter does not support Index")
}
func (iter *MapIter) MapIndex(_ Value) Value {
	panic("MapIter does not support MapIndex")
}
func (iter *MapIter) Field(_ int) Value { panic("MapIter does not support Field") }
func (iter *MapIter) Elem() Value       { panic("MapIter does not support Elem") }
func (iter *MapIter) Set(_ Value)       { panic("MapIter does not support Set") }
func (iter *MapIter) RValue() reflect.Value {
	return reflect.ValueOf(iter)
}
