package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// instanceIdentity 是 {cache_dir}/system/instance-identity.json 的载荷
//（multi-instance-consistency-plan §4.8）。
// instance.id 参与了全部 S3 对象前缀（KeyBuilder.Environment），
// 改 id 等价于换一个全新的空 bucket 前缀——现有所有库都会"消失"。
// 启动时检测变更并拒绝，除非运维明确清空 cache_dir 确认全新部署。
type instanceIdentity struct {
	InstanceID  string `json:"instance_id"`
	S3Prefix    string `json:"s3_prefix"`
	FirstSeenAt string `json:"first_seen_at"`
}

func instanceIdentityPath(cacheDir string) string {
	return filepath.Join(cacheDir, "system", "instance-identity.json")
}

// checkInstanceIdentity 启动检测：本地 identity 与当前配置不一致时返回错误。
// 一致或首次启动时写入/更新 identity 文件。
func checkInstanceIdentity(cacheDir, instanceID, s3Prefix string) error {
	path := instanceIdentityPath(cacheDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("app: identity dir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		var prev instanceIdentity
		if jsonErr := json.Unmarshal(data, &prev); jsonErr == nil {
			if prev.InstanceID != instanceID || prev.S3Prefix != s3Prefix {
				return fmt.Errorf(
					"app: instance.id or s3.prefix changed since first start (was instance_id=%q s3_prefix=%q, now instance_id=%q s3_prefix=%q). "+
						"changing them points the instance at a brand-new empty storage prefix: all existing databases would become invisible. "+
						"If this is a planned migration, follow the runbook in docs/ops/migration.md; "+
						"if this is a fresh deployment, clear the cache_dir (%s) and restart",
					prev.InstanceID, prev.S3Prefix, instanceID, s3Prefix, filepath.Dir(path))
			}
			return nil
		}
		// 损坏的 identity 文件：覆盖重写（保守继续）。
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("app: read instance identity: %w", err)
	}

	next := instanceIdentity{
		InstanceID:  instanceID,
		S3Prefix:    s3Prefix,
		FirstSeenAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("app: write identity tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("app: rename identity: %w", err)
	}
	return nil
}
