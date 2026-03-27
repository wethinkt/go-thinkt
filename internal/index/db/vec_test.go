package db

import (
	"database/sql"
	"testing"
)

func TestSqliteVecAvailable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version string
	err = db.QueryRow("SELECT vec_version()").Scan(&version)
	if err != nil {
		t.Fatalf("sqlite-vec not available: %v", err)
	}
	if version == "" {
		t.Fatal("vec_version() returned empty string")
	}
	t.Logf("sqlite-vec version: %s", version)
}
