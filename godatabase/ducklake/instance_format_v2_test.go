// 文件说明：instance_format_v2_test.go — godatabase/ducklake 的单元测试（TestInstanceFormatRejectsV1Marker、TestInstanceFormatAcceptsFreshAndWritesV2 等）
//
// 用法
//
//	go test ./godatabase/ducklake -run 'TestInstanceFormatRejectsV1Marker|TestInstanceFormatAcceptsFreshAndWritesV2|TestDetectLocalDataRecognizesProjectFilesAndLegacyLayout'
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/instance_format_v2_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//
// 主要符号
//
//	TestInstanceFormatRejectsV1Marker(t *testing.T)
//	TestInstanceFormatAcceptsFreshAndWritesV2(t *testing.T)
//	TestDetectLocalDataRecognizesProjectFilesAndLegacyLayout(t *testing.T)

package ducklake

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceFormatRejectsV1Marker(t *testing.T) {
	root := t.TempDir()
	if err := writeLocalFormat(root, InstanceFormat{CatalogEngine: "duckdb", FormatVersion: 1}); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureInstanceFormat(context.Background(), FormatCheckInput{Engine: "duckdb", DataRoot: root})
	if !errors.Is(err, ErrCatalogEngineMismatch) {
		t.Fatalf("v1 marker must be rejected, got %v", err)
	}
}

func TestInstanceFormatAcceptsFreshAndWritesV2(t *testing.T) {
	root := t.TempDir()
	f, err := EnsureInstanceFormat(context.Background(), FormatCheckInput{Engine: "duckdb", DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if f.FormatVersion != 2 {
		t.Fatalf("format version = %d", f.FormatVersion)
	}
}

func TestDetectLocalDataRecognizesProjectFilesAndLegacyLayout(t *testing.T) {
	for name, setup := range map[string]func(root string){
		"project duckdb": func(root string) {
			_ = os.MkdirAll(filepath.Join(root, "prj00001"), 0o755)
			_ = os.WriteFile(filepath.Join(root, "prj00001", "database.duckdb"), []byte("x"), 0o644)
		},
		"project sqlite": func(root string) {
			_ = os.MkdirAll(filepath.Join(root, "prj00001"), 0o755)
			_ = os.WriteFile(filepath.Join(root, "prj00001", "database.sqlite"), []byte("x"), 0o644)
		},
		"legacy per-database layout": func(root string) {
			_ = os.MkdirAll(filepath.Join(root, "prj00001", "databases", "d1"), 0o755)
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			setup(root)
			if found, _ := DetectLocalData(root); !found {
				t.Fatal("expected existing data to be detected")
			}
			// 无标记 + 已有数据必须拒绝，不得原地初始化。
			if _, err := EnsureInstanceFormat(context.Background(), FormatCheckInput{Engine: "duckdb", DataRoot: root}); !errors.Is(err, ErrCatalogEngineUnknown) {
				t.Fatalf("unmarked data must be rejected, got %v", err)
			}
		})
	}
}
