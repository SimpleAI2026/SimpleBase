// 文件说明：probe.go — internal/objectstore 中 ProbeResult、ProbePutIfAbsent 等符号的实现
//
// 用法
//
//	ProbePutIfAbsent(ctx context.Context, store BlobStore, probePrefix string) ...  // ProbePutIfAbsent 启动探针（multi-instance-consistency-plan §4.5）：
//	type ProbeResult（导出类型，同包与上层均可使用）
//	同包调用：ProbePutIfAbsent() → blob.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/probe.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] blob.go, client.go, content.go, content_local.go, content_s3.go 等 10 个
//	[关键引用] ProbePutIfAbsent() ← blob.go
//
// 主要符号
//
//	type ProbeResult struct {…}
//	ProbePutIfAbsent(ctx context.Context, store BlobStore, probePrefix string) ...
//	... 另有 1 个顶层符号

package objectstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ProbeResult 是启动探针的结果。
type ProbeResult struct {
	// CASSupported 表示 create-if-absent 条件写可用。
	CASSupported bool
	// Err 是探针期间的第一个非冲突错误（网络失败等）。
	Err error
}

// ProbePutIfAbsent 启动探针（multi-instance-consistency-plan §4.5）：
// 对 {prefix}/catalog/.probe-{uuid} 连续两次 PutIfAbsent，
// 第二次必须返回 ErrPreconditionFailed 才算通过。
//
// 探针失败（第二次写入成功或网络错误）意味着端点不支持条件写
// （例如 COS 存储桶开了版本控制导致 x-cos-forbid-overwrite 失效），
// 调用方必须禁用一切依赖互斥的能力（租约、manifest 推进、维护任务）。
//
// probePrefix 应为 {root}/{env}/catalog（探针对象可被 lifecycle 规则清理；
// 探针对象本身无害——每次启动使用随机 UUID key）。
func ProbePutIfAbsent(ctx context.Context, store BlobStore, probePrefix string) ProbeResult {
	key := fmt.Sprintf("%s/.probe-%s", probePrefix, uuid.NewString())
	body := []byte("simplebase cas probe " + time.Now().UTC().Format(time.RFC3339))

	if _, err := store.PutIfAbsent(ctx, key, body, "text/plain"); err != nil {
		return ProbeResult{CASSupported: false, Err: fmt.Errorf("probe first put: %w", err)}
	}
	if _, err := store.PutIfAbsent(ctx, key, body, "text/plain"); err == nil {
		return ProbeResult{CASSupported: false, Err: fmt.Errorf(
			"objectstore: probe detected overwrite on existing key: create-if-absent NOT enforced by endpoint")}
	} else if !isPreconditionFailed(err) {
		return ProbeResult{CASSupported: false, Err: fmt.Errorf("probe second put: %w", err)}
	}
	// 探针对象立即清理；失败无害（lifecycle / 小对象）。
	_ = store.Delete(ctx, key)
	return ProbeResult{CASSupported: true}
}

func isPreconditionFailed(err error) bool {
	return errors.Is(err, ErrPreconditionFailed)
}
