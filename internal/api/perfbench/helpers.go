// helpers.go 用例共享的小工具（插入行、UUID、HTTP 构造、hash argvs）。
package perfbench

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// timeNowNano 返回当前纳秒（benchUUID 降级路径用）。
func timeNowNano() int64 { return time.Now().UnixNano() }

// benchInsert 向 bench_rows 插入一行（seq 唯一）。
func benchInsert(env *benchEnv, seq int) error {
	code, body, err := env.Do("POST", env.sqlPath("execute"), map[string]any{
		"sql":  "INSERT INTO bench_rows (id, payload, seq) VALUES (?, ?, ?)",
		"args": []any{benchUUID(), "payload-" + strconv.Itoa(seq), float64(seq)},
	})
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("perfbench: insert seq=%d: http %d: %s", seq, code, body)
	}
	return nil
}

// benchUUID 生成随机 UUID v4（无需引入额外依赖）。
func benchUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("bench-%d", timeNowNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// hashArgvs 把 fields map 展开为 HSET argv 形式（k1 v1 k2 v2 ...）。
func hashArgvs(fields map[string]string) []string {
	argvs := make([]string, 0, len(fields)*2)
	for i := 0; i < len(fields); i++ {
		k := "f" + strconv.Itoa(i)
		argvs = append(argvs, k, fields[k])
	}
	return argvs
}

// httpNewRequest 包装 http.NewRequest（用例内减少样板）。
func httpNewRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}
