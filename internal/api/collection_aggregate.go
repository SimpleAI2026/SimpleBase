// 文件说明：collection_aggregate.go — 集合聚合接口 GET .../data/collections/:collection/aggregate
//
// 只读：走 ReadOnly lease，不挂 writeGate；参数解析与 SQL 拼装在
// collection_aggregate_query.go（planv6.7 §3）。

package api

import (
	"context"
	"encoding/json"
	collectionquery "github.com/linkxzhou/SimpleBase/internal/collection/query"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

const (
	// defaultAggregateScan 是聚合前的行数扫描上限（planv6.7 Q3：一期用常量，不暴露配置）。
	defaultAggregateScan = 1_000_000
	// defaultAggregateTimeout 是聚合查询的默认超时。
	defaultAggregateTimeout = 30 * time.Second
)

// aggregateLimitConfig 是聚合接口的超时与扫描上限；零值取默认。
// DataHandler 与 SchemaHandler 内嵌它，共用设置与读取逻辑。
type aggregateLimitConfig struct {
	aggregateTimeout time.Duration
	aggregateScan    int
}

// setAggregateLimits 设置上限；非正数保持原值。
func (c *aggregateLimitConfig) setAggregateLimits(timeout time.Duration, scan int) {
	if timeout > 0 {
		c.aggregateTimeout = timeout
	}
	if scan > 0 {
		c.aggregateScan = scan
	}
}

// aggregateLimits 返回生效的超时与扫描上限，未设置时回落到默认值。
func (c *aggregateLimitConfig) aggregateLimits() (time.Duration, int) {
	timeout, scan := c.aggregateTimeout, c.aggregateScan
	if timeout <= 0 {
		timeout = defaultAggregateTimeout
	}
	if scan <= 0 {
		scan = defaultAggregateScan
	}
	return timeout, scan
}

// aggregateResponse 是聚合接口响应。行值经 godatabase.SerializeRows，与 /query 同一序列化路径。
type aggregateResponse struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	// Total 是「同口径总量」：仅当请求带 with_total、分组且单度量可累加时返回（planv6.7 BI-F01）。
	// 用 RawMessage 直接承载序列化后的值，避免大整数 / Decimal 经 float64 失真；没有时不出现该键。
	Total      json.RawMessage `json:"total,omitempty"`
	Truncated  bool            `json:"truncated"`
	Scanned    int64           `json:"scanned"`
	DurationMS int64           `json:"duration_ms"`
	RequestID  string          `json:"request_id,omitempty"`
}

// aggregateTotal 执行同口径总量查询（不分组的 count / sum）。没有 TotalSQL 时返回 nil。
func aggregateTotal(ctx context.Context, lease SQLLease, q collectionquery.AggregateQuery) (json.RawMessage, error) {
	if q.TotalSQL == "" {
		return nil, nil
	}
	result, err := lease.Query(ctx, godatabase.Statement{SQL: q.TotalSQL, Args: q.TotalArgs}, 1)
	if err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		return nil, nil
	}
	serialized, err := godatabase.SerializeRows([][]any{result.Rows[0][:1]})
	if err != nil || len(serialized) == 0 || len(serialized[0]) == 0 {
		return nil, err
	}
	return json.Marshal(serialized[0][0])
}

// WithAggregateLimits 设置聚合的扫描上限与超时；非正数保持默认。
func (h *DataHandler) WithAggregateLimits(timeout time.Duration, scan int) *DataHandler {
	h.setAggregateLimits(timeout, scan)
	return h
}

// Aggregate: GET /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/aggregate
func (h *DataHandler) Aggregate(c echo.Context) error {
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	timeout, scanMax := h.aggregateLimits()
	ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
	defer cancel()

	db, lease, err := h.acquireDatabase(c, godatabase.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(ctx, name)
	if err != nil {
		return validationAPIError(c, err)
	}
	engine, err := catalog.DatabaseEngine(db.DatabasePath)
	if err != nil {
		engine = catalog.EngineDuckDB
	}
	dialect, err := godatabase.DialectFor(engine)
	if err != nil {
		return WriteError(c, err)
	}
	query, err := collectionquery.ParseAggregateQuery(c.QueryParams(), collectionSource(name, schema), schema, dialect)
	if err != nil {
		return validationAPIError(c, err)
	}

	// 先数一遍：超过扫描上限就拒绝，提示调用方缩小时间范围（不静默截断聚合基数）。
	count, err := lease.Query(ctx, godatabase.Statement{SQL: query.CountSQL, Args: query.CountArgs}, 1)
	if err != nil {
		if isMissingRelation(err) {
			return c.JSON(http.StatusOK, emptyAggregate(query, c))
		}
		return WriteError(c, err)
	}
	var scanned int64
	if len(count.Rows) > 0 && len(count.Rows[0]) > 0 {
		scanned = anyInt64(count.Rows[0][0])
	}
	if scanned > int64(scanMax) {
		return scanLimitError(c)
	}

	// 聚合结果行数 ≤ limit+1，普通查询上限同样取 limit+1，多出的一行用来判断 truncated。
	result, err := lease.Query(ctx, godatabase.Statement{SQL: query.SQL, Args: query.Args}, query.Limit+1)
	if err != nil {
		if isMissingRelation(err) {
			return c.JSON(http.StatusOK, emptyAggregate(query, c))
		}
		return WriteError(c, err)
	}
	rows := result.Rows
	truncated := false
	if len(rows) > query.Limit {
		truncated = true
		rows = rows[:query.Limit]
	}
	if query.NeedsSeriesFold && len(query.Columns) > 0 {
		collectionquery.FoldSeries(rows, len(query.Columns)-1)
	}
	serialized, err := godatabase.SerializeRows(rows)
	if err != nil {
		return WriteError(c, err)
	}
	if serialized == nil {
		serialized = [][]any{}
	}
	total, err := aggregateTotal(ctx, lease, query)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, aggregateResponse{
		Columns:    query.Columns,
		Rows:       serialized,
		Total:      total,
		Truncated:  truncated,
		Scanned:    scanned,
		DurationMS: result.Duration.Milliseconds(),
		RequestID:  requestID(c),
	})
}

// emptyAggregate：集合表尚未创建（还没写入过文档）时返回空结果，与 ListDocuments 一致。
func emptyAggregate(q collectionquery.AggregateQuery, c echo.Context) aggregateResponse {
	return aggregateResponse{
		Columns:   q.Columns,
		Rows:      [][]any{},
		RequestID: requestID(c),
	}
}
