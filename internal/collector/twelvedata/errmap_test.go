package twelvedata

// Context Checkpoint: done_criteria → test mapping（TASK-018）
//
// functional[0]  唯一那处 policy.Fetch 的返回错误经映射，errors.Is(ret, policy.ErrTimeout)==false
//                    → TestPolicySentinelDoesNotLeak
// functional[1]  映射后仍保留可诊断信息                → TestMappedErrorKeepsDiagnosis
// boundary[0]    临时性不得被当成永久性                → TestMappedErrorNotConfusableWithPermanent
// boundary[1]    非 policy 错误路径行为一字不变        → TestNonPolicyErrorsUnaffected
// error[0]       映射在 Fetch 返回处而非更外层统一 catch → TestMappingHappensAtFetchNotOuterLayer
//
// ⚠ 本包有一条既有约束会直接影响映射写法：`wrapErr` 是**本包唯一 error 出口**，
// 它刻意用 %v 而非 %w 断链，理由是凭证脱敏（留了链 errors.Unwrap 就能取回未脱敏
// 的原始 error）。映射必须沿用这个出口，不能另起一套绕过脱敏。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// timeoutGate 用 Table.Override 配出必然超时的闸门。
//
// **刻意走 Override 而非直接构造 Policy**：`collector.topics.*.timeout` 是
// TASK-005/006 落地的真实配置路径，Override 正是 config 装载时调用的那个函数。
// 走真实入口才能证明这个缺口今天就能被配出来（教训 13）。
func timeoutGate() *policy.Gate {
	d := time.Nanosecond
	tbl := policy.NewTable()
	tbl.Override(topicTimeSeries, policy.Override{Timeout: &d})
	return policy.New(tbl, nil)
}

// TestPolicySentinelDoesNotLeak 守护 functional[0]。判据是真实哨兵错误而非
// 消息文本 —— 文本断言在包装方式改变时会假绿/假红，而 errors.Is 正是上层
// 实际会用的手段。触发的是**真的** policy.ErrTimeout，不是 mock。
func TestPolicySentinelDoesNotLeak(t *testing.T) {
	srv, _ := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = timeoutGate()

	start, end := recentRange()
	_, err := c.FetchHistory("NVDA", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if errors.Is(err, policy.ErrTimeout) {
		t.Errorf("policy 哨兵错误外泄到上层: %v", err)
	}
	if errors.Is(err, policy.ErrQuotaExceeded) {
		t.Errorf("policy 哨兵错误外泄到上层: %v", err)
	}
}

// TestMappedErrorKeepsDiagnosis 守护 functional[1]：映射是换类型不是丢信息。
// 只把哨兵错误吞掉换成一句无信息的通用错误，线上排障会失去线索。
func TestMappedErrorKeepsDiagnosis(t *testing.T) {
	srv, _ := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = timeoutGate()

	start, end := recentRange()
	_, err := c.FetchHistory("NVDA", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("映射后应保留原始原因，got: %v", err)
	}
	// 沿用本包唯一 error 出口 ⇒ 必带包前缀。绕过 wrapErr 的实现会漏掉它，
	// 而绕过就意味着绕过了凭证脱敏。
	if !strings.HasPrefix(err.Error(), "twelvedata: ") {
		t.Errorf("映射应经本包唯一 error 出口 wrapErr（含脱敏），got: %v", err)
	}
}

// tdPermanentPhrases 是本包表示「永久性失败」的判别文本。
//
// ⚠ 本包**没有任何哨兵错误常量**（`wrapErr` 的注释明写「本包无 sentinel error」），
// 永久性条件全部以文本形式存在。所以 DoD boundary[0] 原本规定的「逐个列举本包
// 现有的永久性错误常量并断言 errors.Is 全为 false」在本包**集合为空、断言空真**
// （契约陷阱 6）—— 什么都不做的实现照样通过。这里改用文本判别，是没有哨兵可用
// 时能给出的**有内容**的断言。
var tdPermanentPhrases = []string{
	"build request", // client.go:129
	"read body",     // client.go:141
	"decode",        // client.go:146
	"code ",         // client.go:151 —— TD 业务错误码（如 400 symbol not found）
}

// TestMappedErrorNotConfusableWithPermanent 守护 boundary[0]：临时性绝不可被
// 当成永久性。policy.ErrTimeout 是**可重试**的临时故障；若映射后长得像
// 「解析失败/业务错误码」，上层会停止重试并把错误结果落库。
func TestMappedErrorNotConfusableWithPermanent(t *testing.T) {
	if len(tdPermanentPhrases) == 0 {
		t.Fatal("永久性判别集合为空，本测试空转") // 下界，防空真
	}
	srv, _ := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = timeoutGate()

	start, end := recentRange()
	_, err := c.FetchHistory("NVDA", start, end)
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	for _, phrase := range tdPermanentPhrases {
		if strings.Contains(err.Error(), phrase) {
			t.Errorf("临时性错误被表述成永久性失败 %q: %v", phrase, err)
		}
	}
}

// TestNonPolicyErrorsUnaffected 守护 boundary[1]。
//
// 这是**否定断言**（契约陷阱 8）：「没有影响到别的错误路径」不能靠「既有测试
// 没变红」间接推断。这里直接观测：让服务端产生真实错误，断言它仍以原有方式
// 可识别，且**没有**被误加上 policy 映射的痕迹。
func TestNonPolicyErrorsUnaffected(t *testing.T) {
	cases := []struct {
		name       string
		handler    http.HandlerFunc
		wantPhrase string
	}{
		{"畸形 JSON", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}, "decode"},
		{"业务错误码", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error","code":400,"message":"symbol not found"}`))
		}, "code 400"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			c := NewWithBaseURL("k", srv.URL)
			c.gate = gateWith(policy.Policy{}) // 零策略：不会产生任何 policy 错误

			start, end := recentRange()
			_, err := c.FetchHistory("NVDA", start, end)
			if err == nil {
				t.Fatal("该场景必须返回错误 —— 本轮未构成检验")
			}
			if !strings.Contains(err.Error(), tc.wantPhrase) {
				t.Errorf("既有错误路径的可识别性被改变了，want 含 %q, got: %v", tc.wantPhrase, err)
			}
			if strings.Contains(err.Error(), "timeout") {
				t.Errorf("非 policy 错误被误标成 timeout: %v", err)
			}
		})
	}
}

// TestMapPolicyErrPassesThroughNonPolicyErrors 是 boundary[1] 的**精确**守护者。
//
// 端到端那条（TestNonPolicyErrorsUnaffected）**不够**：yahoo 侧实测变异「把所有
// 错误都包一层」（等价于在更外层统一 catch）下它仍然全绿 —— 被包过的错误文本里
// 既仍含原有判别词、又不含 "timeout"，两条文本断言同时被绕过。
//
// 这里改为**直接观测那条路径本身**（契约陷阱 8）：非 policy 错误必须原样返回
// **同一个 error 值**。指针相等是最强的「未被改动」断言，且与措辞无关。
func TestMapPolicyErrPassesThroughNonPolicyErrors(t *testing.T) {
	c := NewWithBaseURL("k", "http://example.invalid")
	cases := []error{
		errors.New("twelvedata: NVDA: decode: unexpected EOF"),
		fmt.Errorf("twelvedata: NVDA: %w", errors.New("read body")),
		errors.New("twelvedata: NVDA: code 400: symbol not found"),
		context.DeadlineExceeded, // HTTP 客户端自己的超时：与 policy 超时同名不同源
	}
	if len(cases) == 0 {
		t.Fatal("语料为空，本测试空转")
	}
	for _, in := range cases {
		if got := c.mapPolicyErr(in); got != in {
			t.Errorf("非 policy 错误必须原样返回同一个 error 值\n  in  = %v (%T)\n  got = %v (%T)",
				in, in, got, got)
		}
	}
}

// TestMappingHappensAtFetchNotOuterLayer 守护 error_handling[0]：映射必须发生在
// policy.Fetch 的返回值处，不得在更外层统一 catch。
//
// 判据：同一条调用链上，policy 产生的超时与 fn 内部产生的错误必须可区分。
// 更外层统一 catch 会把两类不同来源的错误压成一类，本断言随之转红。
func TestMappingHappensAtFetchNotOuterLayer(t *testing.T) {
	start, end := recentRange()

	srvOK, _ := probeServer(t, gateSeriesBody)
	c1 := NewWithBaseURL("k", srvOK.URL)
	c1.gate = timeoutGate()
	_, policyErr := c1.FetchHistory("NVDA", start, end)

	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srvBad.Close)
	c2 := NewWithBaseURL("k", srvBad.URL)
	c2.gate = gateWith(policy.Policy{})
	_, decodeErr := c2.FetchHistory("NVDA", start, end)

	if policyErr == nil || decodeErr == nil {
		t.Fatalf("两条路径都必须出错才构成对照: policy=%v decode=%v", policyErr, decodeErr)
	}
	if policyErr.Error() == decodeErr.Error() {
		t.Fatalf("policy 超时与解析错误被压成了同一种表述: %v", policyErr)
	}
	if !strings.Contains(policyErr.Error(), "timeout") {
		t.Errorf("policy 超时应可辨认，got: %v", policyErr)
	}
	if strings.Contains(decodeErr.Error(), "timeout") {
		t.Errorf("解析错误被误标成 timeout（更外层统一 catch 的典型症状）: %v", decodeErr)
	}
}
