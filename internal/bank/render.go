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

// decimals 是三项指标统一的展示精度：源数据有 2 位小数，更粗的精度会让
// 149.96 显示成「150.0% < 150.0%」这种字面上自相矛盾的预警行。
const decimals = 2

// Render 生成纯文本报告（SendText 不设 parse_mode，故用列表式而非表格）。
func Render(s Summary, alerts []Alert, now time.Time) string {
	var b strings.Builder
	total := len(s.Current) + len(s.Stale) + len(s.Ahead) + len(s.Failed)
	fmt.Fprintf(&b, "🏦 银行关键指标月报 %s\n", now.Format("2006-01-02"))
	fmt.Fprintf(&b, "覆盖 %d 家（统计期 %s：%d 家；未更新 %d；失败 %d）\n",
		total, fmtDate(s.Period), len(s.Current), len(s.Stale), len(s.Failed))

	fmt.Fprintf(&b, "\n⚠️ 预警 (%d)\n", len(alerts))
	if len(alerts) == 0 {
		b.WriteString("无\n")
	}
	for _, a := range alerts {
		b.WriteString("· " + fmtAlert(a))
		if !a.Period.Equal(s.Period) {
			b.WriteString("（" + fmtDate(a.Period) + "）")
		}
		b.WriteString("\n")
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

	var info strings.Builder
	infoLine := func(r BankResult, format string, args ...any) {
		fmt.Fprintf(&info, "· "+format+"\n", args...)
		writeAliases(&info, r)
	}
	for _, r := range s.Stale {
		if !r.Latest.IsZero() {
			infoLine(r, "%s 最新 %s（未披露 %s）", r.Name, fmtDate(r.Latest), fmtDate(s.Period))
		}
	}
	for _, r := range s.Stale {
		if r.Latest.IsZero() {
			infoLine(r, "%s 无可用数据", r.Name)
		}
	}
	for _, r := range s.Failed {
		infoLine(r, "%s 拉取失败：%v", r.Name, r.Err)
	}
	for _, group := range [][]BankResult{s.Current, s.Stale, s.Ahead} {
		for _, r := range group {
			if len(r.MissingFields) > 0 {
				fmt.Fprintf(&info, "· %s 字段缺失：%s（疑似数据源结构变化）\n", r.Name, strings.Join(r.MissingFields, ", "))
			}
		}
	}
	for _, r := range s.Ahead {
		infoLine(r, "%s 已披露 %s（晚于统计期）", r.Name, fmtDate(r.Latest))
	}
	if info.Len() > 0 {
		b.WriteString("\nℹ️ 未更新/失败\n" + info.String())
	}
	return b.String()
}

// writeAliases 为共用该主体数据的 H 股等条目写附注行；主体不论处于哪一组都要写，
// 否则配置里的条目会从报告中无痕消失。
func writeAliases(b *strings.Builder, r BankResult) {
	for _, a := range r.Aliases {
		fmt.Fprintf(b, "  （%s 同 %s）\n", a, r.Symbol)
	}
}

func fmtBankLine(r BankResult) string {
	parts := []string{r.Name}
	for _, k := range Indicators {
		c := r.Ind[k]
		v := fmtVal(c.Value)
		if !math.IsNaN(c.Value) && !c.Period.Equal(r.Latest) {
			v += "（截至 " + fmtDate(c.Period) + "）"
		}
		parts = append(parts, fmt.Sprintf("%s %s(环比%s/同比%s)", k.Label(), v, fmtDelta(c.QoQ), fmtDelta(c.YoY)))
	}
	return strings.Join(parts, " ")
}

func fmtStat(k Indicator, st Stat) string {
	if st.N == 0 {
		return k.Label() + " 无数据"
	}
	return fmt.Sprintf("%s 均值 %s | 中位 %s | 最优 %s %s | 最差 %s %s", k.Label(),
		fmtVal(st.Mean), fmtVal(st.Median), st.Best, fmtVal(st.BestVal), st.Worst, fmtVal(st.WorstVal))
}

func fmtAlert(a Alert) string {
	if a.Kind == AlertDeterioration {
		return fmt.Sprintf("%s %s 环比 %spp（超 %.*fpp）", a.Name, a.Ind.Label(), fmtDelta(a.Value), decimals, a.Limit)
	}
	op := "<"
	if a.Ind.HigherIsWorse() {
		op = ">"
	}
	return fmt.Sprintf("%s %s %s %s %s", a.Name, a.Ind.Label(), fmtVal(a.Value), op, fmtVal(a.Limit))
}

func fmtVal(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%.*f%%", decimals, v)
}

func fmtDelta(d float64) string {
	if math.IsNaN(d) {
		return "—"
	}
	return fmt.Sprintf("%+.*f", decimals, d)
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
		// 段尾换行在 flush 时会被去掉，故用去掉换行的行长判断加入后是否超限。
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
