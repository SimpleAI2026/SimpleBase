package ducklake

import (
	"context"
	"database/sql"
	"fmt"
)

// SetCommitMessage 在当前写事务内写入快照提交者与消息（§4.7）。
// 必须在 BEGIN 之后、COMMIT 之前调用；extraJSON 可为空。
func SetCommitMessage(ctx context.Context, db *sql.DB, alias, author, message, extraJSON string) error {
	if !isSafeIdent(alias) {
		return fmt.Errorf("ducklake: invalid lake alias")
	}
	if extraJSON == "" {
		extraJSON = "{}"
	}
	q := fmt.Sprintf(
		"CALL %s.set_commit_message(%s, %s, extra_info => %s)",
		quoteIdent(alias),
		quoteSQLString(author),
		quoteSQLString(message),
		quoteSQLString(extraJSON),
	)
	_, err := db.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("ducklake: set_commit_message: %w", err)
	}
	return nil
}
