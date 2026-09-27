# 银行股关键指标月度监控 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `atlas bank report`：按配置列表拉取 A 股 / A+H 银行的不良贷款率、拨备覆盖率、核心一级资本充足率，生成预警 + 统计 + 排名报告，经 Telegram 每月推送。

**Architecture:** 新包 `internal/bank`（配置、数据源、分析、汇总、渲染，均为可单测的小文件）+ `cmd/atlas/bank.go`（cobra 子命令，依赖注入 Source/Sender）。无状态：每次运行从东方财富（经本地 aktools 侧车）拉全历史现算环比/同比，不落库。launchd 每月 1 日 09:00 触发。

**Tech Stack:** Go 1.24、cobra、viper（独立实例）、testify、`net/http/httptest`；复用 `internal/notifier/telegram`。

**Spec:** `docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md`

## Global Constraints

- 市场仅 `CN_A` 与 `HK`；`HK` 条目必须有 `a_share_ref`（A 股代码），纯港股银行配置校验拒绝。
- 数据接口：`GET {aktools_url}/api/public/stock_financial_analysis_indicator_em?symbol=600036.SH&indicator=按报告期`。
- 字段：不良贷款率 `NONPERLOAN`、拨备覆盖率 `BLDKBBL`、核心一级资本充足率 `HXYJBCZL`、报告期 `REPORT_DATE`（格式 `2026-06-30 00:00:00`）。**不得**使用 `LOAN_PROVISION_RATIO`（拨贷比）、`NEWCAPITALADER`（资本充足率）、`FIRST_ADEQUACY_RATIO`（一级资本充足率）。
- 默认阈值：`npl_max 1.5`、`coverage_min 150`、`cet1_min 8.5`；恶化 `npl_up 0.10`、`coverage_down 20`、`cet1_down 0.50`（百分点）。严格不等号，等于阈值不预警。
- 变动一律以百分点表示，计算后四舍五入到 1e-4（消除 `1.02-0.92=0.10000000000000009` 类浮点假阳）。
- 缺失值为 `NaN`，展示为 `N/A`（取值）/ `—`（变动），**绝不当作 0**。
- 单条 Telegram 消息 ≤ 4000 字符（按 rune 计），只在行边界切分。
- 退出码：0 全部成功；2 部分主体拉取失败（报告照常推送）；1 配置非法 / 全部失败 / 推送失败。
- 只新建文件，不修改任何现有函数（因此无需对现有符号做 gitnexus impact；命令注册经新文件自己的 `init()`）。
- 提交前两步（用户全局规范 + 项目 CLAUDE.md）：① 调用 `code-simplifier:code-simplifier` 子代理简化本任务改动的文件；② 跑 `node .gitnexus/run.cjs detect-changes --scope staged --repo .`。提交信息格式 `feat(bank): …`，结尾附 `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`。
- 分支：`feature/bank-indicator-monitor`（已存在，spec 已在其上提交）。
- 集成测试用仓库既有的 `//go:build integration` 标签（spec §7 写的 `live` 以此为准）。

## Review Focus

1. **浮点边界**：环比恰好等于恶化阈值（NPL 0.92→1.02，+0.10pp）时不应预警 —— Task 3 `TestAlertsDeteriorationBoundaryIsStrict`。
2. **季报部分指标未披露**（一季报/三季报 CET1 或拨备为 null）：最新期取「至少一项非 NaN」，缺的指标显示 N/A、不预警、不进统计 —— Task 3 `TestAnalyzePartialNaNLatest`、Task 4 `TestSummarizeSkipsNaNInStats`、Task 5 `TestRenderNaNShowsNA`。
3. **A+H 组合**：同一 `a_share_ref` 被多个条目引用只拉一次、只计一次；只配 H 股不配 A 股时以 H 股名称为主名 —— Task 4 `TestCollectDedupsAH`、`TestCollectHOnly`。
4. **aktools 数值类型漂移**：数值被序列化成字符串、null、行日期异常、某字段整列消失 —— Task 2 `TestParseEMRowsTolerant`。
5. **银行多时报告超长**：几十家银行时报告须分多条且不在行内截断 —— Task 5 `TestSplitRespectsLimitAndLines`。

---

## File Structure

| 文件 | 职责 |
| --- | --- |
| `internal/bank/types.go` | `Indicator` 枚举、`Observation`、`Series`、`Source`、`Sender` 接口 |
| `internal/bank/config.go` | `Config` 与 `LoadConfig`、校验、`DataSymbol`/`DisplayName`/`RankIndicator` |
| `internal/bank/source.go` | `EMSource`（东方财富经 aktools）与 `parseEMRows` |
| `internal/bank/analyze.go` | `Change`、`BankResult`、`Alert`、`Analyze`、`Alerts` |
| `internal/bank/collect.go` | `Collect`：A+H 去重、逐主体取数并分析 |
| `internal/bank/summary.go` | `Stat`、`Summary`、`Summarize` |
| `internal/bank/render.go` | `Render`、`Split`、`MaxMessageRunes` |
| `internal/bank/testdata/em_600036_sample.json` | 600036.SH 真实响应片段（6 期） |
| `cmd/atlas/bank.go` | `atlas bank report` 命令、`executeBankReport`、`buildBankSender` |
| `configs/bank-monitor.yaml` | 银行列表与阈值（无密钥） |
| `deploy/launchd/com.newthinker.atlas.bank-monthly.plist` | 每月 1 日 09:00 触发 |

---

### Task 1: 类型与配置加载

**Files:**
- Create: `internal/bank/types.go`
- Create: `internal/bank/config.go`
- Create: `configs/bank-monitor.yaml`
- Test: `internal/bank/config_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `type Indicator int`；常量 `IndNPL, IndCoverage, IndCET1`，`numIndicators`（=3，未导出）；`var Indicators = []Indicator{IndNPL, IndCoverage, IndCET1}`；方法 `Label() string`、`HigherIsWorse() bool`
  - `type Observation struct{ Period time.Time; Values [numIndicators]float64 }`
  - `type Series struct{ Obs []Observation; MissingFields []string }`
  - `type Source interface{ Fetch(symbol string) (Series, error) }`
  - `type Sender interface{ SendText(text string) error }`
  - `type Config struct{ Source SourceCfg; RankBy string; Banks []BankCfg; Thresholds ThresholdsCfg }`
  - `type BankCfg struct{ Market, Symbol, Name, AShareRef string }`，方法 `DataSymbol() string`、`DisplayName() string`
  - `type ThresholdsCfg struct{ NPLMax, CoverageMin, CET1Min float64; Deterioration DeteriorationCfg }`
  - `type DeteriorationCfg struct{ NPLUp, CoverageDown, CET1Down float64 }`
  - `func LoadConfig(path string) (*Config, error)`；`func (c *Config) RankIndicator() Indicator`

- [ ] **Step 1: 写失败测试** `internal/bank/config_test.go`

```go
package bank

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validThresholds = `
thresholds:
  npl_max: 1.5
  coverage_min: 150
  cet1_min: 8.5
  deterioration: {npl_up: 0.10, coverage_down: 20, cet1_down: 0.50}
`

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bank.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func TestLoadConfigValid(t *testing.T) {
	p := writeCfg(t, `
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: HK, symbol: 03968.HK, name: 招商银行H, a_share_ref: 600036.SH}
  - {market: CN_A, symbol: 000001.SZ}
`+validThresholds)

	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8180", cfg.Source.AktoolsURL, "缺省 aktools 地址")
	assert.Equal(t, IndNPL, cfg.RankIndicator(), "rank_by 缺省为 npl")
	require.Len(t, cfg.Banks, 3)
	assert.Equal(t, "600036.SH", cfg.Banks[0].DataSymbol())
	assert.Equal(t, "600036.SH", cfg.Banks[1].DataSymbol(), "港股取 a_share_ref")
	assert.Equal(t, "招商银行H", cfg.Banks[1].DisplayName())
	assert.Equal(t, "000001.SZ", cfg.Banks[2].DisplayName(), "无 name 时回退 symbol")
	assert.Equal(t, 1.5, cfg.Thresholds.NPLMax)
	assert.Equal(t, 0.5, cfg.Thresholds.Deterioration.CET1Down)
}

func TestLoadConfigRankBy(t *testing.T) {
	p := writeCfg(t, "rank_by: cet1\nbanks:\n  - {market: CN_A, symbol: 600036.SH}\n"+validThresholds)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.Equal(t, IndCET1, cfg.RankIndicator())
}

func TestLoadConfigRejects(t *testing.T) {
	one := "banks:\n  - {market: CN_A, symbol: 600036.SH}\n"
	cases := map[string]struct{ body, want string }{
		"空列表":   {"banks: []\n" + validThresholds, "banks 不能为空"},
		"市场非法":  {"banks:\n  - {market: US, symbol: JPM}\n" + validThresholds, "仅支持 CN_A / HK"},
		"A股格式":  {"banks:\n  - {market: CN_A, symbol: '600036'}\n" + validThresholds, "格式应为 600036.SH"},
		"港股格式":  {"banks:\n  - {market: HK, symbol: 3968, a_share_ref: 600036.SH}\n" + validThresholds, "格式应为 3968.HK"},
		"纯港股":   {"banks:\n  - {market: HK, symbol: 2388.HK}\n" + validThresholds, "须填 a_share_ref"},
		"映射格式":  {"banks:\n  - {market: HK, symbol: 3968.HK, a_share_ref: '600036'}\n" + validThresholds, "须填 a_share_ref"},
		"重复":    {one + "  - {market: CN_A, symbol: 600036.SH}\n" + validThresholds, "重复"},
		"rank_by": {"rank_by: roe\n" + one + validThresholds, "rank_by"},
		"缺阈值":   {one, "npl_max 须 > 0"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(writeCfg(t, tc.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading bank config")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/bank/ -run TestLoadConfig -v`
Expected: 编译失败 `undefined: LoadConfig`（及 `IndNPL` 等）。

- [ ] **Step 3: 实现 `internal/bank/types.go`**

```go
// Package bank 定期检查银行股三项监管指标（不良贷款率/拨备覆盖率/核心一级资本充足率）
// 并生成统计报告（设计: docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md）。
package bank

import "time"

// Indicator 是三项监控指标的下标，Observation.Values 与 BankResult.Ind 均按它索引。
type Indicator int

const (
	IndNPL      Indicator = iota // 不良贷款率 %
	IndCoverage                  // 拨备覆盖率 %
	IndCET1                      // 核心一级资本充足率 %
	numIndicators
)

// Indicators 按报告展示顺序列出全部指标。
var Indicators = []Indicator{IndNPL, IndCoverage, IndCET1}

func (i Indicator) Label() string {
	return [...]string{"不良率", "拨备覆盖率", "CET1"}[i]
}

// HigherIsWorse：不良率越高越差；拨备覆盖率与 CET1 越低越差。
func (i Indicator) HigherIsWorse() bool { return i == IndNPL }

// Observation 是一个报告期的三项指标，缺失为 NaN。
type Observation struct {
	Period time.Time
	Values [numIndicators]float64
}

// Series 是某主体的全部历史报告期。
type Series struct {
	Obs           []Observation // 按 Period 升序
	MissingFields []string      // 所有行都不含的字段键（疑似数据源结构变化）
}

// Source 按 A 股代码拉取指标历史。
type Source interface {
	Fetch(symbol string) (Series, error)
}

// Sender 是推送通道的最小接口，*telegram.Telegram 直接满足。
type Sender interface {
	SendText(text string) error
}
```

- [ ] **Step 4: 实现 `internal/bank/config.go`**

```go
package bank

import (
	"fmt"
	"regexp"

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
		if seen[b.Symbol] {
			return fmt.Errorf("banks[%d]: symbol %s 重复", i, b.Symbol)
		}
		seen[b.Symbol] = true
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
		if f.v <= 0 {
			return fmt.Errorf("thresholds.%s 须 > 0", f.name)
		}
	}
	return nil
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/bank/ -run TestLoadConfig -v`
Expected: 全部 PASS。

- [ ] **Step 6: 新建 `configs/bank-monitor.yaml`**

```yaml
# 银行股关键指标月度监控（atlas bank report）
# 设计: docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md
# 本文件不含密钥；Telegram 凭据走主配置 notifiers.telegram。
# 港股条目必须填 a_share_ref（对应 A 股代码），纯港股银行一期不支持。
source:
  aktools_url: http://127.0.0.1:8180
rank_by: npl            # npl | coverage | cet1
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: CN_A, symbol: 601658.SH, name: 邮储银行}
  - {market: HK,   symbol: 3968.HK,   name: 招商银行H, a_share_ref: 600036.SH}
thresholds:
  npl_max: 1.5          # %，> 即预警
  coverage_min: 150     # %，< 即预警
  cet1_min: 8.5         # %，< 即预警（7.5% 监管下限 + 1pp 缓冲）
  deterioration:        # 环比恶化超过以下幅度即预警，单位百分点
    npl_up: 0.10
    coverage_down: 20
    cet1_down: 0.50
```

在 `config_test.go` 末尾追加，保证仓库自带的配置始终可加载：

```go
func TestShippedConfigLoads(t *testing.T) {
	cfg, err := LoadConfig("../../configs/bank-monitor.yaml")
	require.NoError(t, err)
	assert.Len(t, cfg.Banks, 3)
}
```

Run: `go test ./internal/bank/ -v` → Expected: 全部 PASS。

- [ ] **Step 7: 提交**（先按 Global Constraints 跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/types.go internal/bank/config.go internal/bank/config_test.go configs/bank-monitor.yaml
git commit -m "feat(bank): 银行指标监控的类型与配置加载"
```

---

### Task 2: 东方财富数据源 EMSource

**Files:**
- Create: `internal/bank/source.go`
- Create: `internal/bank/testdata/em_600036_sample.json`
- Test: `internal/bank/source_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Observation`、`Series`、`Source`、`IndNPL/IndCoverage/IndCET1`、`numIndicators`
- Produces: `func NewEMSource(baseURL string) *EMSource`；`func (s *EMSource) Fetch(symbol string) (Series, error)`（满足 `Source`）；未导出 `parseEMRows(rows []map[string]any) (Series, error)`

- [ ] **Step 1: 写测试样本** `internal/bank/testdata/em_600036_sample.json`（2026-09-26 实测值，按接口原样降序；带两个易混字段作诱饵）

```json
[
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2026-06-30 00:00:00", "NONPERLOAN": 0.94, "BLDKBBL": 385.1, "HXYJBCZL": 14.07, "LOAN_PROVISION_RATIO": 3.63, "NEWCAPITALADER": 18.33},
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2026-03-31 00:00:00", "NONPERLOAN": 0.94, "BLDKBBL": 387.76, "HXYJBCZL": 14.13},
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2025-12-31 00:00:00", "NONPERLOAN": 0.94, "BLDKBBL": 391.79, "HXYJBCZL": 14.16},
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2025-09-30 00:00:00", "NONPERLOAN": 0.94, "BLDKBBL": 405.93, "HXYJBCZL": 13.93},
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2025-06-30 00:00:00", "NONPERLOAN": 0.93, "BLDKBBL": 410.93, "HXYJBCZL": 14.0},
  {"SECUCODE": "600036.SH", "REPORT_DATE": "2025-03-31 00:00:00", "NONPERLOAN": 0.94, "BLDKBBL": 410.03, "HXYJBCZL": 14.86}
]
```

- [ ] **Step 2: 写失败测试** `internal/bank/source_test.go`

```go
package bank

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestEMSourceFetchParsesFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/em_600036_sample.json")
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_financial_analysis_indicator_em", r.URL.Path)
		assert.Equal(t, "600036.SH", r.URL.Query().Get("symbol"))
		assert.Equal(t, "按报告期", r.URL.Query().Get("indicator"))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s, err := NewEMSource(srv.URL + "/").Fetch("600036.SH")
	require.NoError(t, err)
	require.Len(t, s.Obs, 6)
	assert.Equal(t, day("2025-03-31"), s.Obs[0].Period, "升序")
	last := s.Obs[5]
	assert.Equal(t, day("2026-06-30"), last.Period)
	assert.Equal(t, 0.94, last.Values[IndNPL])
	assert.Equal(t, 385.1, last.Values[IndCoverage], "取 BLDKBBL 而非拨贷比 LOAN_PROVISION_RATIO")
	assert.Equal(t, 14.07, last.Values[IndCET1], "取 HXYJBCZL 而非资本充足率 NEWCAPITALADER")
	assert.Empty(t, s.MissingFields)
}

func TestParseEMRowsTolerant(t *testing.T) {
	rows := []map[string]any{
		{"REPORT_DATE": "2026-06-30 00:00:00", "NONPERLOAN": "1.23", "BLDKBBL": nil},
		{"REPORT_DATE": "bad", "NONPERLOAN": 9.9},
		{"REPORT_DATE": "2026-03-31 00:00:00", "NONPERLOAN": 1.2, "BLDKBBL": 200.0},
	}
	s, err := parseEMRows(rows)
	require.NoError(t, err)
	require.Len(t, s.Obs, 2, "日期异常的行跳过")
	assert.Equal(t, day("2026-03-31"), s.Obs[0].Period)
	assert.Equal(t, 1.23, s.Obs[1].Values[IndNPL], "字符串数值可解析")
	assert.True(t, math.IsNaN(s.Obs[1].Values[IndCoverage]), "null → NaN")
	assert.True(t, math.IsNaN(s.Obs[1].Values[IndCET1]), "缺键 → NaN")
	assert.Equal(t, []string{"HXYJBCZL"}, s.MissingFields, "所有行都缺的键才算字段缺失；null 不算")
}

func TestEMSourceFetchErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"非200":  {http.StatusInternalServerError, "boom", "HTTP 500"},
		"空数组":  {http.StatusOK, "[]", "无数据"},
		"无有效期": {http.StatusOK, `[{"REPORT_DATE": "x"}]`, "无可解析报告期"},
		"非JSON": {http.StatusOK, "<html>", "decode"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := NewEMSource(srv.URL).Fetch("600036.SH")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), "600036.SH", "错误带代码，报告里可定位")
		})
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/bank/ -run 'EM|ParseEM' -v`
Expected: 编译失败 `undefined: NewEMSource` / `parseEMRows`。

- [ ] **Step 4: 实现 `internal/bank/source.go`**

```go
package bank

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 东方财富「主要指标」经 aktools 侧车调用。
// ⚠ live 校验点：字段名随上游变动，2026-09-26 对 600036.SH 实测（设计 §2）。
// 易混字段 LOAN_PROVISION_RATIO（拨贷比）、NEWCAPITALADER（资本充足率）、
// FIRST_ADEQUACY_RATIO（一级资本充足率）不得使用。
const (
	emAPI     = "stock_financial_analysis_indicator_em"
	emDateKey = "REPORT_DATE"
)

var emFieldKeys = [numIndicators]string{
	IndNPL:      "NONPERLOAN",
	IndCoverage: "BLDKBBL",
	IndCET1:     "HXYJBCZL",
}

type EMSource struct {
	baseURL string
	hc      *http.Client
}

func NewEMSource(baseURL string) *EMSource {
	return &EMSource{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *EMSource) Fetch(symbol string) (Series, error) {
	q := url.Values{"symbol": {symbol}, "indicator": {"按报告期"}}
	resp, err := s.hc.Get(s.baseURL + "/api/public/" + emAPI + "?" + q.Encode())
	if err != nil {
		return Series{}, fmt.Errorf("%s %s: %w", emAPI, symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return Series{}, fmt.Errorf("%s %s: HTTP %d: %s", emAPI, symbol, resp.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return Series{}, fmt.Errorf("%s %s: decode: %w", emAPI, symbol, err)
	}
	series, err := parseEMRows(rows)
	if err != nil {
		return Series{}, fmt.Errorf("%s %s: %w", emAPI, symbol, err)
	}
	return series, nil
}

func parseEMRows(rows []map[string]any) (Series, error) {
	if len(rows) == 0 {
		return Series{}, errors.New("无数据")
	}
	var s Series
	for _, row := range rows {
		ds, _ := row[emDateKey].(string)
		if len(ds) < 10 {
			continue
		}
		p, err := time.Parse("2006-01-02", ds[:10])
		if err != nil {
			continue
		}
		o := Observation{Period: p}
		for i, k := range emFieldKeys {
			o.Values[i] = toFloat(row[k])
		}
		s.Obs = append(s.Obs, o)
	}
	if len(s.Obs) == 0 {
		return Series{}, errors.New("无可解析报告期")
	}
	sort.Slice(s.Obs, func(i, j int) bool { return s.Obs[i].Period.Before(s.Obs[j].Period) })
	for _, k := range emFieldKeys {
		if !anyRowHas(rows, k) {
			s.MissingFields = append(s.MissingFields, k)
		}
	}
	return s, nil
}

func anyRowHas(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

// toFloat 兼容 aktools 偶把数值序列化为字符串；null / 缺键 / 不可解析 → NaN。
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
	}
	return math.NaN()
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/bank/ -v`
Expected: 全部 PASS。

- [ ] **Step 6: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/source.go internal/bank/source_test.go internal/bank/testdata/em_600036_sample.json
git commit -m "feat(bank): 东方财富银行指标数据源（经 aktools）"
```

---

### Task 3: 最新期 / 环比 / 同比与预警判定

**Files:**
- Create: `internal/bank/analyze.go`
- Test: `internal/bank/analyze_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Observation`、`Indicator`、`Indicators`、`ThresholdsCfg`、`numIndicators`
- Produces:
  - `type Change struct{ Value, QoQ, YoY float64 }`（无法计算为 NaN）
  - `type BankResult struct{ Symbol, Name string; Aliases []string; Latest time.Time; Ind [numIndicators]Change; MissingFields []string; Err error }`
  - `type AlertKind int`；`AlertLevel`、`AlertDeterioration`
  - `type Alert struct{ Name string; Ind Indicator; Kind AlertKind; Value, Limit float64 }`
  - `func Analyze(obs []Observation) (time.Time, [numIndicators]Change)`
  - `func Alerts(r BankResult, t ThresholdsCfg) []Alert`

- [ ] **Step 1: 写失败测试** `internal/bank/analyze_test.go`

```go
package bank

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var nan = math.NaN()

func obs(date string, npl, cov, cet1 float64) Observation {
	return Observation{Period: day(date), Values: [numIndicators]float64{npl, cov, cet1}}
}

// cmb 是 600036.SH 2025-03-31 … 2026-06-30 的实测值（升序）。
var cmb = []Observation{
	obs("2025-03-31", 0.94, 410.03, 14.86),
	obs("2025-06-30", 0.93, 410.93, 14.0),
	obs("2025-09-30", 0.94, 405.93, 13.93),
	obs("2025-12-31", 0.94, 391.79, 14.16),
	obs("2026-03-31", 0.94, 387.76, 14.13),
	obs("2026-06-30", 0.94, 385.1, 14.07),
}

var stdThresholds = ThresholdsCfg{
	NPLMax: 1.5, CoverageMin: 150, CET1Min: 8.5,
	Deterioration: DeteriorationCfg{NPLUp: 0.10, CoverageDown: 20, CET1Down: 0.50},
}

func TestAnalyzeLatestQoQYoY(t *testing.T) {
	latest, ind := Analyze(cmb)
	assert.Equal(t, day("2026-06-30"), latest)
	assert.Equal(t, Change{0.94, 0, 0.01}, ind[IndNPL])
	assert.Equal(t, Change{385.1, -2.66, -25.83}, ind[IndCoverage], "变动四舍五入到 1e-4，可精确比较")
	assert.Equal(t, Change{14.07, -0.06, 0.07}, ind[IndCET1])
}

func TestAnalyzeSkipsAllNaNTail(t *testing.T) {
	latest, ind := Analyze(append(cmb[:6:6], obs("2026-09-30", nan, nan, nan)))
	assert.Equal(t, day("2026-06-30"), latest, "全 NaN 的期不算最新期")
	assert.Equal(t, 0.94, ind[IndNPL].Value)
}

func TestAnalyzePartialNaNLatest(t *testing.T) {
	series := []Observation{obs("2025-12-31", 1.0, 200, 10), obs("2026-03-31", 1.1, 190, nan)}
	latest, ind := Analyze(series)
	assert.Equal(t, day("2026-03-31"), latest, "至少一项非 NaN 即为最新期")
	assert.True(t, math.IsNaN(ind[IndCET1].Value))
	assert.True(t, math.IsNaN(ind[IndCET1].QoQ))
	assert.InDelta(t, 0.1, ind[IndNPL].QoQ, 1e-12)
}

func TestAnalyzeNoPrevNoYoY(t *testing.T) {
	latest, ind := Analyze([]Observation{obs("2026-06-30", 1, 200, 10)})
	assert.Equal(t, day("2026-06-30"), latest)
	for _, k := range Indicators {
		assert.True(t, math.IsNaN(ind[k].QoQ))
		assert.True(t, math.IsNaN(ind[k].YoY))
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	latest, ind := Analyze(nil)
	assert.True(t, latest.IsZero())
	for _, k := range Indicators {
		assert.True(t, math.IsNaN(ind[k].Value))
	}
}

func result(npl, cov, cet1 Change) BankResult {
	return BankResult{Name: "X", Latest: time.Now(), Ind: [numIndicators]Change{npl, cov, cet1}}
}

func val(v float64) Change { return Change{v, nan, nan} }

func TestAlertsLevel(t *testing.T) {
	cases := map[string]struct {
		r    BankResult
		want []Alert
	}{
		"不良率超限":  {result(val(1.62), val(200), val(10)), []Alert{{"X", IndNPL, AlertLevel, 1.62, 1.5}}},
		"不良率等于阈值": {result(val(1.5), val(200), val(10)), nil},
		"拨备不足":   {result(val(1), val(142), val(10)), []Alert{{"X", IndCoverage, AlertLevel, 142, 150}}},
		"CET1等于阈值": {result(val(1), val(200), val(8.5)), nil},
		"CET1不足":  {result(val(1), val(200), val(8.4)), []Alert{{"X", IndCET1, AlertLevel, 8.4, 8.5}}},
		"NaN不预警":  {result(val(nan), val(nan), val(nan)), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Alerts(tc.r, stdThresholds))
		})
	}
}

func TestAlertsDeterioration(t *testing.T) {
	r := result(Change{1.0, 0.11, nan}, Change{200, -20.5, nan}, Change{10, -0.62, nan})
	assert.Equal(t, []Alert{
		{"X", IndNPL, AlertDeterioration, 0.11, 0.10},
		{"X", IndCoverage, AlertDeterioration, -20.5, 20},
		{"X", IndCET1, AlertDeterioration, -0.62, 0.50},
	}, Alerts(r, stdThresholds))

	improving := result(Change{1.0, -0.5, nan}, Change{200, 30, nan}, Change{10, 1, nan})
	assert.Empty(t, Alerts(improving, stdThresholds), "改善方向不预警")
}

func TestAlertsDeteriorationBoundaryIsStrict(t *testing.T) {
	// 1.02-0.92 在 float64 下是 0.10000000000000009；四舍五入后恰等于阈值，不应预警。
	_, ind := Analyze([]Observation{obs("2026-03-31", 0.92, 200, 10), obs("2026-06-30", 1.02, 200, 10)})
	require.Equal(t, 0.1, ind[IndNPL].QoQ)
	assert.Empty(t, Alerts(BankResult{Name: "X", Ind: ind}, stdThresholds))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/bank/ -run 'Analyze|Alerts' -v`
Expected: 编译失败 `undefined: Analyze` / `Change` / `Alerts`。

- [ ] **Step 3: 实现 `internal/bank/analyze.go`**

```go
package bank

import (
	"math"
	"time"
)

// Change 是某指标在最新期的取值与变动（百分点）；无法计算为 NaN。
type Change struct {
	Value, QoQ, YoY float64
}

// BankResult 是一个主体（A 股代码）的分析结果。
type BankResult struct {
	Symbol        string    // 取数用 A 股代码
	Name          string    // 主名（配置中首个指向该代码的条目）
	Aliases       []string  // 共用该主体数据的其余条目名（如 H 股）
	Latest        time.Time // 至少一项指标非 NaN 的最近报告期；零值 = 无数据
	Ind           [numIndicators]Change
	MissingFields []string
	Err           error
}

type AlertKind int

const (
	AlertLevel         AlertKind = iota // 越过阈值
	AlertDeterioration                  // 环比恶化超过幅度
)

// Alert.Value：AlertLevel 为指标值，AlertDeterioration 为环比变动（pp）；Limit 为对应阈值。
type Alert struct {
	Name  string
	Ind   Indicator
	Kind  AlertKind
	Value float64
	Limit float64
}

// Analyze 从按期升序的序列取最新期，并计算环比（紧邻上一期）与同比（上年同一报告期）。
func Analyze(obs []Observation) (time.Time, [numIndicators]Change) {
	var ind [numIndicators]Change
	for k := range ind {
		ind[k] = Change{math.NaN(), math.NaN(), math.NaN()}
	}
	li := -1
	for i := len(obs) - 1; i >= 0; i-- {
		if hasAny(obs[i]) {
			li = i
			break
		}
	}
	if li < 0 {
		return time.Time{}, ind
	}
	cur := obs[li]
	yoyTarget := cur.Period.AddDate(-1, 0, 0)
	for k := range ind {
		ind[k].Value = cur.Values[k]
		if li > 0 {
			ind[k].QoQ = round4(cur.Values[k] - obs[li-1].Values[k])
		}
		for _, o := range obs[:li] {
			if o.Period.Equal(yoyTarget) {
				ind[k].YoY = round4(cur.Values[k] - o.Values[k])
			}
		}
	}
	return cur.Period, ind
}

func hasAny(o Observation) bool {
	for _, v := range o.Values {
		if !math.IsNaN(v) {
			return true
		}
	}
	return false
}

// round4 消除 1.02-0.92=0.10000000000000009 类浮点误差（源数据至多两位小数）。
func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// Alerts 判定阈值与恶化预警；NaN 参与的比较恒假，故缺失值不预警。
func Alerts(r BankResult, t ThresholdsCfg) []Alert {
	level := [numIndicators]float64{t.NPLMax, t.CoverageMin, t.CET1Min}
	deter := [numIndicators]float64{t.Deterioration.NPLUp, t.Deterioration.CoverageDown, t.Deterioration.CET1Down}
	var out []Alert
	for _, k := range Indicators {
		c := r.Ind[k]
		if worseThan(k, c.Value, level[k]) {
			out = append(out, Alert{r.Name, k, AlertLevel, c.Value, level[k]})
		}
		if worsening(k, c.QoQ) > deter[k] {
			out = append(out, Alert{r.Name, k, AlertDeterioration, c.QoQ, deter[k]})
		}
	}
	return out
}

func worseThan(k Indicator, v, limit float64) bool {
	if k.HigherIsWorse() {
		return v > limit
	}
	return v < limit
}

// worsening 把环比变动换算成恶化幅度（正数 = 变差）。
func worsening(k Indicator, delta float64) float64 {
	if k.HigherIsWorse() {
		return delta
	}
	return -delta
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/bank/ -v`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/analyze.go internal/bank/analyze_test.go
git commit -m "feat(bank): 环比同比计算与阈值/恶化预警"
```

---

### Task 4: 取数聚合（A+H 去重）与同期统计

**Files:**
- Create: `internal/bank/collect.go`
- Create: `internal/bank/summary.go`
- Test: `internal/bank/collect_test.go`
- Test: `internal/bank/summary_test.go`

**Interfaces:**
- Consumes: Task 1 `BankCfg`、`Source`、`Series`；Task 3 `BankResult`、`Change`、`Analyze`
- Produces:
  - `func Collect(banks []BankCfg, src Source) []BankResult`
  - `type Stat struct{ N int; Mean, Median float64; Best, Worst string; BestVal, WorstVal float64 }`
  - `type Summary struct{ Period time.Time; RankBy Indicator; Current, Stale, Failed []BankResult; Stats [numIndicators]Stat }`
  - `func Summarize(results []BankResult, rankBy Indicator) Summary`

- [ ] **Step 1: 写失败测试** `internal/bank/collect_test.go`

```go
package bank

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	series map[string]Series
	errs   map[string]error
	calls  map[string]int
}

func (f *fakeSource) Fetch(symbol string) (Series, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[symbol]++
	if err := f.errs[symbol]; err != nil {
		return Series{}, err
	}
	return f.series[symbol], nil
}

func TestCollectDedupsAH(t *testing.T) {
	src := &fakeSource{series: map[string]Series{
		"600036.SH": {Obs: cmb},
		"601658.SH": {Obs: []Observation{obs("2026-06-30", 1.0, 214.93, 10.04)}, MissingFields: []string{"HXYJBCZL"}},
	}}
	banks := []BankCfg{
		{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
		{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
		{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
	}
	rs := Collect(banks, src)
	require.Len(t, rs, 2, "A+H 同一主体只计一次")
	assert.Equal(t, 1, src.calls["600036.SH"], "只拉一次")
	assert.Equal(t, "招商银行", rs[0].Name)
	assert.Equal(t, []string{"招商银行H"}, rs[0].Aliases)
	assert.Equal(t, day("2026-06-30"), rs[0].Latest)
	assert.Equal(t, 385.1, rs[0].Ind[IndCoverage].Value)
	assert.Equal(t, []string{"HXYJBCZL"}, rs[1].MissingFields)
}

func TestCollectHOnly(t *testing.T) {
	src := &fakeSource{series: map[string]Series{"600036.SH": {Obs: cmb}}}
	rs := Collect([]BankCfg{{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"}}, src)
	require.Len(t, rs, 1)
	assert.Equal(t, "招商银行H", rs[0].Name, "只配 H 股时以 H 股名为主名")
	assert.Equal(t, "600036.SH", rs[0].Symbol)
	assert.Empty(t, rs[0].Aliases)
}

func TestCollectKeepsErrors(t *testing.T) {
	src := &fakeSource{
		series: map[string]Series{"600036.SH": {Obs: cmb}},
		errs:   map[string]error{"601658.SH": errors.New("timeout")},
	}
	rs := Collect([]BankCfg{
		{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
		{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
	}, src)
	require.Len(t, rs, 2)
	assert.NoError(t, rs[0].Err)
	assert.EqualError(t, rs[1].Err, "timeout")
	assert.True(t, rs[1].Latest.IsZero())
}
```

- [ ] **Step 2: 写失败测试** `internal/bank/summary_test.go`

```go
package bank

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func br(name, latest string, npl, cov, cet1 float64) BankResult {
	r := BankResult{Name: name, Ind: [numIndicators]Change{val(npl), val(cov), val(cet1)}}
	if latest != "" {
		r.Latest = day(latest)
	}
	return r
}

func names(rs []BankResult) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func sample() []BankResult {
	failed := BankResult{Name: "E银行", Err: errors.New("timeout")}
	return []BankResult{
		br("C银行", "2026-06-30", 1.62, 142, 8.9),
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("D银行", "2026-03-31", 1.1, 180, 9.5),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		failed,
		br("F银行", "", nan, nan, nan),
	}
}

func TestSummarizeGroupsAndRanks(t *testing.T) {
	s := Summarize(sample(), IndNPL)
	assert.Equal(t, day("2026-06-30"), s.Period, "统计期取最晚的最新期")
	assert.Equal(t, IndNPL, s.RankBy)
	assert.Equal(t, []string{"A银行", "B银行", "C银行"}, names(s.Current), "不良率升序")
	assert.Equal(t, []string{"D银行", "F银行"}, names(s.Stale), "未披露统计期 + 无任何数据")
	assert.Equal(t, []string{"E银行"}, names(s.Failed))

	npl := s.Stats[IndNPL]
	assert.Equal(t, 3, npl.N)
	assert.InDelta(t, (0.94+1.0+1.62)/3, npl.Mean, 1e-12)
	assert.Equal(t, 1.0, npl.Median)
	assert.Equal(t, "A银行", npl.Best)
	assert.Equal(t, 0.94, npl.BestVal)
	assert.Equal(t, "C银行", npl.Worst)
	assert.Equal(t, 1.62, npl.WorstVal)

	cov := s.Stats[IndCoverage]
	assert.Equal(t, "A银行", cov.Best, "拨备覆盖率越高越好")
	assert.Equal(t, "C银行", cov.Worst)
}

func TestSummarizeRankByHigherIsBetter(t *testing.T) {
	rs := []BankResult{
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("B银行", "2026-06-30", 1.0, 214.93, 15),
		br("C银行", "2026-06-30", 1.62, 142, 8.9),
	}
	assert.Equal(t, []string{"B银行", "A银行", "C银行"}, names(Summarize(rs, IndCET1).Current), "CET1 降序")
}

func TestSummarizeSkipsNaNInStats(t *testing.T) {
	rs := []BankResult{
		br("A银行", "2026-06-30", 0.94, 385, nan),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		br("C银行", "2026-06-30", 1.62, 142, 14.07),
	}
	s := Summarize(rs, IndCET1)
	assert.Equal(t, []string{"C银行", "B银行", "A银行"}, names(s.Current), "NaN 排最后")
	cet1 := s.Stats[IndCET1]
	assert.Equal(t, 2, cet1.N)
	assert.InDelta(t, (10.04+14.07)/2, cet1.Median, 1e-12, "偶数个取中间两数均值")
}

func TestSummarizeNoCurrent(t *testing.T) {
	s := Summarize([]BankResult{{Name: "E银行", Err: errors.New("x")}}, IndNPL)
	require.True(t, s.Period.IsZero())
	assert.Empty(t, s.Current)
	assert.Equal(t, 0, s.Stats[IndNPL].N)
	assert.True(t, math.IsNaN(s.Stats[IndNPL].Mean))
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/bank/ -run 'Collect|Summarize' -v`
Expected: 编译失败 `undefined: Collect` / `Summarize`。

- [ ] **Step 4: 实现 `internal/bank/collect.go`**

```go
package bank

// Collect 按取数代码去重（A+H 同一主体只拉一次、只计一次，配置中首个条目为主名），
// 逐主体拉取并分析；单个主体失败只记在其 Err 上，不影响其他主体。
func Collect(banks []BankCfg, src Source) []BankResult {
	var results []BankResult
	idx := map[string]int{}
	for _, b := range banks {
		sym := b.DataSymbol()
		if i, ok := idx[sym]; ok {
			results[i].Aliases = append(results[i].Aliases, b.DisplayName())
			continue
		}
		idx[sym] = len(results)
		results = append(results, BankResult{Symbol: sym, Name: b.DisplayName()})
	}
	for i := range results {
		series, err := src.Fetch(results[i].Symbol)
		results[i].Latest, results[i].Ind = Analyze(series.Obs)
		results[i].MissingFields = series.MissingFields
		results[i].Err = err
	}
	return results
}
```

（出错时 `series` 为零值，`Analyze(nil)` 返回零时间与全 NaN，失败主体的 `Ind` 不会是误导性的 0。）

- [ ] **Step 5: 实现 `internal/bank/summary.go`**

```go
package bank

import (
	"math"
	"sort"
	"time"
)

// Stat 是某指标在统计期内的分布；N==0 时数值为 NaN、名称为空。
type Stat struct {
	N                 int
	Mean, Median      float64
	Best, Worst       string
	BestVal, WorstVal float64
}

// Summary 按统计期把主体分为三组。统计期 = 所有未失败主体中最晚的最新期。
type Summary struct {
	Period  time.Time
	RankBy  Indicator
	Current []BankResult // Latest == Period，按 RankBy 由优到劣
	Stale   []BankResult // 最新期早于 Period，或无任何数据
	Failed  []BankResult // 拉取失败
	Stats   [numIndicators]Stat
}

func Summarize(results []BankResult, rankBy Indicator) Summary {
	s := Summary{RankBy: rankBy}
	for _, r := range results {
		if r.Err == nil && r.Latest.After(s.Period) {
			s.Period = r.Latest
		}
	}
	for _, r := range results {
		switch {
		case r.Err != nil:
			s.Failed = append(s.Failed, r)
		case !r.Latest.IsZero() && r.Latest.Equal(s.Period):
			s.Current = append(s.Current, r)
		default:
			s.Stale = append(s.Stale, r)
		}
	}
	sort.SliceStable(s.Current, func(i, j int) bool {
		return better(rankBy, s.Current[i].Ind[rankBy].Value, s.Current[j].Ind[rankBy].Value)
	})
	for _, k := range Indicators {
		s.Stats[k] = stat(s.Current, k)
	}
	return s
}

// better 报告 a 是否严格优于 b；NaN 视为最差。
func better(k Indicator, a, b float64) bool {
	switch {
	case math.IsNaN(a):
		return false
	case math.IsNaN(b):
		return true
	case k.HigherIsWorse():
		return a < b
	default:
		return a > b
	}
}

func stat(rs []BankResult, k Indicator) Stat {
	st := Stat{Mean: math.NaN(), Median: math.NaN(), BestVal: math.NaN(), WorstVal: math.NaN()}
	var vals []float64
	for _, r := range rs {
		v := r.Ind[k].Value
		if math.IsNaN(v) {
			continue
		}
		vals = append(vals, v)
		if st.Best == "" || better(k, v, st.BestVal) {
			st.Best, st.BestVal = r.Name, v
		}
		if st.Worst == "" || better(k, st.WorstVal, v) {
			st.Worst, st.WorstVal = r.Name, v
		}
	}
	st.N = len(vals)
	if st.N == 0 {
		return st
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	st.Mean = sum / float64(st.N)
	sort.Float64s(vals)
	if m := st.N / 2; st.N%2 == 1 {
		st.Median = vals[m]
	} else {
		st.Median = (vals[m-1] + vals[m]) / 2
	}
	return st
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/bank/ -v`
Expected: 全部 PASS。

- [ ] **Step 7: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/collect.go internal/bank/collect_test.go internal/bank/summary.go internal/bank/summary_test.go
git commit -m "feat(bank): A+H 去重取数与同期横向统计排名"
```

---

### Task 5: 报告渲染与分段

**Files:**
- Create: `internal/bank/render.go`
- Test: `internal/bank/render_test.go`

**Interfaces:**
- Consumes: Task 3 `Alert`、`AlertLevel`、`AlertDeterioration`、`BankResult`；Task 4 `Summary`、`Stat`、`Summarize`
- Produces: `const MaxMessageRunes = 4000`；`func Render(s Summary, alerts []Alert, now time.Time) string`；`func Split(text string, limit int) []string`

- [ ] **Step 1: 写失败测试** `internal/bank/render_test.go`

```go
package bank

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderSample() string {
	rs := sample() // summary_test.go：A/B/C 当期，D 未更新，E 失败，F 无数据
	for i := range rs {
		switch rs[i].Name {
		case "A银行":
			rs[i].Symbol, rs[i].Aliases = "600036.SH", []string{"A银行H"}
			rs[i].Ind[IndNPL] = Change{0.94, -0.01, 0.01}
		case "B银行":
			rs[i].MissingFields = []string{"HXYJBCZL"}
		}
	}
	var alerts []Alert
	for _, r := range rs {
		if r.Err != nil {
			continue // 同 executeBankReport：失败主体的 Ind 是零值，不能参与预警
		}
		alerts = append(alerts, Alerts(r, stdThresholds)...)
	}
	return Render(Summarize(rs, IndNPL), alerts, day("2026-10-01"))
}

func TestRenderSections(t *testing.T) {
	out := renderSample()
	for _, want := range []string{
		"🏦 银行关键指标月报 2026-10-01",
		"覆盖 6 家（统计期 2026-06-30：3 家；未更新 2；失败 1）",
		"⚠️ 预警 (2)",
		"· C银行 不良率 1.62% > 1.50%",
		"· C银行 拨备覆盖率 142.0% < 150.0%",
		"📊 同期统计 2026-06-30（n=3）",
		"不良率 均值 1.19% | 中位 1.00% | 最优 A银行 0.94% | 最差 C银行 1.62%",
		"🏷 排名（按不良率由优到劣）",
		"1. A银行 不良率 0.94%(环比-0.01/同比+0.01) 拨备覆盖率 385.0%(环比—/同比—) CET1 14.07%(环比—/同比—)",
		"  （A银行H 同 600036.SH）",
		"3. C银行",
		"ℹ️ 未更新/失败",
		"· D银行 最新 2026-03-31（未披露 2026-06-30）",
		"· F银行 无可用数据",
		"· E银行 拉取失败：timeout",
		"· B银行 字段缺失：HXYJBCZL（疑似数据源结构变化）",
	} {
		assert.Contains(t, out, want)
	}
}

func TestRenderDeteriorationAlert(t *testing.T) {
	out := Render(Summary{}, []Alert{{"X银行", IndCET1, AlertDeterioration, -0.62, 0.5}}, day("2026-10-01"))
	assert.Contains(t, out, "· X银行 CET1 环比 -0.62pp（超 0.50pp）")
}

func TestRenderNoAlertsNoCurrent(t *testing.T) {
	out := Render(Summarize([]BankResult{br("D银行", "", nan, nan, nan)}, IndNPL), nil, day("2026-10-01"))
	assert.Contains(t, out, "⚠️ 预警 (0)\n无")
	assert.Contains(t, out, "统计期 —")
	assert.NotContains(t, out, "📊", "无当期主体时不出统计段")
}

func TestRenderNaNShowsNA(t *testing.T) {
	rs := []BankResult{br("A银行", "2026-06-30", 0.94, 385, nan)}
	out := Render(Summarize(rs, IndNPL), nil, day("2026-10-01"))
	assert.Contains(t, out, "CET1 N/A(环比—/同比—)")
	assert.Contains(t, out, "CET1 无数据")
	assert.NotContains(t, out, "NaN")
}

func TestSplitRespectsLimitAndLines(t *testing.T) {
	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, fmt.Sprintf("%d. 某某农村商业银行 不良率 1.23%%(环比+0.01/同比-0.02)", i))
	}
	text := strings.Join(lines, "\n") + "\n"
	chunks := Split(text, MaxMessageRunes)
	require.Greater(t, len(chunks), 1)
	for _, c := range chunks {
		assert.LessOrEqual(t, utf8.RuneCountInString(c), MaxMessageRunes)
	}
	assert.Equal(t, strings.TrimRight(text, "\n"), strings.Join(chunks, "\n"), "只在行边界切分，拼回无损")
}

func TestSplitOversizedLineStandsAlone(t *testing.T) {
	chunks := Split("短\n"+strings.Repeat("长", 25)+"\n尾\n", 10)
	assert.Equal(t, []string{"短", strings.Repeat("长", 25), "尾"}, chunks, "超长单行独占一段，不在行内截断")
}

func TestSplitShortText(t *testing.T) {
	assert.Equal(t, []string{"a\nb"}, Split("a\nb\n", 100))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/bank/ -run 'Render|Split' -v`
Expected: 编译失败 `undefined: Render` / `Split` / `MaxMessageRunes`。

- [ ] **Step 3: 实现 `internal/bank/render.go`**

```go
package bank

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxMessageRunes 是单条推送上限（Telegram 4096，留余量）。
const MaxMessageRunes = 4000

// decimals 是各指标的展示精度。
var decimals = [numIndicators]int{IndNPL: 2, IndCoverage: 1, IndCET1: 2}

// Render 生成纯文本报告（SendText 不设 parse_mode，故用列表式而非表格）。
func Render(s Summary, alerts []Alert, now time.Time) string {
	var b strings.Builder
	total := len(s.Current) + len(s.Stale) + len(s.Failed)
	fmt.Fprintf(&b, "🏦 银行关键指标月报 %s\n", now.Format("2006-01-02"))
	fmt.Fprintf(&b, "覆盖 %d 家（统计期 %s：%d 家；未更新 %d；失败 %d）\n",
		total, fmtDate(s.Period), len(s.Current), len(s.Stale), len(s.Failed))

	fmt.Fprintf(&b, "\n⚠️ 预警 (%d)\n", len(alerts))
	if len(alerts) == 0 {
		b.WriteString("无\n")
	}
	for _, a := range alerts {
		b.WriteString("· " + fmtAlert(a) + "\n")
	}

	if len(s.Current) > 0 {
		fmt.Fprintf(&b, "\n📊 同期统计 %s（n=%d）\n", fmtDate(s.Period), len(s.Current))
		for _, k := range Indicators {
			b.WriteString(fmtStat(k, s.Stats[k]) + "\n")
		}
		fmt.Fprintf(&b, "\n🏷 排名（按%s由优到劣）\n", s.RankBy.Label())
		for i, r := range s.Current {
			fmt.Fprintf(&b, "%d. %s\n", i+1, fmtBankLine(r))
			writeAliases(&b, r)
		}
	}

	var info []string
	for _, r := range s.Stale {
		if r.Latest.IsZero() {
			info = append(info, fmt.Sprintf("· %s 无可用数据", r.Name))
		} else {
			info = append(info, fmt.Sprintf("· %s 最新 %s（未披露 %s）", r.Name, fmtDate(r.Latest), fmtDate(s.Period)))
		}
	}
	for _, r := range s.Failed {
		info = append(info, fmt.Sprintf("· %s 拉取失败：%v", r.Name, r.Err))
	}
	for _, group := range [][]BankResult{s.Current, s.Stale} {
		for _, r := range group {
			if len(r.MissingFields) > 0 {
				info = append(info, fmt.Sprintf("· %s 字段缺失：%s（疑似数据源结构变化）", r.Name, strings.Join(r.MissingFields, ", ")))
			}
		}
	}
	if len(info) > 0 {
		b.WriteString("\nℹ️ 未更新/失败\n" + strings.Join(info, "\n") + "\n")
	}
	return b.String()
}

func writeAliases(b *strings.Builder, r BankResult) {
	for _, a := range r.Aliases {
		fmt.Fprintf(b, "  （%s 同 %s）\n", a, r.Symbol)
	}
}

func fmtBankLine(r BankResult) string {
	parts := []string{r.Name}
	for _, k := range Indicators {
		c := r.Ind[k]
		parts = append(parts, fmt.Sprintf("%s %s(环比%s/同比%s)", k.Label(), fmtVal(k, c.Value), fmtDelta(k, c.QoQ), fmtDelta(k, c.YoY)))
	}
	return strings.Join(parts, " ")
}

func fmtStat(k Indicator, st Stat) string {
	if st.N == 0 {
		return k.Label() + " 无数据"
	}
	return fmt.Sprintf("%s 均值 %s | 中位 %s | 最优 %s %s | 最差 %s %s", k.Label(),
		fmtVal(k, st.Mean), fmtVal(k, st.Median), st.Best, fmtVal(k, st.BestVal), st.Worst, fmtVal(k, st.WorstVal))
}

func fmtAlert(a Alert) string {
	if a.Kind == AlertDeterioration {
		return fmt.Sprintf("%s %s 环比 %spp（超 %.*fpp）", a.Name, a.Ind.Label(), fmtDelta(a.Ind, a.Value), decimals[a.Ind], a.Limit)
	}
	op := "<"
	if a.Ind.HigherIsWorse() {
		op = ">"
	}
	return fmt.Sprintf("%s %s %s %s %s", a.Name, a.Ind.Label(), fmtVal(a.Ind, a.Value), op, fmtVal(a.Ind, a.Limit))
}

func fmtVal(k Indicator, v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%.*f%%", decimals[k], v)
}

func fmtDelta(k Indicator, d float64) string {
	if math.IsNaN(d) {
		return "—"
	}
	return fmt.Sprintf("%+.*f", decimals[k], d)
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

// Split 按行累加切分，单段不超过 limit 个字符；超长单行独占一段，绝不在行内截断。
func Split(text string, limit int) []string {
	var chunks []string
	var cur strings.Builder
	n := 0
	flush := func() {
		if c := strings.TrimRight(cur.String(), "\n"); c != "" {
			chunks = append(chunks, c)
		}
		cur.Reset()
		n = 0
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		ln := utf8.RuneCountInString(strings.TrimRight(line, "\n"))
		if n > 0 && n+ln > limit {
			flush()
		}
		cur.WriteString(line)
		n += utf8.RuneCountInString(line)
	}
	flush()
	return chunks
}
```

（`n+ln` 用去掉换行的行长：段尾换行会被 flush 时 trim 掉，不计入推送长度。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/bank/ -v`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/render.go internal/bank/render_test.go
git commit -m "feat(bank): 报告渲染与 Telegram 分段"
```

---

### Task 6: `atlas bank report` 命令

**Files:**
- Create: `cmd/atlas/bank.go`
- Test: `cmd/atlas/bank_test.go`

**Interfaces:**
- Consumes: Task 1 `bank.Config`、`bank.LoadConfig`、`bank.Source`、`bank.Sender`、`(*Config).RankIndicator`；Task 2 `bank.NewEMSource`；Task 3 `bank.Alerts`；Task 4 `bank.Collect`、`bank.Summarize`；Task 5 `bank.Render`、`bank.Split`、`bank.MaxMessageRunes`；现有 `rootCmd`（`cmd/atlas/main.go:14`）、`loadConfigOrDefaults`（`cmd/atlas/export_ohlcv.go:283`）、`telegram.New`/`telegram.WithProxy`（`internal/notifier/telegram/telegram.go:62,40`）
- Produces: `bankReportDeps`、`executeBankReport(cfg *bank.Config, d bankReportDeps) (int, error)`、`buildBankSender() bank.Sender`

- [ ] **Step 1: 写失败测试** `cmd/atlas/bank_test.go`

```go
package main

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/bank"
)

type bankFakeSource struct {
	errs  map[string]error
	calls map[string]int
}

func (f *bankFakeSource) Fetch(symbol string) (bank.Series, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[symbol]++
	if err := f.errs[symbol]; err != nil {
		return bank.Series{}, err
	}
	p, _ := time.Parse("2006-01-02", "2026-06-30")
	return bank.Series{Obs: []bank.Observation{{Period: p, Values: [3]float64{1.62, 142, 10}}}}, nil
}

type bankFakeSender struct {
	msgs []string
	err  error
}

func (f *bankFakeSender) SendText(text string) error {
	f.msgs = append(f.msgs, text)
	return f.err
}

func bankTestCfg() *bank.Config {
	return &bank.Config{
		RankBy: "npl",
		Banks: []bank.BankCfg{
			{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
			{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
			{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
		},
		Thresholds: bank.ThresholdsCfg{
			NPLMax: 1.5, CoverageMin: 150, CET1Min: 8.5,
			Deterioration: bank.DeteriorationCfg{NPLUp: 0.1, CoverageDown: 20, CET1Down: 0.5},
		},
	}
}

func bankTestDeps(src bank.Source, snd bank.Sender, out *bytes.Buffer) bankReportDeps {
	now, _ := time.Parse("2006-01-02", "2026-10-01")
	return bankReportDeps{source: src, sender: snd, now: func() time.Time { return now }, out: out}
}

func TestBankReportAllOK(t *testing.T) {
	src, snd := &bankFakeSource{}, &bankFakeSender{}
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, &bytes.Buffer{}))
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Equal(t, 1, src.calls["600036.SH"], "A+H 只拉一次")
	require.Len(t, snd.msgs, 1)
	assert.Contains(t, snd.msgs[0], "🏦 银行关键指标月报 2026-10-01")
	assert.Contains(t, snd.msgs[0], "⚠️ 预警 (4)", "两主体各触发不良率与拨备两条")
	assert.Contains(t, snd.msgs[0], "（招商银行H 同 600036.SH）")
}

func TestBankReportPartialFailure(t *testing.T) {
	src := &bankFakeSource{errs: map[string]error{"601658.SH": errors.New("timeout")}}
	snd := &bankFakeSender{}
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, &bytes.Buffer{}))
	require.NoError(t, err)
	assert.Equal(t, 2, code, "部分失败退出码 2，报告照常推送")
	require.Len(t, snd.msgs, 1)
	assert.Contains(t, snd.msgs[0], "· 邮储银行 拉取失败：timeout")
}

func TestBankReportAllFailed(t *testing.T) {
	boom := errors.New("connection refused")
	src := &bankFakeSource{errs: map[string]error{"600036.SH": boom, "601658.SH": boom}}
	snd := &bankFakeSender{}
	_, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, &bytes.Buffer{}))
	require.Error(t, err)
	require.Len(t, snd.msgs, 1, "全部失败也推一条错误摘要")
	assert.Contains(t, snd.msgs[0], "全部 2 家拉取失败")
	assert.Contains(t, snd.msgs[0], "connection refused")
}

func TestBankReportSendError(t *testing.T) {
	snd := &bankFakeSender{err: errors.New("403")}
	_, err := executeBankReport(bankTestCfg(), bankTestDeps(&bankFakeSource{}, snd, &bytes.Buffer{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram 推送第 1/1 段失败")
}

func TestBankReportNilSenderPrints(t *testing.T) {
	var out bytes.Buffer
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(&bankFakeSource{}, nil, &out))
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "🏦 银行关键指标月报")
}

func TestBankCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"bank", "report"})
	require.NoError(t, err)
	assert.Equal(t, "report", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.InheritedFlags().Lookup("bank-config"), "继承自 bank 的持久 flag")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run 'Bank' -v`
Expected: 编译失败 `undefined: executeBankReport` / `bankReportDeps`。

- [ ] **Step 3: 实现 `cmd/atlas/bank.go`**

```go
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/bank"
	"github.com/newthinker/atlas/internal/notifier/telegram"
)

var (
	bankCfgPath string
	bankDryRun  bool
	bankExit    = os.Exit // 部分失败以退出码 2 结束；测试不经过 runBankReport
)

var bankCmd = &cobra.Command{
	Use:   "bank",
	Short: "银行股关键指标监控（不良率 / 拨备覆盖率 / CET1）",
	Long: `按 configs/bank-monitor.yaml 的银行列表拉取东方财富（经 aktools）三项监管指标，
生成预警 + 同期统计 + 排名报告并推送 Telegram
（设计: docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md）。`,
}

var bankReportCmd = &cobra.Command{
	Use:          "report",
	Short:        "拉取最新指标、生成报告并推送（launchd 每月入口）",
	SilenceUsage: true,
	RunE:         runBankReport,
}

func init() {
	bankCmd.PersistentFlags().StringVar(&bankCfgPath, "bank-config",
		"configs/bank-monitor.yaml", "bank monitor config path")
	bankReportCmd.Flags().BoolVar(&bankDryRun, "dry-run", false, "只打印报告到 stdout，不推送")
	bankCmd.AddCommand(bankReportCmd)
	rootCmd.AddCommand(bankCmd)
}

type bankReportDeps struct {
	source bank.Source
	sender bank.Sender // nil ⇒ 打印到 out
	now    func() time.Time
	out    io.Writer
}

func runBankReport(cmd *cobra.Command, _ []string) error {
	cfg, err := bank.LoadConfig(bankCfgPath)
	if err != nil {
		return err
	}
	d := bankReportDeps{
		source: bank.NewEMSource(cfg.Source.AktoolsURL),
		now:    time.Now,
		out:    cmd.OutOrStdout(),
	}
	if !bankDryRun {
		if s := buildBankSender(); s != nil {
			d.sender = s
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), "telegram 未启用或缺凭据，报告仅打印到 stdout")
		}
	}
	code, err := executeBankReport(cfg, d)
	if err != nil {
		return err
	}
	if code != 0 {
		bankExit(code)
	}
	return nil
}

// executeBankReport 返回退出码：0 全部成功；2 部分主体拉取失败（报告照常推送）。
// error 非 nil 对应退出码 1：全部失败（仍推送一条错误摘要）或推送失败。
func executeBankReport(cfg *bank.Config, d bankReportDeps) (int, error) {
	results := bank.Collect(cfg.Banks, d.source)
	failed := 0
	var alerts []bank.Alert
	for _, r := range results {
		if r.Err != nil {
			failed++
			continue
		}
		alerts = append(alerts, bank.Alerts(r, cfg.Thresholds)...)
	}
	date := d.now().Format("2006-01-02")
	if failed == len(results) {
		msg := fmt.Sprintf("🏦 银行关键指标月报 %s\n❌ 全部 %d 家拉取失败：%v", date, failed, results[0].Err)
		if err := deliver(d, []string{msg}); err != nil {
			return 1, err
		}
		return 1, fmt.Errorf("全部 %d 家银行拉取失败", failed)
	}
	report := bank.Render(bank.Summarize(results, cfg.RankIndicator()), alerts, d.now())
	if err := deliver(d, bank.Split(report, bank.MaxMessageRunes)); err != nil {
		return 1, err
	}
	if failed > 0 {
		return 2, nil
	}
	return 0, nil
}

func deliver(d bankReportDeps, chunks []string) error {
	if d.sender == nil {
		for _, c := range chunks {
			fmt.Fprintln(d.out, c)
			fmt.Fprintln(d.out)
		}
		return nil
	}
	for i, c := range chunks {
		if err := d.sender.SendText(c); err != nil {
			return fmt.Errorf("telegram 推送第 %d/%d 段失败: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

// buildBankSender 复用主配置 notifiers.telegram 凭据（同 buildCrisisSender）。
// 未配置或缺凭据 → nil（退化为打印）。
func buildBankSender() bank.Sender {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return nil
	}
	nc, ok := cfg.Notifiers["telegram"]
	if !ok || !nc.Enabled || nc.BotToken == "" || nc.ChatID == "" {
		return nil
	}
	return telegram.New(nc.BotToken, nc.ChatID, telegram.WithProxy(nc.Proxy))
}
```

- [ ] **Step 4: 运行确认通过，并跑整包回归（含 gate_wiring 静态检查）**

Run: `go test ./cmd/atlas/ ./internal/bank/ && go vet ./cmd/atlas/ ./internal/bank/`
Expected: `ok` ×2，vet 无输出。

- [ ] **Step 5: 本地构建冒烟**

Run: `go build -o /tmp/atlas-bank ./cmd/atlas && /tmp/atlas-bank bank report --help`
Expected: 帮助中含 `--dry-run` 与 `--bank-config`。

- [ ] **Step 6: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add cmd/atlas/bank.go cmd/atlas/bank_test.go
git commit -m "feat(bank): atlas bank report 命令（Telegram 推送，退出码 0/1/2）"
```

---

### Task 7: 集成冒烟测试、launchd 与上线前实测

**Files:**
- Create: `internal/bank/source_integration_test.go`
- Create: `deploy/launchd/com.newthinker.atlas.bank-monthly.plist`

**Interfaces:**
- Consumes: Task 2 `NewEMSource`、Task 3 `Analyze`、Task 6 命令
- Produces: 无（部署产物）

- [ ] **Step 1: 写集成测试** `internal/bank/source_integration_test.go`

```go
//go:build integration

package bank

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实调用本地 aktools：守护东方财富字段名（设计 §2 的 live 校验点）。
// 运行：go test -tags integration ./internal/bank/ -run Integration -v
func TestEMSourceIntegration600036(t *testing.T) {
	base := os.Getenv("ATLAS_AKTOOLS_URL")
	if base == "" {
		base = defaultAktoolsURL
	}
	s, err := NewEMSource(base).Fetch("600036.SH")
	require.NoError(t, err)
	assert.Empty(t, s.MissingFields, "字段名变化 ⇒ 更新 source.go 的 emFieldKeys")
	latest, ind := Analyze(s.Obs)
	assert.WithinDuration(t, time.Now(), latest, 270*24*time.Hour, "最新期应在近 9 个月内")
	for _, k := range Indicators {
		assert.False(t, math.IsNaN(ind[k].Value), "%s 应有值", k.Label())
	}
}
```

- [ ] **Step 2: 运行集成测试（需本机 aktools 在跑）**

Run: `go test -tags integration ./internal/bank/ -run Integration -v`
Expected: PASS。若报 `connection refused`，先 `launchctl list | grep aktools` 确认侧车在跑。

- [ ] **Step 3: 新建 `deploy/launchd/com.newthinker.atlas.bank-monthly.plist`**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.newthinker.atlas.bank-monthly</string>

  <key>ProgramArguments</key>
  <array>
    <string>/Users/zuowei/workspace/runtime/atlas/bin/atlas</string>
    <string>bank</string>
    <string>report</string>
    <string>--config</string>
    <string>/Users/zuowei/workspace/runtime/atlas/configs/config.yaml</string>
    <string>--bank-config</string>
    <string>/Users/zuowei/workspace/runtime/atlas/configs/bank-monitor.yaml</string>
  </array>

  <key>WorkingDirectory</key>
  <string>/Users/zuowei/workspace/runtime/atlas</string>

  <key>EnvironmentVariables</key>
  <dict>
    <!-- aktools 侧车在本机，不走代理；Telegram 代理由主配置 notifiers.telegram.proxy 提供 -->
    <key>no_proxy</key>
    <string>localhost,127.0.0.1</string>
    <key>PATH</key>
    <string>/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>

  <!-- 每月 1 日 09:00（本地 +0800）；休眠错过时 launchd 会在唤醒后补跑一次 -->
  <key>StartCalendarInterval</key>
  <dict>
    <key>Day</key><integer>1</integer>
    <key>Hour</key><integer>9</integer>
    <key>Minute</key><integer>0</integer>
  </dict>

  <key>RunAtLoad</key>
  <false/>

  <key>StandardOutPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/bank-monthly.out.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/bank-monthly.err.log</string>
</dict>
</plist>
```

Run: `plutil -lint deploy/launchd/com.newthinker.atlas.bank-monthly.plist`
Expected: `OK`。

- [ ] **Step 4: 真实数据 dry-run 核对**

Run: `go run ./cmd/atlas bank report --dry-run --bank-config configs/bank-monitor.yaml`
Expected: 打印报告；招商银行 2026-06-30 行为 `不良率 0.94% … 拨备覆盖率 385.1% … CET1 14.07%`，与设计 §2 实测表一致；含 `（招商银行H 同 600036.SH）`；无 `NaN`；退出码 0（`echo $?`）。

- [ ] **Step 5: 提交**（先跑 code-simplifier 与 detect-changes）

```bash
git add internal/bank/source_integration_test.go deploy/launchd/com.newthinker.atlas.bank-monthly.plist
git commit -m "feat(bank): aktools 集成冒烟测试与每月 launchd 任务"
```

- [ ] **Step 6: 交由人类执行的上线动作（不在自动执行范围内）**

以下会真实推送消息或改动运行时环境，须人类确认后手动执行：

```bash
# 1) 部署二进制与配置到运行时目录（按既有部署方式）
# 2) 真实推送一次，确认 Telegram 收到：
/Users/zuowei/workspace/runtime/atlas/bin/atlas bank report \
  --config /Users/zuowei/workspace/runtime/atlas/configs/config.yaml \
  --bank-config /Users/zuowei/workspace/runtime/atlas/configs/bank-monitor.yaml
# 3) 装载定时任务：
cp deploy/launchd/com.newthinker.atlas.bank-monthly.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.bank-monthly.plist
```

---

## Self-Review 记录

- **Spec 覆盖**：§1 范围/A+H → Task 1 校验 + Task 4 Collect；§2 数据源字段 → Task 2；§3.1 包结构 → Task 1–5（新增 `collect.go` 承接 A+H 去重，spec §3.1 未单列但 §4 要求）；§3.2 命令/注入/Sender → Task 6；§3.3 配置与校验 → Task 1；§4 口径（最新期/环比/同比/pp/严格不等号/A+H/统计期）→ Task 3、4；§5 报告格式与分段 → Task 5；§6 退出码表 → Task 6（配置非法由 `LoadConfig` 报错 → 1；Telegram 失败 → 1；部分失败 → 2）；§7 测试 → 各任务 + Task 7 集成；§8 部署 → Task 7；§9 风险 → Task 2 缺字段、Task 3 NaN。
- **与 spec 的有意差异**：集成测试标签用仓库既有的 `integration` 而非 `live`；报告头部把「未更新」与「失败」分开计数（spec 样例合写为「未更新：1 家」）。
- **类型一致性**：`Indicator`/`numIndicators`/`Change`/`BankResult`/`Alert{Name, Ind, Kind, Value, Limit}`/`Summary{Period, RankBy, Current, Stale, Failed, Stats}` 在 Task 1–6 中名称与字段一致；cmd 测试用 `[3]float64` 字面量构造 `Observation.Values`（`numIndicators` 未导出，值为 3）。
