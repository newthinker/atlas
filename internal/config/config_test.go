package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-005)
// functional[0]    "router.percentile_step 经 mapstructure 从配置解析生效" → TestLoad_RouterPercentileStep_FromYAML
// boundary[0]      "未配置时字段为零值 0(Defaults 返回 0)"             → TestDefaults_RouterPercentileStep_Zero
// error_handling[0] "PercentileStep<0 校验返回错误,链含 ErrConfigInvalid" → TestConfig_Validate_PercentileStepNegative

// functional[0]: percentile_step parses from yaml via mapstructure tag.
func TestLoad_RouterPercentileStep_FromYAML(t *testing.T) {
	cfgPath := writeTempConfig(t, `
router:
  percentile_step: 5
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Router.PercentileStep != 5 {
		t.Errorf("Router.PercentileStep = %v, want 5", cfg.Router.PercentileStep)
	}
}

// boundary[0]: unset percentile_step is zero (disabled, back-compat); Defaults omits it.
func TestDefaults_RouterPercentileStep_Zero(t *testing.T) {
	if got := Defaults().Router.PercentileStep; got != 0 {
		t.Errorf("Defaults Router.PercentileStep = %v, want 0", got)
	}
	cfgPath := writeTempConfig(t, "server:\n  port: 8080\n")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Router.PercentileStep != 0 {
		t.Errorf("unconfigured Router.PercentileStep = %v, want 0", cfg.Router.PercentileStep)
	}
}

// error_handling[0]: negative percentile_step rejected, error chain holds ErrConfigInvalid.
func TestConfig_Validate_PercentileStepNegative(t *testing.T) {
	c := validConfig()
	c.Router.PercentileStep = -1
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for negative percentile_step, got nil")
	}
	if !errors.Is(err, core.ErrConfigInvalid) {
		t.Errorf("error chain must contain core.ErrConfigInvalid, got %v", err)
	}
	// zero and positive must pass.
	for _, v := range []float64{0, 5} {
		c.Router.PercentileStep = v
		if err := c.Validate(); err != nil {
			t.Errorf("percentile_step=%v: unexpected error %v", v, err)
		}
	}
}

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

// Context Checkpoint: done_criteria → test mapping (TASK-004)
// functional[0]    "yaml 设置三组配置后 Load 返回对应值"      → TestLoad_NewConfig_FromYAML
// functional[1]    "缺省时取默认 4/15s/true/5m"             → TestLoad_NewConfig_Defaults
// boundary[0]      "workers=0/负数 Load 不报错"             → TestLoad_Workers_ZeroOrNegative
// boundary[1]      "timeout/ttl=0 取各自默认值"             → TestLoad_TimeoutTTL_ZeroUsesDefault
// error_handling[0] "非法 duration 字符串返回错误"           → TestLoad_InvalidDuration

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

// functional[0]
func TestLoad_NewConfig_FromYAML(t *testing.T) {
	cfgPath := writeTempConfig(t, `
analysis:
  workers: 8
meta:
  arbitrator:
    timeout: 30s
collector:
  cache:
    enabled: false
    ttl: 2m
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Analysis.Workers != 8 {
		t.Errorf("Workers = %d, want 8", cfg.Analysis.Workers)
	}
	if cfg.Meta.Arbitrator.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Meta.Arbitrator.Timeout)
	}
	if cfg.Collector.Cache.Enabled {
		t.Errorf("Cache.Enabled = true, want false")
	}
	if cfg.Collector.Cache.TTL != 2*time.Minute {
		t.Errorf("Cache.TTL = %v, want 2m", cfg.Collector.Cache.TTL)
	}
}

// functional[1]
func TestLoad_NewConfig_Defaults(t *testing.T) {
	cfgPath := writeTempConfig(t, `
server:
  port: 8080
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Analysis.Workers != 4 {
		t.Errorf("default Workers = %d, want 4", cfg.Analysis.Workers)
	}
	if cfg.Meta.Arbitrator.Timeout != 15*time.Second {
		t.Errorf("default Timeout = %v, want 15s", cfg.Meta.Arbitrator.Timeout)
	}
	if !cfg.Collector.Cache.Enabled {
		t.Errorf("default Cache.Enabled = false, want true")
	}
	if cfg.Collector.Cache.TTL != 5*time.Minute {
		t.Errorf("default Cache.TTL = %v, want 5m", cfg.Collector.Cache.TTL)
	}
}

// boundary[0]
func TestLoad_Workers_ZeroOrNegative(t *testing.T) {
	for _, w := range []int{0, -1} {
		cfgPath := writeTempConfig(t, fmt.Sprintf("analysis:\n  workers: %d\n", w))
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatalf("workers=%d: Load returned error: %v", w, err)
		}
		if cfg.Analysis.Workers != w {
			t.Errorf("workers=%d: got %d", w, cfg.Analysis.Workers)
		}
	}
}

// boundary[1]
func TestLoad_TimeoutTTL_ZeroUsesDefault(t *testing.T) {
	cfgPath := writeTempConfig(t, `
meta:
  arbitrator:
    timeout: 0
collector:
  cache:
    ttl: 0
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Meta.Arbitrator.Timeout != 15*time.Second {
		t.Errorf("Timeout(0) = %v, want default 15s", cfg.Meta.Arbitrator.Timeout)
	}
	if cfg.Collector.Cache.TTL != 5*time.Minute {
		t.Errorf("Cache.TTL(0) = %v, want default 5m", cfg.Collector.Cache.TTL)
	}
}

// error_handling[0]
func TestLoad_InvalidDuration(t *testing.T) {
	cfgPath := writeTempConfig(t, `
meta:
  arbitrator:
    timeout: "abc"
`)
	if _, err := Load(cfgPath); err == nil {
		t.Errorf("expected error for invalid duration, got nil")
	}
}

// validConfig returns a baseline config that passes Validate.
func validConfig() Config {
	return Config{
		Server: ServerConfig{Port: 8080},
		Router: RouterConfig{MinConfidence: 0.6, CooldownHours: 4},
	}
}

func TestConfig_Validate_Branches(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"port too low", func(c *Config) { c.Server.Port = 0 }, true},
		{"port too high", func(c *Config) { c.Server.Port = 70000 }, true},
		{"min_confidence high", func(c *Config) { c.Router.MinConfidence = 1.5 }, true},
		{"cooldown negative", func(c *Config) { c.Router.CooldownHours = -1 }, true},
		{"claude ok", func(c *Config) { c.LLM.Provider = "claude"; c.LLM.Claude.APIKey = "k" }, false},
		{"claude missing key", func(c *Config) { c.LLM.Provider = "claude" }, true},
		{"openai missing key", func(c *Config) { c.LLM.Provider = "openai" }, true},
		{"openai ok", func(c *Config) { c.LLM.Provider = "openai"; c.LLM.OpenAI.APIKey = "k" }, false},
		{"ollama missing endpoint", func(c *Config) { c.LLM.Provider = "ollama" }, true},
		{"ollama ok", func(c *Config) { c.LLM.Provider = "ollama"; c.LLM.Ollama.Endpoint = "http://x" }, false},
		{"broker live not supported (paper-only)", func(c *Config) {
			c.Broker.Enabled = true
			c.Broker.Mode = "live"
		}, true},
		{"broker invalid exec mode", func(c *Config) {
			c.Broker.Enabled = true
			c.Broker.Execution.Mode = "bogus"
		}, true},
		{"broker ok", func(c *Config) {
			c.Broker.Enabled = true
			c.Broker.Mode = "paper"
			c.Broker.Execution.Mode = "confirm"
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			if err := c.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

// TestConfig_Validate_LiveNotSupported asserts an enabled broker in live mode is
// rejected as paper-only (AD-15, FutuBroker withdrawn 2026-07-02). Covers
// error_handling[1].
func TestConfig_Validate_LiveNotSupported(t *testing.T) {
	c := validConfig()
	c.Broker.Enabled = true
	c.Broker.Mode = "live"

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for live mode")
	}
	if !strings.Contains(err.Error(), "live trading not supported (paper-only)") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "live trading not supported (paper-only)")
	}
}

// Context Checkpoint: done_criteria → test mapping (Task 10 Step 1)
// functional[0] "QlibConfig 三字段经 mapstructure 正确解析" → TestLoad_QlibConfig_FromYAML

func TestLoad_QlibConfig_FromYAML(t *testing.T) {
	cfgPath := writeTempConfig(t, `
qlib:
  enabled: true
  db_path: /data/qlib_warehouse.db
  max_staleness_days: 14
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Qlib.Enabled {
		t.Errorf("Qlib.Enabled = false, want true")
	}
	if cfg.Qlib.DBPath != "/data/qlib_warehouse.db" {
		t.Errorf("Qlib.DBPath = %q, want /data/qlib_warehouse.db", cfg.Qlib.DBPath)
	}
	if cfg.Qlib.MaxStalenessDays != 14 {
		t.Errorf("Qlib.MaxStalenessDays = %d, want 14", cfg.Qlib.MaxStalenessDays)
	}
}

func TestConfig_WarnHardcodedSecrets(t *testing.T) {
	c := Config{}
	c.Server.APIKey = "plain-secret"
	c.LLM.Claude.APIKey = "${CLAUDE_KEY}" // env syntax → no warning
	c.LLM.OpenAI.APIKey = "openai-plain"

	var msgs []string
	c.WarnHardcodedSecrets(func(s string) { msgs = append(msgs, s) })

	if len(msgs) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(msgs), msgs)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "server.api_key") || !strings.Contains(joined, "llm.openai.api_key") {
		t.Errorf("missing expected fields in warnings: %v", msgs)
	}
	if strings.Contains(joined, "llm.claude.api_key") {
		t.Errorf("env-syntax secret should not warn: %v", msgs)
	}

	// No secrets → no warnings.
	var none []string
	(&Config{}).WarnHardcodedSecrets(func(s string) { none = append(none, s) })
	if len(none) != 0 {
		t.Errorf("expected no warnings for empty config, got %v", none)
	}
}

// TASK-004 fix W3: broker enabled but execution.mode omitted must default to
// "confirm" at Load time so Validate passes AND runtime Execute accepts it
// (avoids silent no-op orders). DoD: yaml 缺省 execution.mode → Load 后为 confirm 且 Validate 通过.
func TestLoad_ExecutionMode_DefaultsToConfirm(t *testing.T) {
	cfgPath := writeTempConfig(t, `
server:
  port: 8080
broker:
  enabled: true
  mode: paper
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Broker.Execution.Mode != "confirm" {
		t.Errorf("execution.mode = %q, want default %q", cfg.Broker.Execution.Mode, "confirm")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with defaulted execution.mode returned error: %v", err)
	}
}

// TestLoad_LegacyFutuSection_Ignored asserts an old config carrying a futu:
// block still loads without error after FutuConfig was removed — viper silently
// ignores the now-unmapped keys. Covers boundary[0].
func TestLoad_LegacyFutuSection_Ignored(t *testing.T) {
	cfgPath := writeTempConfig(t, `
server:
  port: 8080
broker:
  enabled: true
  mode: paper
  provider: mock
  futu:
    host: "127.0.0.1"
    port: 11111
    env: simulate
    trade_password: "secret"
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load with legacy futu section failed: %v", err)
	}
	if cfg.Broker.Provider != "mock" {
		t.Errorf("Broker.Provider = %q, want mock", cfg.Broker.Provider)
	}
}

// Context Checkpoint: done_criteria → test mapping (Task 4, phase3)
// functional[0] "YAML valuation.lookback_years:0 解析为 0" → TestLoad_ValuationConfig
// boundary[0]   "Defaults() ValuationConfig.LookbackYears == 5" → TestDefaults_ValuationLookbackIs5

// functional[0]: valuation.lookback_years: 0 in YAML parses to LookbackYears==0 (since inception).
func TestLoad_ValuationConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, `
valuation:
  lookback_years: 0
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Valuation.LookbackYears != 0 {
		t.Errorf("Valuation.LookbackYears = %d, want 0", cfg.Valuation.LookbackYears)
	}
}

// boundary[0]: Defaults() sets ValuationConfig.LookbackYears to 5 (preserve existing behaviour).
func TestDefaults_ValuationLookbackIs5(t *testing.T) {
	cfg := Defaults()
	if cfg.Valuation.LookbackYears != 5 {
		t.Errorf("Defaults Valuation.LookbackYears = %d, want 5", cfg.Valuation.LookbackYears)
	}
}

// boundary[1] (QA C1 fix): a legacy config file with NO valuation block must
// Load() with LookbackYears==5, not 0. Defaults() applies to constructed configs
// but Load() builds via viper.Unmarshal, so it needs its own SetDefault. Without
// it, serve.go SetValuationLookback(0) silently flips every pre-existing
// deployment into since-inception mode (zero-regression break).
func TestLoad_NoValuationBlockDefaultsTo5(t *testing.T) {
	cfgPath := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 8080
storage:
  cold:
    type: localfs
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Valuation.LookbackYears != 5 {
		t.Errorf("Load (no valuation block) Valuation.LookbackYears = %d, want 5", cfg.Valuation.LookbackYears)
	}
}

// Explicit execution.mode must be preserved (default only fills the gap).
func TestLoad_ExecutionMode_ExplicitPreserved(t *testing.T) {
	cfgPath := writeTempConfig(t, `
broker:
  enabled: true
  execution:
    mode: batch
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Broker.Execution.Mode != "batch" {
		t.Errorf("execution.mode = %q, want explicit %q", cfg.Broker.Execution.Mode, "batch")
	}
}

// ---------------------------------------------------------------------------
// TASK-003 config wiring: router.batch_notify
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] "Load 默认 router.batch_notify=true（未配置时）" → TestLoad_BatchNotify_DefaultTrue
// boundary[0]   "显式 batch_notify:false 可覆盖默认"            → TestLoad_BatchNotify_CanOverrideFalse
// ---------------------------------------------------------------------------

// functional[0]: batch_notify defaults to true when absent from config file.
func TestLoad_BatchNotify_DefaultTrue(t *testing.T) {
	cfgPath := writeTempConfig(t, "server:\n  port: 8080\n")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Router.BatchNotify {
		t.Errorf("Router.BatchNotify = false, want true (default)")
	}
}

// boundary[0]: explicit batch_notify: false must override the default.
func TestLoad_BatchNotify_CanOverrideFalse(t *testing.T) {
	cfgPath := writeTempConfig(t, `
router:
  batch_notify: false
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Router.BatchNotify {
		t.Errorf("Router.BatchNotify = true, want false (explicit override)")
	}
}

// Context Checkpoint: done_criteria → test mapping (TASK-302)
// functional[0] "storage.signals.backend/path 解析；缺省 sqlite + data/signals.db" → TestLoad_SignalStorage_FromYAML / TestLoad_SignalStorage_DefaultsToSqlite / TestDefaults_SignalStorage
// boundary[0]   "老配置无 storage 节 → 缺省 sqlite + data/signals.db 不报错"      → TestLoad_SignalStorage_DefaultsToSqlite
// error_handling[1] "backend 非法值校验报错，错误信息含非法值"                   → TestConfig_Validate_SignalBackendInvalid

// functional[0]: explicit storage.signals values parse from yaml.
func TestLoad_SignalStorage_FromYAML(t *testing.T) {
	cfgPath := writeTempConfig(t, `
storage:
  signals:
    backend: memory
    path: /tmp/custom-signals.db
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Storage.Signals.Backend != "memory" {
		t.Errorf("Backend = %q, want memory", cfg.Storage.Signals.Backend)
	}
	if cfg.Storage.Signals.Path != "/tmp/custom-signals.db" {
		t.Errorf("Path = %q, want /tmp/custom-signals.db", cfg.Storage.Signals.Path)
	}
}

// boundary[0]: a legacy config with no storage block loads with the sqlite
// defaults and does not error.
func TestLoad_SignalStorage_DefaultsToSqlite(t *testing.T) {
	cfgPath := writeTempConfig(t, `
server:
  port: 8080
`)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Storage.Signals.Backend != "sqlite" {
		t.Errorf("default Backend = %q, want sqlite", cfg.Storage.Signals.Backend)
	}
	if cfg.Storage.Signals.Path != "data/signals.db" {
		t.Errorf("default Path = %q, want data/signals.db", cfg.Storage.Signals.Path)
	}
}

// functional[0]: Defaults() carries the sqlite signal-store defaults.
func TestDefaults_SignalStorage(t *testing.T) {
	cfg := Defaults()
	if cfg.Storage.Signals.Backend != "sqlite" {
		t.Errorf("Defaults Backend = %q, want sqlite", cfg.Storage.Signals.Backend)
	}
	if cfg.Storage.Signals.Path != "data/signals.db" {
		t.Errorf("Defaults Path = %q, want data/signals.db", cfg.Storage.Signals.Path)
	}
}

// error_handling[1]: an invalid backend is rejected and the error names the
// offending value.
func TestConfig_Validate_SignalBackendInvalid(t *testing.T) {
	cfg := Defaults()
	cfg.Storage.Signals.Backend = "postgres"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid backend, got nil")
	}
	if !errors.Is(err, core.ErrConfigInvalid) {
		t.Errorf("error chain must include ErrConfigInvalid, got %v", err)
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("error must name the invalid value 'postgres', got %v", err)
	}
}

// functional[0]: memory and sqlite backends both pass validation.
func TestConfig_Validate_SignalBackendValid(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		cfg := Defaults()
		cfg.Storage.Signals.Backend = backend
		if err := cfg.Validate(); err != nil {
			t.Errorf("backend %q must be valid, got %v", backend, err)
		}
	}
}
