package main

// Context Checkpoint: done_criteria → test mapping (M1.5 的 TASK-006)
// functional[0]/[1] reg nil 或 config_path 未设 ⇒ 跳过 + 恰一条 "hestia health disabled" 日志
//                                            → TestBuildHestiaHealth_DisabledWhenUnset、TestBuildHestiaHealth_RegistersCollector（reg nil 子段）
// functional[1]     设了但装不上 ⇒ 错误带路径、以 "hestia health:" 开头（error_handling[0]）
//                                            → TestBuildHestiaHealth_FailsLoudlyWhenUnloadable（loading）
//                                              TestBuildHestiaHealth_FailsLoudlyWhenStoreUnopenable（opening）
// functional[1]     正常 ⇒ collector 已注册：pending_review / runs_total 可见，空库 last_run_timestamp 不可见
//                                            → TestBuildHestiaHealth_RegistersCollector
// functional[2]/boundary[0] 样例配置整份可装载，两条 hestia 规则与 hestia.config_path 就位
//                                            → TestExampleConfigDeclaresHestiaRules
// error_handling[0] 红阶段 undefined: buildHestiaHealth → discovery verification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/metrics"
)

// writeHestiaYAML 写一份能过 hestia.LoadConfig 校验的最小配置，db 落在临时目录。
func writeHestiaYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return writeHestiaYAMLWithDB(t, dir, filepath.Join(dir, "hestia.db"))
}

// writeHestiaYAMLWithDB 同上，但 db_path 由调用方给定（用于构造「装得上但库打不开」的场景）。
func writeHestiaYAMLWithDB(t *testing.T, dir, dbPath string) string {
	t.Helper()
	p := filepath.Join(dir, "hestia.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`
storage:
  db_path: `+dbPath+`
  snapshot_dir: `+filepath.Join(dir, "snap")+`
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`), 0o644))
	return p
}

func gatheredNames(t *testing.T, reg *metrics.Registry) map[string]bool {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	return names
}

// 未设 config_path ⇒ 不注册、不报错，且日志恰一行（spec §8：不注册**且日志一行**——
// zap.NewNop() 断不到这一行，用 observer）。
func TestBuildHestiaHealth_DisabledWhenUnset(t *testing.T) {
	reg := metrics.NewRegistry()
	core, logs := observer.New(zapcore.InfoLevel)
	cleanup, err := buildHestiaHealth(&config.Config{}, reg, zap.New(core))
	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.False(t, gatheredNames(t, reg)["hestia_pending_review"], "未设 config_path 不该注册 collector")

	disabled := logs.FilterMessageSnippet("hestia health disabled").All()
	require.Len(t, disabled, 1, "未设 config_path 要恰有一条 disabled 日志，got %d（全部日志 %d 条）", len(disabled), logs.Len())
	assert.Contains(t, disabled[0].Message, "hestia.config_path not set")
	assert.Equal(t, 1, logs.Len(), "跳过路径只该打这一行")
}

// 设了但装不上 ⇒ 返回错误且带路径（serve 启动失败）。静默变成「没有健康度」正是要消灭的形态。
// 错误串以 "hestia health:" 开头：运维 grep err.log 的约定（需求 TASK-009 Step 2 依赖它）。
func TestBuildHestiaHealth_FailsLoudlyWhenUnloadable(t *testing.T) {
	reg := metrics.NewRegistry()
	bad := filepath.Join(t.TempDir(), "nope.yaml")
	_, err := buildHestiaHealth(&config.Config{Hestia: config.HestiaConfig{ConfigPath: bad}}, reg, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope.yaml")
	assert.True(t, strings.HasPrefix(err.Error(), "hestia health:"), "错误串必须以 hestia health: 开头，got %q", err.Error())
}

// hestia.yaml 装得上但库打不开（db_path 的父路径是个普通文件，NewStore 建目录失败）⇒
// 同样响亮失败，错误串以 hestia health: opening 开头并带 db 路径。
func TestBuildHestiaHealth_FailsLoudlyWhenStoreUnopenable(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	dbPath := filepath.Join(blocker, "hestia.db")
	p := writeHestiaYAMLWithDB(t, dir, dbPath)

	_, err := buildHestiaHealth(&config.Config{Hestia: config.HestiaConfig{ConfigPath: p}}, metrics.NewRegistry(), zap.NewNop())
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "hestia health: opening "), "got %q", err.Error())
	assert.Contains(t, err.Error(), dbPath)
}

// 正常 ⇒ collector 已注册，抓取能看到 hestia_pending_review；metrics 关掉（reg nil）⇒ 跳过。
func TestBuildHestiaHealth_RegistersCollector(t *testing.T) {
	reg := metrics.NewRegistry()
	cfg := &config.Config{Hestia: config.HestiaConfig{ConfigPath: writeHestiaYAML(t)}}
	cleanup, err := buildHestiaHealth(cfg, reg, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	names := gatheredNames(t, reg)
	assert.True(t, names["hestia_pending_review"])
	assert.True(t, names["hestia_runs_total"])
	assert.False(t, names["hestia_last_run_timestamp"], "空库不输出时间戳")

	core, logs := observer.New(zapcore.InfoLevel)
	cleanupNil, err := buildHestiaHealth(cfg, nil, zap.New(core))
	require.NoError(t, err, "metrics 未启用时跳过，不是错误")
	cleanupNil()
	disabled := logs.FilterMessageSnippet("hestia health disabled").All()
	require.Len(t, disabled, 1)
	assert.Contains(t, disabled[0].Message, "metrics disabled")
}

// 样例配置必须真能装载（本仓库此前没有任何测试加载 config.example.yaml）：
// hestia.config_path 与两条 hestia 规则（含 cooldown: 24h）都要从整份 yaml 解码出来。
func TestExampleConfigDeclaresHestiaRules(t *testing.T) {
	cfg, err := config.Load("../../configs/config.example.yaml")
	require.NoError(t, err)
	assert.Equal(t, "configs/hestia.yaml", cfg.Hestia.ConfigPath)

	want := map[string]string{"hestia_stalled": "critical", "hestia_no_ingest": "warning"}
	found := map[string]bool{}
	for _, r := range cfg.Alerts.Rules {
		sev, ok := want[r.Name]
		if !ok {
			continue
		}
		found[r.Name] = true
		assert.Equal(t, sev, r.Severity, "%s severity", r.Name)
		assert.Equal(t, 24*time.Hour, r.Cooldown, "%s cooldown", r.Name)
	}
	for name := range want {
		assert.True(t, found[name], "样例配置缺少规则 %s", name)
	}
}
