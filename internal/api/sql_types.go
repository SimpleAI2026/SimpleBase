package api

// sql_types.go 定义 SQL API 的 HTTP DTO（见 plan6.md）。
//
// 请求不接受 raw DSN、database path、S3 key、事务 ID 或任意 driver option。
// 所有 SQL 必须使用参数化占位符，禁止拼接用户输入。

// SQLStatementRequest 是单条 SQL 语句的请求体。
type SQLStatementRequest struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

// QueryRequest 是查询请求，可指定返回行数上限。
type QueryRequest struct {
	SQLStatementRequest
	MaxRows int `json:"max_rows"`
}

// ExecuteRequest 是执行（写）请求。
type ExecuteRequest struct {
	SQLStatementRequest
}

// BatchRequest 是批量语句请求。
// Transactional=true 时使用单个事务，任一失败回滚；
// Transactional=false 时逐条独立执行，某条失败不影响后续但会在 error 中标明位置。
type BatchRequest struct {
	Statements    []SQLStatementRequest `json:"statements"`
	Transactional bool                  `json:"transactional"`
}

// QueryResponse 是查询响应。Rows 中列值经 serialize 安全转换。
type QueryResponse struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"row_count"`
	DurationMS int64    `json:"duration_ms"`
	RequestID  string   `json:"request_id"`
}

// ExecuteResponse 是执行响应。
// LastInsertID 为指针：nil 表示驱动未提供或语句不产生自增 ID。
type ExecuteResponse struct {
	RowsAffected int64  `json:"rows_affected"`
	LastInsertID *int64 `json:"last_insert_id,omitempty"`
	DurationMS   int64  `json:"duration_ms"`
	RequestID    string `json:"request_id"`
}

// BatchResultItem 是批量执行中单条语句的结果或错误。
type BatchResultItem struct {
	Index         int    `json:"index"`
	RowsAffected  int64  `json:"rows_affected,omitempty"`
	LastInsertID  *int64 `json:"last_insert_id,omitempty"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
}

// BatchResponse 是批量执行响应。
// Transactional 模式下任一失败会回滚并返回整体 error；NonTransactional 模式下
// 逐条返回结果，失败项带有 error_code/error_message。
type BatchResponse struct {
	Results    []BatchResultItem `json:"results"`
	DurationMS int64             `json:"duration_ms"`
	RequestID  string            `json:"request_id"`
	// Error 在 transactional 模式回滚时填充，表示整体失败位置。
	Error *BatchError `json:"error,omitempty"`
}

// BatchError 描述事务批量回滚的整体错误。
type BatchError struct {
	FailedIndex int    `json:"failed_index"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}
