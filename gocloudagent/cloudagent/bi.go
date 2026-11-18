// 文件说明：bi.go — 数据看板「Agent 生成图表」的上下文清洗与提示词（planv6.7 §3.4）
//
// 与 starterPrompt 同风格：提示词与清洗都留在模块内，宿主只传原始值。
// BIContext 不含任何密钥；字段名与类型经白名单清洗后才会进入提示词，
// 因此它不是提示词注入通道。

package cloudagent

import (
	"fmt"
	"regexp"
	"strings"
)

// BIChartTypes 是助手可以产出的图表类型全集（与前端 BiChartType 一一对应）。
// `auto` 表示交给前端按规则推断。
var BIChartTypes = []string{"auto", "line", "column", "bar", "pie", "table", "grouped_bar", "stacked_bar", "scatter"}

// BIMetricKinds 是聚合度量种类全集。宿主侧测试会逐项喂给 aggregate 解析器，
// 保证提示词承诺的语法服务端都接受（漂移防护）。
var BIMetricKinds = []string{"count", "sum", "avg", "min", "max", "distinct"}

// biFieldTypes 是允许进入提示词的字段类型（与集合 schema 的字段类型一致）。
var biFieldTypes = map[string]struct{}{
	"text": {}, "number": {}, "bool": {}, "date": {}, "email": {}, "url": {}, "select": {}, "json": {},
}

var (
	biNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	biUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// BIMaxFields 是单次上下文携带的字段数上限。
const BIMaxFields = 64

// BIField 是集合里的一个可用字段。
type BIField struct {
	Name string
	Type string
}

// 数据源种类：集合（文档库里的集合，字段来自登记的 schema）或 SQL 表（SQL 库里的表，字段是真实列）。
const (
	BISourceCollection = "collection"
	BISourceTable      = "table"
)

// BIContext 描述本轮图表生成面向的数据源。
//
// Collection 字段对两种数据源都表示「数据源名」：集合名或 SQL 表名（均须满足同一标识符规则）。
// Source 缺省为 collection，保证旧客户端的请求逐字不变。
type BIContext struct {
	DatabaseID    string
	Collection    string
	Source        string
	PreferredType string
	Fields        []BIField
}

// NormalizeBIContext 校验并清洗上下文。
//
// 数据库 id 与集合名不合规时整体拒绝（ok=false）；偏好类型不在白名单时拒绝；
// 字段逐项过滤：名称或类型不合规的项直接丢弃，总数截断到 BIMaxFields。
func NormalizeBIContext(in BIContext) (BIContext, error) {
	out := BIContext{
		DatabaseID: strings.TrimSpace(in.DatabaseID),
		Collection: strings.TrimSpace(in.Collection),
	}
	if !biUUIDPattern.MatchString(out.DatabaseID) {
		return BIContext{}, fmt.Errorf("bi: database_id must be a uuid")
	}
	if !biNamePattern.MatchString(out.Collection) {
		return BIContext{}, fmt.Errorf("bi: invalid collection name")
	}
	switch strings.TrimSpace(in.Source) {
	case "", BISourceCollection:
		out.Source = BISourceCollection
	case BISourceTable:
		out.Source = BISourceTable
	default:
		return BIContext{}, fmt.Errorf("bi: unknown source")
	}
	preferred := strings.TrimSpace(in.PreferredType)
	if preferred != "" && !isBIChartType(preferred) {
		return BIContext{}, fmt.Errorf("bi: unknown preferred_type")
	}
	if preferred != "auto" {
		out.PreferredType = preferred
	}
	seen := map[string]struct{}{}
	for _, f := range in.Fields {
		name, typ := strings.TrimSpace(f.Name), strings.TrimSpace(f.Type)
		if !biNamePattern.MatchString(name) {
			continue
		}
		if _, ok := biFieldTypes[typ]; !ok {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out.Fields = append(out.Fields, BIField{Name: name, Type: typ})
		if len(out.Fields) >= BIMaxFields {
			break
		}
	}
	return out, nil
}

func isBIChartType(v string) bool {
	for _, t := range BIChartTypes {
		if t == v {
			return true
		}
	}
	return false
}

// biEnv 返回追加到 ProjectEnv 的行。字段已经过清洗，只含字母数字下划线与白名单类型。
func biEnv(c BIContext) string {
	var b strings.Builder
	b.WriteString("\n- bi_database: " + c.DatabaseID)
	if c.Source == BISourceTable {
		b.WriteString("\n- bi_table: " + c.Collection)
	} else {
		b.WriteString("\n- bi_collection: " + c.Collection)
	}
	if c.PreferredType != "" {
		b.WriteString("\n- bi_preferred_type: " + c.PreferredType)
	}
	parts := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		parts = append(parts, f.Name+":"+f.Type)
	}
	b.WriteString("\n- bi_fields: " + strings.Join(parts, ", "))
	return b.String()
}

// biCollectionIntro / biTableIntro 是两种数据源各自的开头两条规则；其余规则（biModuleRules）共用。
const biCollectionIntro = `Chart mode: the user is building a BI dashboard for ONE collection, given in the project env as bi_database / bi_collection / bi_fields.
Rules:
- Propose 1-4 charts about that collection only. Use ONLY the fields listed in bi_fields. created_at and updated_at are always available date fields and may be used for time buckets. Never invent a field.`

const biTableIntro = `Chart mode: the user is building a BI dashboard for ONE SQL table, given in the project env as bi_database / bi_table / bi_fields. bi_fields are the table's real columns with a type of text, number, bool, date or json.
Rules:
- Propose 1-4 charts about that table only. Use ONLY the columns listed in bi_fields. Never invent a column and never assume created_at or updated_at exist unless they are listed. Columns of type json cannot be charted.`

// biModuleRules 是两种数据源共用的规则。
const biModuleRules = `
- Read-only: you may use the read-only tools to sample values, but never write, insert, update, delete, create or drop anything.
- Chart types: line, column (vertical bars), bar (horizontal bars), pie, table, grouped_bar (horizontal, side by side), stacked_bar (horizontal, stacked), scatter, or auto to let the UI decide. A KPI card is a chart with no group_by and exactly one metric, use type auto for it.
- Metrics: count, sum:<field>, avg:<field>, min:<field>, max:<field>, distinct:<field>. sum and avg need a number field; min and max need a number or date field. stacked_bar may only use count or sum. At most 6 metrics.
- group_by: <field>, <date field>:<hour|day|week|month|year> for a time bucket, or <number field>:bin=<step> for a histogram. series: a second grouping field, no modifiers.
- scatter takes x and y (both number fields), optional size (number) and series (text, select or bool), and no metrics or group_by.
- filter uses the same expressions as the data browser, for example status="open" && amount>=100. Do not repeat the dashboard's global filter.
- Reply in the user's language with a short explanation first. Then, at the very end, output one line per chart in exactly this form: <!-- sb:chart {"version":1,"title":"...","type":"...", ...} -->
- Each block is single-line JSON with these keys only: version (always 1), title (plain text, at most 40 characters), type, metrics (array of strings), group_by, series, x, y, size, filter, sort (prefix - for descending, only a returned column), limit, area (line only), donut (pie only). Omit keys you do not need. Never output an id or layout.
- If the data does not suit a chart or a field does not exist, explain why and output no block.
- Blocks contain configuration only, never SQL. Never output secrets of any kind.`

// biPrompt 返回 BI 模式的模块层提示词（集合）。保留给既有调用与测试。
func biPrompt() string { return biPromptFor(BISourceCollection) }

// biPromptFor 按数据源种类拼装提示词。
func biPromptFor(source string) string {
	if source == BISourceTable {
		return biTableIntro + "\n" + biModuleRules
	}
	return biCollectionIntro + "\n" + biModuleRules
}
