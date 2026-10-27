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
