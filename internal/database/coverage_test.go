package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

func TestQueryAndExecuteErrors(t *testing.T) {
	db := newMemDB(t)
	if _, err := Query(context.Background(), db, Statement{SQL: "SELECT * FROM no_such"}, 10); err == nil {
		t.Fatal("query error")
	}
	if _, err := Execute(context.Background(), db, Statement{SQL: "UPDATE no_such SET x=1"}); err == nil {
		t.Fatal("exec error")
	}
	if _, err := Batch(context.Background(), db, []Statement{{SQL: "INSERT INTO t(name) VALUES('ok')"}}, false); err != nil {
		t.Fatal(err)
	}
}

type failResult struct{}

func (failResult) LastInsertId() (int64, error) { return 0, errors.New("no") }
func (failResult) RowsAffected() (int64, error) { return 0, errors.New("no") }

type failExecer struct{}

func (failExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return failResult{}, nil
}

func TestExecuteOnRowsAffectedError(t *testing.T) {
	res, err := executeOn(context.Background(), failExecer{}, Statement{SQL: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("affected=%d", res.RowsAffected)
	}
}

func TestSerializeValueRemainingTypes(t *testing.T) {
	cases := []struct {
		in   any
		check func(t *testing.T, v any)
	}{
		{int(1), func(t *testing.T, v any) {
			if v.(int64) != 1 {
				t.Fatal(v)
			}
		}},
		{int8(2), func(t *testing.T, v any) {
			if v.(int64) != 2 {
				t.Fatal(v)
			}
		}},
		{int16(3), func(t *testing.T, v any) {
			if v.(int64) != 3 {
				t.Fatal(v)
			}
		}},
		{int32(4), func(t *testing.T, v any) {
			if v.(int64) != 4 {
				t.Fatal(v)
			}
		}},
		{uint8(5), func(t *testing.T, v any) {
			if v.(int64) != 5 {
				t.Fatal(v)
			}
		}},
		{uint16(6), func(t *testing.T, v any) {
			if v.(int64) != 6 {
				t.Fatal(v)
			}
		}},
		{uint32(7), func(t *testing.T, v any) {
			if v.(int64) != 7 {
				t.Fatal(v)
			}
		}},
		{float32(1.5), func(t *testing.T, v any) {
			if v.(float64) != 1.5 {
				t.Fatal(v)
			}
		}},
		{(*big.Int)(nil), func(t *testing.T, v any) {
			if v != nil {
				t.Fatal(v)
			}
		}},
		{json.Number("9"), func(t *testing.T, v any) {
			if v.(string) != "9" {
				t.Fatal(v)
			}
		}},
		{json.RawMessage(`{"a":1}`), func(t *testing.T, v any) {
			m := v.(map[string]any)
			if m["a"] == nil {
				t.Fatal(v)
			}
		}},
		{json.RawMessage(nil), func(t *testing.T, v any) {
			if v != nil {
				t.Fatal(v)
			}
		}},
		{json.RawMessage(`not-json`), func(t *testing.T, v any) {
			if v.(string) != "not-json" {
				t.Fatal(v)
			}
		}},
		{duckdb.Interval{Months: 1, Days: 2, Micros: 3}, func(t *testing.T, v any) {
			if v.(string) != "1 months 2 days 3 micros" {
				t.Fatal(v)
			}
		}},
	}
	for i, tc := range cases {
		v, err := SerializeValue(tc.in)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		tc.check(t, v)
	}

	var uuid *duckdb.UUID
	v, err := SerializeValue(uuid)
	if err != nil || v != nil {
		t.Fatalf("nil uuid ptr: %v %v", v, err)
	}
	id := duckdb.UUID{}
	copy(id[:], []byte("0123456789abcdef"))
	v, err = SerializeValue(&id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(string); !ok {
		t.Fatalf("%T", v)
	}

	dm := duckdb.Map{"k": int64(1)}
	v, err = SerializeValue(dm)
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["k"].(int64) != 1 {
		t.Fatal(v)
	}

	u := duckdb.Union{Tag: "int", Value: int64(8)}
	v, err = SerializeValue(u)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["tag"] != "int" {
		t.Fatal(m)
	}

	type jsonable struct {
		A int `json:"a"`
	}
	v, err = SerializeValue(jsonable{A: 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["a"].(float64) != 1 {
		t.Fatal(v)
	}

	if _, err := serializeSlice([]any{complex(1, 2)}); !errors.Is(err, ErrUnsupportedValue) {
		t.Fatal(err)
	}
	if _, err := serializeMap(map[string]any{"x": complex(1, 2)}); !errors.Is(err, ErrUnsupportedValue) {
		t.Fatal(err)
	}
	if _, err := serializeDuckMap(duckdb.Map{"x": complex(1, 2)}); !errors.Is(err, ErrUnsupportedValue) {
		t.Fatal(err)
	}
	if mustSerialize(complex(1, 2)) == nil {
		t.Fatal("mustSerialize fallback")
	}
	if mustSerialize(int64(1)).(int64) != 1 {
		t.Fatal("mustSerialize ok")
	}
}

func TestBatchBeginTxFailure(t *testing.T) {
	db := newMemDB(t)
	_ = db.Close()
	if _, err := Batch(context.Background(), db, []Statement{{SQL: "SELECT 1"}}, true); err == nil {
		t.Fatal("expected begin/exec fail on closed db")
	}
	if _, err := Execute(context.Background(), db, Statement{SQL: "SELECT 1"}); err == nil {
		t.Fatal("execute on closed")
	}
}

func TestAccessModeZero(t *testing.T) {
	var m AccessMode
	if m.String() != "read_only" {
		t.Fatal(m.String())
	}
	_ = ErrQueryConcurrency
	_ = ErrUnsupportedValue
	_ = time.Now()
}
