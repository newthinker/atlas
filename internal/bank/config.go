package bank

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// Config 对应 configs/bank-monitor.yaml（不含密钥；Telegram 凭据走主配置）。
type Config struct {
	Source     SourceCfg     `mapstructure:"source"`
	RankBy     string        `mapstructure:"rank_by"`
	Banks      []BankCfg     `mapstructure:"banks"`
	Thresholds ThresholdsCfg `mapstructure:"thresholds"`
}

type SourceCfg struct {
	AktoolsURL string `mapstructure:"aktools_url"`
}

type BankCfg struct {
	Market    string `mapstructure:"market"`
	Symbol    string `mapstructure:"symbol"`
	Name      string `mapstructure:"name"`
	AShareRef string `mapstructure:"a_share_ref"`
}

type ThresholdsCfg struct {
	NPLMax        float64          `mapstructure:"npl_max"`
	CoverageMin   float64          `mapstructure:"coverage_min"`
	CET1Min       float64          `mapstructure:"cet1_min"`
	Deterioration DeteriorationCfg `mapstructure:"deterioration"`
}

// DeteriorationCfg 是环比恶化幅度上限，单位百分点。
type DeteriorationCfg struct {
	NPLUp        float64 `mapstructure:"npl_up"`
	CoverageDown float64 `mapstructure:"coverage_down"`
	CET1Down     float64 `mapstructure:"cet1_down"`
}

const defaultAktoolsURL = "http://127.0.0.1:8180"

var (
	reAShare = regexp.MustCompile(`^\d{6}\.(SH|SZ)$`)
	reHK     = regexp.MustCompile(`^\d{4,5}\.HK$`)
	rankKeys = map[string]Indicator{"npl": IndNPL, "coverage": IndCoverage, "cet1": IndCET1}
)

// DataSymbol 是实际取数的 A 股代码：港股条目取 a_share_ref（一期仅支持 A+H）。
func (b BankCfg) DataSymbol() string {
	if b.Market == "HK" {
		return b.AShareRef
	}
	return b.Symbol
}

func (b BankCfg) DisplayName() string {
	if b.Name != "" {
		return b.Name
	}
	return b.Symbol
}

// dedupKey 是判重用的代码：港股去前导零，使 3968.HK 与 03968.HK 视为同一只。
func (b BankCfg) dedupKey() string {
	if b.Market == "HK" {
		return strings.TrimLeft(b.Symbol, "0")
	}
	return b.Symbol
}

// RankIndicator 返回排名依据；LoadConfig 已保证 RankBy 合法。
func (c *Config) RankIndicator() Indicator { return rankKeys[c.RankBy] }

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading bank config: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing bank config: %w", err)
	}
	if cfg.Source.AktoolsURL == "" {
		cfg.Source.AktoolsURL = defaultAktoolsURL
	}
	if cfg.RankBy == "" {
		cfg.RankBy = "npl"
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid bank config %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if _, ok := rankKeys[c.RankBy]; !ok {
		return fmt.Errorf("rank_by %q 非法（npl | coverage | cet1）", c.RankBy)
	}
	if len(c.Banks) == 0 {
		return fmt.Errorf("banks 不能为空")
	}
	seen := map[string]bool{}
	for i, b := range c.Banks {
		if seen[b.dedupKey()] {
			return fmt.Errorf("banks[%d]: symbol %s 重复", i, b.Symbol)
		}
		seen[b.dedupKey()] = true
		switch b.Market {
		case "CN_A":
			if !reAShare.MatchString(b.Symbol) {
				return fmt.Errorf("banks[%d]: A 股代码 %q 格式应为 600036.SH", i, b.Symbol)
			}
		case "HK":
			if !reHK.MatchString(b.Symbol) {
				return fmt.Errorf("banks[%d]: 港股代码 %q 格式应为 3968.HK", i, b.Symbol)
			}
			if !reAShare.MatchString(b.AShareRef) {
				return fmt.Errorf("banks[%d]: 港股 %s 须填 a_share_ref（对应 A 股代码，如 600036.SH）；纯港股银行一期不支持", i, b.Symbol)
			}
		default:
			return fmt.Errorf("banks[%d]: market %q 仅支持 CN_A / HK", i, b.Market)
		}
	}
	t, d := c.Thresholds, c.Thresholds.Deterioration
	for _, f := range []struct {
		name string
		v    float64
	}{
		{"npl_max", t.NPLMax}, {"coverage_min", t.CoverageMin}, {"cet1_min", t.CET1Min},
		{"deterioration.npl_up", d.NPLUp}, {"deterioration.coverage_down", d.CoverageDown},
		{"deterioration.cet1_down", d.CET1Down},
	} {
		// 写成 !(v > 0) 而非 v <= 0：后者对 NaN 为 false，会放过 `.nan`。
		if !(f.v > 0) {
			return fmt.Errorf("thresholds.%s 须 > 0", f.name)
		}
	}
	return nil
}
