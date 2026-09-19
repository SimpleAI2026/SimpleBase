package value

import "reflect"

// MapIter range 迭代器，对应 ssa.Range + ssa.Next 指令对的迭代状态。
// Next 返回 (ok, key, val) 三元组，与 Go 的 range 语义一致。
// 除 map 外同时支持 string（rune 序列）与 slice/array（下标序列）
// —— SSA 对这两者也走 Range/Next 协议。
// 实现 Value 接口以便作为解释器通用值在 env 中传递。
type MapIter struct {
	I     int             // 当前迭代下标
	Value Value           // 被迭代的容器
	Keys  []reflect.Value // map：预取的全部 key；string： rune 序列占位；slice： 无用
	// seqKind 序列种类：0 map / 1 string / 2 slice（由 runRange 设置）
	seqKind int
	// runes string 场景下预解码的 rune 切片
	runes []rune
}

// NewStringIter 构造 string 迭代器（按 rune 解码）
func NewStringIter(s string) *MapIter {
	return &MapIter{seqKind: 1, runes: []rune(s), Value: NewRValueOf(reflect.ValueOf(s))}
}

// NewSliceIter 构造 slice/array 迭代器
func NewSliceIter(v Value) *MapIter {
	return &MapIter{seqKind: 2, Value: v}
}

// IsSeq 是否为序列迭代器（string/slice），供 runNext 分派
func (iter *MapIter) IsSeq() bool { return iter.seqKind != 0 }

// Len 序列长度（string rune 数 / slice 元素数）
func (iter *MapIter) SeqLen() int {
	switch iter.seqKind {
	case 1:
		return len(iter.runes)
	case 2:
		rv := iter.Value.RValue()
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		return rv.Len()
	}
	return 0
}

// SeqElem 取序列第 i 个元素：
//   - string：返回 (rune 索引, rune 值)
//   - slice：返回 (下标, 元素值)
func (iter *MapIter) SeqElem(i int) (reflect.Value, reflect.Value) {
	if iter.seqKind == 1 {
		return reflect.ValueOf(i), reflect.ValueOf(iter.runes[i])
	}
	rv := iter.Value.RValue()
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return reflect.ValueOf(i), rv.Index(i)
}

// Next 迭代下一个元素。
// 返回值为 []Value{ok, key, val} 三元组；迭代结束时 ok 为 false。
func (iter *MapIter) Next() Value {
	ok, k, v := iter.next()
	return RValue{Value: reflect.ValueOf([]Value{
		RValue{Value: reflect.ValueOf(ok)},
		RValue{Value: k},
		RValue{Value: v},
	})}
}

func (iter *MapIter) next() (bool, reflect.Value, reflect.Value) {
	if iter.seqKind != 0 {
		n := iter.SeqLen()
		if iter.I >= n {
			return false, reflect.Value{}, reflect.Value{}
		}
		k, v := iter.SeqElem(iter.I)
		iter.I++
		return true, k, v
	}
	// map 路径
	if iter.I >= len(iter.Keys) {
		return false, reflect.Value{}, reflect.Value{}
	}
	key := iter.Keys[iter.I]
	rv := iter.Value.RValue()
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	val := rv.MapIndex(key)
	iter.I++
	return true, key, val
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

func (iter *MapIter) Len() int { return len(iter.Keys) - iter.I }
func (iter *MapIter) Cap() int { return len(iter.Keys) - iter.I }
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

// Package 将多个返回值打包为元组值，
// 用于 SSA 多返回值指令的 Extract 提取。
func Package(results []reflect.Value) Value {
	if len(results) == 1 {
		return RValue{Value: results[0]}
	}
	vals := make([]Value, len(results))
	for i, r := range results {
		vals[i] = RValue{Value: r}
	}
	return RValue{Value: reflect.ValueOf(vals)}
}

// Unpackage 将元组值解包为 reflect.Value 切片，
// 是 Package 的逆操作，用于闭包调用返回值还原。
func Unpackage(v Value) []reflect.Value {
	tuple := v.RValue()
	if tuple.Kind() != reflect.Slice {
		return []reflect.Value{tuple}
	}
	results := make([]reflect.Value, tuple.Len())
	for i := range results {
		results[i] = tuple.Index(i).Interface().(Value).RValue()
	}
	return results
}
