package pricewatch

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Trigger 是本轮需要提醒的一条规则。
type Trigger struct {
	Rule             Rule
	X                float64 // 百分数
	Tier, SentInTier int     // 消息里写的档位（档内重复提醒为当前档位，AD-28 P16）与本档已发次数
	Ref              float64 // 窗口最高价（drop）或最低价（rise）
	ShortDays        int     // >0 时追加「（窗口数据仅 N 天）」
}

// Overview 是多周期概况，均为百分数。
type Overview struct{ Chg24h, Chg7d, Chg30d, FromATH, From4yLow float64 }

// SideAdvice 是 AI 对一个方向的建议。
type SideAdvice struct {
	Side        string  `json:"side"`
	Verdict     string  `json:"verdict"`
	TargetPrice float64 `json:"target_price"`
	Term        string  `json:"term"`
	Reasoning   string  `json:"reasoning"`
}

// Alert 是一条提醒消息的全部内容。
type Alert struct {
	Asset    Asset
	Price    float64
	Source   string
	Triggers []Trigger
	Overview Overview
	Advice   []SideAdvice // nil（或空）表示 AI 不可用
}

// HeartbeatRow 是心跳消息里的一个资产。
type HeartbeatRow struct {
	Asset  Asset
	Price  float64
	LastOK time.Time // 零值表示还没有成功过
	Active []string  // 形如 "day_drop 第2档"
}

// maxMessageUTF16 是 Telegram 单条消息的长度上限（UTF-16 码元，AD-24）。
const maxMessageUTF16 = 4096

const truncatedMark = "…（已截断）"

var windowNames = map[Window]string{
	Window24h: "1天", Window7d: "1周", Window30d: "30天", Window4y: "4年", WindowAll: "历史",
}

// windowRefLabels 带上与「较」「最高价」之间的空格，使中文与数字/字母之间留空。
var windowRefLabels = map[Window]string{
	Window24h: " 24h ", Window7d: " 7d ", Window30d: " 30d ", Window4y: " 4 年", WindowAll: "历史",
}

// FormatAlert 按 spec §8 生成提醒消息，并保证不超过 4096 个 UTF-16 码元（AD-24、AD-31）：
// 超长时先截【AI 分析】，仍超则截【AI 结论】，【触发规则】与【多周期】完整保留；
// 若非 AI 部分本身就超长，最后对整条消息兜底截断。
func FormatAlert(a Alert) string {
	hasAI := len(a.Advice) > 0
	conclusion := "AI 分析不可用"
	var analysis string
	if hasAI {
		lines := make([]string, len(a.Advice))
		for i, s := range a.Advice {
			lines[i] = adviceLine(s)
		}
		conclusion = bySide(lines)
		if len(a.Advice) == 1 {
			analysis = a.Advice[0].Reasoning
		} else {
			for i, s := range a.Advice {
				lines[i] = s.Side + "：" + s.Reasoning
			}
			analysis = bySide(lines)
		}
	}
	ov := a.Overview
	header := fmt.Sprintf("%s %s %s %s（%s）", alertIcon(a.Triggers), a.Asset.Symbol, formatPrice(a.Price), a.Asset.Quote, a.Source)
	body := strings.Join([]string{
		"【触发规则】\n" + triggerLines(a.Triggers),
		fmt.Sprintf("【多周期】24h %s · 7d %s · 30d %s · 距历史最高 %s · 距 4 年最低 %s",
			formatPct(ov.Chg24h, 1), formatPct(ov.Chg7d, 1), formatPct(ov.Chg30d, 1),
			formatPct(ov.FromATH, 1), formatPct(ov.From4yLow, 0)),
	}, "\n\n")
	assemble := func() string {
		parts := []string{header, "【AI 结论】" + conclusion, body}
		if hasAI {
			parts = append(parts, "【AI 分析】"+analysis, "AI 建议仅供参考")
		}
		return strings.Join(parts, "\n\n")
	}

	msg := assemble()
	if !hasAI {
		return capUTF16(msg)
	}
	for _, field := range []*string{&analysis, &conclusion} {
		over := utf16Len(msg) - maxMessageUTF16
		if over <= 0 {
			return msg
		}
		*field = truncateUTF16(*field, utf16Len(*field)-over-utf16Len(truncatedMark)) + truncatedMark
		msg = assemble()
	}
	return capUTF16(msg)
}

// capUTF16 把超过上限的消息截到恰好不超限，并以截断标记结尾。
func capUTF16(msg string) string {
	if utf16Len(msg) <= maxMessageUTF16 {
		return msg
	}
	return truncateUTF16(msg, maxMessageUTF16-utf16Len(truncatedMark)) + truncatedMark
}

// bySide 只有一行时直接接在段标题后；多行时从标题下一行起逐行列出。
func bySide(lines []string) string {
	if len(lines) == 1 {
		return lines[0]
	}
	return "\n" + strings.Join(lines, "\n")
}

func adviceLine(s SideAdvice) string {
	parts := []string{s.Side + "：" + s.Verdict}
	if s.TargetPrice > 0 {
		parts = append(parts, "建议目标价 "+formatPrice(s.TargetPrice))
	}
	if s.Term != "" {
		parts = append(parts, "期限 "+s.Term)
	}
	return strings.Join(parts, " · ")
}

// directions 报告 ts 中是否有跌（非 rise_from_low 一律按跌）与涨的规则。
func directions(ts []Trigger) (drop, rise bool) {
	for _, t := range ts {
		if t.Rule.Type == RiseFromLow {
			rise = true
		} else {
			drop = true
		}
	}
	return drop, rise
}

// refVerb 返回规则类型对应的参照价名称与涨跌动词。
func refVerb(t RuleType) (ref, verb string) {
	if t == RiseFromLow {
		return "最低价", "上涨"
	}
	return "最高价", "下跌"
}

func alertIcon(ts []Trigger) string {
	drop, rise := directions(ts)
	switch {
	case drop && rise:
		return "🔀"
	case rise:
		return "🔺"
	default:
		return "🔻"
	}
}

func triggerLines(ts []Trigger) string {
	lines := make([]string, len(ts))
	for i, t := range ts {
		ref, verb := refVerb(t.Rule.Type)
		tier := fmt.Sprintf("第 %d 档", t.Tier)
		if t.SentInTier > 1 {
			tier += fmt.Sprintf("，第 %d/%d 次提醒", t.SentInTier, t.Rule.Notify.MaxPerTier)
		}
		line := fmt.Sprintf("· %s窗口：较%s%s %s %s %.1f%%（%s）",
			windowNames[t.Rule.Window], windowRefLabels[t.Rule.Window], ref, formatPrice(t.Ref), verb, t.X, tier)
		if t.ShortDays > 0 {
			line += fmt.Sprintf("（窗口数据仅 %d 天）", t.ShortDays)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// FormatHeartbeat 生成每日心跳消息（spec §9）。时间按 now 的时区显示。
func FormatHeartbeat(now time.Time, rows []HeartbeatRow, sent24h int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💓 监控仍在运行（%s）", now.Format("2006-01-02 15:04"))
	for _, r := range rows {
		lastOK := "无"
		if !r.LastOK.IsZero() {
			lastOK = r.LastOK.In(now.Location()).Format("15:04")
		}
		active := "无"
		if len(r.Active) > 0 {
			active = strings.Join(r.Active, "、")
		}
		fmt.Fprintf(&b, "\n· %s %s %s %s · 最近成功 %s · 触发中：%s",
			r.Asset.Name, r.Asset.Symbol, formatPrice(r.Price), r.Asset.Quote, lastOK, active)
	}
	fmt.Fprintf(&b, "\n过去 24 小时发出提醒 %d 条", sent24h)
	return b.String()
}

// FormatHealth 生成健康告警：down=true 为「监控异常」，false 为「已恢复」。detail 为空时只有一行。
// 结果不超过 4096 个 UTF-16 码元。
func FormatHealth(down bool, subject, detail string) string {
	s := "✅ 已恢复：" + subject
	if down {
		s = "⚠️ 监控异常：" + subject
	}
	if detail != "" {
		s += "\n" + detail
	}
	return capUTF16(s) // detail 可能是多段错误的拼接（N1），截尾保留标题
}

// formatPrice：带千分位；≥1000 不保留小数，≥1 保留 2 位，<1 保留 4 位。
func formatPrice(p float64) string {
	decimals := 4
	switch {
	case p >= 1000:
		decimals = 0
	case p >= 1:
		decimals = 2
	}
	s := strconv.FormatFloat(p, 'f', decimals, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	for i := len(intPart) - 3; i > 0; i -= 3 {
		intPart = intPart[:i] + "," + intPart[i:]
	}
	if frac != "" {
		return intPart + "." + frac
	}
	return intPart
}

// formatPct 带正负号，负号用 U+2212；四舍五入后为 0 时不带符号（不出现 −0.0）。
func formatPct(x float64, decimals int) string {
	s := strconv.FormatFloat(math.Abs(x), 'f', decimals, 64)
	if strings.Trim(s, "0.") == "" {
		return s + "%"
	}
	if x < 0 {
		return "−" + s + "%"
	}
	return "+" + s + "%"
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// truncateUTF16 取 s 的最长前缀，使其 UTF-16 码元数不超过 max；不劈开代理对。
func truncateUTF16(s string, max int) string {
	n := 0
	for i, r := range s {
		n += utf16.RuneLen(r)
		if n > max {
			return s[:i]
		}
	}
	return s
}
