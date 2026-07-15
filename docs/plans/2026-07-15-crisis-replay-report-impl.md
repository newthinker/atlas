# Cassandra 危机回放报告实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将手工回放流程产品化为 `atlas crisis report` 子命令：回放引擎（暖机、零写入）、daily/monthly 文本报告、标准总结、自包含 HTML 详细报告（内联 SVG）、telegram sendDocument 发送链路。

**Architecture:** 回放引擎与三个渲染器（ReplayReport / RenderReplaySummary / RenderReplayHTML）全部落在 `internal/crisis`（导出、纯函数、DB 读经 SeriesReader）；落盘与发送在 cmd 层；telegram 扩展 `SendDocument`，`Sender` 接口不动（cmd 用类型断言）。既有 `crisis replay` 子命令重构为调用同一引擎（v1.1 决策：统一暖机语义，全期窗口黄金对照）。

**Tech Stack:** Go（零新第三方依赖）、html/template + 内联 SVG、modernc.org/sqlite（只读）、cobra、testify。

**设计基线:** `docs/plans/2026-07-15-crisis-replay-report-design.md`（v1.1，用户已确认）。

## Global Constraints

- 零新第三方依赖；所有 go 命令带 `GOTOOLCHAIN=local`（sqlite 固定 v1.38.2，见项目 memory）。
- 回放全程零写入：观测只经 `SeriesReader` 读取，评估历史只进 `NewMemHistory()`，生产 crisis.db 只读。
- 渲染纯函数纪律：`internal/crisis` 的引擎/渲染不做 IO（文件落盘、发送、路径拼接都在 cmd 层）。
- 禁词约束全家族沿用：输出不得含「必然」「一定」「即将」；telegram 单条 ≤4096 字符。
- 回放尾注专用常量（含「历史回放」限定），不复用 `notifyFooter`；文本报告（renderDaily/renderMonthly 产物）保留消息家族既有页脚。
- 既有 `crisis replay` 子命令**全期窗口**输出逐字节不变（黄金对照）；非全期窗口期初态改为暖机语义（v1.1 口径修复，预期内行为变化）。
- 测试命令统一：`GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/ ./internal/notifier/telegram/`。
- GitNexus 门禁（项目 CLAUDE.md）：修改既有符号前先 `impact({target, direction: "upstream"})`；每次提交前 `detect_changes()` 核对影响范围；HIGH/CRITICAL 风险须先告知用户。
- 提交纪律（用户全局 CLAUDE.md）：每个任务最后一次提交前运行 code-simplifier sub-agent（Task tool，`subagent_type: "code-simplifier:code-simplifier"`），简化结果并入提交。提交信息遵循 `<type>(crisis): <description>`。

---

### Task 1: ReplayRange 回放引擎 + 既有 replay 子命令重构（黄金对照）

**Files:**
- Create: `internal/crisis/replay.go`
- Create: `internal/crisis/replay_test.go`
- Modify: `cmd/atlas/crisis.go:602-642`（`executeCrisisReplay`）
- Test: 既有 `cmd/atlas/crisis_test.go` 的 `TestExecuteCrisisReplay*` 作为回归黄金

**Interfaces:**
- Consumes: `EvalDay(cfg, date, sr, hist, evalAt)`、`NewMemHistory()`、`SeriesReader.WindowSince`（均既有）
- Produces: `type ReplayDay struct { Date string; Res *DayResult; StateDays int }`；`func ReplayRange(cfg *Config, sr SeriesReader, from, to string) ([]ReplayDay, error)`

**背景（实现者须知）：** 既有 `executeCrisisReplay`（`cmd/atlas/crisis.go:604`）从 `--from` 冷启动（空 MemHistory）。v1.1 决策：统一为暖机语义——从库内最早 vix 观测日起逐日 `EvalDay` 推进，只返回 `[from,to]` 窗口。交易日历 = vix 观测日（与 `Store.EvalDates` 同口径，`store.go:142` 查询 `IndVIX`）。全期窗口（from ≤ 最早观测日）输出必须逐字节不变。

- [ ] **Step 0: gitnexus 门禁**

运行 `impact({target: "executeCrisisReplay", direction: "upstream"})`，确认上游仅 `runCrisisReplay`；HIGH/CRITICAL 则停下告知用户。

- [ ] **Step 1: 录制手工黄金（有真实库时）**

```bash
ls data/crisis.db 2>/dev/null && GOTOOLCHAIN=local go run ./cmd/atlas crisis replay \
  --from 2006-01-01 --to 2009-12-31 \
  > /private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/de01275e-a03a-4703-b098-197a439326e2/scratchpad/replay-golden-before.txt
```

Expected: 转移线 + `final state` + `entered` 统计写入 scratchpad。`data/crisis.db` 不存在则跳过本步（单测黄金兜底），并在任务小结注明。

- [ ] **Step 2: 写失败测试（`internal/crisis/replay_test.go`）**

fixture 复用同包 `rules_test.go` 的 `memSeries`（实现 SeriesReader）与 `testConfig()`（vix Amber 25/Red 30、move Amber 100/Red 120）、`dates.go` 的 `addDays`。构造 12 个连续观测日、7 指标全绿（`vix 15, move 70, sofr_effr -10, hy_oas 400, t10y2y 35, nfci -0.5, usdjpy 150`），末 4 日 vix=35、move=130（情绪双红 → 任意态直入 CRISIS）：

```go
package crisis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaySeries 铺 n 个连续观测日（末日 end），7 指标常值，最后 redTail 日
// vix/move 置红（情绪双红 → CRISIS）。
func replaySeries(end string, n, redTail int) memSeries {
	base := map[string]float64{
		IndVIX: 15, IndMOVE: 70, IndSOFREFFR: -10, IndHYOAS: 400,
		IndT10Y2Y: 35, IndNFCI: -0.5, IndUSDJPY: 150,
	}
	m := memSeries{}
	for i := 0; i < n; i++ {
		d := addDays(end, i-n+1)
		for ind, v := range base {
			if i >= n-redTail {
				if ind == IndVIX {
					v = 35
				}
				if ind == IndMOVE {
					v = 130
				}
			}
			m[ind] = append(m[ind], Observation{Date: d, Indicator: ind, Value: v})
		}
	}
	return m
}

// functional: 暖机逐日推进，窗口切片正确；转移日 StateDays=1、次日=2。
func TestReplayRangeWarmupAndStateDays(t *testing.T) {
	const end = "2026-07-10" // 12 日 = 2026-06-29..07-10，末 4 日红（07-07 起）
	sr := replaySeries(end, 12, 4)
	days, err := ReplayRange(testConfig(), sr, "2026-06-29", end)
	require.NoError(t, err)
	require.Len(t, days, 12)

	assert.Equal(t, StateNormal, days[0].Res.State)
	assert.Equal(t, 1, days[0].StateDays) // 首日 NORMAL 含当日

	trans := days[8] // 2026-07-07：NORMAL → CRISIS
	assert.Equal(t, "2026-07-07", trans.Date)
	require.True(t, trans.Res.Transitioned())
	assert.Equal(t, StateCrisis, trans.Res.State)
	assert.Equal(t, 1, trans.StateDays) // 转移日 = 1
	assert.Equal(t, 2, days[9].StateDays)
	assert.Equal(t, 8, days[7].StateDays) // 转移前 NORMAL 已持续 8 日
}

// boundary: 窗口切片不影响计数（暖机期计入）；期初态为暖机结果而非冷启动。
func TestReplayRangeWindowSlice(t *testing.T) {
	const end = "2026-07-10"
	sr := replaySeries(end, 12, 4)
	days, err := ReplayRange(testConfig(), sr, "2026-07-08", end)
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, StateCrisis, days[0].Res.State)
	assert.Equal(t, StateCrisis, days[0].Res.PrevState) // 暖机：07-07 已入 CRISIS
	assert.Equal(t, 2, days[0].StateDays)               // 07-08 = CRISIS 第 2 日
}

// boundary: 窗口无交易日 → 空切片不报错；from > to → 报错。
func TestReplayRangeEmptyAndBadRange(t *testing.T) {
	sr := replaySeries("2026-07-10", 12, 0)
	days, err := ReplayRange(testConfig(), sr, "2027-01-01", "2027-02-01")
	require.NoError(t, err)
	assert.Empty(t, days)

	_, err = ReplayRange(testConfig(), sr, "2026-07-10", "2026-07-01")
	assert.Error(t, err)
}
```

- [ ] **Step 3: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestReplayRange -v
```

Expected: FAIL（`undefined: ReplayRange` / `ReplayDay`）。

- [ ] **Step 4: 实现 `internal/crisis/replay.go`**

```go
package crisis

import (
	"fmt"
	"time"
)

// ReplayDay 一个回放交易日的完整评估快照。
type ReplayDay struct {
	Date      string
	Res       *DayResult
	StateDays int // 当前状态连续评估日数（含当日；暖机期计入）
}

// ReplayRange 从库内最早 vix 观测日起暖机逐日重放（内存历史、零写入），
// 返回 [from,to] 窗口内的快照。交易日历以 vix 观测日为准（与 Store.EvalDates
// 同口径）。窗口内无交易日返回空切片，措辞由调用方决定。
func ReplayRange(cfg *Config, sr SeriesReader, from, to string) ([]ReplayDay, error) {
	if from > to {
		return nil, fmt.Errorf("from %s is after to %s", from, to)
	}
	cal, err := sr.WindowSince(IndVIX, "", to)
	if err != nil {
		return nil, err
	}
	mem := NewMemHistory()
	evalAt := time.Now()
	var out []ReplayDay
	stateDays := 0
	for _, o := range cal {
		res, err := EvalDay(cfg, o.Date, sr, mem, evalAt)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", o.Date, err)
		}
		mem.Append(res.Evaluations)
		if res.Transitioned() {
			stateDays = 1
		} else {
			stateDays++
		}
		if o.Date >= from {
			out = append(out, ReplayDay{Date: o.Date, Res: res, StateDays: stateDays})
		}
	}
	return out, nil
}
```

- [ ] **Step 5: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestReplayRange -v
```

Expected: PASS（3 个测试）。

- [ ] **Step 6: 重构 `executeCrisisReplay` 调用引擎**

替换 `cmd/atlas/crisis.go:602-642` 全部函数体（cmd 只留转移线/统计打印；输出格式逐字节不变）：

```go
// executeCrisisReplay 逐日重放:观测来自 sqlite,评估历史只进 MemHistory,
// 不写 crisis_evaluations(审计表只属于 live eval)。v1.1 起统一暖机语义:
// 引擎从库内最早观测日推进,窗口期初态为暖机结果。
func executeCrisisReplay(ctx context.Context, cfg *crisis.Config, st *crisis.Store, from, to string, jsonOut bool, out io.Writer) error {
	days, err := crisis.ReplayRange(cfg, st.Reader(ctx), from, to)
	if err != nil {
		return err
	}
	if len(days) == 0 {
		return fmt.Errorf("no observations between %s and %s — run backfill first", from, to)
	}

	entered := map[crisis.SystemState]int{}
	for _, day := range days {
		res := day.Res
		if !res.Transitioned() {
			continue
		}
		entered[res.State]++
		if jsonOut {
			b, _ := json.Marshal(map[string]any{
				"date": day.Date, "from": res.PrevState, "to": res.State, "amber_count": res.Detail.AmberCount,
			})
			fmt.Fprintln(out, string(b))
		} else {
			fmt.Fprintf(out, "%s  %s → %s (amber=%d)\n", day.Date, res.PrevState, res.State, res.Detail.AmberCount)
		}
	}

	fmt.Fprintf(out, "\nfinal state: %s over %d eval days\n", days[len(days)-1].Res.State, len(days))
	for _, s := range []crisis.SystemState{crisis.StateWatch, crisis.StateBrewing, crisis.StateCrisis} {
		fmt.Fprintf(out, "entered %-8s %d times\n", s, entered[s])
	}
	return nil
}
```

注意：`time` 若仅剩此处引用需检查 `cmd/atlas/crisis.go` 其他函数仍在用（`runCrisisBackfill` 等用到，import 不动）。

- [ ] **Step 7: 全量回归（既有 replay 测试 = 单测黄金）**

```bash
GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisReplay -v
GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/
```

Expected: 全 PASS。`TestExecuteCrisisReplayTransitions/JSON/NoData` 逐字节校验输出格式（其窗口起点即库内最早种子日，属全期窗口，暖机不改变结果）。

- [ ] **Step 8: 手工黄金对照（做过 Step 1 时）**

```bash
GOTOOLCHAIN=local go run ./cmd/atlas crisis replay --from 2006-01-01 --to 2009-12-31 \
  > /private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/de01275e-a03a-4703-b098-197a439326e2/scratchpad/replay-golden-after.txt
diff /private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/de01275e-a03a-4703-b098-197a439326e2/scratchpad/replay-golden-{before,after}.txt
```

Expected: diff 无输出（逐字节一致）。

- [ ] **Step 9: 提交**

code-simplifier（`internal/crisis/replay.go`、`cmd/atlas/crisis.go`）→ `detect_changes()` 确认只影响 `executeCrisisReplay` 相关流程 →

```bash
git add internal/crisis/replay.go internal/crisis/replay_test.go cmd/atlas/crisis.go
git commit -m "feat(crisis): add ReplayRange engine, unify replay warm-up semantics"
```

---

### Task 2: ReplayReport 装配（daily/monthly + PrevDay 链）

**Files:**
- Create: `internal/crisis/replay_report.go`
- Create: `internal/crisis/replay_report_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ReplayDay`；既有 `renderDaily`/`renderMonthly`/`NotifyContext`/`Trend`
- Produces: `func ReplayReport(cfg *Config, form string, day ReplayDay, prev *ReplayDay, sr SeriesReader) (string, error)`（form ∈ "daily"|"monthly"）

**背景（实现者须知）：** `renderDaily`/`renderMonthly` 为包内私有（`notify_render.go:278/328`），本任务新增导出装配入口，**忽略消息矩阵门控**（`--form` 指定什么就渲染什么，首行如实打印回放态，如 `NORMAL 日报 第 N 日`）。NotifyContext 组装口径（设计 §3）：`Res`=当日、`StateDays`=引擎值、`PrevDay`=前一回放日 `Res.Results` 转 `Evaluation`（只填 Indicator/Status/Value）、`SummaryDue=false`、`NewStale/StaleLastObs` 空、daily 不装 `Trends`；monthly 用 `sr.Window(ind, day.Date, 21)` 装 `Trends`（空窗口省略该行，与 `buildNotifyContext` 的 `cmd/atlas/crisis.go:401-413` 同式）。渲染保持纯函数。

- [ ] **Step 1: 写失败测试（`internal/crisis/replay_report_test.go`）**

```go
package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkReplayDay 手工构造回放日：7 指标默认 GREEN，statuses/vals 覆盖指定指标。
func mkReplayDay(date string, prev, state SystemState, stateDays, amber int,
	statuses map[string]Status, vals map[string]float64) ReplayDay {
	res := &DayResult{
		Date: date, Results: map[string]IndicatorResult{},
		PrevState: prev, State: state,
		Detail: SysDetail{Date: date, AmberCount: amber, Prev: prev},
	}
	for _, ind := range AllIndicators {
		r := IndicatorResult{Indicator: ind, Status: StatusGreen, Value: 10, Pct5y: -1}
		if s, ok := statuses[ind]; ok {
			r.Status = s
		}
		if v, ok := vals[ind]; ok {
			r.Value = v
		}
		res.Results[ind] = r
	}
	return ReplayDay{Date: date, Res: res, StateDays: stateDays}
}

// functional: daily 首行含回放态与 StateDays；PrevDay 链使差异行真实可用。
func TestReplayReportDaily(t *testing.T) {
	prev := mkReplayDay("2026-07-09", StateCrisis, StateCrisis, 1, 2, nil, nil)
	day := mkReplayDay("2026-07-10", StateCrisis, StateCrisis, 2, 2,
		map[string]Status{IndVIX: StatusRed}, map[string]float64{IndVIX: 42})

	out, err := ReplayReport(testConfig(), "daily", day, &prev, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "CRISIS 日报 第 2 日")
	assert.Contains(t, out, "较昨日：")
	assert.Contains(t, out, "vix 转红（原绿）") // PrevDay 链生效
	assert.Contains(t, out, "非交易信号")       // 消息家族页脚保留
}

// boundary: 窗口首日 prev=nil → PrevDay 空 map → 差异行"无变化"。
func TestReplayReportDailyFirstDay(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateBrewing, StateBrewing, 3, 1, nil, nil)
	out, err := ReplayReport(testConfig(), "daily", day, nil, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "较昨日：无变化")
}

// boundary: 状态门控忽略——NORMAL 日也渲染 daily。
func TestReplayReportDailyIgnoresGate(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateNormal, StateNormal, 5, 0, nil, nil)
	out, err := ReplayReport(testConfig(), "daily", day, nil, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "NORMAL 日报 第 5 日")
}

// functional: monthly 从 sr 装 21 日 Trends；空窗口指标省略趋势行。
func TestReplayReportMonthly(t *testing.T) {
	sr := memSeries{IndVIX: seriesEnding("2026-07-01", 21, 15, 18)}
	day := mkReplayDay("2026-07-01", StateNormal, StateNormal, 40, 0, nil,
		map[string]float64{IndVIX: 18})
	out, err := ReplayReport(testConfig(), "monthly", day, nil, sr)
	require.NoError(t, err)
	assert.Contains(t, out, "Cassandra 月报 · 2026-07")
	assert.Contains(t, out, "近 21 个交易日趋势")
	require.Equal(t, 1, strings.Count(out, "vix"), "仅 vix 有趋势行")
	assert.NotContains(t, out, "hy_oas") // 空窗口省略
}

// error_handling: 未知 form 报错。
func TestReplayReportUnknownForm(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateNormal, StateNormal, 1, 0, nil, nil)
	_, err := ReplayReport(testConfig(), "weekly", day, nil, memSeries{})
	assert.ErrorContains(t, err, "weekly")
}
```

（`seriesEnding` 为 `rules_test.go:50` 既有 helper：n 个逐日观测、末日 end。）

- [ ] **Step 2: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestReplayReport -v
```

Expected: FAIL（`undefined: ReplayReport`）。

- [ ] **Step 3: 实现 `internal/crisis/replay_report.go`**

```go
package crisis

import "fmt"

// ReplayReport 渲染一个回放日的指定形式报告（忽略消息矩阵门控，回放前缀由
// cmd 层拼接，渲染器不感知回放）。prev 为前一回放日，窗口首日传 nil——PrevDay
// 空 map 使差异行输出"无变化"（设计 §3，可接受）。
func ReplayReport(cfg *Config, form string, day ReplayDay, prev *ReplayDay, sr SeriesReader) (string, error) {
	nc := NotifyContext{
		Res:       day.Res,
		StateDays: day.StateDays,
		PrevDay:   map[string]Evaluation{},
	}
	if prev != nil {
		for _, ind := range AllIndicators {
			r := prev.Res.Results[ind]
			nc.PrevDay[ind] = Evaluation{Indicator: ind, Status: r.Status, Value: r.Value}
		}
	}
	switch form {
	case "daily":
		return renderDaily(cfg, nc), nil
	case "monthly":
		nc.Trends = map[string]Trend{}
		for _, ind := range AllIndicators {
			win, err := sr.Window(ind, day.Date, 21)
			if err != nil {
				return "", err
			}
			if len(win) == 0 {
				continue
			}
			nc.Trends[ind] = Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
		}
		return renderMonthly(cfg, nc), nil
	}
	return "", fmt.Errorf("unknown report form %q (want daily or monthly)", form)
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestReplayReport -v
GOTOOLCHAIN=local go test ./internal/crisis/
```

Expected: 全 PASS（含既有渲染测试不回归）。

- [ ] **Step 5: 提交**

code-simplifier（`internal/crisis/replay_report.go`）→ `detect_changes()` →

```bash
git add internal/crisis/replay_report.go internal/crisis/replay_report_test.go
git commit -m "feat(crisis): add ReplayReport assembly for daily/monthly replay"
```

---

### Task 3: RenderReplaySummary 标准总结

**Files:**
- Create: `internal/crisis/replay_summary.go`
- Create: `internal/crisis/replay_summary_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ReplayDay`；既有 `formatReading`/`isColor`/`SysDetail`
- Produces: `func RenderReplaySummary(cfg *Config, days []ReplayDay) string`；回放专用尾注常量 `replayFooter`

**背景（实现者须知）：** 输出格式见设计 §4（单条 ≤4096）。要点：期初态 = 窗口首日 `State`（首日即转移时取 `PrevState`）；「最差」方向与红灯方向一致——`t10y2y`/`usdjpy` 取期间最小值，其余 5 指标取最大值；极值只统计有新鲜读数的日（`isColor(status) || StatusSuppressed`，STALE/NO_DATA 跳过）；各态停留按严重度降序只列出现过的态；STALE 统计仅列非零指标、全零省略整行；尾注为回放专用常量，**不复用** `notifyFooter`。

- [ ] **Step 1: 写失败测试（`internal/crisis/replay_summary_test.go`）**

复用 Task 2 的 `mkReplayDay`（同包）：

```go
package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// functional: 转移列表、期初态、各态停留、AMBER 峰值。
func TestRenderReplaySummaryTransitions(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-06", StateNormal, StateNormal, 3, 0, nil, nil),
		mkReplayDay("2026-07-07", StateNormal, StateWatch, 1, 3, nil, nil), // 转移
		mkReplayDay("2026-07-08", StateWatch, StateWatch, 2, 2, nil, nil),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "【回放总结 2026-07-06 ~ 2026-07-08】")
	assert.Contains(t, out, "状态：NORMAL 起步 · 期间转移 1 次")
	assert.Contains(t, out, "2026-07-07 NORMAL → WATCH")
	assert.NotContains(t, out, "转移：无")
	assert.Contains(t, out, "WATCH 2 日 · NORMAL 1 日") // 严重度降序、仅出现过的态
	assert.Contains(t, out, "AMBER 峰值：3/7（2026-07-07）")
	assert.Contains(t, out, "历史回放，非实时告警；阈值为当前配置，非事后调参。")
	assert.NotContains(t, out, "操作决策不在本模块范围") // 不复用 notifyFooter
}

// boundary: 无转移 → 「转移：无」；全零 STALE → 省略整行。
func TestRenderReplaySummaryNoTransition(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-09", StateNormal, StateNormal, 1, 0, nil, nil),
		mkReplayDay("2026-07-10", StateNormal, StateNormal, 2, 0, nil, nil),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "期间转移 0 次")
	assert.Contains(t, out, "转移：无")
	assert.NotContains(t, out, "STALE 统计")
	assert.Contains(t, out, "AMBER 峰值：0/7")
}

// functional: 极值方向逐指标——vix 取最大、t10y2y 取最小（落界）；
// STALE 日读数不计入极值；STALE 统计仅列非零。
func TestRenderReplaySummaryExtremesAndStale(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-08", StateNormal, StateNormal, 1, 0, nil,
			map[string]float64{IndVIX: 30, IndT10Y2Y: -20}),
		mkReplayDay("2026-07-09", StateNormal, StateNormal, 2, 0,
			map[string]Status{IndMOVE: StatusStale},
			map[string]float64{IndVIX: 80.9, IndT10Y2Y: -55, IndMOVE: 999}),
		mkReplayDay("2026-07-10", StateNormal, StateNormal, 3, 0, nil,
			map[string]float64{IndVIX: 25, IndT10Y2Y: 10, IndMOVE: 120}),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "vix 80.9（2026-07-09）")      // 最大值方向
	assert.Contains(t, out, "t10y2y -55bp（2026-07-09）")  // 最小值方向
	assert.Contains(t, out, "move 120.0（2026-07-10）")    // STALE 日 999 不计入
	assert.Contains(t, out, "move 缺数 1 交易日")
	assert.NotContains(t, out, "vix 缺数")
}

// non_functional: 千日量级 + 多次转移仍 ≤4096（2006–2009 全期近似）。
func TestRenderReplaySummaryUnder4096(t *testing.T) {
	var days []ReplayDay
	state := StateNormal
	for i := 0; i < 1000; i++ {
		d := addDays("2006-01-02", i)
		prev := state
		if i%100 == 99 { // 10 次转移
			if state == StateNormal {
				state = StateCrisis
			} else {
				state = StateNormal
			}
		}
		sd := i%100 + 1
		days = append(days, mkReplayDay(d, prev, state, sd, i%8, nil, nil))
	}
	out := RenderReplaySummary(testConfig(), days)
	require.LessOrEqual(t, len([]rune(out)), 4096)
	for _, banned := range []string{"必然", "一定", "即将"} { // 禁词沿用
		assert.NotContains(t, out, banned)
	}
	assert.Equal(t, 1, strings.Count(out, "指标极值"))
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderReplaySummary -v
```

Expected: FAIL（`undefined: RenderReplaySummary`）。

- [ ] **Step 3: 实现 `internal/crisis/replay_summary.go`**

```go
package crisis

import (
	"fmt"
	"strings"
)

// replayFooter 回放专用尾注（设计 §4：不复用 notifyFooter，含"历史回放"限定）。
const replayFooter = "\n—\n历史回放，非实时告警；阈值为当前配置，非事后调参。"

// worstIsMin "最差"方向与各指标红灯方向一致：t10y2y（倒挂向下）、usdjpy
// （急跌向下）取期间最小值，其余取最大值（设计 §4）。
func worstIsMin(ind string) bool { return ind == IndT10Y2Y || ind == IndUSDJPY }

// hasFreshReading 极值只统计有新鲜读数的日：色彩态与季末抑制有当日观测，
// STALE/NO_DATA 无（STALE 的 Value 是旧读数，其原日已计入）。
func hasFreshReading(s Status) bool { return isColor(s) || s == StatusSuppressed }

// RenderReplaySummary 渲染回放窗口标准总结（单条 ≤4096，telegram 直发）。
// days 为空返回空串（调用方保证非空）。
func RenderReplaySummary(cfg *Config, days []ReplayDay) string {
	if len(days) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "【回放总结 %s ~ %s】\n", days[0].Date, days[len(days)-1].Date)

	start := days[0].Res.State
	if days[0].Res.Transitioned() {
		start = days[0].Res.PrevState
	}
	var trans []ReplayDay
	for _, d := range days {
		if d.Res.Transitioned() {
			trans = append(trans, d)
		}
	}
	fmt.Fprintf(&b, "状态：%s 起步 · 期间转移 %d 次\n", start, len(trans))
	if len(trans) == 0 {
		b.WriteString("转移：无\n")
	}
	for _, d := range trans {
		fmt.Fprintf(&b, "%s %s → %s\n", d.Date, d.Res.PrevState, d.Res.State)
	}

	stay := map[SystemState]int{}
	for _, d := range days {
		stay[d.Res.State]++
	}
	var stays []string
	for _, s := range []SystemState{StateCrisis, StateBrewing, StateWatch, StateNormal} {
		if stay[s] > 0 {
			stays = append(stays, fmt.Sprintf("%s %d 日", s, stay[s]))
		}
	}
	b.WriteString("各态停留：" + strings.Join(stays, " · ") + "\n")

	var extremes []string
	for _, ind := range AllIndicators {
		var v float64
		var date string
		for _, d := range days {
			r := d.Res.Results[ind]
			if !hasFreshReading(r.Status) {
				continue
			}
			worse := r.Value > v
			if worstIsMin(ind) {
				worse = r.Value < v
			}
			if date == "" || worse {
				v, date = r.Value, d.Date
			}
		}
		if date != "" {
			extremes = append(extremes, fmt.Sprintf("%s %s（%s）", ind, formatReading(ind, v), date))
		}
	}
	if len(extremes) > 0 {
		b.WriteString("指标极值（期间最差读数）：\n" + strings.Join(extremes, " · ") + "\n")
	}

	peak, peakDate := 0, ""
	for _, d := range days {
		if d.Res.Detail.AmberCount > peak {
			peak, peakDate = d.Res.Detail.AmberCount, d.Date
		}
	}
	if peakDate == "" {
		fmt.Fprintf(&b, "AMBER 峰值：0/%d\n", len(AllIndicators))
	} else {
		fmt.Fprintf(&b, "AMBER 峰值：%d/%d（%s）\n", peak, len(AllIndicators), peakDate)
	}

	var stales []string
	for _, ind := range AllIndicators {
		n := 0
		for _, d := range days {
			if d.Res.Results[ind].Status == StatusStale {
				n++
			}
		}
		if n > 0 {
			stales = append(stales, fmt.Sprintf("%s 缺数 %d 交易日", ind, n))
		}
	}
	if len(stales) > 0 {
		b.WriteString("STALE 统计：" + strings.Join(stales, " · ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + replayFooter
}
```

（`cfg` 参数当前未使用，按设计签名保留——Go 允许未使用的函数参数。）

- [ ] **Step 4: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderReplaySummary -v
GOTOOLCHAIN=local go test ./internal/crisis/
```

Expected: 全 PASS。

- [ ] **Step 5: 提交**

code-simplifier（`internal/crisis/replay_summary.go`）→ `detect_changes()` →

```bash
git add internal/crisis/replay_summary.go internal/crisis/replay_summary_test.go
git commit -m "feat(crisis): add RenderReplaySummary with replay-only footer"
```

---

### Task 4: RenderReplayHTML 详细报告（SVG 点阵/折线/报表）

**Files:**
- Create: `internal/crisis/replay_html.go`
- Create: `internal/crisis/replay_html_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ReplayDay`；Task 3 的 `hasFreshReading`（月度表读数口径与总结极值一致）；既有 `formatReading`/`SeriesReader.WindowSince`；cfg 各指标阈值
- Produces: `func RenderReplayHTML(cfg *Config, days []ReplayDay, sr SeriesReader) (string, error)`（自包含单文件、无外链、亮暗兼容）

**背景（实现者须知）：** 设计 §5 五段结构：头部（期间/生成时间/阈值摘要表）→ 状态时间线点阵 SVG → 7 指标折线 SVG（阈值横线 + STALE/缺数打点）→ 月度汇总表 → 状态转移明细表。HTML 始终全量日粒度（与 `--form` 无关）。约束：`html/template` + 手工拼 SVG 字符串（经 `template.HTML` 注入——SVG 内容全部来自受控数据：日期、状态枚举、`formatReading` 数字，无用户输入）；无任何外链/CDN；`prefers-color-scheme` 亮暗兼容；表格套 `overflow-x` 容器；SVG `viewBox` 响应式；色板同 emoji 语义（绿 `#16a34a` 黄 `#eab308` 橙 `#f97316` 红 `#dc2626`）。阈值摘要与折线阈值线**读 cfg**（key=value 风格陈列，不做语义解读以免歪曲规则方向）；usdjpy 为周环比规则无水平阈值线；`sofr_effr` 全期无数据 → 省略该幅并注明「该指标自 2018-04 起才有数据」。测试用 golden 片段断言，不做整文件快照。

- [ ] **Step 1: 写失败测试（`internal/crisis/replay_html_test.go`）**

复用 `mkReplayDay`（Task 2）与 `memSeries`/`seriesEnding`（rules_test.go）：

```go
package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlFixture 3 个交易日（跨月 06-30/07-01/07-02，触发一次月度分组切换与一次转移），
// vix/hy_oas 有观测序列，sofr_effr 全期无数据；hy_oas 第 2 日观测缺口且 STALE
// （其折线图应出现 x 轴打点，打点只画在有图指标上）。
func htmlFixture(t *testing.T) ([]ReplayDay, memSeries) {
	t.Helper()
	days := []ReplayDay{
		mkReplayDay("2026-06-30", StateNormal, StateNormal, 9, 0, nil,
			map[string]float64{IndVIX: 18}),
		mkReplayDay("2026-07-01", StateNormal, StateWatch, 1, 3,
			map[string]Status{IndHYOAS: StatusStale, IndVIX: StatusRed},
			map[string]float64{IndVIX: 33}),
		mkReplayDay("2026-07-02", StateWatch, StateWatch, 2, 2, nil,
			map[string]float64{IndVIX: 28}),
	}
	sr := memSeries{
		IndVIX: {
			{Date: "2026-06-30", Indicator: IndVIX, Value: 18},
			{Date: "2026-07-01", Indicator: IndVIX, Value: 33},
			{Date: "2026-07-02", Indicator: IndVIX, Value: 28},
		},
		IndHYOAS: {
			{Date: "2026-06-30", Indicator: IndHYOAS, Value: 400},
			{Date: "2026-07-02", Indicator: IndHYOAS, Value: 410},
		},
	}
	return days, sr
}

// functional: 点阵色块数=交易日数；转移明细行数；月度表行数=跨月数。
func TestRenderReplayHTMLGoldenFragments(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.Equal(t, 3, strings.Count(html, `class="day"`), "点阵色块数 = 交易日数")
	assert.Contains(t, html, "2026-06-30 ~ 2026-07-02")
	assert.Contains(t, html, "NORMAL → WATCH")                       // 转移明细
	assert.Equal(t, 1, strings.Count(html, "→"), "仅 1 次转移")
	assert.Contains(t, html, "<td>2026-06</td>")                     // 月度表 2 行
	assert.Contains(t, html, "<td>2026-07</td>")
	assert.Contains(t, html, "#eab308")                              // WATCH 黄色块
}

// functional: 折线 polyline 点数 = 该指标观测数；缺观测日不补点。
func TestRenderReplayHTMLPolylinePoints(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	// 有观测的指标各 1 条 polyline：vix(3 点) + hy_oas(2 点)
	assert.Equal(t, 2, strings.Count(html, "<polyline"))
	vix := extractChart(t, html, IndVIX)
	assert.Equal(t, 3, strings.Count(vix, ","), "vix polyline 3 个坐标点")
	hy := extractChart(t, html, IndHYOAS)
	assert.Equal(t, 2, strings.Count(hy, ","), "hy_oas polyline 2 个坐标点")
}

// extractChart 取 <polyline ... points="..."> 的 points 属性值（按指标节顺序）。
func extractChart(t *testing.T, html, ind string) string {
	t.Helper()
	i := strings.Index(html, "<h3>"+ind+"</h3>")
	require.GreaterOrEqual(t, i, 0, ind)
	rest := html[i:]
	j := strings.Index(rest, `points="`)
	require.GreaterOrEqual(t, j, 0, ind)
	rest = rest[j+len(`points="`):]
	return rest[:strings.Index(rest, `"`)]
}

// boundary: sofr_effr 全期无数据 → 省略该幅 + 专用注记；STALE 日打点存在。
func TestRenderReplayHTMLNotesAndMarkers(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.Contains(t, html, "该指标自 2018-04 起才有数据")
	assert.Contains(t, html, `<circle`, "STALE/缺数日 x 轴打点")
	assert.Contains(t, html, "2026-07-01 STALE")
}

// non_functional: 自包含（无外链）、亮暗兼容、禁词、阈值来自 cfg。
func TestRenderReplayHTMLSelfContained(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.NotContains(t, html, "http://")
	assert.NotContains(t, html, "https://")
	assert.Contains(t, html, "prefers-color-scheme")
	assert.Contains(t, html, "overflow-x")
	assert.Contains(t, html, "amber=25") // testConfig 的 vix amber，证明读 cfg
	for _, banned := range []string{"必然", "一定", "即将"} {
		assert.NotContains(t, html, banned)
	}
	assert.Contains(t, html, "历史回放，非实时告警")
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderReplayHTML -v
```

Expected: FAIL（`undefined: RenderReplayHTML`）。

- [ ] **Step 3: 实现 `internal/crisis/replay_html.go`**

```go
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
```

- [ ] **Step 4: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderReplayHTML -v
GOTOOLCHAIN=local go test ./internal/crisis/
```

Expected: 全 PASS。

注意 polyline 点数断言口径：`strings.Count(points, ",")` 每个 `x,y` 坐标恰 1 个逗号 → 逗号数 = 点数。

- [ ] **Step 5: 人工目检（可选但推荐）**

```bash
GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderReplayHTMLGoldenFragments -v
```

另写临时 main 或在测试里 `os.WriteFile` 到 scratchpad 打开浏览器目检亮暗两态（勿提交临时代码）。

- [ ] **Step 6: 提交**

code-simplifier（`internal/crisis/replay_html.go`）→ `detect_changes()` →

```bash
git add internal/crisis/replay_html.go internal/crisis/replay_html_test.go
git commit -m "feat(crisis): add RenderReplayHTML self-contained SVG report"
```

---

### Task 5: telegram SendDocument

**Files:**
- Modify: `internal/notifier/telegram/telegram.go`
- Modify: `internal/notifier/telegram/telegram_test.go`

**Interfaces:**
- Consumes: 既有 `Telegram.client`/`botToken`/`chatID`、`sendPayload` 错误语义
- Produces: `func (t *Telegram) SendDocument(path, caption string) error`（multipart/form-data 上传 `sendDocument`；caption >1024 rune 截断；文件名取 basename）

**背景（实现者须知）：** 追加新方法，`Sender` 接口（`internal/crisis/notify.go:12`，仅 `SendText`）**不动**——crisis 包不感知，cmd 层类型断言（Task 6）。错误语义与 `sendPayload`（`telegram.go:299`）一致：非 200 读 body 报 `telegram: API error (status %d)`。复用既有 `t.client`（含 proxy transport）。测试用 httptest 假 bot API + 重写 RoundTripper（既有代码 URL 写死 `api.telegram.org`，不改产线代码，测试侧重定向）。

- [ ] **Step 0: gitnexus 门禁**

`impact({target: "Telegram", direction: "upstream"})`——新增方法为纯追加，确认无既有调用方受影响即可。

- [ ] **Step 1: 写失败测试（追加到 `internal/notifier/telegram/telegram_test.go`）**

```go
// rewriteTransport 把所有请求重定向到 httptest server（产线 URL 写死
// api.telegram.org，测试侧重写 host 而不改产线代码）。
type rewriteTransport struct{ base *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = rt.base.Scheme, rt.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newDocServer(t *testing.T, status int) (*Telegram, *struct {
	path, chatID, caption, filename string
	body                            []byte
}) {
	t.Helper()
	got := &struct {
		path, chatID, caption, filename string
		body                            []byte
	}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		got.chatID = r.FormValue("chat_id")
		got.caption = r.FormValue("caption")
		f, hdr, err := r.FormFile("document")
		if err != nil {
			t.Errorf("form file: %v", err)
		} else {
			got.filename = hdr.Filename
			got.body, _ = io.ReadAll(f)
			f.Close()
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"ok": status == http.StatusOK})
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	tg := New("test-token", "test-chat")
	tg.client = &http.Client{Transport: rewriteTransport{base: u}}
	return tg, got
}

// functional: multipart 字段齐全，文件名取 basename，路径含 sendDocument。
func TestTelegram_SendDocument(t *testing.T) {
	tg, got := newDocServer(t, http.StatusOK)
	dir := t.TempDir()
	file := filepath.Join(dir, "crisis-replay-2008-01-01-2009-12-31.html")
	if err := os.WriteFile(file, []byte("<!DOCTYPE html><p>x</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := tg.SendDocument(file, "【回放总结 2008-01-01 ~ 2009-12-31】"); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	if got.path != "/bottest-token/sendDocument" {
		t.Errorf("path = %q", got.path)
	}
	if got.chatID != "test-chat" {
		t.Errorf("chat_id = %q", got.chatID)
	}
	if got.caption != "【回放总结 2008-01-01 ~ 2009-12-31】" {
		t.Errorf("caption = %q", got.caption)
	}
	if got.filename != "crisis-replay-2008-01-01-2009-12-31.html" {
		t.Errorf("filename = %q (want basename)", got.filename)
	}
	if string(got.body) != "<!DOCTYPE html><p>x</p>" {
		t.Errorf("body mismatch")
	}
}

// boundary: caption 恰 1024 rune 不截断；1025 截为 1024（按 rune，防多字节截半）。
func TestTelegram_SendDocumentCaptionTruncation(t *testing.T) {
	tg, got := newDocServer(t, http.StatusOK)
	file := filepath.Join(t.TempDir(), "r.html")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	exact := strings.Repeat("危", 1024)
	if err := tg.SendDocument(file, exact); err != nil {
		t.Fatal(err)
	}
	if got.caption != exact {
		t.Errorf("1024 rune caption 不应截断")
	}

	if err := tg.SendDocument(file, exact+"溢"); err != nil {
		t.Fatal(err)
	}
	if got.caption != exact {
		t.Errorf("1025 rune caption 应截为 1024，got len %d", len([]rune(got.caption)))
	}
}

// error_handling: 文件不存在 → 报错；API 非 200 → telegram: API error。
func TestTelegram_SendDocumentErrors(t *testing.T) {
	tg, _ := newDocServer(t, http.StatusBadRequest)
	if err := tg.SendDocument("/no/such/file.html", "c"); err == nil {
		t.Error("missing file should error")
	}
	file := filepath.Join(t.TempDir(), "r.html")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := tg.SendDocument(file, "c")
	if err == nil || !strings.Contains(err.Error(), "telegram: API error (status 400)") {
		t.Errorf("want API error, got %v", err)
	}
}
```

（需要的新增 import：`io`、`net/url`、`os`、`path/filepath`、`strings`；`json`/`http`/`httptest` 已有。）

- [ ] **Step 2: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./internal/notifier/telegram/ -run TestTelegram_SendDocument -v
```

Expected: FAIL（`tg.SendDocument undefined`）。

- [ ] **Step 3: 实现 `SendDocument`（追加到 `telegram.go`）**

```go
// documentCaptionMax 是 Bot API sendDocument 的 caption 上限（字符数）。
const documentCaptionMax = 1024

// SendDocument uploads a local file to the Bot API sendDocument endpoint via
// multipart/form-data. The filename seen by the chat is the path's basename;
// captions beyond the API limit are truncated by rune so multi-byte text never
// splits. Error semantics follow sendPayload (non-200 → telegram: API error).
func (t *Telegram) SendDocument(path, caption string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("telegram: open document: %w", err)
	}
	defer f.Close()

	if r := []rune(caption); len(r) > documentCaptionMax {
		caption = string(r[:documentCaptionMax])
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("chat_id", t.chatID); err != nil {
		return fmt.Errorf("telegram: build multipart: %w", err)
	}
	if caption != "" {
		if err := w.WriteField("caption", caption); err != nil {
			return fmt.Errorf("telegram: build multipart: %w", err)
		}
	}
	fw, err := w.CreateFormFile("document", filepath.Base(path))
	if err != nil {
		return fmt.Errorf("telegram: build multipart: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return fmt.Errorf("telegram: read document: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("telegram: build multipart: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", t.botToken)
	resp, err := t.client.Post(apiURL, w.FormDataContentType(), &buf)
	if err != nil {
		return fmt.Errorf("telegram: failed to send document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram: API error (status %d): %v", resp.StatusCode, result)
	}
	return nil
}
```

（`telegram.go` 需新增 import：`io`、`mime/multipart`、`os`、`path/filepath`。）

- [ ] **Step 4: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./internal/notifier/telegram/ -v
```

Expected: 全 PASS（含既有测试不回归）。

- [ ] **Step 5: 提交**

code-simplifier（`internal/notifier/telegram/telegram.go`）→ `detect_changes()` →

```bash
git add internal/notifier/telegram/telegram.go internal/notifier/telegram/telegram_test.go
git commit -m "feat(telegram): add SendDocument multipart upload"
```

---

### Task 6: report 子命令装配（参数/量控/发送/落盘）

**Files:**
- Create: `cmd/atlas/crisis_report.go`
- Create: `cmd/atlas/crisis_report_test.go`
- Modify: `cmd/atlas/crisis_test.go`（`snapshotCrisisFlags` 补充 report 标志位）

**Interfaces:**
- Consumes: Task 1-4 的四个导出函数；Task 5 的 `SendDocument`（经 `interface{ SendDocument(string, string) error }` 类型断言）；既有 `openCrisisStore`/`buildCrisisSender`/`Store.EvalDates`
- Produces: `atlas crisis report --from --to --form daily|monthly [--send]`；HTML 落盘 `reports/crisis-replay-<from>-<to>.html`

**背景（实现者须知）：** 设计 §1 全部行为在本任务落地。要点：回放前缀行 `【历史回放 <日期或月份> · 非实时告警】` 由 cmd 拼接（渲染器不感知）；量控在**启动前**（跑 ReplayRange 之前）用 `Store.EvalDates` 的日历算报告条数，`--send` 且 >31 报错（消息字面值固定，测试断言）；monthly 报告日 = 日历（全库）中每月首交易日落在窗口内者；`--send` 逐条发送间隔 3s（注入 `sleep` 便于测试）、单条失败记 stderr 继续；发完文本 → 总结 → HTML 经 `SendDocument` 类型断言发送（caption=总结首行），断言失败降级为「总结尾附文件路径」；stdout 打印不因 `--send` 关闭。依赖注入模式沿用 `crisisEvalDeps`（`cmd/atlas/crisis.go:208`）。

- [ ] **Step 0: gitnexus 门禁**

新文件为主；`impact({target: "buildCrisisSender", direction: "upstream"})` 与 `impact({target: "openCrisisStore", direction: "upstream"})` 确认复用安全。

- [ ] **Step 1: 写失败测试（`cmd/atlas/crisis_report_test.go`）**

复用既有 helpers：`newCrisisTestStore`、`crisisTestConfig`、`seedObservations`（全绿 N 日）、`seedReplayWatch`（80 日全绿+末 3 日 NFCI 红 → 一次 NORMAL→WATCH，末日 2026-07-10）、`stubSender`、`mustAddDays`。

```go
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/crisis"
)

// docStubSender 在 stubSender 上扩展 SendDocument 记录。
type docStubSender struct {
	stubSender
	docs [][2]string // {path, caption}
}

func (s *docStubSender) SendDocument(path, caption string) error {
	s.docs = append(s.docs, [2]string{path, caption})
	return nil
}

func reportDeps(t *testing.T, st *crisis.Store, sender crisis.Sender) (crisisReportDeps, *strings.Builder, *strings.Builder) {
	t.Helper()
	var out, errOut strings.Builder
	return crisisReportDeps{
		cfg: crisisTestConfig(), store: st,
		out: &out, errOut: &errOut,
		sender: sender, sleep: func(time.Duration) {}, htmlDir: t.TempDir(),
	}, &out, &errOut
}

// error_handling: 参数校验（必填/格式/顺序/枚举/早于库内最早日）。
func TestExecuteCrisisReportValidation(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 10)
	d, _, _ := reportDeps(t, st, nil)
	ctx := context.Background()

	for _, tc := range []struct{ from, to, form, want string }{
		{"", "2026-07-10", "daily", "--from and --to are required"},
		{"2026-7-1", "2026-07-10", "daily", "bad date"},
		{"2026-07-10", "2026-07-01", "daily", "is after"},
		{"2026-07-01", "2026-07-10", "weekly", "--form"},
	} {
		err := executeCrisisReport(ctx, d, tc.from, tc.to, tc.form, false)
		assert.ErrorContains(t, err, tc.want, tc.want)
	}

	err := executeCrisisReport(ctx, d, "2020-01-01", "2026-07-10", "daily", false)
	assert.ErrorContains(t, err, "实际可用起点")
	assert.ErrorContains(t, err, "2026-07-01") // seedObservations 10 日的首日
}

// functional: stdout 模式——逐条报告 + 总结 + HTML 落盘并打印路径；无量控。
func TestExecuteCrisisReportStdout(t *testing.T) {
	st := newCrisisTestStore(t)
	seedReplayWatch(t, st) // 末日 2026-07-10，末 3 日 NFCI 红 → NORMAL→WATCH
	d, out, _ := reportDeps(t, st, nil)

	err := executeCrisisReport(context.Background(), d, "2026-07-06", "2026-07-10", "daily", false)
	require.NoError(t, err)

	s := out.String()
	assert.Contains(t, s, "【历史回放 2026-07-10 · 非实时告警】")
	assert.Contains(t, s, "日报 第")
	assert.Contains(t, s, "【回放总结 2026-07-06 ~ 2026-07-10】")

	htmlPath := filepath.Join(d.htmlDir, "crisis-replay-2026-07-06-2026-07-10.html")
	assert.Contains(t, s, htmlPath)
	data, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<!DOCTYPE html>")
}

// 量控: 31 条恰通过、32 条拒绝（消息字面值），校验发生在启动前。
func TestExecuteCrisisReportSendQuota(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 120)
	sender := &docStubSender{}
	d, _, _ := reportDeps(t, st, sender)
	ctx := context.Background()

	err := executeCrisisReport(ctx, d, mustAddDays("2026-07-10", -31), "2026-07-10", "daily", true)
	require.Error(t, err)
	assert.Equal(t,
		"daily 回放共 32 条报告，超过 --send 上限 31。请缩短周期，或用 --form monthly，或去掉 --send 输出到 stdout。",
		err.Error())
	assert.Empty(t, sender.sent, "量控须在任何发送前拦截")

	err = executeCrisisReport(ctx, d, mustAddDays("2026-07-10", -30), "2026-07-10", "daily", true)
	require.NoError(t, err)
	assert.Len(t, sender.sent, 31+1, "31 条报告 + 1 条总结")
	require.Len(t, sender.docs, 1)
	assert.Equal(t, "【回放总结", sender.docs[0][1][:len("【回放总结")], "caption = 总结首行前缀")
	assert.Contains(t, sender.docs[0][0], "crisis-replay-")
}

// functional: monthly 报告日 = 全库日历的每月首交易日；前缀用月份。
func TestExecuteCrisisReportMonthly(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 120) // 覆盖 2026-03..07 共 5 个月首日
	sender := &docStubSender{}
	d, _, _ := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-05-01", "2026-07-10", "monthly", true)
	require.NoError(t, err)

	var reports []string
	for _, m := range sender.sent {
		if strings.HasPrefix(m, "【历史回放") {
			reports = append(reports, m)
		}
	}
	require.Len(t, reports, 3) // 2026-05 / 2026-06 / 2026-07
	assert.Contains(t, reports[0], "【历史回放 2026-05 · 非实时告警】")
	assert.Contains(t, reports[0], "Cassandra 月报")
}

// error_handling: --send 单条失败记 stderr 继续（沿评估链错误语义）。
func TestExecuteCrisisReportSendFailureContinues(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 40)
	sender := &docStubSender{}
	sender.err = assert.AnError
	d, _, errOut := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-07-08", "2026-07-10", "daily", true)
	require.NoError(t, err, "发送失败不失败退出")
	assert.Len(t, sender.sent, 3+1, "全部尝试发送")
	assert.Contains(t, errOut.String(), "warning: notify failed")
}

// boundary: sender 不支持 SendDocument → 降级为总结尾附文件路径。
func TestExecuteCrisisReportNoDocSenderDegrades(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 40)
	sender := &stubSender{} // 仅 SendText
	d, _, _ := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-07-09", "2026-07-10", "daily", true)
	require.NoError(t, err)
	last := sender.sent[len(sender.sent)-1]
	assert.Contains(t, last, "【回放总结")
	assert.Contains(t, last, "详细报告（本机）：")
	assert.Contains(t, last, "crisis-replay-2026-07-09-2026-07-10.html")
}

// error_handling: --send 且未配置 telegram → 明确报错。
func TestExecuteCrisisReportSendNeedsSender(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 10)
	d, _, _ := reportDeps(t, st, nil)

	err := executeCrisisReport(context.Background(), d, "2026-07-09", "2026-07-10", "daily", true)
	assert.ErrorContains(t, err, "notifiers.telegram")
}
```

另在 `cmd/atlas/crisis_test.go` 的 `snapshotCrisisFlags` 中补充保存/恢复 `reportFrom, reportTo, reportForm, reportSend`。

- [ ] **Step 2: 跑测试确认失败**

```bash
GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisReport -v
```

Expected: FAIL（`undefined: crisisReportDeps` / `executeCrisisReport`）。

- [ ] **Step 3: 实现 `cmd/atlas/crisis_report.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/crisis"
)

var (
	reportFrom string
	reportTo   string
	reportForm string
	reportSend bool
)

// reportSendLimit --send 模式的报告条数硬上限（设计 §1 量控）。
const reportSendLimit = 31

var crisisReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Replay history into daily/monthly reports, a summary and an HTML file",
	Long: `Replays the evaluation pipeline (warm-started from the earliest observation,
zero writes) and renders per-day or per-month reports plus a summary. Always
writes a self-contained HTML report under reports/. With --send, delivers the
texts and the HTML document to telegram (hard cap 31 reports per run).`,
	RunE: runCrisisReport,
}

func init() {
	crisisReportCmd.Flags().StringVar(&reportFrom, "from", "", "start date YYYY-MM-DD (required)")
	crisisReportCmd.Flags().StringVar(&reportTo, "to", "", "end date YYYY-MM-DD (required)")
	crisisReportCmd.Flags().StringVar(&reportForm, "form", "", "daily | monthly (required)")
	crisisReportCmd.Flags().BoolVar(&reportSend, "send", false, "send reports, summary and HTML to telegram")
	crisisCmd.AddCommand(crisisReportCmd)
}

// crisisReportDeps 注入依赖使 report 流程可单测（模式同 crisisEvalDeps）。
type crisisReportDeps struct {
	cfg     *crisis.Config
	store   *crisis.Store
	out     io.Writer
	errOut  io.Writer
	sender  crisis.Sender
	sleep   func(time.Duration)
	htmlDir string
}

func runCrisisReport(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	deps := crisisReportDeps{
		cfg: ccfg, store: st,
		out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		sender: buildCrisisSender(), sleep: time.Sleep, htmlDir: "reports",
	}
	return executeCrisisReport(cmd.Context(), deps, reportFrom, reportTo, reportForm, reportSend)
}

// documentSender 是 --send 附件路径的能力断言目标（Sender 接口不动，
// crisis 包不感知；断言失败降级为总结尾附文件路径提示）。
type documentSender interface {
	SendDocument(path, caption string) error
}

func executeCrisisReport(ctx context.Context, d crisisReportDeps, from, to, form string, send bool) error {
	if from == "" || to == "" {
		return fmt.Errorf("--from and --to are required")
	}
	for _, s := range []string{from, to} {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("bad date %q: want YYYY-MM-DD", s)
		}
	}
	if from > to {
		return fmt.Errorf("--from %s is after --to %s", from, to)
	}
	if form != "daily" && form != "monthly" {
		return fmt.Errorf("--form must be daily or monthly, got %q", form)
	}
	if send && d.sender == nil {
		return fmt.Errorf("--send 需要主配置（-c）notifiers.telegram 凭据")
	}

	// 量控与参数下界都在启动（逐日评估）前用日历完成（设计 §1：启动前直接报错退出）。
	cal, err := d.store.EvalDates(ctx, "", to)
	if err != nil {
		return err
	}
	if len(cal) == 0 {
		return fmt.Errorf("no observations up to %s — run backfill first", to)
	}
	if from < cal[0] {
		return fmt.Errorf("--from %s 早于库内最早观测日，实际可用起点：%s", from, cal[0])
	}
	isReport := reportDates(cal, from, to, form)
	nReports := 0
	for _, ok := range isReport {
		if ok {
			nReports++
		}
	}
	if send && nReports > reportSendLimit {
		return fmt.Errorf("%s 回放共 %d 条报告，超过 --send 上限 %d。请缩短周期，或用 --form monthly，或去掉 --send 输出到 stdout。",
			form, nReports, reportSendLimit)
	}

	days, err := crisis.ReplayRange(d.cfg, d.store.Reader(ctx), from, to)
	if err != nil {
		return err
	}
	if len(days) == 0 {
		return fmt.Errorf("no observations between %s and %s — run backfill first", from, to)
	}

	sr := d.store.Reader(ctx)
	var texts []string
	for i, day := range days {
		if !isReport[day.Date] {
			continue
		}
		var prev *crisis.ReplayDay
		if i > 0 {
			prev = &days[i-1]
		}
		body, err := crisis.ReplayReport(d.cfg, form, day, prev, sr)
		if err != nil {
			return err
		}
		texts = append(texts, replayPrefix(form, day.Date)+"\n"+body)
	}
	summary := crisis.RenderReplaySummary(d.cfg, days)

	for _, txt := range texts {
		fmt.Fprintln(d.out, txt)
		fmt.Fprintln(d.out)
	}
	fmt.Fprintln(d.out, summary)

	html, err := crisis.RenderReplayHTML(d.cfg, days, sr)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.htmlDir, 0o755); err != nil {
		return err
	}
	htmlPath := filepath.Join(d.htmlDir, fmt.Sprintf("crisis-replay-%s-%s.html", from, to))
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(d.out, "HTML 报告已写入 %s\n", htmlPath)

	if !send {
		return nil
	}
	sendText := func(txt string) {
		// 单条失败记 stderr 继续（沿评估链 executeCrisisEvalDaily 的错误语义）
		if err := d.sender.SendText(txt); err != nil {
			fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
		}
		d.sleep(3 * time.Second)
	}
	for _, txt := range texts {
		sendText(txt)
	}
	ds, ok := d.sender.(documentSender)
	if !ok {
		sendText(summary + "\n详细报告（本机）：" + htmlPath)
		return nil
	}
	sendText(summary)
	if err := ds.SendDocument(htmlPath, firstLine(summary)); err != nil {
		fmt.Fprintf(d.errOut, "warning: send document failed: %v\n", err)
	}
	return nil
}

// reportDates 报告日集合：daily = 窗口内全部交易日；monthly = 全库日历中
// 每月首交易日且落在窗口内者（月首判定不受窗口截断影响）。
func reportDates(cal []string, from, to, form string) map[string]bool {
	out := map[string]bool{}
	prevMonth := ""
	for _, d := range cal {
		monthFirst := d[:7] != prevMonth
		prevMonth = d[:7]
		if d < from || d > to {
			continue
		}
		if form == "daily" || monthFirst {
			out[d] = true
		}
	}
	return out
}

// replayPrefix 回放标记前缀行（cmd 层拼接，渲染器不感知回放）。
func replayPrefix(form, date string) string {
	label := date
	if form == "monthly" && len(date) >= 7 {
		label = date[:7]
	}
	return fmt.Sprintf("【历史回放 %s · 非实时告警】", label)
}

// firstLine 取总结首行作 sendDocument caption。
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisReport -v
GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/ ./internal/notifier/telegram/
```

Expected: 全 PASS。

- [ ] **Step 5: 冒烟验证（有真实库时）**

```bash
GOTOOLCHAIN=local go build ./cmd/atlas && ./atlas crisis report --from 2008-09-01 --to 2008-10-31 --form daily | head -40
open reports/crisis-replay-2008-09-01-2008-10-31.html   # 目检亮暗两态
```

Expected: 逐条日报（含回放前缀与真实差异行）、总结、HTML 路径；**不带 --send 无任何网络调用**。

- [ ] **Step 6: 提交**

code-simplifier（`cmd/atlas/crisis_report.go`）→ `detect_changes()`（确认新增 report 流程 + 无既有流程受影响）→

```bash
git add cmd/atlas/crisis_report.go cmd/atlas/crisis_report_test.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): add report subcommand with send quota and HTML output"
```

---

## Self-Review 记录

**Spec 覆盖（设计 v1.1 §1–§9 → 任务映射）：**

| 设计条目 | 任务 |
|---|---|
| §1 CLI 规格（默认输出 / --send / 量控 31 / 回放前缀 / 门控忽略 / 参数校验含最早日提示） | Task 6 |
| §2 回放引擎（暖机、窗口切片、replay 重构黄金对照、数据不足不特判） | Task 1 |
| §3 文本报告（daily PrevDay 链 / monthly Trends / 导出装配入口） | Task 2 |
| §4 标准总结（转移 / 停留 / 极值方向 / AMBER 峰值 / STALE / 回放尾注 ≤4096） | Task 3 |
| §5 HTML（点阵 / 折线阈值线 / STALE 打点 / 月度表 / 转移明细 / 自包含亮暗） | Task 4 |
| §6 telegram SendDocument（multipart / caption 1024 / Sender 接口不动） | Task 5 |
| §7 约束 | Global Constraints |
| §8 测试要点 1–8 | 逐条落在 Task 1/1/6/2/3/4/5/6 的测试代码 |
| §9 拆分 6 任务 | Task 1–6 一一对应 |

**占位符扫描：** 全部 `*(步骤待补充)*` 已替换为完整代码步骤；无 TBD/TODO。

**类型一致性核对（跨任务）：** `ReplayDay{Date, Res, StateDays}`（Task 1 定义，2/3/4/6 消费）；`ReplayReport(cfg, form, day, prev, sr)`（Task 2 定义 = Task 6 调用）；`hasFreshReading`（Task 3 定义，Task 4 复用）；`SendDocument(path, caption string) error`（Task 5 方法 = Task 6 `documentSender` 断言签名）；测试 helper 依赖顺序：`mkReplayDay` 在 Task 2 测试文件中定义、Task 3/4 同包复用——**Task 3/4 若先于 Task 2 执行会缺 helper，故任务须按序执行**（或将 `mkReplayDay` 提前抽到共享测试文件）。

**已知取舍（记录备查）：**
- `--send` 模式仍打印 stdout（默认输出行为不因发送关闭），审计友好。
- 总结 `cfg` 参数暂未使用，按设计签名保留。
- 阈值摘要用 key=value 陈列而非语义化描述，避免歪曲各指标规则方向。
- HTML 折线 x 轴 = vix 交易日历序；非 vix 交易日的观测（如周频 NFCI 落在周末的极端情形）不入折线。
