package catalog

import (
	"fmt"
	"strings"
)

// NormalizeDataModel 把创建请求里的 data_model 收成 collection 或 sql。
// 空字符串表示调用方没传，按集合文档处理，兼容只提交 name 的旧客户端。
func NormalizeDataModel(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", DataModelCollection:
		return DataModelCollection, nil
	case DataModelSQL:
		return DataModelSQL, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidDataModel, raw)
	}
}

// EffectiveDataModel 返回对外展示用的形态。
// 系统库底层是 sys_* SQL 表，对外固定为 sql，不看列上残留的 collection。
// 其它行不是精确的 sql 时一律视为集合文档，迁移前空列与未填列的测试行行为不变。
func EffectiveDataModel(d Database) string {
	if d.Kind == DatabaseKindSystem {
		return DataModelSQL
	}
	if d.DataModel == DataModelSQL {
		return DataModelSQL
	}
	return DataModelCollection
}

// IsSQLDataModel 判断该行是否应按 SQL 关系库处理。
// KV 即使列上残留 sql，也不进入 SQL 表结构 / 拒绝集合的分支。
// 系统库始终进入只读 SQL 分支。
func IsSQLDataModel(d Database) bool {
	if d.Kind == DatabaseKindKV {
		return false
	}
	if d.Kind == DatabaseKindSystem {
		return true
	}
	return EffectiveDataModel(d) == DataModelSQL
}
