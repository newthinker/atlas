package crisis

// Context Checkpoint: done_criteria → test mapping (notify)
// functional[0] 状态变更 P1/进入 BREWING·CRISIS P0/无变更日报/月报·周报 → TestMessagesTransitionPriorities / TestMessagesDigestsAndSummaries
// functional[1] STALE→P2；模板含状态/读数/分位/持续天数/下一评估；页脚边界声明 → TestMessagesDigestsAndSummaries / TestMessagesForbiddenWords
// boundary[0]   全部文案不含"必然/一定/即将" → TestMessagesForbiddenWords
// boundary[1]   NORMAL 无变更且非 summaryDue → 零消息 → TestMessagesDigestsAndSummaries

import (
	"strings"
	"testing"

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

func TestMessagesTransitionPriorities(t *testing.T) {
	msgs := Messages(dayResult(StateWatch, StateBrewing), 1, false, nil)
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P0]")) // 进入 BREWING = P0 一次（设计 §3.3）
	assert.Contains(t, msgs[0], "WATCH → BREWING")

	msgs = Messages(dayResult(StateNormal, StateWatch), 1, false, nil)
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P1]")) // 一般状态变更 = P1

	// 进入 CRISIS 同样 P0
	msgs = Messages(dayResult(StateBrewing, StateCrisis), 1, false, nil)
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P0]"))
	assert.Contains(t, msgs[0], "BREWING → CRISIS")
}

func TestMessagesDigestsAndSummaries(t *testing.T) {
	// BREWING 无变更日 → 每日推送
	msgs := Messages(dayResult(StateBrewing, StateBrewing), 5, false, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "日报")

	// NORMAL：仅 summaryDue 时发月报，否则静默（设计 §3.3 表）
	msgs = Messages(dayResult(StateNormal, StateNormal), 30, true, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "月度摘要")
	assert.Empty(t, Messages(dayResult(StateNormal, StateNormal), 30, false, nil))

	// WATCH + summaryDue → 周度摘要
	msgs = Messages(dayResult(StateWatch, StateWatch), 3, true, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "周度摘要")

	// STALE 指标 → P2 运维告警
	msgs = Messages(dayResult(StateNormal, StateNormal), 1, false, []string{IndMOVE})
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P2]"))
	assert.Contains(t, msgs[0], "move")
}

// TestMessagesIndicatorLineDetails 覆盖 indicatorLines 的 tag 与空分位（Pct5y<0）
// 两个渲染分支（functional[1] 模板要素）。
func TestMessagesIndicatorLineDetails(t *testing.T) {
	res := dayResult(StateNormal, StateWatch) // 状态变更 → 有正文
	hy := res.Results[IndHYOAS]
	hy.Tag = TagComplacency
	res.Results[IndHYOAS] = hy
	nfci := res.Results[IndNFCI]
	nfci.Pct5y = -1 // 分位窗口为空 → 不显示分位
	res.Results[IndNFCI] = nfci

	msgs := Messages(res, 1, false, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "[COMPLACENCY]") // tag 分支
	assert.Contains(t, msgs[0], "5y分位")          // 其它指标 Pct5y>=0 仍显示
}

// Global Constraints：所有文案禁止确定性字样，状态类通知必须带边界声明页脚。
func TestMessagesForbiddenWords(t *testing.T) {
	all := [][]string{
		Messages(dayResult(StateWatch, StateBrewing), 1, false, []string{IndMOVE}),
		Messages(dayResult(StateBrewing, StateBrewing), 5, true, nil),
		Messages(dayResult(StateNormal, StateNormal), 30, true, nil),
		Messages(dayResult(StateNormal, StateCrisis), 1, false, nil),
	}
	for _, msgs := range all {
		for _, m := range msgs {
			for _, banned := range []string{"必然", "一定", "即将"} {
				assert.NotContains(t, m, banned)
			}
			if strings.HasPrefix(m, "[P0]") || strings.HasPrefix(m, "[P1]") {
				assert.Contains(t, m, "非交易信号")
			}
		}
	}
}
