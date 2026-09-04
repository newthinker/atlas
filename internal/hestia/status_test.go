package hestia

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// —— M1b-4b / TASK-006：status 的渲染 ——
//
// Context Checkpoint: done_criteria → test mapping (status render)
// functional[0]     RenderStatus 渲染观测与 pending
//                                          → TestRenderStatus
// boundary[0]       空库上跑通并输出可读内容（不是 panic、不是空白）
//                                          → TestRenderStatusEmpty、TestStatusOnEmptyStore
// non_functional[0] status 必须打印**解析后的绝对路径**（filepath.IsAbs 钉住）
//                                          → TestRenderStatusPrintsAbsoluteDBPath
//
// 查询侧（RecentObservations / RecentPending）的映射见 store_test.go 尾部那份。
//
// —— M1.5 的 TASK-007：runs 段 ——
//
// Context Checkpoint: done_criteria → test mapping (runs 段)
// functional[0]     RenderStatus 第五参数 runs []Run，末尾 runs 段逐行渲染
//                                          → TestRenderStatusRuns
// functional[1]     runs: 3 三行 / runs: 0；既有 5 处调用点补 nil、断言不动
//                                          → TestRenderStatusRuns、TestRenderStatusRunsEmpty、TestRenderStatusReportsWriteError
// boundary[0]       nil 与空切片同形（runs: 0）；%-9s 对 duplicate 不截断；RunAt 打 Z 后缀
//                                          → TestRenderStatusRunsEmpty、TestRenderStatusRuns

func TestRenderStatus(t *testing.T) {
	var b bytes.Buffer
	v := 0.1543
	err := RenderStatus(&b, "data/hestia.db",
		[]StatusRow{{"2025-12", "annual", "2026-01-15", "rule@v2"}},
		[]PendingRow{{
			Period: "2025-11", PeriodType: "monthly",
			PublishedAt: "2025-12-15", IngestedAt: "2025-12-15T15:30:04Z",
			Failed: []Check{{
				ID: "deposit_sum", Status: CheckFailed, Value: &v,
				Reason: "tolerance_exceeded: residual 0.1543 exceeds 0.1200",
			}},
		}}, nil)
	require.NoError(t, err)

	s := b.String()
	assert.Contains(t, s, "2025-12")
	assert.Contains(t, s, "rule@v2")
	assert.Contains(t, s, "deposit_sum")
	assert.Contains(t, s, "tolerance_exceeded", "失败理由要出现，否则还是得开 sqlite")

	// 计数要如实：上面各给了一行，渲染成 0 就等于把内容丢了而输出仍然「像样」。
	assert.Contains(t, s, "observations: 1")
	assert.Contains(t, s, "pending: 1")
}

// 打印**绝对路径**。
//
// db_path 是相对路径（约束 C8），在错误的 cwd 下会静默指向另一个不存在的库，
// 然后如实报告「0 期」—— 看起来像数据没进来，实际是查错了地方。
// 打印绝对路径把这个陷阱变成一眼可见。
//
// ⚠️ 职责归属（reviewer D7）：本 Sprint 指定解析发生在 **RenderStatus 内部**，
// 不在 cmd 层。TASK-008 原先那句「cmd 层是唯一被解析的地方」已相应删去 ——
// 两处都实现会解析两次，行为无害，但下一个人读到「唯一」会去找另一处并删掉它。
func TestRenderStatusPrintsAbsoluteDBPath(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, RenderStatus(&b, "data/hestia.db", nil, nil, nil))

	want, err := filepath.Abs("data/hestia.db")
	require.NoError(t, err)
	assert.Contains(t, b.String(), want)

	// 上面那条 Contains 已经蕴含「是绝对路径」，但它把判据绑在**当前 cwd** 上；
	// 这条直接对输出里那一段做 filepath.IsAbs，是 DoD 点名的形式，且在
	// 「原样打印相对路径」的实现下会**独立**转红（不依赖 want 怎么算出来）。
	line := firstLine(t, b.String())
	require.Contains(t, line, "db: ", "库路径应出现在抬头行")
	got := strings.TrimSuffix(strings.SplitN(line, "db: ", 2)[1], ")")
	assert.True(t, filepath.IsAbs(got), "打印的必须是绝对路径，实际是 %q", got)
	assert.NotEqual(t, "data/hestia.db", got, "不得原样打印相对路径")
}

// 空库不该报错，也不该输出误导性的东西。
//
// 「输出可读内容」不等于「不 panic」：一个返回空串的实现同样不报错，而运维
// 看到空白只会以为命令挂了。observations/pending 两个计数是它必须说出来的话。
func TestRenderStatusEmpty(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, RenderStatus(&b, "data/hestia.db", nil, nil, nil))

	s := b.String()
	assert.Contains(t, s, "observations: 0")
	assert.Contains(t, s, "pending: 0")
	assert.NotEmpty(t, strings.TrimSpace(s), "空库也必须有可读输出，不能是空白")
}

// 空库上从查询到渲染跑通一遍 —— DoD boundary[0] 要的是这条链路整体成立。
//
// 上面 TestRenderStatusEmpty 喂的是 nil 切片，证明不了「查询在空库上也返回得了
// 那两个 nil」；本条从真库出发，把 status 命令在**首次部署**时的完整路径走一遍。
// 计划 Step 9 手工验收的第 2 步依赖它：配置装载错与库路径错的表现都是「0 期」，
// 空库 status 是唯一能把它们分开的东西。
func TestStatusOnEmptyStore(t *testing.T) {
	ctx := context.Background()
	s, path := newTestStoreAt(t)

	obs, err := s.RecentObservations(ctx, 10)
	require.NoError(t, err)
	pending, err := s.RecentPending(ctx, 10)
	require.NoError(t, err)

	var b bytes.Buffer
	require.NoError(t, RenderStatus(&b, path, obs, pending, nil))

	out := b.String()
	assert.Contains(t, out, "observations: 0")
	assert.Contains(t, out, "pending: 0")
	assert.Contains(t, out, path, "抬头要指出查的是哪个库文件")
}

// 写失败必须上报，不能静默成功。
//
// status 的输出常被重定向或接管道，写失败是真会发生的（磁盘满、下游提前退出）。
// 一个吞掉写错误的 RenderStatus 会让调用方以为报告已经发出去了。
//
// ⚠️ **已知缺口，本任务按计划原样保留、不扩范围**：当前实现只检查**第一次**写入的
// 错误，后续 Fprintf 的返回值一律丢弃（计划 1264-1281 行就是这么写的）。所以本条
// 只能钉住「首次写失败会上报」；若写失败发生在第二行及以后，RenderStatus 仍会
// 返回 nil。要闭合它得把所有写入串到一个 sticky-error writer 上——那是实现层面的
// 改动，已在 discovery 里登记，留给需要它的那个任务。
func TestRenderStatusReportsWriteError(t *testing.T) {
	err := RenderStatus(&failingWriter{failAt: 1}, "data/hestia.db",
		[]StatusRow{{"2025-12", "annual", "2026-01-15", "rule@v2"}}, nil, nil)
	require.Error(t, err, "写失败必须上报，不得静默成功")
	assert.ErrorContains(t, err, "disk full", "要把底层写错误带出来，而不是换成一个自造的")
}

// failingWriter 从第 failAt 次写入起一律失败。
type failingWriter struct {
	failAt int
	n      int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.n++
	if f.n >= f.failAt {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}

// firstLine 取渲染输出的抬头行，供路径断言切出 db 那一段。
func firstLine(t *testing.T, s string) string {
	t.Helper()
	line, _, ok := strings.Cut(s, "\n")
	require.True(t, ok, "输出应当至少有一行")
	return line
}

// runs 段：最近几行，outcome / 期次 / 阶段 / notify_error 都在一行里能看见。
//
// 断言里 `failed` 后 5 个空格、`ingested` 后 3 个空格是 `%-9s`（对齐到最长的
// `duplicate`）+ 两个分隔空格的结果，不是手工凑的；宽度改了这两条会一起红。
func TestRenderStatusRuns(t *testing.T) {
	at := time.Date(2026, 9, 20, 15, 30, 0, 0, time.UTC)
	runs := []Run{
		{RunAt: at, Outcome: RunFailed, Period: "2026-08", PeriodType: "monthly", Stage: "parse", Error: "boom"},
		{RunAt: at.Add(-2 * time.Hour), Outcome: RunIngested, Period: "2026-08", PeriodType: "monthly", NotifyError: "telegram down"},
		{RunAt: at.Add(-24 * time.Hour), Outcome: RunNoNew},
	}
	var b bytes.Buffer
	require.NoError(t, RenderStatus(&b, "data/hestia.db", nil, nil, runs))
	out := b.String()
	assert.Contains(t, out, "runs: 3")
	assert.Contains(t, out, "2026-09-20T15:30:00Z  failed     2026-08/monthly  stage=parse  boom")
	assert.Contains(t, out, "ingested   2026-08/monthly  notify_error=telegram down")
	assert.Contains(t, out, "no_new")
}

// nil 与空切片同形：RecentRuns 对空表返回 nil，status 要打 `runs: 0` 而不是漏掉整段。
func TestRenderStatusRunsEmpty(t *testing.T) {
	for name, runs := range map[string][]Run{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			require.NoError(t, RenderStatus(&b, "data/hestia.db", nil, nil, runs))
			assert.Contains(t, b.String(), "runs: 0")
		})
	}
}
