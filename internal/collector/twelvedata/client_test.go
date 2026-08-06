package twelvedata

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-002 Twelve Data 客户端):
//   functional[0]     "GET {base}/time_series;query 含 symbol/interval=1day/start_date/
//                      end_date/outputsize/apikey;字符串 close 解析为 float64" → TestFetchHistoryParsesAndSorts
//                      (end_date 发 end+1 天:TD 该参数排他,live 实测,见 client.go 注释)
//                      ⚠ **与 DoD 字面偏离**:apikey 已从 query 移到 Authorization 头
//                      (QA C1 CRITICAL 密钥外泄返工)。DoD 写的是「query 含 …/apikey」,
//                      但 key 留在 query 正是外泄根因,二者不可兼得,安全优先。
//                      其余五个 query 参数不变。见 TestAPIKeyNeverInURLOrErrors。
//   functional[1]     "返回 []core.OHLCV 按 Time 升序(响应为倒序)"            → TestFetchHistoryParsesAndSorts
//   boundary[0]       "values 空或缺失 → 空切片 + err=nil"                     → TestFetchHistoryEmptyValues
//   boundary[1]       "close 不可解析(空串/\"null\")跳过该行,不中断整段"      → TestFetchHistorySkipsUnparsableClose
//   error_handling[0] "status=error → 含 message 的 error"                     → TestFetchHistoryAPIError
//   non_functional[0] "最小间隔默认 8s;注入 50ms,两次调用耗时 ≥50ms"
//                      → **已迁出**(TASK-008):8s 数值移入 policy 内置表,节流由 Gate 承接。
//                      见本文件末尾的迁出说明与 gate_test.go。

// tdServer 起一个记录 query 的 httptest server,返回 client 与取回 query 的闭包。
func tdServer(t *testing.T, body string) (*Client, func() []url.Values) {
	t.Helper()
	var mu sync.Mutex
	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Query())
		mu.Unlock()
		if r.URL.Path != "/time_series" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	// 生产默认 8s 会让用例挂死;零间隔零 TTL 的闸门由 gate_test.go 的 TestMain 装上。
	c := NewWithBaseURL("k1", srv.URL)
	return c, func() []url.Values { mu.Lock(); defer mu.Unlock(); return queries }
}

// functional[0] + functional[1]
func TestFetchHistoryParsesAndSorts(t *testing.T) {
	// 响应按 TD 实际形态给成倒序(最新在前),数值为字符串。
	body := `{"meta":{"symbol":"NVDA","interval":"1day"},"values":[
		{"datetime":"2026-08-01","open":"199.10","high":"201.00","low":"198.50","close":"200.75","volume":"1000"},
		{"datetime":"2026-07-31","open":"195.00","high":"199.00","low":"194.20","close":"198.20","volume":"900"}
	],"status":"ok"}`
	c, queries := tdServer(t, body)

	pts, err := c.FetchHistory("NVDA",
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	q := queries()
	require.Len(t, q, 1)
	assert.Equal(t, "NVDA", q[0].Get("symbol"))
	assert.Equal(t, "1day", q[0].Get("interval"))
	assert.Equal(t, "2026-07-31", q[0].Get("start_date"))
	// TD 的 end_date 排他(2026-08-02 live 实测),客户端发 end+1 天兑现闭区间契约。
	assert.Equal(t, "2026-08-02", q[0].Get("end_date"))
	assert.NotEmpty(t, q[0].Get("outputsize"))
	// 凭证不在 query 里(C1 返工):改走 Authorization 头,见 TestAPIKeyNeverInURLOrErrors。
	assert.Empty(t, q[0].Get("apikey"))

	require.Len(t, pts, 2)
	assert.True(t, pts[0].Time.Before(pts[1].Time), "结果须按 Time 升序")
	assert.Equal(t, "2026-07-31", pts[0].Time.Format("2006-01-02"))
	assert.InDelta(t, 198.20, pts[0].Close, 1e-9)
	assert.InDelta(t, 200.75, pts[1].Close, 1e-9)
	assert.Equal(t, "NVDA", pts[0].Symbol)
}

// boundary[0]:values 为空数组,以及响应完全没有 values 字段。
func TestFetchHistoryEmptyValues(t *testing.T) {
	for name, body := range map[string]string{
		"empty":   `{"values":[],"status":"ok"}`,
		"missing": `{"status":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := tdServer(t, body)
			pts, err := c.FetchHistory("NVDA", time.Now().AddDate(0, 0, -5), time.Now())
			require.NoError(t, err)
			assert.Empty(t, pts)
		})
	}
}

// boundary[1]:坏行被跳过,好行照常返回。
func TestFetchHistorySkipsUnparsableClose(t *testing.T) {
	body := `{"values":[
		{"datetime":"2026-08-01","close":"200.75"},
		{"datetime":"2026-07-31","close":""},
		{"datetime":"2026-07-30","close":"null"},
		{"datetime":"2026-07-29","close":"190.00"},
		{"datetime":"bad-date","close":"1.00"}
	],"status":"ok"}`
	c, _ := tdServer(t, body)

	pts, err := c.FetchHistory("NVDA", time.Now().AddDate(0, 0, -5), time.Now())
	require.NoError(t, err)
	require.Len(t, pts, 2, "两条坏 close + 一条坏 datetime 应被跳过,好行保留")
	assert.Equal(t, "2026-07-29", pts[0].Time.Format("2006-01-02"))
	assert.Equal(t, "2026-08-01", pts[1].Time.Format("2006-01-02"))
}

// error_handling[0]
func TestFetchHistoryAPIError(t *testing.T) {
	c, _ := tdServer(t, `{"code":401,"message":"**symbol** not found: XYZ","status":"error"}`)
	_, err := c.FetchHistory("XYZ", time.Now().AddDate(0, 0, -5), time.Now())
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "**symbol** not found: XYZ"),
		"error 必须携带 API message,实际: %v", err)
}

// C1(QA CRITICAL,rework 1):凭证绝不进 URL,且任何 error 文本都不得含 apikey。
// 本包 error 会经 prism Report.Failed 外发 Telegram(不可撤回)+ launchd 日志,
// 而传输层失败时 Go 的 *url.Error 携带完整 URL——key 若在 query 就此明文外泄。
const probeKey = "SUPERSECRETKEY123456"

func TestAPIKeyNeverInURLOrErrors(t *testing.T) {
	t.Run("凭证走请求头,query 不含 apikey", func(t *testing.T) {
		var gotAuth, gotRawQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"values":[],"status":"ok"}`))
		}))
		t.Cleanup(srv.Close)
		c := NewWithBaseURL(probeKey, srv.URL)
		_, err := c.FetchHistory("NVDA", time.Now().AddDate(0, 0, -5), time.Now())
		require.NoError(t, err)

		assert.Equal(t, "apikey "+probeKey, gotAuth, "凭证须走 Authorization 头")
		assert.NotContains(t, gotRawQuery, probeKey, "query 绝不能含 apikey")
		assert.NotContains(t, gotRawQuery, "apikey", "query 连 apikey 参数名都不该有")
	})

	// 传输层失败:127.0.0.1:1 必然 connection refused,Go 返回携带完整 URL 的 *url.Error。
	t.Run("传输层失败的 error 文本不含 apikey", func(t *testing.T) {
		c := NewWithBaseURL(probeKey, "http://127.0.0.1:1")
		_, err := c.FetchHistory("NVDA", time.Now().AddDate(0, 0, -5), time.Now())
		require.Error(t, err)
		assert.NotContains(t, err.Error(), probeKey, "error 文本外泄 apikey:%v", err)
	})

	// 兜底层单测:即便 key 因将来改动重新进了文本,wrapErr 也要抹掉它。
	t.Run("wrapErr 对任意含 key 的文本脱敏", func(t *testing.T) {
		c := NewWithBaseURL(probeKey, defaultBaseURL)
		err := c.wrapErr("boom: %s", "http://x/?apikey="+probeKey)
		assert.NotContains(t, err.Error(), probeKey)
		assert.Contains(t, err.Error(), "<redacted>")
	})

	// 空 key 时不得触发 ReplaceAll 的空串病理行为(会在每个字符间插入替换串)。
	t.Run("空 key 不破坏 error 文本", func(t *testing.T) {
		c := NewWithBaseURL("", defaultBaseURL)
		assert.Equal(t, "twelvedata: boom", c.wrapErr("boom").Error())
	})
}

// non_functional[0](TASK-002)的节流用例已随私有节流状态一并迁出:
//   - 「生产默认最小间隔 8s」→ policy 包 TestLookupBuiltinTopics(twelvedata.time_series → 8s)
//   - 「连续两次调用被节流」→ 本包 gate_test.go 的 TestFetchHistoryThrottledByGate
