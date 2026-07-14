package crisis

import (
	"fmt"
	"sort"
	"strings"
)

// Trend is one indicator's monthly-summary window (通知设计 §8).
type Trend struct {
	Window []Observation // 近 21 观测（可短）
	Delta  float64       // 当前 − 窗口首
}

// NotifyContext 收拢渲染输入，cmd 层组装、渲染保持纯函数（通知设计 §8）。
type NotifyContext struct {
	Res          *DayResult
	StateDays    int                   // 变更消息=前状态持续日数；否则=当前状态含当日（补充决策 6）
	SummaryDue   bool                  // 月报/周报到期（cmd 计算）
	NewStale     []string              // 今日新进入 STALE 的指标（P2 去重后）
	StaleLastObs map[string]string     // NewStale 指标的最后观测日（补充决策 1）
	PrevDay      map[string]Evaluation // 前一评估日指标行（较昨日 & NewStale 依据）
	ClearStreak  int                   // any_trigger=false 连续日数，含当日（周报退出进度）
	Trends       map[string]Trend      // 仅月报到期时组装
}

// monthDay renders YYYY-MM-DD as MM-DD（首行规范，通知设计 §3）。
func monthDay(date string) string {
	if len(date) == 10 {
		return date[5:]
	}
	return date
}

// stateRank 严重度序（通知设计 §2）：NORMAL < WATCH < BREWING < CRISIS。
func stateRank(s SystemState) int {
	switch s {
	case StateWatch:
		return 1
	case StateBrewing:
		return 2
	case StateCrisis:
		return 3
	}
	return 0
}

// indicatorLine 渲染一行指标（通知设计 §5 示例格式）：
// 🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)
func indicatorLine(cfg *Config, r IndicatorResult) string {
	if note := nonColorNote(r.Status); note != "" {
		head := fmt.Sprintf("⚪ %s %s", layerName(r.Indicator), r.Indicator)
		if r.Status == StatusNoData { // 无读数：直接空格接说明（boundary[0]）
			return head + " " + note
		}
		return head + " " + formatReading(r.Indicator, r.Value) + " · " + note
	}
	head := fmt.Sprintf("%s %s %s %s", statusEmoji(r.Status), layerName(r.Indicator),
		r.Indicator, formatReading(r.Indicator, r.Value))
	var parts []string
	if showPct5y(r.Indicator) && r.Pct5y >= 0 {
		parts = append(parts, "5y分位 "+formatPct5y(r.Pct5y))
	}
	if r.Indicator == IndSOFREFFR && severity(r.Status) >= severity(StatusAmber) && r.PersistDays > 0 {
		parts = append(parts, fmt.Sprintf("持续 %d 个交易日", r.PersistDays))
	}
	if r.Indicator == IndUSDJPY && severity(r.Status) >= severity(StatusAmber) &&
		r.WowOK && r.Wow <= cfg.Indicators.USDJPY.AmberWowPct {
		parts = append(parts, fmt.Sprintf("周跌 %.1f%%", -r.Wow*100))
	}
	if t := tagText(r.Tag); t != "" {
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return head
	}
	return head + " · " + strings.Join(parts, " · ")
}

// splitZones 通知设计 §6.2：异常区 = 🔴🟡，严重度降序、同级按冰山层序、再按
// AllIndicators 序；其余区 = 🟢 后接 ⚪（已退出共振，视觉最弱、殿后）。
func splitZones(res *DayResult) (abnormal, rest []IndicatorResult) {
	var noncolor []IndicatorResult
	for _, ind := range AllIndicators {
		r := res.Results[ind]
		switch {
		case severity(r.Status) >= severity(StatusAmber):
			abnormal = append(abnormal, r)
		case isColor(r.Status):
			rest = append(rest, r)
		default:
			noncolor = append(noncolor, r)
		}
	}
	sort.Slice(abnormal, func(i, j int) bool {
		a, b := abnormal[i], abnormal[j]
		if severity(a.Status) != severity(b.Status) {
			return severity(a.Status) > severity(b.Status) // 一级：严重度降序
		}
		if icebergRank(a.Indicator) != icebergRank(b.Indicator) {
			return icebergRank(a.Indicator) < icebergRank(b.Indicator) // 二级：冰山层序
		}
		return indicatorIndex(a.Indicator) < indicatorIndex(b.Indicator) // 三级：AllIndicators 序（显式，不靠排序稳定性）
	})
	return abnormal, append(rest, noncolor...)
}

// indicatorIndex is the position of ind in AllIndicators — the third-level
// abnormal-zone tiebreak (通知设计 §6.2). Made explicit so the ordering is a
// total comparator rather than an artifact of sort stability (which n≤7 slices
// can't distinguish stable-vs-unstable and so can't lock via mutation).
func indicatorIndex(ind string) int {
	for i, x := range AllIndicators {
		if x == ind {
			return i
		}
	}
	return len(AllIndicators)
}

// bodyZones 渲染异常区 + 其余区（通知设计 §4 骨架第三段）。
func bodyZones(cfg *Config, res *DayResult, abnormalTitle string) string {
	abnormal, rest := splitZones(res)
	lines := func(rs []IndicatorResult) string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = indicatorLine(cfg, r)
		}
		return strings.Join(out, "\n")
	}
	restTitle := "其余指标："
	if len(abnormal) == 0 {
		if allGreen(rest) { // 无异常且 7 指标全 GREEN 才叫全绿；含 ⚪ 仍用"其余指标："（补充决策 5）
			restTitle = "7 指标全绿："
		}
		return restTitle + "\n" + lines(rest)
	}
	return abnormalTitle + "\n" + lines(abnormal) + "\n\n" + restTitle + "\n" + lines(rest)
}

// allGreen reports whether every result is GREEN — the "7 指标全绿" gate. rest
// from splitZones merges greens and ⚪ (STALE/NO_DATA/季末抑制); a single ⚪
// disqualifies the all-green title (补充决策 5).
func allGreen(rs []IndicatorResult) bool {
	for _, r := range rs {
		if r.Status != StatusGreen {
			return false
		}
	}
	return true
}

// notifyFooter 页脚（通知设计 §4.2）：只挂结构化家族；速报是事实陈述不带页脚。
// 措辞集中为常量，禁词与"非交易信号"由单测全家族兜底（通知设计 §7）。
const notifyFooter = "\n—\n风险状态提示（概率语言），非交易信号；指标基于有限历史样本，可能失效；操作决策不在本模块范围。"

const crisisSentence = "情绪层双红：危机进行中。此阶段执行预案而非预测。"

// semanticSentences 语义句查表（通知设计 §4.1），键 = "FROM→TO"（状态机可达的
// 全部转移）。含天数处用 %d 占位，semanticSentence 注入 state_machine 配置值。
var semanticSentences = map[string]string{
	"NORMAL→WATCH":   "领先层或多指标共振异常。观察期：提高警觉，尚无行动含义。",
	"WATCH→BREWING":  "信用与流动性双红共振。历史样本中，此组合出现后 3–12 个月内系统性风险显著抬升（样本量小，存在失效可能）。",
	"NORMAL→CRISIS":  crisisSentence,
	"WATCH→CRISIS":   crisisSentence,
	"BREWING→CRISIS": crisisSentence,
	"CRISIS→WATCH":   "情绪层连续 %d 个交易日回落至绿。危机状态解除，转入观察期。",
	"BREWING→WATCH":  "信用/流动性共振解除并稳定 %d 个交易日。回到观察期。",
	"WATCH→NORMAL":   "全部触发条件解除并稳定 %d 个交易日。回到常态。",
}

// semanticSentence 查表并注入 %d（避免 YAML 调参后文案失真，通知设计 §4.1）。
// 未知转移返回空串（渲染时省略语义句段）。
func semanticSentence(cfg *Config, from, to SystemState) string {
	s, ok := semanticSentences[string(from)+"→"+string(to)]
	if !ok {
		return ""
	}
	sm := cfg.StateMachine
	switch {
	case from == StateCrisis && to == StateWatch:
		return fmt.Sprintf(s, sm.CrisisExitDays)
	case from == StateBrewing && to == StateWatch:
		return fmt.Sprintf(s, sm.BrewingExitDays)
	case from == StateWatch && to == StateNormal:
		return fmt.Sprintf(s, sm.WatchExitDays)
	}
	return s
}

// renderTransition 消息 1/2：状态升级/降级（通知设计 §5.1/§5.2）。
func renderTransition(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	var first, title, tail string
	if stateRank(res.State) > stateRank(res.PrevState) {
		prefix := "[P1] ⚠️"
		if res.State == StateBrewing || res.State == StateCrisis {
			prefix = "[P0] 🚨"
		}
		first = fmt.Sprintf("%s 状态升级 %s → %s · %s", prefix, res.PrevState, res.State, monthDay(res.Date))
		title = "触发共振："
		tail = fmt.Sprintf("%s 已持续 %d 个评估日 → %s · 下一评估：下一交易日",
			res.PrevState, nc.StateDays, res.State)
	} else {
		first = fmt.Sprintf("[P1] ✅ 状态解除 %s → %s · %s", res.PrevState, res.State, monthDay(res.Date))
		title = "仍异常："
		tail = fmt.Sprintf("%s 共持续 %d 个评估日 · 下一评估：下一交易日", res.PrevState, nc.StateDays)
	}
	parts := []string{first}
	if s := semanticSentence(cfg, res.PrevState, res.State); s != "" {
		parts = append(parts, s)
	}
	parts = append(parts, bodyZones(cfg, res, title), tail)
	return strings.Join(parts, "\n\n") + notifyFooter
}

func colorWord(s Status) string {
	switch s {
	case StatusGreen:
		return "绿"
	case StatusAmber:
		return "黄"
	case StatusRed:
		return "红"
	}
	return "白" // STALE / NO_DATA / SUPPRESSED_SEASONAL
}

// diffLine "较昨日"差异行（通知设计 §6.5）：状态迁移优先（usdjpy 转黄（原绿）），
// 读数变化仅列当日异常区指标（hy_oas +6bp）；完全无变化 → 无变化。
// 读数无变化用浮点直等判断（d != 0）——PrevDay.Value 与当日 Value 同出一个 store 的
// float 读写、无精度损失，故"完全相等=无变化"成立，刻意不引入 epsilon（补充决策）。
func diffLine(nc NotifyContext) string {
	abnormal, _ := splitZones(nc.Res)
	inAbnormal := map[string]bool{}
	for _, r := range abnormal {
		inAbnormal[r.Indicator] = true
	}
	var parts []string
	for _, ind := range AllIndicators {
		prev, ok := nc.PrevDay[ind]
		if !ok {
			continue
		}
		cur := nc.Res.Results[ind]
		if prev.Status != cur.Status {
			parts = append(parts, fmt.Sprintf("%s 转%s（原%s）", ind, colorWord(cur.Status), colorWord(prev.Status)))
			continue
		}
		if d := cur.Value - prev.Value; inAbnormal[ind] && d != 0 {
			parts = append(parts, ind+" "+formatDelta(ind, d))
		}
	}
	if len(parts) == 0 {
		return "较昨日：无变化"
	}
	return "较昨日：" + strings.Join(parts, " · ")
}

// renderDaily 消息 3：BREWING/CRISIS 无变更日报（通知设计 §5.3）。
func renderDaily(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	first := fmt.Sprintf("[P1] 📍 %s 日报 第 %d 日 · %s", res.State, nc.StateDays, monthDay(res.Date))
	tail := diffLine(nc) + "\n盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日"
	return strings.Join([]string{first, bodyZones(cfg, res, "异常指标："), tail}, "\n\n") + notifyFooter
}

// renderWeekly 消息 5：WATCH 周报（通知设计 §5.5，退出进度见 §6.6）。
func renderWeekly(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	first := fmt.Sprintf("[P1] 📅 Cassandra 周报 · %s 当周 · %s 已持续 %d 个评估日",
		monthDay(res.Date), res.State, nc.StateDays)
	tail := fmt.Sprintf("退出进度：触发条件已连续解除 %d 日（回 NORMAL 需连续 %d 日）\n下次周报：下周一 · 状态变更即时通知",
		nc.ClearStreak, cfg.StateMachine.WatchExitDays)
	return strings.Join([]string{first, bodyZones(cfg, res, "异常指标："), tail}, "\n\n") + notifyFooter
}
