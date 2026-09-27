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
