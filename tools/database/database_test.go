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
	if !isReadOnlyKeyword(firstSQLKeyword("SELECT * FROM users")) {
		t.Fatal("SELECT should be treated as read-only")
	}
	if isReadOnlyKeyword(firstSQLKeyword("DELETE FROM users")) {
		t.Fatal("DELETE query should not be treated as read-only")
	}
	// WITH（CTE）不再默认视为只读：写 CTE 可能借道绕过只读限制
	if isReadOnlyKeyword(firstSQLKeyword("WITH cte AS (SELECT 1) SELECT * FROM cte")) {
		t.Fatal("WITH query must not be treated as read-only by default")
	}
}

// 写 CTE 与多语句在只读模式下被拒绝。
func TestWriteCTEAndMultiStatementRejected(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"write cte delete", "WITH old AS (SELECT id FROM logs WHERE ts < 100) DELETE FROM logs WHERE id IN (SELECT id FROM old)"},
		{"write cte update", "WITH t AS (SELECT 1) UPDATE users SET name = 'x'"},
		{"select then drop", "SELECT 1; DROP TABLE users"},
		{"select then delete", "SELECT 1 ; DELETE FROM users"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &DatabaseExecuteTool{}
			_, err := tool.execute(context.Background(), ExecuteParams{Query: tc.query})
			if err == nil {
				t.Fatalf("query %q should be rejected in read-only mode", tc.query)
			}
		})
	}
}

// 字符串字面量中的分号不构成多语句，合法 SELECT 不受影响。
func TestStringLiteralSemicolonIsNotMultiStatement(t *testing.T) {
	cases := []string{
		"SELECT * FROM users WHERE note = 'a;b'",
		"SELECT * FROM users WHERE note = 'it''s;fine'",
		"SELECT 1;",
	}
	for _, query := range cases {
		if hasMultipleStatements(query) {
			t.Fatalf("query %q should not be treated as multi-statement", query)
		}
		tool := &DatabaseExecuteTool{}
		// 只读判定通过即可（无数据库连接时在 Rows() 处失败，而不是被只读策略拒绝）
		if _, err := tool.execute(context.Background(), ExecuteParams{Query: query}); err != nil {
			toolErr := err.Error()
			if contains(toolErr, "read-only") {
				t.Fatalf("query %q wrongly rejected by read-only policy", query)
			}
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
