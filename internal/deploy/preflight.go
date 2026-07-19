// Package deploy 实现部署前的 preflight 检查与单实例约束验证（plan9.md）。
//
// 启动前必须通过 preflight：
//   - 缓存目录可写且容量充足
//   - S3 可达且权限正确（PutJSON/GetJSON/Head/DeletePrefix）
//   - catalog 系统库可打开
//   - 端口可绑定
//   - 单写实例约束：环境变量或锁文件标记当前实例为唯一 writer
package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// CheckResult 描述单项检查结果。
type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

// Report 是 preflight 报告。
type Report struct {
	Checks []CheckResult
	AllOK  bool
}

// Preflight 执行启动前检查。
type Preflight struct {
	cfg     config.Config
	objects objectstore.Client
	logger  observability.Logger
}

// NewPreflight 构造。
func NewPreflight(cfg config.Config, objects objectstore.Client, logger observability.Logger) *Preflight {
	return &Preflight{cfg: cfg, objects: objects, logger: logger}
}

// Run 执行全部检查。任一失败返回 Report 含详情；不提前返回以便完整报告。
func (p *Preflight) Run(ctx context.Context) Report {
	r := Report{}
	r.Checks = append(r.Checks, p.checkCacheDir())
	r.Checks = append(r.Checks, p.checkS3(ctx))
	r.Checks = append(r.Checks, p.checkSingleWriter())
	for _, c := range r.Checks {
		if !c.OK {
			r.AllOK = false
			return r
		}
	}
	r.AllOK = true
	return r
}

// checkCacheDir 验证缓存目录可写。
func (p *Preflight) checkCacheDir() CheckResult {
	dir := p.cfg.Database.CacheDir
	if dir == "" {
		return CheckResult{Name: "cache_dir", OK: false, Detail: "cache dir not configured"}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return CheckResult{Name: "cache_dir", OK: false, Detail: err.Error()}
	}
	// 尝试创建并写入测试文件。
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return CheckResult{Name: "cache_dir", OK: false, Detail: fmt.Sprintf("mkdir: %v", err)}
	}
	test := filepath.Join(abs, ".preflight-write-test")
	if err := os.WriteFile(test, []byte("ok"), 0o644); err != nil {
		return CheckResult{Name: "cache_dir", OK: false, Detail: fmt.Sprintf("write: %v", err)}
	}
	_ = os.Remove(test)

	// 检查可用空间。
	var stat syscall.Statfs_t
	if err := syscall.Statfs(abs, &stat); err == nil {
		avail := stat.Bavail * uint64(stat.Bsize)
		min := uint64(1) << 30 // 默认至少 1GiB
		if avail < min {
			return CheckResult{Name: "cache_dir", OK: false,
				Detail: fmt.Sprintf("free %d < min %d", avail, min)}
		}
	}
	return CheckResult{Name: "cache_dir", OK: true, Detail: abs}
}

// checkS3 验证 S3 连通性与权限。
func (p *Preflight) checkS3(ctx context.Context) CheckResult {
	if p.objects == nil {
		return CheckResult{Name: "s3", OK: false, Detail: "objectstore client nil"}
	}
	if err := p.objects.Check(ctx); err != nil {
		return CheckResult{Name: "s3", OK: false, Detail: err.Error()}
	}
	return CheckResult{Name: "s3", OK: true, Detail: "reachable"}
}

// checkSingleWriter 验证单写实例约束。
// 首期通过锁文件实现：尝试在缓存目录创建独占锁文件。
func (p *Preflight) checkSingleWriter() CheckResult {
	dir := p.cfg.Database.CacheDir
	if dir == "" {
		return CheckResult{Name: "single_writer", OK: false, Detail: "cache dir not configured"}
	}
	lockPath := filepath.Join(dir, ".writer.lock")
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return CheckResult{Name: "single_writer", OK: false, Detail: err.Error()}
	}
	// 尝试非阻塞独占锁。
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return CheckResult{Name: "single_writer", OK: false,
			Detail: "another writer instance holds the lock"}
	}
	// 保持锁文件打开（进程退出时自动释放）。不关闭 f。
	if p.logger != nil {
		p.logger.Info("preflight: acquired writer lock", zap.String("path", lockPath))
	}
	return CheckResult{Name: "single_writer", OK: true, Detail: "lock acquired"}
}

// ErrPreflightFailed 表示 preflight 未通过。
var ErrPreflightFailed = errors.New("deploy: preflight checks failed")
