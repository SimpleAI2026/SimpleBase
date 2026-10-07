package systemdb

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func TestAgentMessagesReturnMostRecentPageInChronologicalOrder(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := NewStoreForTest(db)
	thread, err := store.CreateAgentThread(ctx, AgentThread{ProjectID: "p", Title: "recent"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if _, err := store.AppendAgentMessage(ctx, AgentMessage{ID: fmt.Sprintf("%03d", i), ThreadID: thread.ID, ProjectID: "p", Role: "user", Content: fmt.Sprintf("message-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := store.ListAgentMessages(ctx, "p", thread.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 || msgs[0].Content != "message-45" || msgs[4].Content != "message-49" {
		t.Fatalf("history=%+v", msgs)
	}
}
