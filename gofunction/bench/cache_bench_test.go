package main

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction"
)

// BenchmarkRunJSONCacheHit 对比缓存命中（fork+replayInit）与
// 未命中（全量编译）的 RunJSON 端到端开销（B2）。
// -count=1 下两者交替执行会互相污染缓存计数，故拆两个基准：
// Hit 只跑同一源码（除首次全 miss），Miss 用 body 变体改变源码
// 逐次强制全量编译。
func BenchmarkRunJSONCacheHit(b *testing.B) {
	src := "package main\nfunc Run(x struct{ N int }) int { sum := 0; for i := 0; i < x.N; i++ { sum += i }; return sum }"
	gofunction.SetProgramCacheLimit(128)
	body, _ := json.Marshal(map[string]int{"N": 100})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := gofunction.RunJSON(context.Background(), "bench", "main", src, "Run", body)
		if err != nil {
			b.Fatal(err)
		}
		var v int
		_ = json.Unmarshal(out, &v)
	}
}

func BenchmarkRunJSONCacheMiss(b *testing.B) {
	gofunction.SetProgramCacheLimit(128)
	body, _ := json.Marshal(map[string]int{"N": 100})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 源码逐次不同（注释行变体），强制全量编译路径
		src := "package main\n// v" + itoa(i) + "\nfunc Run(x struct{ N int }) int { sum := 0; for i := 0; i < x.N; i++ { sum += i }; return sum }"
		out, err := gofunction.RunJSON(context.Background(), "bench", "main", src, "Run", body)
		if err != nil {
			b.Fatal(err)
		}
		var v int
		_ = json.Unmarshal(out, &v)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
