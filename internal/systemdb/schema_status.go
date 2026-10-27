// 文件说明：schema_status.go — internal/systemdb 中 SystemSchemaFormatVersion、SchemaTableStatus 等符号的…
//
// 用法
//
//	Status(ctx context.Context, db *sql.DB) (SchemaStatus, error)
//	Plan(ctx context.Context, db *sql.DB) (SchemaPlan, error)
//	同包调用：Status() → agent_schedules.go, agents.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/schema_status.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] Status() ← agent_schedules.go, agents.go, bootstrap.go
//
// 主要符号
//
//	const SystemSchemaFormatVersion
//	type SchemaTableStatus struct {…}
//	type SchemaStatus struct {…}
//	type SchemaPlan struct {…}
//	Status(ctx context.Context, db *sql.DB) (SchemaStatus, error)
//	Plan(ctx context.Context, db *sql.DB) (SchemaPlan, error)
//	... 另有 1 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"strings"
)

const SystemSchemaFormatVersion = 1

type SchemaTableStatus struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
}

type SchemaStatus struct {
	FormatVersion int                 `json:"format_version"`
	Ready         bool                `json:"ready"`
	Expected      int                 `json:"expected"`
	Present       int                 `json:"present"`
	Missing       []string            `json:"missing"`
	Tables        []SchemaTableStatus `json:"tables"`
}

type SchemaPlan struct {
	FormatVersion int      `json:"format_version"`
	Pending       []string `json:"pending"`
	Statements    []string `json:"statements"`
}

func Status(ctx context.Context, db *sql.DB) (SchemaStatus, error) {
	status := SchemaStatus{FormatVersion: SystemSchemaFormatVersion, Expected: len(systemTables), Missing: []string{}, Tables: []SchemaTableStatus{}}
	if db == nil {
		return status, ErrUnavailable
	}
	for _, table := range systemTables {
		_, err := db.ExecContext(ctx, "SELECT 1 FROM "+table.name+" LIMIT 0")
		present := err == nil
		if err != nil && !missingSystemTable(err) {
			return status, err
		}
		status.Tables = append(status.Tables, SchemaTableStatus{Name: table.name, Present: present})
		if present {
			status.Present++
		} else {
			status.Missing = append(status.Missing, table.name)
		}
	}
	status.Ready = len(status.Missing) == 0
	return status, nil
}

func Plan(ctx context.Context, db *sql.DB) (SchemaPlan, error) {
	status, err := Status(ctx, db)
	if err != nil {
		return SchemaPlan{}, err
	}
	plan := SchemaPlan{FormatVersion: SystemSchemaFormatVersion, Pending: []string{}, Statements: []string{}}
	missing := map[string]bool{}
	for _, name := range status.Missing {
		missing[name] = true
	}
	for _, table := range systemTables {
		if missing[table.name] {
			plan.Pending = append(plan.Pending, table.name)
			plan.Statements = append(plan.Statements, table.ddl)
		}
	}
	return plan, nil
}

func missingSystemTable(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "does not exist") || strings.Contains(message, "no such table") || strings.Contains(message, "not found")
}
