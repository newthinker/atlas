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
//
// Context Checkpoint: done_criteria → test mapping (M4 的 TASK-004，队列接线)
// functional[0]     queue.dir 绝对路径 ⇒ nil error、queue_up==1、hestia_queue_items 键在
//                                            → TestBuildHestiaHealth_WiresQueueDir
// functional[1]     queue.dir 相对路径 ⇒ 按进程 cwd 解析            → TestBuildHestiaHealth_QueueDirRelativeToCwd
// boundary[0]       既有启动语义不变、既有测试函数体零改动            → TestBuildHestiaHealth_SkippedPathsEmitNoHestiaMetrics + 既有测试
// boundary[1]       不存在「queue.dir 为空」分支                    → review（config.go:257 拒绝空值）
// error_handling[0] queue.dir 不存在 ⇒ nil error、queue_up==0、db_up==1 → TestBuildHestiaHealth_MissingQueueDirDoesNotFailStartup
// error_handling[1] 每轮现读：删 pending ⇒ 0，建回 ⇒ 1               → TestBuildHestiaHealth_QueueReadEveryScrape

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
	"github.com/newthinker/atlas/internal/hestia"
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

// writeHestiaYAMLWithQueue 同 writeHestiaYAML，但追加显式的 queue.dir（绝对或相对均可）。
func writeHestiaYAMLWithQueue(t *testing.T, queueDir string) string {
	t.Helper()
	p := writeHestiaYAML(t)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("queue:\n  dir: " + queueDir + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return p
}

func buildWithQueue(t *testing.T, queueDir string) *metrics.Registry {
	t.Helper()
	reg := metrics.NewRegistry()
	cfg := &config.Config{Hestia: config.HestiaConfig{ConfigPath: writeHestiaYAMLWithQueue(t, queueDir)}}
	cleanup, err := buildHestiaHealth(cfg, reg, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return reg
}

func TestBuildHestiaHealth_WiresQueueDir(t *testing.T) {
	q := t.TempDir()
	require.NoError(t, hestia.EnsureQueueDirs(q))

	snap := buildWithQueue(t, q).Snapshot()
	assert.Equal(t, 1.0, snap["hestia_queue_up"])
	_, ok := snap["hestia_queue_items"]
	assert.True(t, ok, "hestia_queue_items 键应存在")
}

// 配置默认值 queue/hestia 是相对路径，相对**进程 cwd** 解析（与 hestia-ingest 同约定）。
func TestBuildHestiaHealth_QueueDirRelativeToCwd(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, hestia.EnsureQueueDirs(filepath.Join(root, "queue", "hestia")))
	t.Chdir(root)

	assert.Equal(t, 1.0, buildWithQueue(t, "queue/hestia").Snapshot()["hestia_queue_up"])
}

// 跳过路径不输出任何 hestia_ 指标（reg nil 时根本没有注册表可看，只断 nil error）。
func TestBuildHestiaHealth_SkippedPathsEmitNoHestiaMetrics(t *testing.T) {
	reg := metrics.NewRegistry()
	cleanup, err := buildHestiaHealth(&config.Config{}, reg, zap.NewNop())
	require.NoError(t, err)
	cleanup()
	for k := range reg.Snapshot() {
		assert.False(t, strings.HasPrefix(k, "hestia_"), "未设 config_path 却输出了 %s", k)
	}

	cfg := &config.Config{Hestia: config.HestiaConfig{ConfigPath: writeHestiaYAML(t)}}
	cleanup, err = buildHestiaHealth(cfg, nil, zap.NewNop())
	require.NoError(t, err)
	cleanup()
}

// 队列目录缺失是运行期可告警事实（C3），不是启动失败——与「库打不开 ⇒ 启动失败」刻意不同。
func TestBuildHestiaHealth_MissingQueueDirDoesNotFailStartup(t *testing.T) {
	snap := buildWithQueue(t, filepath.Join(t.TempDir(), "no-such-queue")).Snapshot()
	_, ok := snap["hestia_queue_up"]
	assert.True(t, ok, "queue_up 必须输出 0，而不是缺席")
	assert.Equal(t, 0.0, snap["hestia_queue_up"])
	assert.Equal(t, 1.0, snap["hestia_db_up"], "队列读不到不得牵连 DB 指标")
}

// 每轮现读、不在启动时快照：删掉 pending/ 下一轮就是 0，建回下一轮就是 1。
func TestBuildHestiaHealth_QueueReadEveryScrape(t *testing.T) {
	q := t.TempDir()
	require.NoError(t, hestia.EnsureQueueDirs(q))
	reg := buildWithQueue(t, q)
	require.Equal(t, 1.0, reg.Snapshot()["hestia_queue_up"])

	require.NoError(t, os.RemoveAll(filepath.Join(q, "pending")))
	snap := reg.Snapshot()
	_, ok := snap["hestia_queue_up"]
	require.True(t, ok)
	assert.Equal(t, 0.0, snap["hestia_queue_up"])

	require.NoError(t, hestia.EnsureQueueDirs(q))
	assert.Equal(t, 1.0, reg.Snapshot()["hestia_queue_up"])
}
