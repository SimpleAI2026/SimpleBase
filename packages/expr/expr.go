package expr

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrUnsupportedType = errors.New("unsupported type")
	ErrDivideZero      = errors.New("divide zero")
	ErrPowOfZero       = errors.New("power of zero")

	// default Value
	nilValue = Value{kind: KindInvalid}
	varZero  = Value{kind: KindInt, rawValue: "0"}
	varTrue  = Value{kind: KindInt, intValue: 1, rawValue: "true"}
	varFalse = Value{kind: KindInt, intValue: 0, rawValue: "false"}
	varType  = reflect.TypeOf(nilValue).Kind()
)

const (
	KindInvalid Kind = iota
	KindInt
	KindFloat
	KindString

	OpKindAdd OpKind = iota
	OpKindSub
	OpKindMul
	OpKindQuo
	OpKindRem
	OpKindPow
	OpKindOr
	OpKindNot
	OpKindEq
	OpKindNe
	OpKindGt
	OpKindGe
	OpKindLt
	OpKindLe
)

type (
	Kind   int
	OpKind int

	Value struct {
		kind       Kind
		rawValue   string
		intValue   int64
		floatValue float64
	}
)

func NewValue(v interface{}) (Value, error) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int:
		return Value{kind: KindInt, intValue: int64(v.(int))}, nil
	case reflect.Int16:
		return Value{kind: KindInt, intValue: int64(v.(int16))}, nil
	case reflect.Int32:
		return Value{kind: KindInt, intValue: int64(v.(int32))}, nil
	case reflect.Int64:
		return Value{kind: KindInt, intValue: v.(int64)}, nil
	case reflect.Float32:
		return Value{kind: KindFloat, floatValue: float64(v.(float32))}, nil
	case reflect.Float64:
		return Value{kind: KindFloat, floatValue: v.(float64)}, nil
	case reflect.String:
		return Value{kind: KindString, rawValue: v.(string)}, nil
	case varType: // if value type and return
		return v.(Value), nil
	}
	return Value{kind: KindInvalid}, ErrUnsupportedType
}

func (v Value) Kind() Kind     { return v.kind }
func (v Value) String() string { return v.rawValue }
func (v Value) Int() int64 {
	switch v.kind {
	case KindFloat:
		return int64(v.floatValue)
	}
	return v.intValue
}
func (v Value) Float() float64 {
	switch v.kind {
	case KindInt:
		return float64(v.intValue)
	}
	return v.floatValue
}
func (v Value) Bool() bool {
	switch v.kind {
	case KindString:
		return v.rawValue != ""
	case KindInt:
		return v.intValue != 0
	case KindFloat:
		return v.floatValue != 0
	}
	return false
}
func (v Value) Interface() interface{} {
	switch v.kind {
	case KindString:
		return v.rawValue
	case KindInt:
		return v.intValue
	case KindFloat:
		return v.floatValue
	}
	return nil
}

func (v Value) IsString() bool { return v.kind == KindString }
func (v Value) IsInt() bool    { return v.kind == KindFloat || v.kind == KindInt }
func (v Value) IsFloat() bool  { return v.kind == KindFloat || v.kind == KindInt }
func (v Value) IsBool() bool {
	return v.kind == KindString || v.kind == KindInt || v.kind == KindFloat || v.kind == KindInvalid
}
func (v Value) IsInvalid() bool {
	return v.kind == KindInvalid
}

func (v Value) Add(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindAdd) }
func (v Value) Sub(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindSub) }
func (v Value) Mul(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindMul) }
func (v Value) Quo(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindQuo) }
func (v Value) Rem(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindRem) }
func (v Value) Pow(v2 Value) (Value, error) { return binaryOp(v, v2, OpKindPow) }
func (v Value) And(v2 Value) Value          { return Bool(v.Bool() && v2.Bool()) }
func (v Value) Or(v2 Value) Value           { return Bool(v.Bool() || v2.Bool()) }
func (v Value) Not() Value                  { return Bool(!v.Bool()) }
func (v Value) Eq(v2 Value) (Value, error)  { return compare(v, v2, OpKindEq) }
func (v Value) Ne(v2 Value) (Value, error) {
	if result, err := v.Eq(v2); err == nil {
		return result.Not(), nil
	} else {
		return result, err
	}
}
func (v Value) Gt(v2 Value) (Value, error) { return compare(v, v2, OpKindGt) }
func (v Value) Ge(v2 Value) (Value, error) { return compare(v, v2, OpKindGe) }
func (v Value) Lt(v2 Value) (Value, error) { return v2.Gt(v) }
func (v Value) Le(v2 Value) (Value, error) { return v2.Ge(v) }

func Bool(ok bool) Value {
	if ok {
		return varTrue
	}
	return varFalse
}
func Int(i int64) Value { return Value{kind: KindInt, intValue: i, rawValue: strconv.FormatInt(i, 10)} }
func Float(f float64) Value {
	return Value{kind: KindFloat, floatValue: f, rawValue: fmt.Sprintf("%f", f)}
}
func String(s string) Value { return Value{kind: KindString, rawValue: s} }
func ValuesToInterfaces(args ...Value) []interface{} {
	var rInterfaces []interface{}
	for _, v := range args {
		rInterfaces = append(rInterfaces, v.Interface())
	}
	return rInterfaces
}

type (
	BuiltinFunc func(...interface{}) (interface{}, error)

	Expr struct {
		root ast.Expr
		pool *Pool
	}

	Getter map[string]interface{}
)

func (getter Getter) get(name string) (Value, bool) {
	if getter == nil {
		return nilValue, false
	}
	if v, ok := getter[name]; !ok {
		switch name {
		case "true", "True": // true True => varTrue
			return varTrue, true
		case "false", "False": // false False nil null => varFalse
			return varFalse, true
		case "nil":
			return nilValue, true
		}
		return nilValue, false
	} else {
		rValue, err := NewValue(v)
		return rValue, err == nil
	}
}

var (
	defaultPool = func() *Pool {
		if p, err := NewPool(); err != nil {
			panic(err)
		} else {
			return p
		}
	}()

	defaultOnVarMissing = func(varName string) (Value, error) {
		return varZero, fmt.Errorf("var `%s' missing", varName)
	}
)

func New(s string, pool *Pool) (*Expr, error) {
	s = strings.TrimSpace(s)
	if pool == nil {
		pool = defaultPool
	}
	if e, ok := pool.get(s); ok {
		return e, nil
	}
	e := new(Expr)
	e.pool = pool
	if err := e.parse(s); err != nil {
		return nil, err
	}
	pool.set(s, e)
	return e, nil
}

// parse parses string s
func (e *Expr) parse(s string) error {
	if s == "" {
		return nil
	}
	node, err := parser.ParseExpr(s)
	if err != nil {
		return err
	}
	e.root = node
	v := &visitor{pool: e.pool}
	ast.Walk(v, e.root)
	return v.err
}

type visitor struct {
	pool *Pool
	err  error
}

// Visit implements ast.Visitor Visit method
func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if n, ok := node.(*ast.CallExpr); ok {
		if fnIdent, ok := n.Fun.(*ast.Ident); ok {
			if _, ok := v.pool.builtinList[fnIdent.Name]; ok {
				return v
			} else {
				v.err = fmt.Errorf("undefined function `%v`", fnIdent.Name)
			}
		} else {
			v.err = fmt.Errorf("unsupported call expr")
		}
		return nil
	}
	return v
}

func (e *Expr) Eval(getter Getter) (Value, error) {
	if e.root == nil {
		return varZero, nil
	}
	v, err := eval(e, getter, e.root)
	if err != nil {
		return varZero, err
	}
	return v, nil
}

func Eval(s string, getter map[string]interface{}, pool *Pool) (Value, error) {
	e, err := New(s, pool)
	if err != nil {
		return varZero, err
	}
	return e.Eval(Getter(getter))
}

func eval(e *Expr, getter Getter, node ast.Expr) (Value, error) {
	switch n := node.(type) {
	case *ast.Ident:
		if getter == nil {
			return e.pool.onVarMissing(n.Name)
		}
		if val, ok := getter.get(n.Name); !ok {
			return e.pool.onVarMissing(n.Name)
		} else {
			return val, nil
		}

	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			if i, err := strconv.ParseInt(n.Value, 10, 64); err != nil {
				return varZero, err
			} else {
				return Int(i), nil
			}
		case token.FLOAT:
			if f, err := strconv.ParseFloat(n.Value, 64); err != nil {
				return varZero, err
			} else {
				return Float(f), nil
			}
		case token.CHAR, token.STRING:
			if s, err := strconv.Unquote(n.Value); err != nil {
				return varZero, err
			} else {
				return String(s), nil
			}
		default:
			return varZero, fmt.Errorf("unsupported token: %s(%v)", n.Value, n.Kind)
		}

	case *ast.ParenExpr:
		return eval(e, getter, n.X)

	case *ast.CallExpr:
		args := make([]Value, 0, len(n.Args))
		for _, arg := range n.Args {
			if val, err := eval(e, getter, arg); err != nil {
				return varZero, err
			} else {
				args = append(args, val)
			}
		}
		if fnIdent, ok := n.Fun.(*ast.Ident); ok {
			return e.pool.builtinCall(fnIdent.Name, args...)
		}
		return varZero, fmt.Errorf("unexpected func type: %T", n.Fun)

	case *ast.UnaryExpr:
		switch n.Op {
		case token.ADD:
			return eval(e, getter, n.X)
		case token.SUB:
			x, err := eval(e, getter, n.X)
			if err == nil {
				x, err = varZero.Sub(x)
			}
			return x, err
		case token.NOT:
			x, err := eval(e, getter, n.X)
			if err == nil {
				x = x.Not()
			}
			return x, err
		default:
			return varZero, fmt.Errorf("unsupported unary op: %v", n.Op)
		}

	case *ast.BinaryExpr:
		x, err := eval(e, getter, n.X)
		if err != nil {
			return varZero, err
		}
		y, err := eval(e, getter, n.Y)
		if err != nil {
			return varZero, err
		}
		switch n.Op {
		case token.ADD:
			return x.Add(y)
		case token.SUB:
			return x.Sub(y)
		case token.MUL:
			return x.Mul(y)
		case token.QUO:
			return x.Quo(y)
		case token.REM:
			return x.Rem(y)
		case token.XOR:
			return x.Pow(y)
		case token.LAND:
			return x.And(y), nil
		case token.LOR:
			return x.Or(y), nil
		case token.EQL:
			return x.Eq(y)
		case token.NEQ:
			return x.Ne(y)
		case token.GTR:
			return x.Gt(y)
		case token.GEQ:
			return x.Ge(y)
		case token.LSS:
			return x.Lt(y)
		case token.LEQ:
			return x.Le(y)
		default:
			return varZero, fmt.Errorf("unexpected binary operator: %v", n.Op)
		}

	default:
		return varZero, fmt.Errorf("unexpected node type %T", n)
	}
}

type (
	VarMissingFunc func(string) (Value, error)

	Pool struct {
		locker sync.RWMutex
		pool   map[string]*Expr

		builtinList  map[string]BuiltinFunc
		onVarMissing VarMissingFunc
	}
)

func NewPool(builtinList ...map[string]BuiltinFunc) (*Pool, error) {
	p := &Pool{
		pool:         make(map[string]*Expr),
		builtinList:  map[string]BuiltinFunc{},
		onVarMissing: defaultOnVarMissing,
	}
	for _, builtin := range builtinList {
		for name, fn := range builtin {
			p.builtinList[name] = fn
		}
	}
	return p, nil
}

func (p *Pool) SetOnVarMissing(fn VarMissingFunc) {
	p.onVarMissing = fn
}

func (p *Pool) get(s string) (*Expr, bool) {
	p.locker.RLock()
	defer p.locker.RUnlock()
	e, ok := p.pool[s]
	return e, ok && e != nil
}

func (p *Pool) set(s string, e *Expr) {
	p.locker.Lock()
	defer p.locker.Unlock()
	p.pool[s] = e
}

func (p *Pool) builtinCall(name string, args ...Value) (Value, error) {
	if fn, ok := p.builtinList[name]; ok {
		if v, err := fn(ValuesToInterfaces(args...)...); err == nil {
			return NewValue(v)
		} else {
			return nilValue, err
		}
	}
	return nilValue, fmt.Errorf("undefined function `%v`", name)
}

var (
	floatOpFunc = func(v1, v2 Value, op OpKind) (Value, error) {
		switch op {
		case OpKindAdd:
			return Float(v1.Float() + v2.Float()), nil
		case OpKindSub:
			return Float(v1.Float() - v2.Float()), nil
		case OpKindMul:
			return Float(v1.Float() * v2.Float()), nil
		case OpKindQuo:
			return Float(v1.Float() / v2.Float()), nil
		case OpKindRem:
			return Float(math.Remainder(v1.Float(), v2.Float())), nil
		case OpKindPow:
			if v1.Float() == 0 {
				return varZero, ErrPowOfZero
			}
			return Float(math.Pow(v1.Float(), v2.Float())), nil
		}
		return nilValue, ErrUnsupportedType
	}

	intOpFunc = func(v1, v2 Value, op OpKind) (Value, error) {
		switch op {
		case OpKindAdd:
			return Int(v1.Int() + v2.Int()), nil
		case OpKindSub:
			return Int(v1.Int() - v2.Int()), nil
		case OpKindMul:
			return Int(v1.Int() * v2.Int()), nil
		case OpKindQuo:
			if v2.Int() == 0 {
				return varZero, ErrDivideZero
			}
			return Int(v1.Int() / v2.Int()), nil
		case OpKindRem:
			if v2.Int() == 0 {
				return varZero, ErrDivideZero
			}
			return Int(v1.Int() % v2.Int()), nil
		case OpKindPow:
			if v1.Int() == 0 {
				return varZero, ErrPowOfZero
			}
			return Int(int64(math.Pow(float64(v1.Int()), float64(v2.Int())))), nil
		}
		return nilValue, ErrUnsupportedType
	}
)

func binaryOp(v1, v2 Value, op OpKind) (Value, error) {
	switch v1.kind {
	case KindString:
		if v2.IsString() {
			switch op {
			case OpKindAdd:
				return String(v1.String() + v2.String()), nil
			}
		}
	case KindFloat:
		if v2.IsFloat() { // if is float type and call floatOpFunc
			return floatOpFunc(v1, v2, op)
		}
	case KindInt:
		if v2.kind == KindFloat { // if any var is float type and call floatOpFunc
			return floatOpFunc(v1, v2, op)
		}
		if v2.IsInt() { // if is int type and call intOpFunc
			return intOpFunc(v1, v2, op)
		}
	}
	return varZero, ErrUnsupportedType
}

func compare(v1, v2 Value, op OpKind) (Value, error) {
	switch v1.kind {
	case KindString:
		if v2.IsString() {
			switch op {
			case OpKindEq:
				return Bool(v1.String() == v2.String()), nil
			case OpKindGt:
				return Bool(v1.String() > v2.String()), nil
			case OpKindGe:
				return Bool(v1.String() >= v2.String()), nil
			}
		} else if v2.IsInvalid() {
			return varFalse, nil
		}
	case KindFloat:
		if v2.IsFloat() {
			switch op {
			case OpKindEq:
				return Bool(v1.Float() == v2.Float()), nil
			case OpKindGt:
				return Bool(v1.Float() > v2.Float()), nil
			case OpKindGe:
				return Bool(v1.Float() >= v2.Float()), nil
			}
		} else if v2.IsInvalid() {
			return varFalse, nil
		}
	case KindInt:
		if v2.IsInt() {
			switch op {
			case OpKindEq:
				return Bool(v1.Int() == v2.Int()), nil
			case OpKindGt:
				return Bool(v1.Int() > v2.Int()), nil
			case OpKindGe:
				return Bool(v1.Int() >= v2.Int()), nil
			}
		} else if v2.IsInvalid() {
			return varFalse, nil
		}
	case KindInvalid:
		if v2.Kind() == KindInvalid {
			return varTrue, nil
		}
		return varFalse, nil
	}
	return varFalse, ErrUnsupportedType
}
