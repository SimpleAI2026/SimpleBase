// 文件说明：commands.go — internal/sbcli 中 CommandReadOnly、CommandReadOnlyWithArgv 等符号的实现
//
// 用法
//
//	CommandReadOnly(resource, action string) bool  // CommandReadOnly 报告命令是否只读（planv6.0 v2 §3.4）
//	CommandReadOnlyWithArgv(argv []string) bool  // CommandReadOnlyWithArgv 处理带子动作的命令（planv6.0 v2 §3.4）：
//	同包调用：one() → redact.go, run.go；need() → redact.go；dispatch() → run.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/commands.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] collection_transfer.go, config.go, migrations.go, output.go, redact.go 等 6 个
//	[关键引用] one() ← redact.go, run.go
//
// 主要符号
//
//	CommandReadOnly(resource, action string) bool
//	CommandReadOnlyWithArgv(argv []string) bool
//	... 另有 91 个顶层符号

package sbcli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

type commandFunc func(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error)

// readOnlyCommands 是「不改变任何服务端状态」的命令集合（planv6.0 v2 §3.4）。
// 它是助手写权限分级（不可写 Agent 只能执行这里登记的命令）的唯一事实来源：
// gocloudagent 的 isWriteArgv 直接查 CommandReadOnly，而不是再维护一份会漂移的表。
//
// 判定口径：只做 GET/查询类读取，或只影响沙盒内临时环境（sandbox exec/run）
// 而不触碰项目持久数据。凡不在表内的命令一律视为写命令（fail-closed）。
//
// 带子动作的命令（function versions / sandbox files / log retention）不在此表，
// 由 CommandReadOnlyWithArgv 按第三段细分。
var readOnlyCommands = map[string]bool{
	"database list":         true,
	"database get":          true,
	"sql query":             true,
	"schema show":           true,
	"schema rows":           true,
	"collection list":       true,
	"collection schema-get": true,
	"document list":         true,
	"document get":          true,
	"document export":       true,
	"object list":           true,
	"object presign":        true,
	"site status":           true,
	"function list":         true,
	"function get":          true,
	"function test":         true,
	"cron list":             true,
	"cron get":              true,
	"cron runs":             true,
	"sandbox capabilities":  true,
	"sandbox list":          true,
	"sandbox get":           true,
	"log search":            true,
	"user list":             true,
	"user get":              true,
	"apikey list":           true,
	"project list":          true,
	"quota get":             true,
	"audit list":            true,
	"backup list":           true,
	"migrate status":        true,
	"migrate plan":          true,
	"settings get":          true,
	// dev plan/validate/status 只读，但读取宿主工作区文件；助手模式下命令自身拒绝（file_not_allowed）。
	"dev plan":     true,
	"dev validate": true,
	"dev status":   true,
}

// CommandReadOnly 报告命令是否只读（planv6.0 v2 §3.4）。
// 未知命令返回 false（fail-closed）：新增命令必须显式登记才会被放行。
func CommandReadOnly(resource, action string) bool {
	return readOnlyCommands[resource+" "+action]
}

// CommandReadOnlyWithArgv 处理带子动作的命令（planv6.0 v2 §3.4）：
//   - log retention get|set
//   - function versions list|get|create|activate
//
// 其余命令退化为 CommandReadOnly（只看前两段）。沙盒的 files 子命令已删除。
func CommandReadOnlyWithArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	resource := argv[0]
	action := ""
	if len(argv) > 1 {
		action = argv[1]
	}
	sub := ""
	if len(argv) > 2 {
		sub = argv[2]
	}
	switch resource + " " + action {
	case "log retention":
		return sub == "get"
	case "function versions":
		return sub == "list" || sub == "get"
	}
	return CommandReadOnly(resource, action)
}

func dispatch(resource, action string) commandFunc {
	switch resource + " " + action {
	case "database list":
		return cmdDatabaseList
	case "database get":
		return cmdDatabaseGet
	case "database create":
		return cmdDatabaseCreate
	case "database delete":
		return cmdDatabaseDelete
	case "sql query":
		return cmdSQLQuery
	case "sql exec":
		return cmdSQLExec
	case "sql batch":
		return cmdSQLBatch
	case "schema show":
		return cmdSchemaShow
	case "schema rows":
		return cmdSchemaRows
	case "schema create-table":
		return cmdSchemaCreate
	case "schema add-column":
		return cmdSchemaAddColumn
	case "collection list":
		return cmdCollectionList
	case "collection create":
		return cmdCollectionCreate
	case "collection truncate":
		return cmdCollectionTruncate
	case "collection schema-get":
		return cmdCollectionSchemaGet
	case "collection schema-put":
		return cmdCollectionSchemaPut
	case "document list":
		return cmdDocumentList
	case "document get":
		return cmdDocumentGet
	case "document import":
		return cmdDocumentImport
	case "document export":
		return cmdDocumentExport
	case "document insert":
		return cmdDocumentInsert
	case "document update":
		return cmdDocumentUpdate
	case "document patch":
		return cmdDocumentPatch
	case "document batch":
		return cmdDocumentBatch
	case "document delete":
		return cmdDocumentDelete
	case "backup list":
		return cmdBackupList
	case "backup create":
		return cmdBackupCreate
	case "backup restore":
		return cmdBackupRestore
	case "backup delete":
		return cmdBackupDelete
	case "migrate status":
		return cmdMigrateStatus
	case "migrate plan":
		return cmdMigratePlan
	case "kv exec":
		return cmdKV
	case "object list":
		return cmdObjectList
	case "object upload":
		return cmdObjectUpload
	case "object delete":
		return cmdObjectDelete
	case "object presign":
		return cmdObjectPresign
	case "site status":
		return cmdSiteStatus
	case "site publish":
		return cmdSitePublish
	case "function list":
		return cmdFunctionList
	case "function get":
		return cmdFunctionGet
	case "function create":
		return cmdFunctionCreate
	case "function update":
		return cmdFunctionUpdate
	case "function delete":
		return cmdFunctionDelete
	case "function version":
		return cmdFunctionVersions
	case "function test":
		return cmdFunctionTest
	case "function invoke":
		return cmdFunctionInvoke
	case "function activate":
		return cmdFunctionActivate
	case "cron list":
		return cmdCronList
	case "cron get":
		return cmdCronGet
	case "cron create":
		return cmdCronCreate
	case "cron update":
		return cmdCronUpdate
	case "cron delete":
		return cmdCronDelete
	case "cron runs":
		return cmdCronRuns
	case "cron trigger":
		return cmdCronTrigger
	case "sandbox capabilities":
		return cmdSandboxCaps
	case "sandbox list":
		return cmdSandboxList
	case "sandbox get":
		return cmdSandboxGet
	case "sandbox create":
		return cmdSandboxCreate
	case "sandbox delete":
		return cmdSandboxDelete
	case "sandbox connect":
		return cmdSandboxConnect
	case "sandbox pause":
		return cmdSandboxPause
	case "log search":
		return cmdLogSearch
	case "log retention":
		return cmdLogRetention
	case "user list":
		return cmdUserList
	case "user get":
		return cmdUserGet
	case "user create":
		return cmdUserCreate
	case "user update":
		return cmdUserUpdate
	case "user disable":
		return cmdUserDisable
	case "user delete":
		return cmdUserDelete
	case "project list":
		return cmdProjectList
	case "project create":
		return cmdProjectCreate
	case "apikey list":
		return cmdAPIKeyList
	case "apikey create":
		return cmdAPIKeyCreate
	case "apikey revoke":
		return cmdAPIKeyRevoke
	case "settings get":
		return cmdSettingsGet
	case "settings set":
		return cmdSettingsSet
	case "quota get":
		return cmdQuotaGet
	case "audit list":
		return cmdAuditList
	case "dev plan":
		return cmdDevPlan
	case "dev apply":
		return cmdDevApply
	case "dev validate":
		return cmdDevValidate
	case "dev status":
		return cmdDevStatus
	default:
		return dispatchNested(resource, action)
	}
}

func dispatchNested(resource, action string) commandFunc {
	switch resource + " " + action {
	case "function versions":
		return cmdFunctionVersions
	default:
		return nil
	}
}

func mustFlags(args []string, names ...string) (map[string]*string, *flag.FlagSet, error) {
	set := flag.NewFlagSet("simplebase", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	out := map[string]*string{}
	for _, n := range names {
		out[n] = set.String(n, "", "")
	}
	if err := set.Parse(args); err != nil {
		return nil, nil, usagef("%s", err.Error())
	}
	return out, set, nil
}

func need(flags map[string]*string, name string) (string, error) {
	v := strings.TrimSpace(*flags[name])
	if v == "" {
		return "", usagef("--%s is required", name)
	}
	return v, nil
}

func readFileOrDash(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// readSource 在 readFileOrDash 基础上执行助手模式文件隔离（planv6.0 D3）：
// Agent 运行时（SIMPLEBASE_AGENT_RUN=1）只接受 stdin（"-"）或显式 base64 内联
// （sourceB64 非空），传入宿主机路径一律拒绝，杜绝助手读取宿主机任意文件。
// 人类操作者行为不变。
func readSource(path, sourceB64 string, stdin io.Reader, agent bool) (string, error) {
	if sourceB64 != "" {
		b, err := base64.StdEncoding.DecodeString(sourceB64)
		if err != nil {
			return "", usagef("--source-b64: invalid base64")
		}
		if len(b) > 256<<10 {
			return "", usagef("--source-b64: exceeds 256KiB limit")
		}
		return string(b), nil
	}
	if agent && path != "-" {
		return "", usagef("file_not_allowed: agent mode requires --file - (stdin) or --source-b64")
	}
	return readFileOrDash(path, stdin)
}

func decodeRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{"ok": true}
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	return v
}

func stripAgentSecrets(data any) any {
	m, ok := data.(map[string]any)
	if !ok {
		return data
	}
	if _, has := m["secret"]; has {
		delete(m, "secret")
		m["secret_withheld"] = true
	}
	if url, ok := m["url"].(string); ok && strings.Contains(url, "://") {
		delete(m, "url")
		if _, ok := m["expires_at"]; !ok {
			m["expires_at"] = time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
		}
	}
	return m
}

func one(args []string, name string) (string, error) {
	f, _, err := mustFlags(args, name)
	if err != nil {
		return "", err
	}
	return need(f, name)
}

// log retention is two actions under one resource word pair: handled here.
func init() {
	_ = cmdLogRetention
}

func cmdLogRetention(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	if len(args) == 0 {
		return nil, usagef("usage: log retention <get|set>")
	}
	switch args[0] {
	case "get":
		raw, err := c.GetLogRetention(ctx)
		return decodeRaw(raw), err
	case "set":
		f, _, err := mustFlags(args[1:], "days")
		if err != nil {
			return nil, err
		}
		days, err := strconv.Atoi(strings.TrimSpace(*f["days"]))
		if err != nil {
			return nil, usagef("--days is required")
		}
		raw, err := c.SetLogRetention(ctx, days)
		return decodeRaw(raw), err
	default:
		return nil, usagef("unknown log retention action")
	}
}
