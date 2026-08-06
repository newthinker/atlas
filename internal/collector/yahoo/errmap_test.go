package yahoo

// Context Checkpoint: done_criteria → test mapping（TASK-018）
//
// functional[0]  三处 policy.Fetch 的返回错误均经映射，errors.Is(ret, policy.ErrTimeout)==false
//                    → TestPolicySentinelDoesNotLeak（表驱动，逐个调用点）
// functional[1]  映射后仍保留可诊断信息（Error() 含原始原因）
//                    → TestMappedErrorKeepsDiagnosis
// boundary[0]    临时性不得被当成永久性
//                    → TestMappedErrorNotConfusableWithPermanent
// boundary[1]    非 policy 错误路径行为一字不变（否定断言，直接观测而非「没变红」）
//                    → TestNonPolicyErrorsUnaffected
// error[0]       映射发生在 policy.Fetch 返回值处，不在更外层统一 catch
//                    → TestMappingHappensAtFetchNotOuterLayer

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

// timeoutGate 用 Table.Override 配出一个必然超时的闸门。
//
// **刻意走 Override 而不是直接构造 Policy**：`collector.topics.*.timeout` 是
// TASK-005/006 落地的真实配置路径，Override 正是 config 装载时调用的那个函数。
// 走真实入口才能证明这个缺口今天就能被配出来，而不是只在内存里拼一个 Policy
// 自说自话（教训 13：方案自带的测试可能与方案自带的缺陷共谋）。
func timeoutGate() *policy.Gate {
	d := time.Nanosecond
	tbl := policy.NewTable()
	for _, topic := range []string{topicChart, topicEPS, topicQuote} {
		tbl.Override(topic, policy.Override{Timeout: &d})
	}
	return policy.New(tbl, nil)
}

// yahooCallSite 是一个 policy.Fetch 调用点的可执行描述。
//
// 用表驱动逐个覆盖，而不是只测一个：三处调用点各自独立返回，**漏映射任何一处
// 都是一个真实的外泄口**，只测 FetchHistory 时另外两处坏了照样全绿。
type yahooCallSite struct {
	name string
	call func(*Yahoo) error
}

func yahooCallSites() []yahooCallSite {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	return []yahooCallSite{
		{"FetchHistory/chart", func(y *Yahoo) error {
			_, err := y.FetchHistory("AAPL", start, end, "1d")
			return err
		}},
		{"FetchQuote/quote", func(y *Yahoo) error {
			_, err := y.FetchQuote("AAPL")
			return err
		}},
		{"FetchEPSHistory/eps", func(y *Yahoo) error {
			_, err := y.FetchEPSHistory("AAPL", start, end)
			return err
		}},
	}
}

// okServer 永远返回一份合法响应：本文件的错误全部来自 Gate，不来自服务端。
func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":101.5,` +
			`"chartPreviousClose":100.0,"currency":"USD"},"timestamp":[1600000000],` +
			`"indicators":{"quote":[{"open":[1],"high":[2],"low":[0.5],"close":[1.5],"volume":[10]}]}}]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestPolicySentinelDoesNotLeak 守护 functional[0]。
//
// 判据是**真实哨兵错误**而非消息文本：文本断言在包装方式改变时会假绿/假红，
// 而 errors.Is 判定的正是上层实际会用的那个手段。
//
// 触发的是**真的** policy.ErrTimeout（Timeout=1ns 让 runWithTimeout 必然超时），
// 不是 mock —— 先自证「不映射时确实会外泄」，这条断言才有意义。
func TestPolicySentinelDoesNotLeak(t *testing.T) {
	for _, cs := range yahooCallSites() {
		t.Run(cs.name, func(t *testing.T) {
			y := NewWithBaseURL(okServer(t).URL)
			y.gate = timeoutGate()

			err := cs.call(y)
			if err == nil {
				t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
			}
			if errors.Is(err, policy.ErrTimeout) {
				t.Errorf("policy 哨兵错误外泄到上层: %v", err)
			}
			if errors.Is(err, policy.ErrQuotaExceeded) {
				t.Errorf("policy 哨兵错误外泄到上层: %v", err)
			}
		})
	}
}

// TestMappedErrorKeepsDiagnosis 守护 functional[1]：映射是换类型不是丢信息。
//
// 只把哨兵错误吞掉换成一句无信息的通用错误，线上排障会失去线索 —— 那种实现
// 能通过上一条断言（Is 为 false），却让人无从判断到底发生了什么。
func TestMappedErrorKeepsDiagnosis(t *testing.T) {
	for _, cs := range yahooCallSites() {
		t.Run(cs.name, func(t *testing.T) {
			y := NewWithBaseURL(okServer(t).URL)
			y.gate = timeoutGate()

			err := cs.call(y)
			if err == nil {
				t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
			}
			if !strings.Contains(err.Error(), "timeout") {
				t.Errorf("映射后应保留原始原因，got: %v", err)
			}
		})
	}
}

// yahooPermanentPhrases 是本包表示「永久性失败」的判别文本。
//
// ⚠ 本包**没有任何哨兵错误常量**（实测：0 个 var Err / errors.New），永久性
// 条件全部以 ad-hoc fmt.Errorf 的文本形式存在。所以 DoD boundary[0] 原本规定的
// 「逐个列举本包现有的永久性错误常量并断言 errors.Is 全为 false」在本包**集合为空、
// 断言空真**（契约陷阱 6）—— 一个什么都不做的实现照样能通过。
// 这里改用这些文本做判别，是在没有哨兵可用时能给出的**有内容**的断言。
var yahooPermanentPhrases = []string{
	"no data for symbol",     // yahoo.go:259 / :337
	"invalid symbol format",  // yahoo.go:42
	"symbol too long",        // yahoo.go:39
	"symbol cannot be empty", // yahoo.go:36
	"unavailable for index",  // eps.go:44
}

// TestMappedErrorNotConfusableWithPermanent 守护 boundary[0]：临时性绝不可被
// 当成永久性。
//
// policy.ErrTimeout 是**可重试**的临时故障；若映射后长得像「无此标的/无数据」，
// 上层会停止重试并把错误结果落库 —— 那是一次静默的数据损坏，不是一次失败。
func TestMappedErrorNotConfusableWithPermanent(t *testing.T) {
	if len(yahooPermanentPhrases) == 0 {
		t.Fatal("永久性判别集合为空，本测试空转") // 下界，防空真
	}
	for _, cs := range yahooCallSites() {
		t.Run(cs.name, func(t *testing.T) {
			y := NewWithBaseURL(okServer(t).URL)
			y.gate = timeoutGate()

			err := cs.call(y)
			if err == nil {
				t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
			}
			for _, phrase := range yahooPermanentPhrases {
				if strings.Contains(err.Error(), phrase) {
					t.Errorf("临时性错误被表述成永久性失败 %q: %v", phrase, err)
				}
			}
		})
	}
}

// TestNonPolicyErrorsUnaffected 守护 boundary[1]。
//
// 这是**否定断言**（契约陷阱 8）：「没有影响到别的错误路径」不能靠「既有测试
// 没变红」间接推断 —— 那个观测量由多种原因产生（比如那些测试本来就没覆盖到）。
// 这里直接观测：让服务端产生真实的 HTTP/业务错误，断言它仍以原有方式可识别，
// 且**没有**被误加上 policy 映射的痕迹。
func TestNonPolicyErrorsUnaffected(t *testing.T) {
	cases := []struct {
		name       string
		handler    http.HandlerFunc
		wantPhrase string
	}{
		// 取 400 而非 500：两者走同一条 unexpected status 路径，但 500 是
		// retryableStatus 会触发退避重试，白等 7s 且与本用例要验的东西无关。
		{"HTTP 400", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}, "unexpected status"},
		{"畸形 JSON", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}, "decoding response"},
		{"业务错误", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"chart":{"error":{"description":"Not Found"}}}`))
		}, "yahoo error"},
		{"空数据", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"chart":{"result":[]}}`))
		}, "no data for symbol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			y := NewWithBaseURL(srv.URL)
			y.gate = gateWith(policy.Policy{}) // 零策略：不会产生任何 policy 错误

			start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
			_, err := y.FetchHistory("AAPL", start, end, "1d")
			if err == nil {
				t.Fatal("该场景必须返回错误 —— 本轮未构成检验")
			}
			if !strings.Contains(err.Error(), tc.wantPhrase) {
				t.Errorf("既有错误路径的可识别性被改变了，want 含 %q, got: %v", tc.wantPhrase, err)
			}
		})
	}
}

// TestMapPolicyErrPassesThroughNonPolicyErrors 是 boundary[1] 的**精确**守护者。
//
// 端到端那条（TestNonPolicyErrorsUnaffected）**不够**：实测变异「把所有错误都包
// 一层」（等价于在更外层统一 catch）下它仍然全绿 —— 被包过的 HTTP 错误文本里
// 既仍含 "unexpected status"，又不含 "timeout"，两条文本断言同时被绕过。
//
// 这里改为**直接观测那条路径本身**（契约陷阱 8）：非 policy 错误必须原样返回
// **同一个 error 值**。指针相等是最强的「未被改动」断言，且与措辞无关 ——
// 换个包装文案不会让它悄悄失效。
func TestMapPolicyErrPassesThroughNonPolicyErrors(t *testing.T) {
	cases := []error{
		errors.New("unexpected status: 400"),
		fmt.Errorf("decoding response: %w", errors.New("unexpected EOF")),
		errors.New("no data for symbol: AAPL"),
		context.DeadlineExceeded, // HTTP 客户端自己的超时：与 policy 超时同名不同源
	}
	if len(cases) == 0 {
		t.Fatal("语料为空，本测试空转")
	}
	for _, in := range cases {
		if got := mapPolicyErr(in); got != in {
			t.Errorf("非 policy 错误必须原样返回同一个 error 值\n  in  = %v (%T)\n  got = %v (%T)",
				in, in, got, got)
		}
	}
}

// TestMappingHappensAtFetchNotOuterLayer 守护 error_handling[0]：映射必须发生在
// policy.Fetch 的返回值处，不得在更外层统一 catch。
//
// 判据：**同一条调用链上，policy 产生的超时与非 policy 错误必须可区分**。
// 更外层统一 catch 会把两类不同来源的错误压成一类——那样的实现下，两者的
// 表述会变得同构，本断言转红。
func TestMappingHappensAtFetchNotOuterLayer(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	y1 := NewWithBaseURL(okServer(t).URL)
	y1.gate = timeoutGate()
	_, policyErr := y1.FetchHistory("AAPL", start, end, "1d")

	// 400 而非 500：同一条 unexpected status 路径，但不触发退避重试（见上）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	y2 := NewWithBaseURL(srv.URL)
	y2.gate = gateWith(policy.Policy{})
	_, httpErr := y2.FetchHistory("AAPL", start, end, "1d")

	if policyErr == nil || httpErr == nil {
		t.Fatalf("两条路径都必须出错才构成对照: policy=%v http=%v", policyErr, httpErr)
	}
	if policyErr.Error() == httpErr.Error() {
		t.Fatalf("policy 超时与 HTTP 错误被压成了同一种表述: %v", policyErr)
	}
	if !strings.Contains(policyErr.Error(), "timeout") {
		t.Errorf("policy 超时应可辨认，got: %v", policyErr)
	}
	if strings.Contains(httpErr.Error(), "timeout") {
		t.Errorf("HTTP 错误被误标成 timeout（更外层统一 catch 的典型症状）: %v", httpErr)
	}
}
