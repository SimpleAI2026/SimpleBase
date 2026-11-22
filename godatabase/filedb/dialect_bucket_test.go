package filedb

import (
	"context"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

// TestDateBucketRealEngines 在 DuckDB 与 SQLite 上真实执行 DateBucket，
// 保证两个引擎对同一 unit 输出相同的文本桶（含 week 对齐到周一）。
func TestDateBucketRealEngines(t *testing.T) {
	cases := []struct {
		unit string
		in   string
		want string
	}{
		{"hour", "2026-11-18T09:45:12Z", "2026-11-18 09:00"},
		{"day", "2026-11-18T09:45:12Z", "2026-11-18"},
		// 2026-11-18 是周三，所在周的周一是 2026-11-16。
		{"week", "2026-11-18T09:45:12Z", "2026-11-16"},
		// 周日应回到同一周的周一（2026-11-16），而不是下一周。
		{"week", "2026-11-22T23:59:59Z", "2026-11-16"},
		// 周一本身不变。
		{"week", "2026-11-16T00:00:00Z", "2026-11-16"},
		{"month", "2026-11-18T09:45:12Z", "2026-11"},
		{"year", "2026-11-18T09:45:12Z", "2026"},
	}
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			databasePath, err := godatabase.PathForProject("prj00003", engine)
			if err != nil {
				t.Fatal(err)
			}
			factory := &Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: NewLocalSyncer()}
			db, err := factory.Open(ctx, godatabase.Spec{ID: "db3", ProjectID: "prj00003", DatabasePath: databasePath}, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			dialect, err := godatabase.DialectFor(engine)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cases {
				expr, ok := dialect.DateBucket(dialect.CastTimestamp("?"), c.unit)
				if !ok {
					t.Fatalf("%s: unit %s rejected", engine, c.unit)
				}
				// SQLite 的 datetime() 不接受带 Z 的 RFC3339 之外的歧义输入；此处统一传 RFC3339。
				var got string
				if err := db.QueryRowContext(ctx, "SELECT "+expr, bindN(expr, c.in)...).Scan(&got); err != nil {
					t.Fatalf("%s %s %s: %v\nsql: %s", engine, c.unit, c.in, err, expr)
				}
				if got != c.want {
					t.Fatalf("%s %s %s: got %q want %q", engine, c.unit, c.in, got, c.want)
				}
			}
		})
	}
}

// bindN 按占位符个数重复同一个参数（week 在 SQLite 里引用了两次表达式）。
func bindN(sql, value string) []any {
	n := 0
	for _, r := range sql {
		if r == '?' {
			n++
		}
	}
	args := make([]any, n)
	for i := range args {
		args[i] = value
	}
	return args
}

func TestDateBucketRejectsUnknownUnit(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		dialect, err := godatabase.DialectFor(engine)
		if err != nil {
			t.Fatal(err)
		}
		for _, unit := range []string{"", "minute", "day'; DROP TABLE x;--", "DAY"} {
			if sql, ok := dialect.DateBucket("created_at", unit); ok || sql != "" {
				t.Fatalf("%s accepted unit %q: %q", engine, unit, sql)
			}
		}
		for _, unit := range godatabase.DateBucketUnits {
			if _, ok := dialect.DateBucket("created_at", unit); !ok {
				t.Fatalf("%s rejected whitelisted unit %q", engine, unit)
			}
		}
	}
}

// TestFloorAndTimestampParamRealEngines 在两个引擎上验证取整与时间参数规整。
func TestFloorAndTimestampParamRealEngines(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			databasePath, err := godatabase.PathForProject("prj00004", engine)
			if err != nil {
				t.Fatal(err)
			}
			factory := &Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: NewLocalSyncer()}
			db, err := factory.Open(ctx, godatabase.Spec{ID: "db4", ProjectID: "prj00004", DatabasePath: databasePath}, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			dialect, _ := godatabase.DialectFor(engine)
			// 负数非整数必须向负无穷取整：-0.5 → -1，-1 → -1，2.9 → 2。
			for _, c := range []struct{ in, want float64 }{{-0.5, -1}, {-1, -1}, {2.9, 2}, {0, 0}, {10, 10}} {
				expr := dialect.Floor("CAST(? AS DOUBLE)")
				args := make([]any, 0, 4)
				for i := 0; i < countPlaceholders(expr); i++ {
					args = append(args, c.in)
				}
				var got float64
				if err := db.QueryRowContext(ctx, "SELECT "+expr, args...).Scan(&got); err != nil {
					t.Fatalf("%s floor(%v): %v\nsql: %s", engine, c.in, err, expr)
				}
				if got != c.want {
					t.Fatalf("%s floor(%v) = %v want %v", engine, c.in, got, c.want)
				}
			}
			// 带 T 的 RFC3339 参数与列侧表达式比较：两侧同口径时 20:00 必须晚于 10:00。
			sql := dialect.TimestampParam() + " >= " + dialect.TimestampParam()
			var ok bool
			if err := db.QueryRowContext(ctx, "SELECT "+sql, "2026-11-16T20:00:00Z", "2026-11-16T10:00:00Z").Scan(&ok); err != nil {
				t.Fatalf("%s: %v", engine, err)
			}
			if !ok {
				t.Fatalf("%s: 20:00 should be >= 10:00", engine)
			}
		})
	}
}

func countPlaceholders(sql string) int {
	n := 0
	for _, r := range sql {
		if r == '?' {
			n++
		}
	}
	return n
}
