package main

// Context Checkpoint: done_criteria → test mapping (M4 的 TASK-005，五条告警规则)
// functional[0]     样例配置经 config.Load + mapRules 读出五条，name/expr/for/cooldown/severity 逐项相等
//                                            → TestHestiaAlertRules_DeclaredInExampleConfig
// functional[1]     五条 expr 经 alert.Rule.Evaluate 应触发 true / 不应触发 false，阈值取等号各一例
//                                            → TestHestiaAlertRules_ExprEvaluable
// functional[2]     端到端：failed/ 放 1 份 ⇒ hestia_queue_failed true，删掉 ⇒ false
//                                            → TestHestiaAlertRules_E2EFailedItem
// boundary[0]       四目录都空 ⇒ stuck / processing_stuck / failed 均 false → TestHestiaAlertRules_E2EEmptyQueueDoesNotFire
// boundary[1]       删 pending/ ⇒ queue_blind true，建回 ⇒ false       → TestHestiaAlertRules_E2EQueueBlindRecovers
// error_handling[0] 规则层 C2：队列缺失 ⇔ 只有 queue_blind；DB 失败 ⇔ 只有 db_blind
//                                            → TestHestiaAlertRules_E2EBlindRulesIsolated
// non_functional[0] config.example.yaml 规则段注释 → review
// non_functional[1] discovery 列出 runtime config.yaml 需追加的规则原文；go test ./... 全绿 → review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/alert"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/hestia"
	"github.com/newthinker/atlas/internal/metrics"
)

// loadExampleRules 走 serve 的同一条路径（config.Load → mapRules）读出样例配置里的规则，按名索引。
// 同名规则会共享 evaluator 的 pending/lastFired 状态，所以这里顺带断言名字不重复。
func loadExampleRules(t *testing.T) map[string]alert.Rule {
	t.Helper()
	cfg, err := config.Load("../../configs/config.example.yaml")
	require.NoError(t, err)
	out := map[string]alert.Rule{}
	for _, r := range mapRules(cfg.Alerts.Rules) {
		_, dup := out[r.Name]
		require.False(t, dup, "规则名重复：%s", r.Name)
		out[r.Name] = r
	}
	return out
}

// evalRule 取出具名规则并对 m 求值；规则缺席直接 Fatal，免得「找不到规则」被读成 false。
func evalRule(t *testing.T, rules map[string]alert.Rule, name string, m map[string]float64) bool {
	t.Helper()
	r, ok := rules[name]
	require.True(t, ok, "样例配置缺少规则 %s", name)
	return r.Evaluate(m)
}

func TestHestiaAlertRules_DeclaredInExampleConfig(t *testing.T) {
	rules := loadExampleRules(t)
	tests := []struct {
		name, expr, severity string
		forDur, cooldown     time.Duration
	}{
		{"hestia_queue_stuck", "hestia_queue_pending_age_hours > 24", "warning", 10 * time.Minute, 24 * time.Hour},
		{"hestia_queue_failed", "hestia_queue_items_failed > 0", "warning", 10 * time.Minute, 24 * time.Hour},
		{"hestia_queue_processing_stuck", "hestia_queue_processing_age_hours > 0.5", "warning", 10 * time.Minute, 6 * time.Hour},
		{"hestia_db_blind", "hestia_db_up == 0", "critical", 10 * time.Minute, 6 * time.Hour},
		{"hestia_queue_blind", "hestia_queue_up == 0", "critical", 10 * time.Minute, 6 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := rules[tt.name]
			require.True(t, ok, "样例配置缺少规则 %s", tt.name)
			assert.Equal(t, tt.expr, r.Expr)
			assert.Equal(t, tt.forDur, r.For)
			assert.Equal(t, tt.cooldown, r.Cooldown)
			assert.Equal(t, tt.severity, r.Severity)
			assert.NotEmpty(t, r.Message)
		})
	}
}

// 求值器只认 `^(\w+) op 数字$`，不认 label 选择器与 or：写了它不认的语法，Evaluate 恒 false，
// 「应触发」那一例就会红——这是「先确认引擎表达式能力再写」的机制化版本。
func TestHestiaAlertRules_ExprEvaluable(t *testing.T) {
	rules := loadExampleRules(t)
	tests := []struct {
		desc, rule string
		metrics    map[string]float64
		want       bool
	}{
		{"pending 等了 25h", "hestia_queue_stuck", map[string]float64{"hestia_queue_pending_age_hours": 25}, true},
		{"pending 恰 24h（等号不触发）", "hestia_queue_stuck", map[string]float64{"hestia_queue_pending_age_hours": 24}, false},
		{"failed 1 件", "hestia_queue_failed", map[string]float64{"hestia_queue_items_failed": 1}, true},
		{"failed 0 件（等号不触发）", "hestia_queue_failed", map[string]float64{"hestia_queue_items_failed": 0}, false},
		{"processing 卡 0.6h", "hestia_queue_processing_stuck", map[string]float64{"hestia_queue_processing_age_hours": 0.6}, true},
		{"processing 恰 0.5h（等号不触发）", "hestia_queue_processing_stuck", map[string]float64{"hestia_queue_processing_age_hours": 0.5}, false},
		{"db 读失败", "hestia_db_blind", map[string]float64{"hestia_db_up": 0}, true},
		{"db 读成功", "hestia_db_blind", map[string]float64{"hestia_db_up": 1}, false},
		{"队列读失败", "hestia_queue_blind", map[string]float64{"hestia_queue_up": 0}, true},
		{"队列读成功", "hestia_queue_blind", map[string]float64{"hestia_queue_up": 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.rule+"/"+tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.want, evalRule(t, rules, tt.rule, tt.metrics))
		})
	}
}

func okHealth(context.Context) (hestia.Health, error) { return hestia.Health{}, nil }

func failingHealth(context.Context) (hestia.Health, error) {
	return hestia.Health{}, errors.New("db locked")
}

// queueRegistry 把真实 HestiaCollector（队列侧现读 QueueHealthOf(dir)）注册进 metrics.Registry。
func queueRegistry(t *testing.T, fetch metrics.HealthFunc, dir string) *metrics.Registry {
	t.Helper()
	reg := metrics.NewRegistry()
	reg.MustRegister(metrics.NewHestiaCollector(fetch, func() (hestia.QueueHealth, error) {
		return hestia.QueueHealthOf(dir)
	}, time.Now))
	return reg
}

// newQueueDir 建好四个子目录的临时队列根。
func newQueueDir(t *testing.T) string {
	t.Helper()
	q := t.TempDir()
	require.NoError(t, hestia.EnsureQueueDirs(q))
	return q
}

func TestHestiaAlertRules_E2EFailedItem(t *testing.T) {
	rules := loadExampleRules(t)
	q := newQueueDir(t)
	failed := filepath.Join(q, "failed", "contract.json")
	require.NoError(t, os.WriteFile(failed, []byte("{}"), 0o644))
	reg := queueRegistry(t, okHealth, q)

	assert.True(t, evalRule(t, rules, "hestia_queue_failed", reg.Snapshot()), "failed/ 有 1 件应触发")

	require.NoError(t, os.Remove(failed))
	assert.False(t, evalRule(t, rules, "hestia_queue_failed", reg.Snapshot()), "failed/ 清空后应熄灭")
}

// 空队列：两个 age 指标缺席 ⇒ 求值器找不到键 ⇒ false；先断 queue_up==1，证明不是「队列根本没读到」。
func TestHestiaAlertRules_E2EEmptyQueueDoesNotFire(t *testing.T) {
	rules := loadExampleRules(t)
	snap := queueRegistry(t, okHealth, newQueueDir(t)).Snapshot()

	require.Equal(t, 1.0, snap["hestia_queue_up"])
	for _, name := range []string{"hestia_queue_stuck", "hestia_queue_processing_stuck", "hestia_queue_failed"} {
		assert.False(t, evalRule(t, rules, name, snap), "%s 在空队列上不应触发", name)
	}
}

// blind 规则基于 up gauge：目录删掉后亮、建回后下一轮就熄（累计计数器做不到）。
func TestHestiaAlertRules_E2EQueueBlindRecovers(t *testing.T) {
	rules := loadExampleRules(t)
	q := newQueueDir(t)
	reg := queueRegistry(t, okHealth, q)

	require.NoError(t, os.RemoveAll(filepath.Join(q, "pending")))
	assert.True(t, evalRule(t, rules, "hestia_queue_blind", reg.Snapshot()), "pending/ 缺失应触发")

	require.NoError(t, hestia.EnsureQueueDirs(q))
	assert.False(t, evalRule(t, rules, "hestia_queue_blind", reg.Snapshot()), "建回后应熄灭")
}

// 规则层 C2：一侧读不到只点亮自己那条 blind 规则。
func TestHestiaAlertRules_E2EBlindRulesIsolated(t *testing.T) {
	rules := loadExampleRules(t)

	snap := queueRegistry(t, okHealth, filepath.Join(t.TempDir(), "no-such-queue")).Snapshot()
	assert.True(t, evalRule(t, rules, "hestia_queue_blind", snap), "队列缺失：queue_blind 应触发")
	assert.False(t, evalRule(t, rules, "hestia_db_blind", snap), "队列缺失：db_blind 不应触发")

	snap = queueRegistry(t, failingHealth, newQueueDir(t)).Snapshot()
	assert.True(t, evalRule(t, rules, "hestia_db_blind", snap), "DB 失败：db_blind 应触发")
	assert.False(t, evalRule(t, rules, "hestia_queue_blind", snap), "DB 失败：queue_blind 不应触发")
}
