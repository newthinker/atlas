package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FromFile(t *testing.T) {
	content := []byte(`
server:
  host: "127.0.0.1"
  port: 8080

storage:
  hot:
    dsn: "postgres://localhost:5432/atlas"
  cold:
    type: localfs
    path: "/tmp/atlas/archive"
`)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Storage.Cold.Type != "localfs" {
		t.Errorf("expected localfs, got %s", cfg.Storage.Cold.Type)
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Router.MinConfidence != 0.6 {
		t.Errorf("expected default min_confidence 0.6, got %f", cfg.Router.MinConfidence)
	}
}
