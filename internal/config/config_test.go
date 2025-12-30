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

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
			},
			wantErr: false,
		},
		{
			name: "invalid port - zero",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 0},
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 70000},
			},
			wantErr: true,
		},
		{
			name: "invalid router confidence",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
				Router: RouterConfig{MinConfidence: 1.5},
			},
			wantErr: true,
		},
		{
			name: "negative cooldown",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
				Router: RouterConfig{CooldownHours: -1},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
