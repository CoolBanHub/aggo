package database

import (
	"context"
	"testing"
)

func TestDatabaseExecuteRejectsWriteByDefault(t *testing.T) {
	tool := &DatabaseExecuteTool{}

	_, err := tool.execute(context.Background(), ExecuteParams{Query: "UPDATE users SET name = ?", Params: []interface{}{"x"}})
	if err == nil {
		t.Fatal("expected write query to be rejected")
	}
}

func TestFirstSQLKeywordSkipsLeadingComments(t *testing.T) {
	got := firstSQLKeyword("-- comment\n/* block */\nselect 1")
	if got != "SELECT" {
		t.Fatalf("firstSQLKeyword = %q, want SELECT", got)
	}
}

func TestReadOnlyKeywordPolicy(t *testing.T) {
	if !isReadOnlyKeyword(firstSQLKeyword("WITH cte AS (SELECT 1) SELECT * FROM cte")) {
		t.Fatal("WITH query should be treated as read-only")
	}
	if isReadOnlyKeyword(firstSQLKeyword("DELETE FROM users")) {
		t.Fatal("DELETE query should not be treated as read-only")
	}
}
