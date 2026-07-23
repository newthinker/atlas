package crisis

// Context Checkpoint: done_criteria → test mapping (notify v2，通知设计 v1.0)
// §2 消息类型矩阵/装配唯一性 → TestMessagesDispatch
// §7 禁词 + 页脚（结构化含"非交易信号"、速报不含页脚）→ TestMessagesForbiddenWordsAllFamilies
// §5.7 盘中速报 → TestFormatIntradayAlert

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dayResult(prev, cur SystemState) *DayResult {
	results := map[string]IndicatorResult{}
	for _, ind := range AllIndicators {
		results[ind] = IndicatorResult{Indicator: ind, Status: StatusGreen, RawStatus: StatusGreen, Value: 1, Pct5y: 0.5}
	}
	return &DayResult{Date: "2026-07-10", Results: results, PrevState: prev, State: cur}
}

// 装配矩阵（通知设计 §2）：结构化家族至多一条 + NewStale 各一条 P2。
// switch 四分支「A 或 B」逐支 + 优先级否定路径 + NewStale 空/单/多。
func TestMessagesDispatch(t *testing.T) {
	cfg := testConfig()

	// —— 分支 1：状态变更优先（即使同时是 BREWING 日）——
	msgs := Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "状态升级 WATCH → BREWING")
	// 降级也通知，仅 P1（设计 §2）
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateWatch), StateDays: 34})
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P1] ✅ 状态解除"))
	// 否定路径：变更日即使 Summary 到期也出状态变更、不出周报（变更 > 摘要）
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateWatch), StateDays: 1, Summary: SummaryWeekly, ClearStreak: 8})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "状态升级 NORMAL → WATCH")
	assert.NotContains(t, msgs[0], "周报")

	// —— 分支 2：BREWING/CRISIS 无变更 → 日报（"A 或 B" 两支各一）——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateBrewing), StateDays: 5})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "BREWING 日报 第 5 日")
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateCrisis, StateCrisis), StateDays: 3})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "CRISIS 日报 第 3 日")

	// —— 分支 3/4：NORMAL+到期 → 月报；WATCH+到期 → 周报 ——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 63,
		Summary: SummaryMonthly, Trends: testTrends("2026-07-10")})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 月报")
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateWatch), StateDays: 18, Summary: SummaryWeekly, ClearStreak: 8})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 周报")

	// —— 分支 4'：NORMAL+周报到期 → 周报（NORMAL 周报设计 2026-07-23）——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 30, Summary: SummaryWeekly})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 周报")

	// —— boundary：NORMAL 非到期 → 零消息（NewStale 空）——
	assert.Empty(t, Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 30}))

	// —— NewStale 单：与结构化消息并发（1 结构化 + 1 P2）——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateBrewing), StateDays: 5,
		NewStale: []string{IndMOVE}, StaleLastObs: map[string]string{IndMOVE: "2026-07-05"}})
	require.Len(t, msgs, 2)
	assert.True(t, strings.HasPrefix(msgs[1], "[P2] 🔧 move 数据源断更"))

	// —— NewStale 多：每指标各一条 P2（结构化 + 2 P2）；顺序即 NewStale 顺序 ——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateBrewing), StateDays: 5,
		NewStale:     []string{IndMOVE, IndNFCI},
		StaleLastObs: map[string]string{IndMOVE: "2026-07-05", IndNFCI: "2026-06-30"}})
	require.Len(t, msgs, 3)
	assert.Contains(t, msgs[1], "[P2] 🔧 move")
	assert.Contains(t, msgs[2], "[P2] 🔧 nfci")

	// —— NewStale 单 + 无结构化（NORMAL 非到期）→ 仅 1 条 P2 ——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 1,
		NewStale: []string{IndMOVE}, StaleLastObs: map[string]string{IndMOVE: "2026-07-05"}})
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P2] 🔧 move"))
}

// 盘中速报（§5.7）：wow 为负渲染急跌百分比、本地时分、无页脚。
func TestFormatIntradayAlert(t *testing.T) {
	at := time.Date(2026, 7, 18, 14, 30, 0, 0, time.Local)
	msg := FormatIntradayAlert(152.1, 157.5, -0.034, StateBrewing, at)
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 USD/JPY 盘中急跌 -3.4% · 07-18 14:30"))
	assert.Contains(t, msg, "现价 152.1（5 观测日前 157.5）")
	assert.Contains(t, msg, "系统状态 BREWING")
	assert.Contains(t, msg, "成因未核实，非交易信号")    // R5 内联限定语
	assert.NotContains(t, msg, "carry trade") // R5 去归因
	assert.Contains(t, msg, "今日此告警不再重复")
	assert.False(t, strings.HasSuffix(msg, notifyFooter)) // 速报仍不挂页脚常量
}

// Global Constraints（通知设计 §7，测试要点 1）：7 类消息全覆盖禁词与页脚归属。
func TestMessagesForbiddenWordsAllFamilies(t *testing.T) {
	cfg := testConfig()
	staleCtx := func(res *DayResult) NotifyContext {
		return NotifyContext{Res: res, StateDays: 5, NewStale: []string{IndMOVE},
			StaleLastObs: map[string]string{IndMOVE: "2026-07-05"}}
	}
	var all []string
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})...) // 1 升级
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateWatch), StateDays: 34})...) // 2 降级
	all = append(all, Messages(cfg, staleCtx(dayResult(StateCrisis, StateCrisis)))...)                          // 3 日报 + 6 P2
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 63,
		Summary: SummaryMonthly, Trends: testTrends("2026-07-10")})...) // 4 月报
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateWatch), StateDays: 18,
		Summary: SummaryWeekly, ClearStreak: 8})...) // 5 周报
	all = append(all, FormatIntradayAlert(152.1, 157.5, -0.034, StateBrewing,
		time.Date(2026, 7, 18, 14, 30, 0, 0, time.UTC))) // 7 盘中
	require.Len(t, all, 7)

	structuredCount, alertCount := 0, 0
	for _, m := range all {
		for _, banned := range []string{"必然", "一定", "即将"} {
			assert.NotContains(t, m, banned)
		}
		// v1.1 R5 连锁：页脚归属改按 notifyFooter 完整常量判定（盘中速报现含
		// "非交易信号"内联限定语，子串判定失效）
		if strings.HasPrefix(m, "[P2]") || strings.Contains(m, "盘中急跌") {
			assert.False(t, strings.HasSuffix(m, notifyFooter), "速报家族不得挂页脚: %s", m[:20])
			alertCount++
		} else {
			assert.True(t, strings.HasSuffix(m, notifyFooter), "结构化家族必须挂页脚: %s", m[:20])
			structuredCount++
		}
		assert.LessOrEqual(t, len(m), 4096) // telegram 上限（设计 §7）
	}
	assert.Equal(t, 5, structuredCount) // 升级/降级/日报/月报/周报
	assert.Equal(t, 2, alertCount)      // P2 + 盘中
}

// N3（反审补充，原则 3 集成断言）：降级 ∧ NewStale ∧ 断更前 RED 时，装配层同时
// 产出带 ⚠ 警示的转移消息（T1 staleDowngradeWarning）与带 ⚠ 警示的 P2 速报（T2 R1b）。
func TestMessagesStaleDowngradeIntegration(t *testing.T) {
	cfg := testConfig()
	msgs := Messages(cfg, NotifyContext{
		Res: dayResult(StateBrewing, StateWatch), StateDays: 34,
		NewStale:     []string{IndMOVE},
		StaleLastObs: map[string]string{IndMOVE: "2026-07-05"},
		PrevDay:      map[string]Evaluation{IndMOVE: {Indicator: IndMOVE, Status: StatusRed}},
	})
	require.Len(t, msgs, 2)
	// 转移消息（结构化，[P1]）带 T1 警示 + 页脚
	assert.True(t, strings.HasPrefix(msgs[0], "[P1]"))
	assert.Contains(t, msgs[0], "⚠ 注意：本次变更当日 move 数据断更")
	assert.True(t, strings.HasSuffix(msgs[0], notifyFooter))
	// P2 速报带 T2 警示、无页脚
	assert.True(t, strings.HasPrefix(msgs[1], "[P2] 🔧 move"))
	assert.Contains(t, msgs[1], "⚠ 断更前为红且计入触发判定")
	assert.False(t, strings.HasSuffix(msgs[1], notifyFooter))
}
