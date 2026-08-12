package hestia

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Config 是 configs/hestia.yaml 的内存形态。
//
// 独立文件、独立装载器，照 internal/crisis/config.go 的先例，不并进
// internal/config 的大 Config：方案报告 5.3.1 定的「单一读者原则」——
// 这个文件只有 Atlas 读，Loom 从契约 JSON 的 thresholds 段取值。
type Config struct {
	// ConfigVersion 不参与逻辑，只是让「这期用的是哪版配置」在契约里一眼可见
	// （方案报告 5.3.2）。改配置时手工递增，用日期串。
	ConfigVersion string      `mapstructure:"config_version"`
	Discover      DiscoverCfg `mapstructure:"discover"`
	Thresholds    Thresholds  `mapstructure:"thresholds"`
}

// LoadConfig 读配置文件并立即校验。
//
// 预填 DefaultThresholds 再 Unmarshal —— mapstructure 只覆盖 YAML 里出现的键，
// 没写的保持预填值。这一步不能省：deposit_sum_tolerance 静默变成 0 会让每一期
// 都因残差超容差进 pending（实测残差本就有 7.6–9.1%），整条管线停摆，
// 而现场看起来像「数据全都有问题」。
func LoadConfig(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("hestia: reading config %s: %w", path, err)
	}

	cfg := Config{Thresholds: DefaultThresholds()}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("hestia: parsing config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("hestia: invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// validate 检查配置自洽。装载即校验，不给「先读进来再说」留口子。
func (c Config) validate() error {
	switch {
	case c.Discover.IndexURL == "":
		return errors.New("discover.index_url is required")
	case c.Discover.MaxPages < 1:
		return errors.New("discover.max_pages must be >= 1")
	case c.Discover.Timeout <= 0:
		return errors.New("discover.timeout must be > 0")
	}

	// 第二道防线：预填只挡住「没写」，**挡不住「写了 0」**。有人显式写
	// deposit_sum_tolerance: 0 时，Unmarshal 会如实覆盖掉预填的默认值，
	// 而 0 容差让每一期都超差进 pending —— 与漏写的后果完全一样。
	//
	// MagnitudeRanges **不在此检查** —— 它有意为空（区间要等 M1c 用回填分布标定，
	// 表为空时 magnitude_sanity 记 skipped{not_calibrated}）。把「非空」当成
	// 合法性要求会让默认配置自己就装载不了。
	t := c.Thresholds
	switch {
	case t.DepositSumTolerance <= 0:
		return errors.New("thresholds.deposit_sum_tolerance must be > 0")
	case t.DepositSumDriftMax <= 0:
		return errors.New("thresholds.deposit_sum_drift_max must be > 0")
	case t.CorpLoanTolerance <= 0:
		return errors.New("thresholds.corp_loan_tolerance must be > 0")
	case t.StockContinuityMax <= 0:
		return errors.New("thresholds.stock_continuity_max must be > 0")
	case t.YoYSanityMax <= 0:
		return errors.New("thresholds.yoy_sanity_max must be > 0")
	}
	return t.validate() // 豁免校验（含 T6 补的 PeriodTypes 必填非空）
}
