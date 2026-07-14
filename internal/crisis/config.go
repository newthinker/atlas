package crisis

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config mirrors configs/crisis-monitor.yaml. Typed per indicator rather than
// a generic threshold DSL because the seven rule shapes differ (persistence,
// two-sided amber, momentum, steepening); every number stays in YAML so
// tuning never needs a release (design §4.1).
type Config struct {
	Storage      StorageCfg      `mapstructure:"storage"`
	FRED         FREDCfg         `mapstructure:"fred"`
	Freshness    FreshnessCfg    `mapstructure:"freshness"`
	Percentile   PercentileCfg   `mapstructure:"percentile"`
	Indicators   IndicatorsCfg   `mapstructure:"indicators"`
	StateMachine StateMachineCfg `mapstructure:"state_machine"`
}

type StorageCfg struct {
	Path string `mapstructure:"path"`
}

type FREDCfg struct {
	APIKeyEnv string `mapstructure:"api_key_env"`
}

type FreshnessCfg struct {
	DailyMaxLagDays  int `mapstructure:"daily_max_lag_days"`
	WeeklyMaxLagDays int `mapstructure:"weekly_max_lag_days"`
}

type PercentileCfg struct {
	WindowYears int     `mapstructure:"window_years"`
	Amber       float64 `mapstructure:"amber"`
	Red         float64 `mapstructure:"red"`
}

type IndicatorsCfg struct {
	VIX      VIXCfg      `mapstructure:"vix"`
	MOVE     MOVECfg     `mapstructure:"move"`
	SOFREFFR SOFREFFRCfg `mapstructure:"sofr_effr"`
	HYOAS    HYOASCfg    `mapstructure:"hy_oas"`
	T10Y2Y   T10Y2YCfg   `mapstructure:"t10y2y"`
	NFCI     NFCICfg     `mapstructure:"nfci"`
	USDJPY   USDJPYCfg   `mapstructure:"usdjpy"`
}

type VIXCfg struct {
	Series          string  `mapstructure:"series"`
	Amber           float64 `mapstructure:"amber"`
	Red             float64 `mapstructure:"red"`
	WeeklySpikePct  float64 `mapstructure:"weekly_spike_pct"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type MOVECfg struct {
	Series          string  `mapstructure:"series"`
	Amber           float64 `mapstructure:"amber"`
	Red             float64 `mapstructure:"red"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type SOFREFFRCfg struct {
	AmberBp            float64 `mapstructure:"amber_bp"`
	AmberPersistDays   int     `mapstructure:"amber_persist_days"`
	RedBp              float64 `mapstructure:"red_bp"`
	RedPersistDays     int     `mapstructure:"red_persist_days"`
	PercentileTrack    bool    `mapstructure:"percentile_track"`
	SuppressQuarterEnd bool    `mapstructure:"suppress_quarter_end"`
}

type HYOASCfg struct {
	Series            string  `mapstructure:"series"`
	AmberLowBp        float64 `mapstructure:"amber_low_bp"`
	AmberHighBp       float64 `mapstructure:"amber_high_bp"`
	RedBp             float64 `mapstructure:"red_bp"`
	MomentumBp        float64 `mapstructure:"momentum_bp"`
	MomentumWindowObs int     `mapstructure:"momentum_window_obs"`
	PercentileTrack   bool    `mapstructure:"percentile_track"`
}

type T10Y2YCfg struct {
	Series                string  `mapstructure:"series"`
	AmberBp               float64 `mapstructure:"amber_bp"`
	SteepeningBp          float64 `mapstructure:"steepening_bp"`
	SteepeningLookbackObs int     `mapstructure:"steepening_lookback_obs"`
	PercentileTrack       bool    `mapstructure:"percentile_track"`
}

type NFCICfg struct {
	Series          string  `mapstructure:"series"`
	GreenBelow      float64 `mapstructure:"green_below"`
	RedAbove        float64 `mapstructure:"red_above"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type USDJPYCfg struct {
	AmberWowPct     float64 `mapstructure:"amber_wow_pct"`
	RedWowPct       float64 `mapstructure:"red_wow_pct"`
	Crowded52wPct   float64 `mapstructure:"crowded_52w_pct"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type StateMachineCfg struct {
	WatchAmberCount      int `mapstructure:"watch_amber_count"`
	CrisisExitDays       int `mapstructure:"crisis_exit_days"`
	WatchExitDays        int `mapstructure:"watch_exit_days"`
	BrewingExitDays      int `mapstructure:"brewing_exit_days"`
	DemoteHysteresisDays int `mapstructure:"demote_hysteresis_days"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading crisis config: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing crisis config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid crisis config %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.Storage.Path == "":
		return fmt.Errorf("storage.path is required")
	case c.FRED.APIKeyEnv == "":
		return fmt.Errorf("fred.api_key_env is required")
	case c.Percentile.WindowYears <= 0:
		return fmt.Errorf("percentile.window_years must be > 0")
	case c.Freshness.DailyMaxLagDays <= 0 || c.Freshness.WeeklyMaxLagDays <= 0:
		return fmt.Errorf("freshness lag days must be > 0")
	case c.StateMachine.WatchAmberCount < 1:
		return fmt.Errorf("state_machine.watch_amber_count must be >= 1")
	case c.StateMachine.DemoteHysteresisDays < 1:
		return fmt.Errorf("state_machine.demote_hysteresis_days must be >= 1")
	case c.StateMachine.CrisisExitDays < 1 || c.StateMachine.WatchExitDays < 1 || c.StateMachine.BrewingExitDays < 1:
		return fmt.Errorf("state_machine exit days must be >= 1")
	}
	return nil
}
