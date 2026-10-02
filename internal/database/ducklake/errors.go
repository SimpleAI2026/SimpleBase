package ducklake

import "errors"

// ErrCatalogEngineMismatch 表示配置的 catalog 引擎与已有数据（实例标记 /
// manifest / 本地文件）不一致（ducklake-duckdb-catalog-plan §4.5）。
// 引擎切换不受支持；唯一途径是 reset 清空全部数据。
var ErrCatalogEngineMismatch = errors.New("ducklake: catalog engine mismatch")

// ErrCatalogEngineUnknown 表示存在历史数据但没有实例级格式标记，
// 无法判定其 catalog 引擎（§4.2）。同样要求先 reset。
var ErrCatalogEngineUnknown = errors.New("ducklake: catalog engine of existing data is unknown (missing instance-format marker)")

// IsCatalogEngineMismatch 判定是否为引擎不一致错误。
func IsCatalogEngineMismatch(err error) bool {
	return errors.Is(err, ErrCatalogEngineMismatch)
}

// IsCatalogEngineUnknown 判定是否为「历史数据无标记」错误。
func IsCatalogEngineUnknown(err error) bool {
	return errors.Is(err, ErrCatalogEngineUnknown)
}
