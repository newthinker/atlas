package prism

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/collector/tushare"
)

// Context Checkpoint: done_criteria → test mapping (TASK-014)
// functional[0]     "本地配额耗尽时 HTTP 一次都不发出"
//                   → TestRefreshTushareLocalQuotaBlocksBeforeHTTP
// functional[1]     "配额撞墙的错误满足 errors.Is(err, tushare.ErrRateLimited)"
//                   → TestQuotaErrorSatisfiesRateLimitedAssertion
// functional[2]     "配额撞墙与限频撞墙的降级链行为路径一致"
//                   → TestLocalQuotaAndServerRateLimitDegradeIdentically
// error_handling[0] "policy.ErrQuotaExceeded 不出现在 prism 可见的错误链上"
//                   → TestQuotaErrorSatisfiesRateLimitedAssertion 的 NotErrorIs 断言
// boundary[0]       "配额未耗尽时行为与改造前一致" → refresh_test.go 既有用例全部保持通过
//                   + TestQuotaErrorSatisfiesRateLimitedAssertion 的首次调用（正向对照）

// countingTushareServer 返回一个记录**实际收到的 HTTP 请求数**的 tushare 桩服务端。
//
// 为什么必须直接数请求（design-spec 陷阱 8）：「配额耗尽时一次都不发」是**否定断言**，
// 用「服务端计数没变」或「返回了错误」去推断都不成立——错误也可能来自请求发出后被
// 服务端限流。否定断言必须直接观测那条路径本身。
func countingTushareServer(t *testing.T, body string) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

const dailyBasicBody = `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],"items":[["600519.SH","20260805",25.1,8.2,11.3]]}}`

// newRealTushare 构造一个接到指定闸门上的**真实** tushare client。
//
// ⚠ 顺序是语义的一部分（design-spec 陷阱 4）：`tushare.NewWithBaseURL` 在**构造时
// 快照** `policy.Default()`，不是每次调用现取。所以 SetDefault 必须发生在构造之前。
// 对调这两行不会编译失败，断言也大概率仍绿——client 只是拿到生产策略而非本测试的
// 策略，表现为「变慢或偶发失败」，极易被当成 flaky 掩盖。故封装在一个函数里，
// 让顺序不可能写错。调用方负责 defer 还原（见 swapDefaultGate）。
func newRealTushare(baseURL string, g *policy.Gate) *tushare.Client {
	policy.SetDefault(g)
	return tushare.NewWithBaseURL("tok", baseURL)
}

// swapDefaultGate 保存当前全局闸门，返回还原函数。
func swapDefaultGate(t *testing.T) func() {
	t.Helper()
	prev := policy.Default()
	return func() { policy.SetDefault(prev) }
}

// exhaustedLedger 写一份「本窗口已用满」的配额账本，返回其路径。
//
// 等价于「今天前面几次已经用光」。窗口起点必须与 policy 的算法一致：
// 内置 tushare.daily_basic 的 Window=24h（>=24h），故按 Quota.Loc（Asia/Shanghai）
// 的自然日 00:00 对齐；写错时间会让账本被判为「上一个窗口」而重置计数。
func exhaustedLedger(t *testing.T, topic string, count int) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(loc)
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	path := filepath.Join(t.TempDir(), "collector-quota.json")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(
		`{%q:{"window_start":%q,"count":%d}}`,
		topic, windowStart.Format(time.RFC3339), count)), 0o644))
	return path
}

// builtinDailyBasicTable 返回内置策略表，只把节流与缓存归零以加速用例，
// **保留生产的 Quota（Limit=5/自然日）** —— 配额正是被测对象，不能改。
func builtinDailyBasicTable(t *testing.T) *policy.Table {
	t.Helper()
	tbl := policy.NewTable()
	p, ok := tbl.Lookup("tushare.daily_basic")
	require.True(t, ok, "内置表必须已登记 tushare.daily_basic")
	require.NotNil(t, p.Quota, "内置 daily_basic 必须带 Quota，否则本测试无从谈起")
	require.Equal(t, 5, p.Quota.Limit, "生产配额为 5 次/自然日（ea5ac30 实测）")
	p.MinInterval = 0
	p.TTL = 0
	tbl.Set("tushare.daily_basic", p)
	return tbl
}

// 设计 §7.6 / 验收标准 4：把 tushare.daily_basic 打到配额上限后，降级链行为与
// 「撞墙后降级」一致（Degraded 文案报限频、标的判 Failed），且**不再真的发出
// 那次注定失败的 HTTP 请求**。
func TestRefreshTushareLocalQuotaBlocksBeforeHTTP(t *testing.T) {
	srv, hits := countingTushareServer(t, dailyBasicBody)
	defer swapDefaultGate(t)()

	gate := policy.New(builtinDailyBasicTable(t), policy.NewFileStore(
		exhaustedLedger(t, "tushare.daily_basic", 5)))
	ts := newRealTushare(srv.URL, gate) // 真实 client，非桩：这条链路必须端到端成立才算数

	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	refreshAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, refreshAt)

	assert.Zero(t, hits(), "配额已满时不得真的发出 HTTP 请求（撞墙前拦截）")

	require.NotEmpty(t, rep.Degraded, "应产出限频降级提示")
	assert.Contains(t, rep.Degraded[0], "限频", "文案须与撞墙后降级一致，运维口径不变")
	assert.Contains(t, rep.Degraded[0], "600519.SH")
	assert.NotContains(t, rep.Degraded[0], "配置性问题", "配额是临时性的，不得报成配置问题")
	require.Len(t, rep.Failed, 1, "兜底未成功，标的仍判失败")
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, store.upserts["600519.SH"])
}

// 验收标准 1 的锚点：refresh.go 靠 errors.Is(err, tushare.ErrRateLimited) 分叉，
// 本地配额预判必须满足同一判定，否则降级链会掉进「未识别错误」分支。
//
// ⚠ Quota.Limit 取 1 而非方案原文的 0：policy 包里 `Limit <= 0` 的语义是**不设上限**
// （quota.go:57 的 `q.Limit > 0` 是计数上限判定的前置条件，TASK-003 的决策，
// 方案写于其定稿之前）。照抄 `Limit: 0` 会让配额永不触发、请求真的发出去——
// 而方案原文那个 handler 写的是 `t.Error("配额已满时不应发出请求")`，于是失败会
// **表现成「实现有问题」而不是「测试写错了」**，极易误诊。
//
// 改为 Limit:1 后首次请求是合法的，故 handler 改为计数：第一次断言 hits==1
// （正向对照，证明链路真的通、不是整条配额链没接上），第二次断言 hits 仍为 1。
func TestQuotaErrorSatisfiesRateLimitedAssertion(t *testing.T) {
	srv, hits := countingTushareServer(t, dailyBasicBody)
	defer swapDefaultGate(t)()

	tbl := policy.NewTable()
	p, _ := tbl.Lookup("tushare.daily_basic")
	p.MinInterval, p.TTL = 0, 0
	p.Quota = &policy.Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	tbl.Set("tushare.daily_basic", p)

	c := newRealTushare(srv.URL, policy.New(tbl, policy.NewMemStore()))
	start, end := time.Now().AddDate(0, 0, -5), time.Now()

	// 正向对照：配额未耗尽时请求确实发得出去（boundary[0]）。
	_, err := c.FetchDailyBasic("600519.SH", start, end)
	require.NoError(t, err, "配额未耗尽时应正常成功")
	require.Equal(t, 1, hits(), "首次调用应真的发出请求")

	// 配额耗尽：错误必须满足 prism 降级链依赖的判定。
	_, err = c.FetchDailyBasic("000001.SZ", start, end)
	require.Error(t, err)
	assert.True(t, errors.Is(err, tushare.ErrRateLimited),
		"err = %v，必须满足 errors.Is(err, tushare.ErrRateLimited)", err)
	assert.False(t, errors.Is(err, tushare.ErrNoPermission),
		"配额是临时性错误，不得满足永久性的 ErrNoPermission")
	assert.False(t, errors.Is(err, policy.ErrQuotaExceeded),
		"policy 包的错误不得外泄到 prism 可见的错误链上（约束 C2）")
	assert.Equal(t, 1, hits(), "配额耗尽的那次不得发出请求")
}

// functional[2]：配额撞墙与服务端限频撞墙必须走**同一条**降级路径。
//
// 为什么要并排比较而不是各自断言一遍：DoD 说的是「行为路径一致」，那是一条
// **关系**性质。分别断言两次只能证明「各自符合我写下的期望」，两组期望若都写窄了，
// 不一致仍可能溜过去。直接比对两个 Report 的形状，判据不依赖我对文案的预设。
func TestLocalQuotaAndServerRateLimitDegradeIdentically(t *testing.T) {
	refreshAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	// A：服务端限频（既有路径，用桩注入 ErrRateLimited）
	serverSide := func() Report {
		store := newFakeStore()
		ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
		ts := &fakeTushare{failVal: map[string]error{
			"600519.SH": fmt.Errorf("%w: daily_basic (抱歉，您访问接口(daily_basic)频率超限(1次/分钟))",
				tushare.ErrRateLimited),
		}}
		return Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, refreshAt)
	}()

	// B：本地配额预判（新路径，真实 client + 已用满的账本）
	localSide := func() Report {
		srv, hits := countingTushareServer(t, dailyBasicBody)
		defer swapDefaultGate(t)()
		gate := policy.New(builtinDailyBasicTable(t), policy.NewFileStore(
			exhaustedLedger(t, "tushare.daily_basic", 5)))
		ts := newRealTushare(srv.URL, gate)

		store := newFakeStore()
		ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
		rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, refreshAt)
		assert.Zero(t, hits(), "本地预判不应发出请求")
		return rep
	}()

	assert.Equal(t, len(serverSide.Degraded), len(localSide.Degraded), "Degraded 条数须一致")
	assert.Equal(t, len(serverSide.Failed), len(localSide.Failed), "Failed 条数须一致")
	assert.Equal(t, serverSide.Refreshed, localSide.Refreshed, "Refreshed 计数须一致")

	// 文案的**分类**必须一致：都走限频分支，都不走权限分支。
	for _, tc := range []struct {
		name string
		rep  Report
	}{{"服务端限频", serverSide}, {"本地配额", localSide}} {
		require.NotEmpty(t, tc.rep.Degraded, "%s: 应有 Degraded", tc.name)
		assert.Contains(t, tc.rep.Degraded[0], "限频", "%s: 须走限频分支", tc.name)
		assert.Contains(t, tc.rep.Degraded[0], "下次自动重试", "%s: 须传达临时性语义", tc.name)
		assert.NotContains(t, tc.rep.Degraded[0], "配置性问题", "%s: 不得报成配置问题", tc.name)
		assert.NotContains(t, tc.rep.Degraded[0], "权限不足", "%s: 不得报成权限问题", tc.name)
	}
}
