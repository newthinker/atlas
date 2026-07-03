package main

// Context Checkpoint: done_criteria → test mapping (TASK-302)
// functional[1]     "backend=sqlite → SQLiteStore(用配置 path)；backend=memory → MemoryStore" → TestBuildSignalStore_Memory / TestBuildSignalStore_Sqlite
// error_handling[0] "sqlite 打开失败 → 启动即返回错误退出，不降级内存"                        → TestBuildSignalStore_SqliteOpenFailure
// error_handling[1] "backend 非法值 → 报错，错误信息含非法值"                                → TestBuildSignalStore_InvalidBackend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	signalstore "github.com/newthinker/atlas/internal/storage/signal"
	"go.uber.org/zap"
)

func signalStoreTestConfig(backend, path string) *config.Config {
	cfg := config.Defaults()
	cfg.Storage.Signals.Backend = backend
	cfg.Storage.Signals.Path = path
	return cfg
}

// functional[1]: backend=memory builds an in-memory store with a no-op cleanup.
func TestBuildSignalStore_Memory(t *testing.T) {
	store, cleanup, err := buildSignalStore(signalStoreTestConfig("memory", ""), zap.NewNop())
	if err != nil {
		t.Fatalf("buildSignalStore(memory) error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup must be non-nil (even for memory)")
	}
	defer cleanup()
	if _, ok := store.(*signalstore.MemoryStore); !ok {
		t.Errorf("memory backend must build *MemoryStore, got %T", store)
	}
}

// functional[1]: backend=sqlite builds a SQLiteStore at the configured path; the
// store is usable and the file is created; cleanup closes it without error.
func TestBuildSignalStore_Sqlite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signals.db")
	store, cleanup, err := buildSignalStore(signalStoreTestConfig("sqlite", path), zap.NewNop())
	if err != nil {
		t.Fatalf("buildSignalStore(sqlite) error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup must be non-nil for sqlite")
	}
	defer cleanup()

	if _, ok := store.(*signalstore.SQLiteStore); !ok {
		t.Errorf("sqlite backend must build *SQLiteStore, got %T", store)
	}
	// Round-trip proves it is a real, open store (not a silent fallback).
	if err := store.Save(context.Background(), core.Signal{Symbol: "AAPL", Strategy: "s"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	n, err := store.Count(context.Background(), signalstore.ListFilter{})
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("sqlite db file not created at %s: %v", path, err)
	}
}

// error_handling[0]: an unopenable sqlite path returns an error so serve exits;
// it must NOT silently fall back to an in-memory store.
func TestBuildSignalStore_SqliteOpenFailure(t *testing.T) {
	// Make the parent path a regular file so NewSQLiteStore's MkdirAll fails.
	fileAsDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(fileAsDir, "sub", "signals.db")

	store, cleanup, err := buildSignalStore(signalStoreTestConfig("sqlite", badPath), zap.NewNop())
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("expected error on unopenable sqlite path, got nil (must not fall back to memory)")
	}
	if store != nil {
		t.Errorf("store must be nil on open failure, got %T (no memory fallback allowed)", store)
	}
}

// error_handling[1]: an invalid backend is rejected and the error names the value.
func TestBuildSignalStore_InvalidBackend(t *testing.T) {
	store, _, err := buildSignalStore(signalStoreTestConfig("bogus", ""), zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid backend, got nil")
	}
	if store != nil {
		t.Errorf("store must be nil for invalid backend, got %T", store)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error must name the invalid backend 'bogus', got %v", err)
	}
}
