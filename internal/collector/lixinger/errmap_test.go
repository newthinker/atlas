package lixinger

// Context Checkpoint: done_criteria → test mapping（TASK-018，epoch 2 扩范围后新增）
//
// functional[0]  request() 那处 policy.Fetch 的返回错误经映射，errors.Is(ret, policy.ErrTimeout)==false
//                    → TestPolicySentinelDoesNotLeak
// functional[1]  映射后仍保留可诊断信息            → TestMappedErrorKeepsDiagnosis
// boundary[0]    临时性不得被当成永久性            → TestMappedErrorNotConfusableWithPermanent
// boundary[1]    非 policy 错误路径不受影响        → TestMapPolicyErrPassesThroughNonPolicyErrors（精确）
//                                                    + TestNonPolicyErrorsUnaffected（端到端）
// boundary[2]    文案传达临时性                    → TestMappedErrorReadsAsTemporary
// error[0]       映射在 Fetch 返回处而非更外层     → TestMappingHappensAtFetchNotOuterLayer
//
// **本包属「无哨兵错误」那一支**（实测 0 个 var Err / errors.New）⇒ 断链 + 本包前缀（%v）。
// 有哨兵的包（tushare）走另一支：映射成本包哨兵并保留 %w，因为 refresh.go 的降级链靠
// errors.Is 分叉。两种口径由 DoD error_handling[1] 按「本包有无哨兵」分派，不是各写各的。

import (
	"context"
	"encoding/json"
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
	tbl.Override("lixinger."+gateEndpoint, policy.Override{Timeout: &d})
	return policy.New(tbl, nil)
}

// TestPolicySentinelDoesNotLeak 守护 functional[0]。
//
// 判据是**真实哨兵错误**而非消息文本：DoD error_handling[1] 记的那个陷阱
// （dev-agent-35 变异 B8 实证）——用 %w 包一层本包前缀时消息变了，
// 但 errors.Is 仍然成立，只查文本会被骗过。
func TestPolicySentinelDoesNotLeak(t *testing.T) {
	srv, _ := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = timeoutGate()

	_, err := l.request(gateEndpoint, map[string]any{"x": 1})
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
func TestMappedErrorKeepsDiagnosis(t *testing.T) {
	srv, _ := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = timeoutGate()

	_, err := l.request(gateEndpoint, map[string]any{"x": 1})
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("映射后应保留原始原因，got: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "lixinger: ") {
		t.Errorf("无哨兵的包须带本包前缀，got: %v", err)
	}
}

// TestMappedErrorReadsAsTemporary 守护 boundary[2]：文案必须传达**临时性**。
//
// 与 boundary[0] 的类型断言是同一要求的两面 —— 一个给程序判，一个给人读。
// 文案写成「不支持/无此数据」会让运维照着去改配置，而问题只是要等窗口。
func TestMappedErrorReadsAsTemporary(t *testing.T) {
	srv, _ := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = timeoutGate()

	_, err := l.request(gateEndpoint, map[string]any{"x": 1})
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	if !strings.Contains(err.Error(), "retryable") && !strings.Contains(err.Error(), "temporary") {
		t.Errorf("文案须传达临时性（可重试/稍后恢复），got: %v", err)
	}
}

// lixPermanentPhrases 是本包表示「永久性失败」的判别文本。
//
// ⚠ 本包**没有任何哨兵错误常量**，永久性条件全部以 ad-hoc fmt.Errorf 文本形式
// 存在，故 DoD boundary[0] 走「无哨兵」那一支：断言映射结果的 Error() 文本不含
// 这些判别串（逐条列举）。
var lixPermanentPhrases = []string{
	"API error",              // client.go:149/:152/:155/:158 —— 信封业务错误（含无此标的）
	"unexpected HTTP status", // client.go:105 —— 4xx，改配置前重试无意义
	"decode envelope",        // client.go:142
}

// TestMappedErrorNotConfusableWithPermanent 守护 boundary[0]。
func TestMappedErrorNotConfusableWithPermanent(t *testing.T) {
	if len(lixPermanentPhrases) == 0 {
		t.Fatal("永久性判别集合为空，本测试空转") // 下界，防空真
	}
	srv, _ := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = timeoutGate()

	_, err := l.request(gateEndpoint, map[string]any{"x": 1})
	if err == nil {
		t.Fatal("Timeout=1ns 下必须返回错误 —— 本轮未构成检验")
	}
	for _, phrase := range lixPermanentPhrases {
		if strings.Contains(err.Error(), phrase) {
			t.Errorf("临时性错误被表述成永久性失败 %q: %v", phrase, err)
		}
	}
}

// TestMapPolicyErrPassesThroughNonPolicyErrors 是 boundary[1] 的**精确**守护者。
//
// 端到端那条不够：yahoo 侧实测变异「把所有错误都包一层」（= 更外层统一 catch）
// 下端到端断言仍全绿 —— 被包过的错误文本里既仍含原有判别词、又不含 "timeout"。
// 这里直接观测那条路径本身（契约陷阱 8）：非 policy 错误必须原样返回**同一个
// error 值**。指针相等是最强的「未被改动」断言，且与措辞无关。
func TestMapPolicyErrPassesThroughNonPolicyErrors(t *testing.T) {
	cases := []error{
		errors.New("lixinger: API error: 无此标的"),
		fmt.Errorf("lixinger: decode envelope: %w", errors.New("unexpected EOF")),
		errors.New("lixinger: unexpected HTTP status 400"),
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

// TestNonPolicyErrorsUnaffected 守护 boundary[1] 的端到端一面。
func TestNonPolicyErrorsUnaffected(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		status     int
		wantPhrase string
	}{
		{"信封业务错误", `{"code":0,"error":{"message":"无此标的"}}`, http.StatusOK, "API error"},
		{"畸形 JSON", `not-json`, http.StatusOK, "decode envelope"},
		// 4xx 分支要走到「unexpected HTTP status」，body 必须是**合法且 code==1**
		// 的信封（client.go:102-105：先试 parseEnvelope，它报错就返回那个错）。
		// 首版给 `{}` 会解析成 code=0 → 落到「API error code 0」那条，测不到本分支。
		{"4xx", `{"code":1}`, http.StatusBadRequest, "unexpected HTTP status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			l := NewWithBaseURL("key", srv.URL)
			l.gate = policy.New(gateTable(policy.Policy{}), nil) // 零策略：不产生 policy 错误

			_, err := l.request(gateEndpoint, map[string]any{"x": 1})
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

// TestMappingHappensAtFetchNotOuterLayer 守护 error_handling[0]。
//
// 判据：同一条调用链上，policy 产生的超时与 fn 内部产生的错误必须可区分。
// 更外层统一 catch 会把两类不同来源的错误压成一类，本断言随之转红。
func TestMappingHappensAtFetchNotOuterLayer(t *testing.T) {
	srvOK, _ := countingLixServer(t)
	l1 := NewWithBaseURL("key", srvOK.URL)
	l1.gate = timeoutGate()
	_, policyErr := l1.request(gateEndpoint, map[string]any{"x": 1})

	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srvBad.Close)
	l2 := NewWithBaseURL("key", srvBad.URL)
	l2.gate = policy.New(gateTable(policy.Policy{}), nil)
	_, decodeErr := l2.request(gateEndpoint, map[string]any{"x": 1})

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

// TestNonPolicyErrorChainPreserved 守护 boundary[1] 的**第三面**：错误**链**本身。
//
// 前两条守不住这一面，这不是推测而是实测出来的（test-agent-17 在 TASK-018 上造的
// 对抗变异）：`mapPolicyErr` 一个字不动，只在**调用点**多包一层——
// `return nil, mapPolicyErr(err)` → `return nil, fmt.Errorf("lixinger: %v", mapPolicyErr(err))`
// ——整个包**无一转红**。
//
//   - TestMapPolicyErrPassesThroughNonPolicyErrors 直测映射函数，函数没改 ⇒ 绿
//   - TestNonPolicyErrorsUnaffected 判据是「文本里还有没有原判别词」，`%v` 把原文
//     原样带过 ⇒ 绿
//
// 两者的判据分别是「是不是同一个 error 值」和「文本对不对」，**都不看链**。而 `%v`
// 恰好只切链、不改文本。⇒ 有哨兵的包（tushare）靠 errors.Is 判据免费拿到类型级守护，
// **无哨兵的包必须显式对某个上游可判定错误写 errors.As/errors.Is**，否则「链保留」
// 这个属性没有任何东西钉住。
//
// 判据选 `*json.SyntaxError` 而非 `io.ErrUnexpectedEOF`：两者都对但**互斥**，由 body
// 形态唯一决定——`not-json` 这类非法字符 body 产生 SyntaxError，`{"a":` 这类截断 body
// 才产生 ErrUnexpectedEOF。test-agent-17 首版在本包照搬 yahoo 的 ErrUnexpectedEOF，
// **基线就红**。本包用 json.Unmarshal、body 用 `not-json`，已现读实测确认走 SyntaxError。
func TestNonPolicyErrorChainPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)

	l := NewWithBaseURL("key", srv.URL)
	l.gate = policy.New(gateTable(policy.Policy{}), nil) // 零策略：不产生 policy 错误

	_, err := l.request(gateEndpoint, map[string]any{"x": 1})
	if err == nil {
		t.Fatal("畸形 JSON 必须返回错误 —— 本轮未构成检验")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("链被切断: 上游解码错误必须能被 errors.As 穿透到 *json.SyntaxError\n"+
			"  got: %v (%T)\n"+
			"  调用点若用 %%v 多包一层（而非 %%w），文本一字不变、映射函数也没改，"+
			"但上层再也无法按类型判别上游错误", err, err)
	}
}
