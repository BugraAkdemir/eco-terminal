package storage

import (
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func TestOpenMigratesAndHasExtensions(t *testing.T) {
	db, err := OpenTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM roles`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("roles seeded? n=%d err=%v", n, err)
	}
	// migrations are idempotent
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	// FTS5 + RTree + vec0 available
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t_vec USING vec0(emb float[3])`); err != nil {
		t.Fatalf("vec0: %v", err)
	}
	b, _ := sqlite_vec.SerializeFloat32([]float32{1, 2, 3})
	if _, err := db.Exec(`INSERT INTO t_vec(rowid, emb) VALUES (1, ?)`, b); err != nil {
		t.Fatal(err)
	}
	var fk int
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys not enabled")
	}
	var mode string
	db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if mode != "wal" {
		t.Fatalf("journal_mode=%s", mode)
	}
}

func TestNewIDUnique(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b || len(a) != 36 {
		t.Fatalf("bad ids %s %s", a, b)
	}
}
