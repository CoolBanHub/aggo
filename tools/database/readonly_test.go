package database

import (
	"context"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestReadOnlyExecutionEnforcedBySQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	if err := db.Exec("CREATE TABLE victims (id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("INSERT INTO victims VALUES (1), (2), (3)")
	tool := &DatabaseExecuteTool{db: db, maxResultRows: 2}
	for _, query := range []string{
		"SELECT /* ' */ 1; DROP TABLE victims",
		"PRAGMA user_version = 731",
		"EXPLAIN ANALYZE DELETE FROM victims",
		"SELECT * INTO new_users FROM victims",
		"SELECT 1 /*!; DROP TABLE victims */",
		"SELECT 1; /* ' */ DROP TABLE victims",
		"SELECT load_extension('something')",
		"SELECT 1 /* outer /* nested */; DROP TABLE victims -- */",
	} {
		if _, err := tool.execute(context.Background(), ExecuteParams{Query: query}); err == nil {
			t.Errorf("unsafe query accepted: %s", query)
		}
	}
	result, err := tool.execute(context.Background(), ExecuteParams{Query: "SELECT /* ' ignored */ id FROM victims; -- trailing comment"})
	if err != nil {
		t.Fatal(err)
	}
	output := result.(map[string]interface{})
	if output["count"] != 2 || output["truncated"] != true {
		t.Fatalf("unexpected result: %#v", output)
	}
	// Query-only state must not leak into the connection pool.
	if err := db.Exec("INSERT INTO victims VALUES (4)").Error; err != nil {
		t.Fatal(err)
	}
	// Execute the forbidden write through the database guard itself, bypassing
	// the syntax check, to verify the underlying boundary is effective.
	err = db.Connection(func(conn *gorm.DB) error {
		conn.Exec("PRAGMA query_only = ON")
		defer conn.Exec("PRAGMA query_only = OFF")
		_, err := tool.readRows(context.Background(), conn, ExecuteParams{Query: "DELETE FROM victims RETURNING id"})
		return err
	})
	if err == nil {
		t.Fatal("SQLite query_only allowed DELETE")
	}
	var count int64
	if err := db.Table("victims").Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("victims changed: count=%d err=%v", count, err)
	}
}

func TestSQLScannerRejectsAmbiguousOrMultipleStatements(t *testing.T) {
	for _, query := range []string{
		"SELECT /* unterminated", "SELECT 'unterminated", "SELECT 1; SELECT 2", "SELECT 1 # comment\n; DROP TABLE t", "SELECT $$;$$", "SELECT 'a\\'; DELETE FROM t", "EXPLAIN (ANALYZE true) DELETE FROM t",
		"SELECT /* outer /* inner */ ' */ 1;",
		"SELECT 1 /* outer /* nested */; DROP TABLE victims -- */",
	} {
		if isSafeReadQuery(query) {
			t.Errorf("accepted %q", query)
		}
	}
	for _, query := range []string{"SELECT 'it''s;fine'", "SELECT \"a;b\" FROM t", "SELECT /* ordinary comment */ 1;", "EXPLAIN SELECT 1"} {
		if !isSafeReadQuery(query) {
			t.Errorf("rejected safe query %q", query)
		}
	}
}

func TestReadOnlyRejectsExistingTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	tool := &DatabaseExecuteTool{db: tx, maxResultRows: 1}
	if _, err := tool.execute(context.Background(), ExecuteParams{Query: "SELECT 1"}); err == nil {
		t.Fatal("read-only tool accepted unknown transaction mode")
	}
}
