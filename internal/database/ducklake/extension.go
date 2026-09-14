package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// MinDuckDBVersion 是 DuckLake v1.0 的硬性下限（§2.1）。
const MinDuckDBVersion = "1.5.2"

var requiredExtensions = []string{"ducklake", "sqlite", "httpfs"}

// extensionAliases maps INSTALL 名到 duckdb_extensions() 可能返回的名字。
var extensionAliases = map[string][]string{
	"sqlite": {"sqlite", "sqlite_scanner"},
}

func extensionBootSQL(opts Options) []string {
	out := make([]string, 0, 8)
	if opts.ExtensionDir != "" {
		out = append(out, "SET extension_directory = "+quoteSQLString(opts.ExtensionDir))
	}
	for _, ext := range requiredExtensions {
		out = append(out, "INSTALL "+ext)
		out = append(out, "LOAD "+ext)
	}
	return out
}

// AssertDuckDBVersion 确认当前连接的 DuckDB ≥ MinDuckDBVersion。
func AssertDuckDBVersion(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err != nil {
		return "", fmt.Errorf("ducklake: query duckdb version: %w", err)
	}
	normalized := normalizeDuckDBVersion(version)
	ok, err := versionAtLeast(normalized, MinDuckDBVersion)
	if err != nil {
		return version, fmt.Errorf("ducklake: parse duckdb version %q: %w", version, err)
	}
	if !ok {
		return version, fmt.Errorf("ducklake: duckdb %s is below required %s", version, MinDuckDBVersion)
	}
	return version, nil
}

// AssertExtensionsLoaded 确认 ducklake/sqlite/httpfs 均已加载。
func AssertExtensionsLoaded(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT extension_name, loaded
		FROM duckdb_extensions()
		WHERE extension_name IN ('ducklake', 'sqlite', 'sqlite_scanner', 'httpfs')`)
	if err != nil {
		return fmt.Errorf("ducklake: list extensions: %w", err)
	}
	defer rows.Close()

	loaded := map[string]bool{}
	for rows.Next() {
		var name string
		var isLoaded bool
		if err := rows.Scan(&name, &isLoaded); err != nil {
			return err
		}
		loaded[name] = isLoaded
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, ext := range requiredExtensions {
		if !extensionLoaded(loaded, ext) {
			return fmt.Errorf("ducklake: required extension %s is not loaded (seen=%v)", ext, loaded)
		}
	}
	return nil
}

func extensionLoaded(loaded map[string]bool, name string) bool {
	candidates := extensionAliases[name]
	if len(candidates) == 0 {
		candidates = []string{name}
	}
	for _, c := range candidates {
		if loaded[c] {
			return true
		}
	}
	return false
}

func normalizeDuckDBVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, " -+"); i >= 0 {
		v = v[:i]
	}
	return v
}

func versionAtLeast(got, min string) (bool, error) {
	g, err := parseSemver3(got)
	if err != nil {
		return false, err
	}
	m, err := parseSemver3(min)
	if err != nil {
		return false, err
	}
	for i := 0; i < 3; i++ {
		if g[i] > m[i] {
			return true, nil
		}
		if g[i] < m[i] {
			return false, nil
		}
	}
	return true, nil
}

func parseSemver3(v string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return out, fmt.Errorf("invalid semver %q", v)
	}
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, fmt.Errorf("invalid semver %q", v)
		}
		out[i] = n
	}
	return out, nil
}
