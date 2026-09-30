package tiingo

// Context Checkpoint: done_criteria → test mapping（TASK-004，gate_test.go 部分）
// functional[2]     主题常量 × 生产内置表 40/1h；构造时快照 Default  → TestTopicMatchesBuiltinPolicy / TestNewSnapshotsDefaultGate
// boundary[0]       缓存：同 key 1 次 HTTP 且返回值隔离            → TestFetchHistoryCachedAndIndependent
//                   start / end 各自进键                          → TestFetchHistoryCacheKeyCoversStartAndEnd
//                   失败不缓存                                    → TestFetchHistoryErrorNotCached
// error_handling[2] ErrQuotaExceeded / ErrTimeout 映射为 retryable 且断链 → TestFetchHistoryQuotaExceededIsRetryable / TestFetchHistoryTimeoutIsRetryable

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/policy"
)

func cachingGate() *policy.Gate {
	return gateWith(policy.Policy{TTL: time.Minute, Coalesce: true}, nil)
}

// 本包其余用例都用 topicDaily 自己登记测试策略，所以**常量拼错它们全绿**；
// 而生产上 Lookup 落空 = 该主题完全不计配额（Gate 对未登记主题直通）。
func TestTopicMatchesBuiltinPolicy(t *testing.T) {
	p, ok := policy.NewTable().Lookup(topicDaily)
	require.True(t, ok, "主题 %q 未登记在 policy 内置表里", topicDaily)
	require.NotNil(t, p.Quota, "tiingo.daily 必须计配额")
	assert.Equal(t, 40, p.Quota.Limit)
	assert.Equal(t, time.Hour, p.Quota.Window)
}

func TestNewSnapshotsDefaultGate(t *testing.T) {
	want := gateWith(policy.Policy{}, nil)
	policy.SetDefault(want)
	t.Cleanup(func() { policy.SetDefault(testDefaultGate) })

	c := New(testKey)
	c2 := NewWithBaseURL(testKey, "http://127.0.0.1:1")
	assert.Same(t, want, c.gate)
	assert.Same(t, want, c2.gate)

	policy.SetDefault(gateWith(policy.Policy{}, nil))
	assert.Same(t, want, c.gate, "构造之后的 SetDefault 不影响已构造 client")
	assert.Same(t, want, c2.gate)
}

func TestFetchHistoryCachedAndIndependent(t *testing.T) {
	srv, log := serveFixture(t)
	c := NewWithBaseURL(testKey, srv.URL)
	c.gate = cachingGate()

	first, err := c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	require.Len(t, first, 5)
	first[0].Close = -1

	second, err := c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	require.Len(t, second, 5)
	_, _, n := log.last()
	assert.Equal(t, 1, n, "同 key 只发 1 次 HTTP")
	assert.InDelta(t, 122.44, second[0].Close, 1e-9, "修改第一次返回值不得污染缓存")
	assert.NotSame(t, &first[0], &second[0], "两次返回不得共享底层数组")
}

func TestFetchHistoryCacheKeyCoversStartAndEnd(t *testing.T) {
	srv, log := serveFixture(t)
	c := NewWithBaseURL(testKey, srv.URL)
	c.gate = cachingGate()

	for i, tc := range []struct {
		name       string
		start, end string
		wantHits   int
	}{
		{"首次填充缓存", "2024-06-05", "2024-06-11", 1},
		{"仅 start 不同", "2024-06-04", "2024-06-11", 2},
		{"仅 end 不同", "2024-06-05", "2024-06-12", 3},
		{"回到首个组合命中缓存", "2024-06-05", "2024-06-11", 3},
	} {
		_, err := c.FetchHistory("NVDA", day(tc.start), day(tc.end))
		require.NoError(t, err, tc.name)
		_, _, n := log.last()
		assert.Equal(t, tc.wantHits, n, "#%d %s：累计 HTTP 次数", i, tc.name)
	}
}

// 判据是「第二次真的发了 HTTP 且成功」，而非「第二次返回什么」——后者在缓存错误的实现下也可能成立。
func TestFetchHistoryErrorNotCached(t *testing.T) {
	body, err := os.ReadFile("testdata/nvda_split_sample.json")
	require.NoError(t, err)
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		n++
		first := n == 1
		mu.Unlock()
		if first {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := NewWithBaseURL(testKey, srv.URL)
	c.gate = cachingGate()
	_, err = c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.Error(t, err)
	bars, err := c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err, "失败不得写缓存")
	assert.Len(t, bars, 5)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, n)
}

// 配额窗口按 UTC 整点对齐：两次调用若恰好跨整点，第 2 次落进新窗口会被放行。
// 故观测到跨窗口就换新 Gate 重跑一轮，而不是放宽断言。
func TestFetchHistoryQuotaExceededIsRetryable(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		window := time.Now().UTC().Truncate(time.Hour)
		srv, log := serveFixture(t)
		c := NewWithBaseURL(testKey, srv.URL)
		c.gate = gateWith(policy.Policy{Quota: &policy.Quota{Limit: 1, Window: time.Hour}}, policy.NewMemStore())

		_, err1 := c.FetchHistory("NVDA", day("2024-06-03"), day("2024-06-14"))
		_, err2 := c.FetchHistory("NVDA", day("2024-06-04"), day("2024-06-14")) // 不同 key
		if !time.Now().UTC().Truncate(time.Hour).Equal(window) {
			continue
		}
		require.NoError(t, err1)
		require.Error(t, err2)
		assert.Contains(t, err2.Error(), "retryable")
		assert.True(t, strings.HasPrefix(err2.Error(), "tiingo: "))
		assert.False(t, errors.Is(err2, policy.ErrQuotaExceeded), "哨兵须断链")
		_, _, n := log.last()
		assert.Equal(t, 1, n, "超额时不得发出 HTTP")
		return
	}
	t.Fatal("连续 3 次跨整点，无法取证")
}

func TestFetchHistoryTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	c := NewWithBaseURL(testKey, srv.URL)
	c.gate = gateWith(policy.Policy{Timeout: 30 * time.Millisecond}, nil)
	_, err := c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retryable")
	assert.True(t, strings.HasPrefix(err.Error(), "tiingo: "))
	assert.False(t, errors.Is(err, policy.ErrTimeout), "哨兵须断链")
}
