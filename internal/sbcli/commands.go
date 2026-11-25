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
//	idAction / idResult / sqlCommand / devCommand（单 ID 动作与薄包装命令的公共适配）

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

// commandSpec 是单条 CLI 命令的全部元数据：处理函数与只读标记放在同一处，
// 新增命令只改这一张表，不会再出现「已分发但漏标只读」或反向漂移。
//
// readOnly 是助手写权限分级（不可写 Agent 只能执行只读命令）的唯一事实来源：
// gocloudagent 的 isWriteArgv 直接查 CommandReadOnly，而不是再维护一份会漂移的表。
// 判定口径：只做 GET/查询类读取，或只影响沙盒内临时环境（sandbox exec/run）
// 而不触碰项目持久数据。凡不在表内的命令一律视为写命令（fail-closed）。
//
// 带子动作的命令（function versions / log retention）在表内按写处理，
// 由 CommandReadOnlyWithArgv 按第三段细分。
// dev plan/validate/status 只读，但读取宿主工作区文件；助手模式下命令自身拒绝（file_not_allowed）。
type commandSpec struct {
	fn       commandFunc
	readOnly bool
}

var commandSpecs = map[string]commandSpec{
	"database list":         {cmdDatabaseList, true},
	"database get":          {idResult("id", (*gosdk.Client).GetDatabase), true},
	"database create":       {cmdDatabaseCreate, false},
	"database delete":       {idResult("id", (*gosdk.Client).DeleteDatabase), false},
	"sql query":             {sqlCommand(false), true},
	"sql exec":              {sqlCommand(true), false},
	"sql batch":             {cmdSQLBatch, false},
	"schema show":           {idResult("database", (*gosdk.Client).DatabaseSchema), true},
	"schema rows":           {cmdSchemaRows, true},
	"schema create-table":   {cmdSchemaCreate, false},
	"schema add-column":     {cmdSchemaAddColumn, false},
	"collection list":       {cmdCollectionList, true},
	"collection create":     {cmdCollectionCreate, false},
	"collection truncate":   {cmdCollectionTruncate, false},
	"collection schema-get": {cmdCollectionSchemaGet, true},
	"collection schema-put": {cmdCollectionSchemaPut, false},
	"document list":         {cmdDocumentList, true},
	"document get":          {cmdDocumentGet, true},
	"document import":       {cmdDocumentImport, false},
	"document export":       {cmdDocumentExport, true},
	"document insert":       {cmdDocumentInsert, false},
	"document update":       {cmdDocumentUpdate, false},
	"document patch":        {cmdDocumentPatch, false},
	"document batch":        {cmdDocumentBatch, false},
	"document delete":       {cmdDocumentDelete, false},
	"backup list":           {cmdBackupList, true},
	"backup create":         {cmdBackupCreate, false},
	"backup restore":        {cmdBackupRestore, false},
	"backup delete":         {cmdBackupDelete, false},
	"migrate status":        {cmdMigrateStatus, true},
	"migrate plan":          {cmdMigratePlan, true},
	"kv exec":               {cmdKV, false},
	"object list":           {cmdObjectList, true},
	"object upload":         {cmdObjectUpload, false},
	"object delete":         {idResult("key", (*gosdk.Client).DeleteObject), false},
	"object presign":        {cmdObjectPresign, true},
	"site status":           {cmdSiteStatus, true},
	"site publish":          {cmdSitePublish, false},
	"function list":         {cmdFunctionList, true},
	"function get":          {idResult("name", (*gosdk.Client).GetFunction), true},
	"function create":       {cmdFunctionCreate, false},
	"function update":       {cmdFunctionUpdate, false},
	"function delete":       {idAction("name", (*gosdk.Client).DeleteFunction), false},
	"function version":      {cmdFunctionVersions, false},
	"function test":         {cmdFunctionTest, true},
	"function invoke":       {cmdFunctionInvoke, false},
	"function activate":     {cmdFunctionActivate, false},
	"cron list":             {cmdCronList, true},
	"cron get":              {idResult("id", (*gosdk.Client).GetCronJob), true},
	"cron create":           {cmdCronCreate, false},
	"cron update":           {cmdCronUpdate, false},
	"cron delete":           {idAction("id", (*gosdk.Client).DeleteCronJob), false},
	"cron runs":             {idResult("id", (*gosdk.Client).ListCronRuns), true},
	"cron trigger":          {idResult("id", (*gosdk.Client).TriggerCronJob), false},
	"sandbox capabilities":  {cmdSandboxCaps, true},
	"sandbox list":          {cmdSandboxList, true},
	"sandbox get":           {cmdSandboxGet, true},
	"sandbox create":        {cmdSandboxCreate, false},
	"sandbox delete":        {idAction("id", (*gosdk.Client).DeleteSandbox), false},
	"sandbox connect":       {cmdSandboxConnect, false},
	"sandbox pause":         {idAction("id", (*gosdk.Client).PauseSandbox), false},
	"log search":            {cmdLogSearch, true},
	"log retention":         {cmdLogRetention, false},
	"user list":             {cmdUserList, true},
	"user get":              {idResult("id", (*gosdk.Client).GetUser), true},
	"user create":           {cmdUserCreate, false},
	"user update":           {cmdUserUpdate, false},
	"user disable":          {cmdUserDisable, false},
	"user delete":           {idAction("id", (*gosdk.Client).DeleteUser), false},
	"project list":          {cmdProjectList, true},
	"project create":        {cmdProjectCreate, false},
	"apikey list":           {cmdAPIKeyList, true},
	"apikey create":         {cmdAPIKeyCreate, false},
	"apikey revoke":         {idAction("id", (*gosdk.Client).RevokeAPIKey), false},
	"settings get":          {cmdSettingsGet, true},
	"settings set":          {cmdSettingsSet, false},
	"quota get":             {cmdQuotaGet, true},
	"audit list":            {cmdAuditList, true},
	"dev plan":              {devCommand("plan", false), true},
	"dev apply":             {devCommand("apply", true), false},
	"dev validate":          {cmdDevValidate, true},
	"dev status":            {cmdDevStatus, true},
	"function versions":     {cmdFunctionVersions, false},
}

// CommandReadOnly 报告命令是否只读（planv6.0 v2 §3.4）。
// 未知命令返回 false（fail-closed）：新增命令必须显式登记才会被放行。
func CommandReadOnly(resource, action string) bool {
	return commandSpecs[resource+" "+action].readOnly
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

// dispatch 按 "resource action" 查命令表，未登记返回 nil。
func dispatch(resource, action string) commandFunc {
	return commandSpecs[resource+" "+action].fn
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

func idAction(name string, fn func(*gosdk.Client, context.Context, string) error) commandFunc {
	return func(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
		id, err := one(args, name)
		if err != nil {
			return nil, err
		}
		if err := fn(c, ctx, id); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
}

// idResult 取单个 ID（flag 名为 name）后调用 fn，把 SDK 的返回值原样作为命令输出：
// json.RawMessage 解码成通用 JSON 值，结构体直接交给输出层序列化。
func idResult[T any](name string, fn func(*gosdk.Client, context.Context, string) (T, error)) commandFunc {
	return func(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
		id, err := one(args, name)
		if err != nil {
			return nil, err
		}
		v, err := fn(c, ctx, id)
		return resultOf(v), err
	}
}

func resultOf(v any) any {
	if raw, ok := v.(json.RawMessage); ok {
		return decodeRaw(raw)
	}
	return v
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
