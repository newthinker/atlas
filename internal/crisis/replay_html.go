package crisis

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

// ---------- 视图模型 ----------

type replayHTMLData struct {
	From, To       string
	GeneratedAt    string
	IndicatorNames []string
	Thresholds     [][2]string
	Timeline       template.HTML
	Charts         []indicatorChart
	Months         []monthRow
	Transitions    []transitionRow
}

type indicatorChart struct {
	Indicator string
	Note      string // 非空 = 全期无数据，省略图形
	SVG       template.HTML
}

type monthRow struct {
	Month     string
	Cells     []string // 按 AllIndicators 序：min / max / 月末值
	AmberDays int
	StaleDays int
	EndState  SystemState
}

type transitionRow struct {
	Date, Detail string
	From, To     SystemState
}

// stateColor SVG 色板，与 emoji 语义一致（绿/黄/橙/红）。
func stateColor(s SystemState) string {
	switch s {
	case StateWatch:
		return "#eab308"
	case StateBrewing:
		return "#f97316"
	case StateCrisis:
		return "#dc2626"
	}
	return "#16a34a"
}

// RenderReplayHTML 渲染自包含单文件详细报告（无外链、prefers-color-scheme
// 亮暗兼容）。始终全量日粒度，与 --form 无关（设计 §5）。
func RenderReplayHTML(cfg *Config, days []ReplayDay, sr SeriesReader) (string, error) {
	if len(days) == 0 {
		return "", fmt.Errorf("no replay days to render")
	}
	from, to := days[0].Date, days[len(days)-1].Date
	data := replayHTMLData{
		From: from, To: to,
		GeneratedAt:    time.Now().Format("2006-01-02 15:04"),
		IndicatorNames: AllIndicators,
		Thresholds:     thresholdRows(cfg),
		Timeline:       template.HTML(timelineSVG(days)),
		Months:         monthRows(days),
		Transitions:    transitionRows(days),
	}
	for _, ind := range AllIndicators {
		obs, err := sr.WindowSince(ind, from, to)
		if err != nil {
			return "", err
		}
		c := indicatorChart{Indicator: ind}
		if len(obs) == 0 {
			c.Note = "全期无观测数据"
			if ind == IndSOFREFFR {
				c.Note = "全期无观测数据（该指标自 2018-04 起才有数据）"
			}
		} else {
			c.SVG = template.HTML(lineChartSVG(cfg, ind, days, obs))
		}
		data.Charts = append(data.Charts, c)
	}
	var b strings.Builder
	if err := replayTmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// thresholdRows 阈值摘要：key=value 原样陈列 cfg 数值，不做语义解读。
func thresholdRows(cfg *Config) [][2]string {
	ic := cfg.Indicators
	return [][2]string{
		{IndVIX, fmt.Sprintf("amber=%.0f red=%.0f weekly_spike=%.0f%%", ic.VIX.Amber, ic.VIX.Red, ic.VIX.WeeklySpikePct*100)},
		{IndMOVE, fmt.Sprintf("amber=%.0f red=%.0f", ic.MOVE.Amber, ic.MOVE.Red)},
		{IndSOFREFFR, fmt.Sprintf("amber_bp=%+.0f×%d日 red_bp=%+.0f×%d日", ic.SOFREFFR.AmberBp, ic.SOFREFFR.AmberPersistDays, ic.SOFREFFR.RedBp, ic.SOFREFFR.RedPersistDays)},
		{IndHYOAS, fmt.Sprintf("amber_low_bp=%.0f amber_high_bp=%.0f red_bp=%.0f momentum_bp=%.0f/%d观测", ic.HYOAS.AmberLowBp, ic.HYOAS.AmberHighBp, ic.HYOAS.RedBp, ic.HYOAS.MomentumBp, ic.HYOAS.MomentumWindowObs)},
		{IndT10Y2Y, fmt.Sprintf("amber_bp=%+.0f steepening_bp=%+.0f/%d观测", ic.T10Y2Y.AmberBp, ic.T10Y2Y.SteepeningBp, ic.T10Y2Y.SteepeningLookbackObs)},
		{IndNFCI, fmt.Sprintf("green_below=%+.2f red_above=%+.2f", ic.NFCI.GreenBelow, ic.NFCI.RedAbove)},
		{IndUSDJPY, fmt.Sprintf("amber_wow=%.1f%% red_wow=%.1f%%（周环比，无水平阈值线）", ic.USDJPY.AmberWowPct*100, ic.USDJPY.RedWowPct*100)},
	}
}

// timelineSVG 状态时间线点阵：x=交易日序、每日一个色块；月份变更画刻度线，
// 首月与每年 1 月标注文本（多年跨度防拥挤）。
func timelineSVG(days []ReplayDay) string {
	const cell, gap, h, axis = 3, 1, 24, 14
	w := len(days)*(cell+gap) + 1
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="timeline" viewBox="0 0 %d %d" preserveAspectRatio="none" role="img" aria-label="状态时间线">`, w, h+axis)
	prevMonth := ""
	for i, d := range days {
		x := i * (cell + gap)
		fmt.Fprintf(&b, `<rect class="day" x="%d" y="0" width="%d" height="%d" fill="%s"><title>%s %s</title></rect>`,
			x, cell, h, stateColor(d.Res.State), d.Date, d.Res.State)
		if m := d.Date[:7]; m != prevMonth {
			if prevMonth != "" {
				fmt.Fprintf(&b, `<line x1="%d" y1="0" x2="%d" y2="%d" stroke="currentColor" stroke-width="0.5" opacity="0.4"/>`, x, x, h+3)
			}
			if prevMonth == "" || strings.HasSuffix(m, "-01") {
				fmt.Fprintf(&b, `<text x="%d" y="%d" class="lbl">%s</text>`, x, h+axis-3, m)
			}
			prevMonth = m
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

type thLine struct {
	v     float64
	color string
}

// thresholdLines 折线图阈值横线（usdjpy 周环比规则无水平阈值 → 无横线；
// nfci 只画 red_above；t10y2y 只画 amber_bp）。
func thresholdLines(cfg *Config, ind string) []thLine {
	const amber, red = "#eab308", "#dc2626"
	ic := cfg.Indicators
	switch ind {
	case IndVIX:
		return []thLine{{ic.VIX.Amber, amber}, {ic.VIX.Red, red}}
	case IndMOVE:
		return []thLine{{ic.MOVE.Amber, amber}, {ic.MOVE.Red, red}}
	case IndSOFREFFR:
		return []thLine{{ic.SOFREFFR.AmberBp, amber}, {ic.SOFREFFR.RedBp, red}}
	case IndHYOAS:
		return []thLine{{ic.HYOAS.AmberHighBp, amber}, {ic.HYOAS.RedBp, red}}
	case IndNFCI:
		return []thLine{{ic.NFCI.RedAbove, red}}
	case IndT10Y2Y:
		return []thLine{{ic.T10Y2Y.AmberBp, amber}}
	}
	return nil // usdjpy
}

// lineChartSVG 单指标折线：x=交易日序（与时间线对齐）、y=读数（formatReading
// 同款量纲标注上下界与阈值）；仅有观测的交易日入折线（缺口不补点）；
// STALE/NO_DATA 日在 x 轴上方打红点。
func lineChartSVG(cfg *Config, ind string, days []ReplayDay, obs []Observation) string {
	const w, h, pad = 720, 160, 36
	idx := map[string]int{}
	for i, d := range days {
		idx[d.Date] = i
	}
	xOf := func(i int) float64 {
		if len(days) == 1 {
			return pad
		}
		return pad + float64(i)*float64(w-2*pad)/float64(len(days)-1)
	}
	lo, hi := obs[0].Value, obs[0].Value
	for _, o := range obs {
		if o.Value < lo {
			lo = o.Value
		}
		if o.Value > hi {
			hi = o.Value
		}
	}
	lines := thresholdLines(cfg, ind)
	for _, t := range lines {
		if t.v < lo {
			lo = t.v
		}
		if t.v > hi {
			hi = t.v
		}
	}
	if hi == lo {
		hi = lo + 1
	}
	yOf := func(v float64) float64 { return pad + (hi-v)*float64(h-2*pad)/(hi-lo) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="%s">`, w, h, ind)
	fmt.Fprintf(&b, `<text x="2" y="%.1f" class="lbl">%s</text>`, yOf(hi)+4, formatReading(ind, hi))
	fmt.Fprintf(&b, `<text x="2" y="%.1f" class="lbl">%s</text>`, yOf(lo), formatReading(ind, lo))
	for _, t := range lines {
		y := yOf(t.v)
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="%s" stroke-dasharray="4 3" stroke-width="1"/>`, pad, y, w-pad/2, y, t.color)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" class="lbl" fill="%s">%s</text>`, w-pad/2+2, y+3, t.color, formatReading(ind, t.v))
	}
	var pts []string
	for _, o := range obs {
		i, ok := idx[o.Date]
		if !ok {
			continue // 非 vix 交易日的观测（周末等）不入折线
		}
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", xOf(i), yOf(o.Value)))
	}
	fmt.Fprintf(&b, `<polyline fill="none" stroke="currentColor" stroke-width="1.2" points="%s"/>`, strings.Join(pts, " "))
	for i, d := range days {
		s := d.Res.Results[ind].Status
		if s == StatusStale || s == StatusNoData {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%d" r="2" fill="#dc2626"><title>%s %s</title></circle>`, xOf(i), h-pad/2, d.Date, s)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// monthRows 月度汇总：月份 × {各指标 min/max/月末值、AMBER 天数、STALE 天数、
// 月末状态}。读数只取有新鲜观测的日（hasFreshReading，与总结极值同口径）。
func monthRows(days []ReplayDay) []monthRow {
	type agg struct {
		lo, hi, end float64
		seen        bool
	}
	var rows []monthRow
	var cur *monthRow
	var aggs map[string]*agg
	flush := func() {
		if cur == nil {
			return
		}
		for i, ind := range AllIndicators {
			a := aggs[ind]
			if !a.seen {
				cur.Cells[i] = "—"
				continue
			}
			cur.Cells[i] = fmt.Sprintf("%s / %s / %s",
				formatReading(ind, a.lo), formatReading(ind, a.hi), formatReading(ind, a.end))
		}
		rows = append(rows, *cur)
	}
	for _, d := range days {
		m := d.Date[:7]
		if cur == nil || cur.Month != m {
			flush()
			cur = &monthRow{Month: m, Cells: make([]string, len(AllIndicators))}
			aggs = map[string]*agg{}
			for _, ind := range AllIndicators {
				aggs[ind] = &agg{}
			}
		}
		for _, ind := range AllIndicators {
			r := d.Res.Results[ind]
			if !hasFreshReading(r.Status) {
				continue
			}
			a := aggs[ind]
			if !a.seen {
				a.lo, a.hi, a.seen = r.Value, r.Value, true
			} else {
				if r.Value < a.lo {
					a.lo = r.Value
				}
				if r.Value > a.hi {
					a.hi = r.Value
				}
			}
			a.end = r.Value
		}
		if d.Res.Detail.AmberCount > 0 {
			cur.AmberDays++
		}
		for _, ind := range AllIndicators {
			if d.Res.Results[ind].Status == StatusStale {
				cur.StaleDays++
				break
			}
		}
		cur.EndState = d.Res.State
	}
	flush()
	return rows
}

// transitionRows 状态转移明细：日期、FROM→TO、当日触发指标摘要（红/黄名单 +
// amber 计数，detail 摘要口径）。
func transitionRows(days []ReplayDay) []transitionRow {
	var rows []transitionRow
	for _, d := range days {
		if !d.Res.Transitioned() {
			continue
		}
		var reds, ambers []string
		for _, ind := range AllIndicators {
			switch d.Res.Results[ind].Status {
			case StatusRed:
				reds = append(reds, ind)
			case StatusAmber:
				ambers = append(ambers, ind)
			}
		}
		var parts []string
		if len(reds) > 0 {
			parts = append(parts, "红："+strings.Join(reds, "、"))
		}
		if len(ambers) > 0 {
			parts = append(parts, "黄："+strings.Join(ambers, "、"))
		}
		parts = append(parts, fmt.Sprintf("amber=%d", d.Res.Detail.AmberCount))
		rows = append(rows, transitionRow{
			Date: d.Date, From: d.Res.PrevState, To: d.Res.State,
			Detail: strings.Join(parts, " · "),
		})
	}
	return rows
}

var replayTmpl = template.Must(template.New("replay").Parse(`<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cassandra 危机回放 {{.From}} ~ {{.To}}</title>
<style>
:root { color-scheme: light dark; }
body { font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif; margin: 24px auto; max-width: 960px; padding: 0 16px; background: #fff; color: #1f2937; }
h1 { font-size: 1.3rem; } h2 { font-size: 1.05rem; margin-top: 2rem; } h3 { font-size: 0.9rem; margin: 1.2rem 0 0.3rem; }
table { border-collapse: collapse; font-size: 0.78rem; white-space: nowrap; }
th, td { border: 1px solid #6b728066; padding: 4px 8px; text-align: right; }
th { background: #f3f4f6; }
td:first-child, th:first-child { text-align: left; }
.scroll { overflow-x: auto; }
svg.timeline { width: 100%; height: 52px; display: block; }
svg.chart { width: 100%; height: auto; display: block; }
.lbl { font-size: 7px; fill: currentColor; }
.meta { color: #6b7280; font-size: 0.85rem; }
footer { margin-top: 2rem; color: #6b7280; font-size: 0.8rem; border-top: 1px solid #6b728066; padding-top: 8px; }
@media (prefers-color-scheme: dark) {
  body { background: #111827; color: #e5e7eb; }
  th { background: #1f2937; }
}
</style>
</head>
<body>
<h1>Cassandra 危机监控历史回放 · {{.From}} ~ {{.To}}</h1>
<p class="meta">生成时间 {{.GeneratedAt}} · 全量日粒度（与 --form 无关） · 阈值为当前配置，非事后调参</p>

<h2>当前配置阈值</h2>
<div class="scroll"><table>
<tr><th>指标</th><th style="text-align:left">阈值</th></tr>
{{range .Thresholds}}<tr><td>{{index . 0}}</td><td style="text-align:left">{{index . 1}}</td></tr>
{{end}}</table></div>

<h2>状态时间线</h2>
<p class="meta">绿 NORMAL · 黄 WATCH · 橙 BREWING · 红 CRISIS（悬停色块看日期）</p>
{{.Timeline}}

<h2>指标走势</h2>
{{range .Charts}}<h3>{{.Indicator}}</h3>
{{if .Note}}<p class="meta">{{.Note}}</p>{{else}}{{.SVG}}{{end}}
{{end}}

<h2>月度汇总</h2>
<p class="meta">指标单元格 = 期间 min / max / 月末值（— 表示当月无新鲜读数）</p>
<div class="scroll"><table>
<tr><th>月份</th>{{range .IndicatorNames}}<th>{{.}}</th>{{end}}<th>AMBER 天数</th><th>STALE 天数</th><th>月末状态</th></tr>
{{range .Months}}<tr><td>{{.Month}}</td>{{range .Cells}}<td>{{.}}</td>{{end}}<td>{{.AmberDays}}</td><td>{{.StaleDays}}</td><td>{{.EndState}}</td></tr>
{{end}}</table></div>

<h2>状态转移明细</h2>
<div class="scroll"><table>
<tr><th>日期</th><th>转移</th><th style="text-align:left">当日触发指标</th></tr>
{{range .Transitions}}<tr><td>{{.Date}}</td><td>{{.From}} → {{.To}}</td><td style="text-align:left">{{.Detail}}</td></tr>
{{end}}</table></div>

<footer>历史回放，非实时告警；阈值为当前配置，非事后调参。风险状态提示（概率语言），非交易信号。</footer>
</body>
</html>
`))
