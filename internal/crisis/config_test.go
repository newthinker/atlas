package crisis

// Context Checkpoint: done_criteria → test mapping (config)
// functional[0] LoadConfig 返回 typed *Config，字段覆盖 Storage/FRED/Freshness/Percentile/7 指标/StateMachine 且数值与 YAML 一致 → TestLoadConfigFromRepoFile
// functional[1] configs/crisis-monitor.yaml 原样落地（冒烟直接加载正式配置） → TestLoadConfigFromRepoFile
// boundary[0]   关键字段缺失（storage.path）报错；文件不存在返回错误 → TestLoadConfigValidation
// error_handling[0] 非法 YAML 返回包装错误 → TestLoadConfigBadYAML

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 冒烟测试直接加载仓库内的正式配置，保证 yaml 与 struct 永不脱节。
func TestLoadConfigFromRepoFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "crisis-monitor.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "data/crisis.db", cfg.Storage.Path)
	assert.Equal(t, "FRED_API_KEY", cfg.FRED.APIKeyEnv)
	assert.Equal(t, 4, cfg.Freshness.DailyMaxLagDays)
	assert.Equal(t, 12, cfg.Freshness.WeeklyMaxLagDays)
	assert.Equal(t, 5, cfg.Percentile.WindowYears)
	assert.Equal(t, 0.90, cfg.Percentile.Amber)
	assert.Equal(t, 0.97, cfg.Percentile.Red)

	assert.Equal(t, 30.0, cfg.Indicators.VIX.Red)
	assert.Equal(t, 0.50, cfg.Indicators.VIX.WeeklySpikePct)
	assert.Equal(t, 120.0, cfg.Indicators.MOVE.Red)
	assert.Equal(t, 3, cfg.Indicators.SOFREFFR.AmberPersistDays)
	assert.Equal(t, 5, cfg.Indicators.SOFREFFR.RedPersistDays)
	assert.True(t, cfg.Indicators.SOFREFFR.SuppressQuarterEnd)
	assert.Equal(t, 350.0, cfg.Indicators.HYOAS.AmberLowBp)
	assert.Equal(t, 600.0, cfg.Indicators.HYOAS.RedBp)
	assert.Equal(t, 21, cfg.Indicators.HYOAS.MomentumWindowObs)
	assert.Equal(t, 250, cfg.Indicators.T10Y2Y.SteepeningLookbackObs)
	assert.False(t, cfg.Indicators.T10Y2Y.PercentileTrack)
	assert.Equal(t, -0.3, cfg.Indicators.NFCI.GreenBelow)
	assert.Equal(t, -0.02, cfg.Indicators.USDJPY.AmberWowPct)
	assert.Equal(t, 0.98, cfg.Indicators.USDJPY.Crowded52wPct)

	assert.Equal(t, 3, cfg.StateMachine.WatchAmberCount)
	assert.Equal(t, 10, cfg.StateMachine.CrisisExitDays)
	assert.Equal(t, 20, cfg.StateMachine.WatchExitDays)
	assert.Equal(t, 10, cfg.StateMachine.BrewingExitDays)
	assert.Equal(t, 3, cfg.StateMachine.DemoteHysteresisDays)
}

func TestLoadConfigValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(p, []byte("fred:\n  api_key_env: X\n"), 0o644))
	_, err := LoadConfig(p)
	require.ErrorContains(t, err, "storage.path")

	_, err = LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)
}

// TestLoadConfigBadYAML 覆盖 error_handling[0]：非法 YAML 语法返回包装错误。
func TestLoadConfigBadYAML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "malformed.yaml")
	require.NoError(t, os.WriteFile(p, []byte("storage:\n  path: [unterminated\n"), 0o644))
	_, err := LoadConfig(p)
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading crisis config")
}

// TestConfigValidateBranches 覆盖 boundary：validate 逐条必填/正数约束。
// 从一份合法基线出发，每次只破坏一个字段，断言命中对应错误。
func TestConfigValidateBranches(t *testing.T) {
	valid := func() Config {
		return Config{
			Storage:      StorageCfg{Path: "data/crisis.db"},
			FRED:         FREDCfg{APIKeyEnv: "FRED_API_KEY"},
			Freshness:    FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
			Percentile:   PercentileCfg{WindowYears: 5},
			StateMachine: StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
		}
	}
	base := valid()
	require.NoError(t, base.validate())

	tests := []struct {
		name   string
		mutate func(*Config)
		errSub string
	}{
		{"missing storage.path", func(c *Config) { c.Storage.Path = "" }, "storage.path"},
		{"missing api_key_env", func(c *Config) { c.FRED.APIKeyEnv = "" }, "fred.api_key_env"},
		{"bad window_years", func(c *Config) { c.Percentile.WindowYears = 0 }, "window_years"},
		{"bad daily lag", func(c *Config) { c.Freshness.DailyMaxLagDays = 0 }, "freshness lag"},
		{"bad weekly lag", func(c *Config) { c.Freshness.WeeklyMaxLagDays = 0 }, "freshness lag"},
		{"bad watch_amber_count", func(c *Config) { c.StateMachine.WatchAmberCount = 0 }, "watch_amber_count"},
		{"bad demote_hysteresis", func(c *Config) { c.StateMachine.DemoteHysteresisDays = 0 }, "demote_hysteresis_days"},
		{"bad crisis exit", func(c *Config) { c.StateMachine.CrisisExitDays = 0 }, "exit days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.mutate(&c)
			err := c.validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.errSub)
		})
	}
}
