package tushare

// Context Checkpoint: done_criteria → test mapping（TASK-018，epoch 2 扩范围后新增）
//
// **本包属「有哨兵错误」那一支** —— 与 yahoo/twelvedata/lixinger 相反：
// 那三家无哨兵 ⇒ 断链 + 本包前缀（%v）；本包有 ErrRateLimited / ErrNoPermission，
// 且 refresh.go:450/:453 的降级链靠 errors.Is 分叉，故必须**映射成本包哨兵并保留 %w**。
// 断链会让降级链两个分支都匹配不上、退化成无分类的通用失败。
//
// functional[0]  ErrTimeout 不外泄                → TestPolicyTimeoutDoesNotLeak
// functional[1]  保留可诊断信息                    → TestMappedTimeoutKeepsDiagnosis
// boundary[0]    临时性绝不可映射成永久性          → TestMappedTimeoutIsRateLimitedNotPermission
// boundary[1]    非 policy 错误路径不受影响        → TestNonPolicyErrorsUnaffected
// boundary[2]    文案传达临时性                    → TestMappedTimeoutReadsAsTemporary

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// timeoutGate 用 Table.Override 配出必然超时的闸门。走 Override 而非直接构造
// Policy：collector.topics.*.timeout 是 TASK-005/006 落地的真实配置路径。
func timeoutGate() *policy.Gate {
	d := time.Nanosecond
	tbl := policy.NewTable()
	for _, api := range []string{"daily", "index_daily", "hk_daily", "daily_basic"} {
		tbl.Override(topicPrefix+api, policy.Override{Timeout: &d})
	}
	return policy.New(tbl, nil)
}

func timeoutClient(t *testing.T) *Client {
	t.Helper()
	srv, _ := countingServer(t, gateDailyBasicBody)
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = timeoutGate()
	return c
}

func recentSpan() (time.Time, time.Time) {
	return time.Now().AddDate(0, 0, -5), time.Now()
}

// TestPolicyTimeoutDoesNotLeak 守护 functional[0]。
//
// **参照形态自身有缺口**：本包原先只映射 ErrQuotaExceeded，`return cloneRows(rows), err`
// 把 ErrTimeout 原样外泄 —— 而 test-agent-17 实测的触发点恰恰是 ErrTimeout
// （配 timeout 即可，不需要 Quota）。这条测试守的就是那个缺口。
func TestPolicyTimeoutDoesNotLeak(t *testing.T) {
	start, end := recentSpan()
	_, err := timeoutClient(t).FetchDailyBasic("600519.SH", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if errors.Is(err, policy.ErrTimeout) {
		t.Errorf("policy 哨兵错误外泄到上层: %v", err)
	}
}

// TestMappedTimeoutIsRateLimitedNotPermission 守护 boundary[0] —— 本包走
// 「有哨兵」那一支，故断言是**正反两面**，比无哨兵包的文本判别强得多：
//
//   - 正向：必须 errors.Is 到 ErrRateLimited（refresh.go:450「本次跳过，下次自动重试」）
//   - 反向：绝不可 errors.Is 到 ErrNoPermission（refresh.go:453「永久性错误，永不重试」）
//
// 映射成永久性会让降级链把「等窗口即可」报成「去改配置」，运维照着查积分档而
// 问题根本不在那儿，且该标的**再也不会被重试**。
func TestMappedTimeoutIsRateLimitedNotPermission(t *testing.T) {
	start, end := recentSpan()
	_, err := timeoutClient(t).FetchDailyBasic("600519.SH", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("超时属临时性，必须可被 errors.Is(ErrRateLimited) 判定（降级链据此重试），got: %v", err)
	}
	if errors.Is(err, ErrNoPermission) {
		t.Errorf("临时性错误被映射成永久性 ErrNoPermission —— 降级链将永不重试该标的: %v", err)
	}
}

// TestMappedTimeoutKeepsDiagnosis 守护 functional[1]：映射是换类型不是丢信息。
func TestMappedTimeoutKeepsDiagnosis(t *testing.T) {
	start, end := recentSpan()
	_, err := timeoutClient(t).FetchDailyBasic("600519.SH", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if !strings.Contains(err.Error(), "daily_basic") {
		t.Errorf("映射后应保留出错的 api 名，got: %v", err)
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("映射后应保留原始原因，got: %v", err)
	}
}

// TestMappedTimeoutReadsAsTemporary 守护 boundary[2]：文案传达**临时性**。
// 与 boundary[0] 的类型断言是同一要求的两面（一个给程序判，一个给人读）。
func TestMappedTimeoutReadsAsTemporary(t *testing.T) {
	start, end := recentSpan()
	_, err := timeoutClient(t).FetchDailyBasic("600519.SH", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	for _, bad := range []string{"无权限", "不支持", "无此", "配置"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("临时性错误的文案暗示了永久失败 %q: %v", bad, err)
		}
	}
}

// TestNonPolicyErrorsUnaffected 守护 boundary[1]：既有的 40203 拆分不得被影响。
//
// 这是**否定断言**（契约陷阱 8）：直接观测既有错误路径仍以原有方式可识别，
// 不用「既有测试没变红」间接推断。
func TestNonPolicyErrorsUnaffected(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantIs  error
		wantNot error
	}{
		// msg 必须含 rateLimitMarker（"频率超限"，client.go:40）才走限频分支——
		// 判别串现读自实现，不凭印象编 msg 文本（我首版编了一句没有该子串的
		// 「每分钟最多访问 500 次」，结果落到无权限分支）。
		{"40203 限频", `{"code":40203,"msg":"抱歉，您访问该接口的频率超限"}`, ErrRateLimited, ErrNoPermission},
		{"40203 无权限", `{"code":40203,"msg":"抱歉，您没有接口访问权限"}`, ErrNoPermission, ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			c := NewWithBaseURL("tok", srv.URL)
			c.gate = policy.New(zeroTable(), nil) // 零策略：不产生 policy 错误

			start, end := recentSpan()
			_, err := c.FetchDailyBasic("600519.SH", start, end)
			if err == nil {
				t.Fatal("该场景必须返回错误 —— 本轮未构成检验")
			}
			if !errors.Is(err, tc.wantIs) {
				t.Errorf("既有哨兵判定被改变了，want errors.Is %v, got: %v", tc.wantIs, err)
			}
			if errors.Is(err, tc.wantNot) {
				t.Errorf("既有哨兵判定串味到 %v: %v", tc.wantNot, err)
			}
		})
	}
}
