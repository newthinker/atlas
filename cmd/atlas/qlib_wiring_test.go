package main

// Context Checkpoint: done_criteria → test mapping (TASK-010 wiring, TASK-016 PIT EPS)
// functional[0]  "Enabled=true, DB valid → register qlib collector, return (db,true), db non-nil"
//                → TestWireQlib_SuccessRegisters
// boundary[0]    "Enabled=false → return (nil,false), openFn not called, no qlib in reg"
//                → TestWireQlib_DisabledSkips
// error_handling[0] "openFn returns error → return (nil,false), no panic, no qlib in reg"
//                → TestWireQlib_OpenFailSkips
// error_handling[1] "Ping fails → return (nil,false), no qlib in reg"
//                → TestWireQlib_PingFailSkips

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// boundary[0]: cfg.Enabled=false → return (nil,false), openFn never called, reg has no qlib.
func TestWireQlib_DisabledSkips(t *testing.T) {
	cfg := config.QlibConfig{Enabled: false, DBPath: "/some/path.db"}
	reg := collector.NewRegistry()
	openCalled := false
	openFn := func(dbPath string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	}

	db, ok := wireQlibWarehouse(cfg, reg, openFn, zap.NewNop())

	if ok {
		t.Fatal("expected false when Enabled=false")
	}
	if db != nil {
		t.Fatal("expected nil db when Enabled=false")
	}
	if openCalled {
		t.Fatal("openFn must not be called when Enabled=false")
	}
	if _, ok := reg.Get("qlib"); ok {
		t.Fatal("qlib must not be registered when Enabled=false")
	}
}

// error_handling[0]: openFn returns error → return (nil,false), no panic, qlib not in reg.
func TestWireQlib_OpenFailSkips(t *testing.T) {
	cfg := config.QlibConfig{Enabled: true, DBPath: "/any/path.db"}
	reg := collector.NewRegistry()
	openFn := func(dbPath string) (*sql.DB, error) {
		return nil, errors.New("boom")
	}

	db, ok := wireQlibWarehouse(cfg, reg, openFn, zap.NewNop())

	if ok {
		t.Fatal("expected false when openFn errors")
	}
	if db != nil {
		t.Fatal("expected nil db when openFn errors")
	}
	if _, ok := reg.Get("qlib"); ok {
		t.Fatal("qlib must not be registered when openFn errors")
	}
}

// error_handling[1]: openFn returns a DB whose Ping fails → return (nil,false), no qlib.
// Uses file:/nonexistent-dir/x.db?mode=ro which Open succeeds but Ping fails.
func TestWireQlib_PingFailSkips(t *testing.T) {
	cfg := config.QlibConfig{Enabled: true, DBPath: "/nonexistent-dir/x.db"}
	reg := collector.NewRegistry()
	openFn := func(dbPath string) (*sql.DB, error) {
		// This DSN causes Open to succeed but Ping to fail under modernc sqlite.
		return sql.Open("sqlite", "file:/nonexistent-dir/x.db?mode=ro")
	}

	db, ok := wireQlibWarehouse(cfg, reg, openFn, zap.NewNop())

	if ok {
		t.Fatal("expected false when Ping fails")
	}
	if db != nil {
		t.Fatal("expected nil db when Ping fails")
	}
	if _, ok := reg.Get("qlib"); ok {
		t.Fatal("qlib must not be registered when Ping fails")
	}
}

// functional[0]: openFn returns a healthy in-memory DB with schema → return (db,true),
// db is non-nil, reg.Get("qlib") returns the collector, Name()=="qlib".
// Also exercises MaxStalenessDays=0 path (defaults to 7*24h, must not panic).
// The returned db and the db inside the registered collector are the same handle
// (no double-open): both use the single *sql.DB returned by openFn.
func TestWireQlib_SuccessRegisters(t *testing.T) {
	cfg := config.QlibConfig{Enabled: true, DBPath: ":memory:", MaxStalenessDays: 0}
	reg := collector.NewRegistry()

	openFn := func(dbPath string) (*sql.DB, error) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return nil, err
		}
		// Create the schema qlib.New expects for Covers() and FetchHistory(),
		// then seed one row so warehouse_meta has data.
		stmts := []string{
			`CREATE TABLE IF NOT EXISTS ohlcv (
				symbol TEXT NOT NULL,
				date   TEXT NOT NULL,
				open   REAL, high REAL, low REAL, close REAL, volume INTEGER,
				PRIMARY KEY (symbol, date)
			)`,
			`CREATE TABLE IF NOT EXISTS warehouse_meta (
				symbol    TEXT PRIMARY KEY,
				last_date TEXT NOT NULL
			)`,
			`INSERT INTO warehouse_meta (symbol, last_date) VALUES ('AAPL', '2024-01-01')`,
		}
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return db, nil
	}

	db, ok := wireQlibWarehouse(cfg, reg, openFn, zap.NewNop())

	if !ok {
		t.Fatal("expected true on successful wiring")
	}
	if db == nil {
		t.Fatal("expected non-nil db on successful wiring")
	}
	c, ok := reg.Get("qlib")
	if !ok {
		t.Fatal("qlib collector must be registered on success")
	}
	if c.Name() != "qlib" {
		t.Fatalf("collector Name() must be 'qlib', got %q", c.Name())
	}
}
