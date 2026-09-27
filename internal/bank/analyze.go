package bank

import (
	"math"
	"time"
)

// Change 是某指标最近一个有效值及其变动（百分点）；无法计算为 NaN。
// 季报常不披露 CET1 等指标，故各指标按自己的最近非 NaN 期回退，Period 标明该值所属报告期
// （全 NaN ⇒ 零值），可能早于 BankResult.Latest。
type Change struct {
	Value, QoQ, YoY float64
	Period          time.Time
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

// Alert.Value：AlertLevel 为指标值，AlertDeterioration 为环比变动（pp）；Limit 为对应阈值；
// Period 为该指标 Change.Period（回退值所属报告期）。
type Alert struct {
	Name   string
	Ind    Indicator
	Kind   AlertKind
	Value  float64
	Limit  float64
	Period time.Time
}

// Analyze 对按期升序的序列逐指标取最近非 NaN 值，环比对该指标此前最近的非 NaN 值，
// 同比对上年同一报告期（该期缺失或为 NaN 则无同比）。Latest 为各指标 Period 的最大值。
func Analyze(obs []Observation) (time.Time, [numIndicators]Change) {
	var latest time.Time
	var ind [numIndicators]Change
	for _, k := range Indicators {
		ind[k] = analyzeIndicator(obs, k)
		if ind[k].Period.After(latest) {
			latest = ind[k].Period
		}
	}
	return latest, ind
}

func analyzeIndicator(obs []Observation, k Indicator) Change {
	c := Change{Value: math.NaN(), QoQ: math.NaN(), YoY: math.NaN()}
	cur := lastValid(obs, len(obs), k)
	if cur < 0 {
		return c
	}
	v := obs[cur].Values[k]
	c.Value, c.Period = v, obs[cur].Period
	if prev := lastValid(obs, cur, k); prev >= 0 {
		c.QoQ = round4(v - obs[prev].Values[k])
	}
	yoyTarget := c.Period.AddDate(-1, 0, 0)
	for _, o := range obs[:cur] {
		if o.Period.Equal(yoyTarget) {
			c.YoY = round4(v - o.Values[k])
		}
	}
	return c
}

// lastValid 返回 obs[:end] 中指标 k 最后一个非 NaN 值的下标，没有则 -1。
func lastValid(obs []Observation, end int, k Indicator) int {
	for i := end - 1; i >= 0; i-- {
		if !math.IsNaN(obs[i].Values[k]) {
			return i
		}
	}
	return -1
}

// round4 消除十进制小数相减的二进制舍入误差：如 0.67-0.57 在 float64 下是 0.10000000000000009，
// 会让「恰等于阈值」越过严格不等号；四舍五入到 1e-4 后与阈值字面量是同一个 float64。
func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// Alerts 判定阈值与恶化预警（均针对各指标回退后的值）；NaN 参与的比较恒假，故缺失值不预警。
func Alerts(r BankResult, t ThresholdsCfg) []Alert {
	level := [numIndicators]float64{t.NPLMax, t.CoverageMin, t.CET1Min}
	deter := [numIndicators]float64{t.Deterioration.NPLUp, t.Deterioration.CoverageDown, t.Deterioration.CET1Down}
	var out []Alert
	for _, k := range Indicators {
		c := r.Ind[k]
		if worseThan(k, c.Value, level[k]) {
			out = append(out, Alert{r.Name, k, AlertLevel, c.Value, level[k], c.Period})
		}
		if worsening(k, c.QoQ) > deter[k] {
			out = append(out, Alert{r.Name, k, AlertDeterioration, c.QoQ, deter[k], c.Period})
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
