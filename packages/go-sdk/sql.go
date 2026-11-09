package gosdk

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// SQLStatement is one parameterized statement in a batch.
type SQLStatement struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

// QueryResult returns positional rows corresponding to Columns.
type QueryResult struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"row_count"`
	DurationMS int64    `json:"duration_ms"`
	RequestID  string   `json:"request_id"`
}

// ExecuteResult contains write execution metadata.
type ExecuteResult struct {
	RowsAffected int64  `json:"rows_affected"`
	Durability   string `json:"durability"`
	DurationMS   int64  `json:"duration_ms"`
	RequestID    string `json:"request_id"`
}

// BatchItem is the result of one statement; ErrorCode may be set for non-transactional batches.
type BatchItem struct {
	Index        int    `json:"index"`
	RowsAffected int64  `json:"rows_affected"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// BatchFailure indicates an HTTP 200 transactional rollback; inspect this field before treating a batch as successful.
type BatchFailure struct {
	FailedIndex int    `json:"failed_index"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

// BatchResult may contain Error (transactional) or per-item ErrorCode (non-transactional).
type BatchResult struct {
	Results    []BatchItem   `json:"results"`
	Durability string        `json:"durability"`
	DurationMS int64         `json:"duration_ms"`
	RequestID  string        `json:"request_id"`
	Error      *BatchFailure `json:"error,omitempty"`
}

// Query executes a read-only SQL statement; maxRows 0 uses the server limit.
func (c *Client) Query(ctx context.Context, sql string, args []any, maxRows int) (*QueryResult, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("gosdk: SQL is required")
	}
	path, err := c.databasePath("/query")
	if err != nil {
		return nil, err
	}
	var out QueryResult
	err = c.requestJSON(ctx, http.MethodPost, path, nil, struct {
		SQL     string `json:"sql"`
		Args    []any  `json:"args"`
		MaxRows int    `json:"max_rows,omitempty"`
	}{sql, nonNilArgs(args), maxRows}, &out)
	return &out, err
}

// Execute runs one write statement.
func (c *Client) Execute(ctx context.Context, sql string, args []any) (*ExecuteResult, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("gosdk: SQL is required")
	}
	path, err := c.databasePath("/execute")
	if err != nil {
		return nil, err
	}
	var out ExecuteResult
	err = c.requestJSON(ctx, http.MethodPost, path, nil, SQLStatement{sql, nonNilArgs(args)}, &out)
	return &out, err
}

// Batch executes statements, optionally in one transaction. Inspect result.Error and item errors.
func (c *Client) Batch(ctx context.Context, statements []SQLStatement, transactional bool) (*BatchResult, error) {
	path, err := c.databasePath("/batch")
	if err != nil {
		return nil, err
	}
	normalized := make([]SQLStatement, len(statements))
	for i, statement := range statements {
		if strings.TrimSpace(statement.SQL) == "" {
			return nil, errors.New("gosdk: batch SQL is required")
		}
		normalized[i] = SQLStatement{SQL: statement.SQL, Args: nonNilArgs(statement.Args)}
	}
	var out BatchResult
	err = c.requestJSON(ctx, http.MethodPost, path, nil, struct {
		Statements    []SQLStatement `json:"statements"`
		Transactional bool           `json:"transactional"`
	}{normalized, transactional}, &out)
	return &out, err
}

func nonNilArgs(args []any) []any {
	if args == nil {
		return []any{}
	}
	return args
}
