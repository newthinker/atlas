package edgar

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   [TASK-005] XBRL filings 分部营收解析器
//   functional[0] submissions 双重筛选 + AD-3 实例 URL 推导      → TestFetchSegmentRevenueRequestFlow
//   functional[1] 单维且轴匹配才收;交叉维/轴不匹配/instant 排除  → TestFetchSegmentRevenueSingleAxisOnly
//   functional[2] 期间过滤 70~100 / 350~380(四端点对偶 + 366闰年) → TestFetchSegmentRevenuePeriodFilter
//   functional[3] AD-2 一份 10-Q 产多条 SegmentPeriod            → TestFetchSegmentRevenueMultiPeriod
//   functional[4] AD-11 unitRef 非 USD 排除(含 USD/share divide) → TestFetchSegmentRevenueUnitFilter
//   加强项        同 (period,member) 多 tag 取链上优先级最高者     → TestFetchSegmentRevenueTagPriority
//   AD-11 核心    fact 先于 context 出现仍能关联(单遍+流末关联)   → TestFetchSegmentRevenueFactBeforeContext
//   boundary[0]   instant context 安全跳过不 panic               → TestFetchSegmentRevenueSingleAxisOnly
//   boundary[0]   同 (period,member) 跨报告取 FilingDate 更晚者   → TestFetchSegmentRevenueLaterFilingWins
//   boundary[0]   回看上限 12 份                                 → TestFetchSegmentRevenueLookbackCap
//   error[0]      实例文档 404 → 跳过该期可观测,不中断整家        → TestFetchSegmentRevenueInstanceNotFound
//   error[0]      响应体超限 → 跳过该期,不 OOM                    → TestFetchSegmentRevenueBodyLimit
//   error[0]      submissions 非 200 → 带 CIK/URL 的 error        → TestFetchSegmentRevenueSubmissionsError
//   non_func      两个 host 都携带 UA                            → TestFetchSegmentRevenueRequestFlow
//   non_func      NewWithBaseURL 语义不变 / New 默认双 host       → TestSegmentClientConstructors

// recordedReq 记录一次请求的路径与 UA,供「两个 host 都带 UA」断言。
type recordedReq struct{ path, ua string }

type hostRecorder struct {
	mu   sync.Mutex
	reqs []recordedReq
}

func (h *hostRecorder) record(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reqs = append(h.reqs, recordedReq{r.URL.Path, r.Header.Get("User-Agent")})
}

func (h *hostRecorder) paths() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.reqs))
	for _, r := range h.reqs {
		out = append(out, r.path)
	}
	return out
}

// newSegmentServers 起 data host 与 archives host **两个独立 server** —— 只有分开才能验证
// NewWithBaseURLs 的双 host 注入,以及两个 host 的请求各自都携带 UA(www.sec.gov 无 UA 同样 403)。
// docs 以实例文档 basename 为键映射到 fixture 路径;未登记的 basename 返回 404
// (模拟 2019 前老式非 inline XBRL filing 的 _htm.xml 推导不成立)。
func newSegmentServers(t *testing.T, submissionsBody string, docs map[string]string) (*Client, *hostRecorder, *hostRecorder) {
	t.Helper()
	dataRec, archRec := &hostRecorder{}, &hostRecorder{}

	dataSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataRec.record(r)
		if submissionsBody == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, submissionsBody)
	}))
	t.Cleanup(dataSrv.Close)

	archSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archRec.record(r)
		fixture, ok := docs[path.Base(r.URL.Path)]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := os.ReadFile(fixture)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(archSrv.Close)

	return NewWithBaseURLs("atlas-prism/1.0 (test@example.com)", dataSrv.URL, archSrv.URL), dataRec, archRec
}

// newMiniClient 用 submissions_mini.json(单份 10-Q 命中筛选)起双 host server,实例文档
// 指向 instanceFixture。只关心解析结果、不关心请求记录的用例走这个薄封装。
func newMiniClient(t *testing.T, instanceFixture string) *Client {
	t.Helper()
	c, _, _ := newSegmentServers(t, readFixture(t, "testdata/submissions_mini.json"),
		map[string]string{"msft-20260331_htm.xml": instanceFixture})
	return c
}

func readFixture(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

// findPeriod 取指定 (start,end) 的那一期,便于按期断言。
func findPeriod(t *testing.T, got []SegmentPeriod, start, end string) SegmentPeriod {
	t.Helper()
	for _, p := range got {
		if p.PeriodStart.Format("2006-01-02") == start && p.PeriodEnd.Format("2006-01-02") == end {
			return p
		}
	}
	t.Fatalf("期间 %s~%s 未出现在结果中", start, end)
	return SegmentPeriod{}
}

const segAxis = "StatementBusinessSegmentsAxis"

var since2025 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// functional[0] + non_functional:submissions 只取 form∈{10-Q,10-K} 且 reportDate>since;
// 实例 URL 按 AD-3 推导(/Archives/ 大写 A、CIK 整数、accession 去连字符、.htm→_htm.xml);
// 两个 host 的请求都必须带 UA。
func TestFetchSegmentRevenueRequestFlow(t *testing.T) {
	c, dataRec, archRec := newSegmentServers(t,
		readFixture(t, "testdata/submissions_mini.json"),
		map[string]string{"msft-20260331_htm.xml": "testdata/instance_mini.xml"})

	_, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	require.Equal(t, []string{"/submissions/CIK0000789019.json"}, dataRec.paths(),
		"submissions 用 10 位补零 CIK,且只请求一次")
	require.Equal(t, []string{"/Archives/edgar/data/789019/000119312526191507/msft-20260331_htm.xml"},
		archRec.paths(),
		"AD-3: /Archives/ 大写 A + CIK 整数形式 + accession 去连字符 + .htm→_htm.xml;"+
			"8-K(form 不符)与 2019 年 10-K(reportDate 早于 since)都不得请求")

	for _, rec := range []*hostRecorder{dataRec, archRec} {
		for _, r := range rec.reqs {
			assert.Contains(t, r.ua, "test@example.com", "%s 缺 UA —— 两个 host 无 UA 都会被 403", r.path)
		}
	}
}

// functional[1] + boundary[0]:只收「恰好一个 explicitMember 且其 dimension local name == axis」
// 的 context。交叉维、单维但轴不匹配、instant(无 startDate)、无 member 的合并口径 context 全部排除;
// member 值去命名空间前缀。
func TestFetchSegmentRevenueSingleAxisOnly(t *testing.T) {
	c := newMiniClient(t, "testdata/instance_mini.xml")

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	q3 := findPeriod(t, got, "2026-01-01", "2026-03-31")
	assert.Equal(t, "10-Q", q3.Form)
	assert.Equal(t, time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC), q3.FilingDate)

	// member 已去 "msft:" 前缀;数值与真实文档一致(可对账)
	assert.InDelta(t, 35013000000.0, q3.Values["ProductivityAndBusinessProcessesMember"], 1)
	assert.InDelta(t, 34681000000.0, q3.Values["IntelligentCloudMember"], 1)
	assert.InDelta(t, 13192000000.0, q3.Values["MorePersonalComputingMember"], 1)
	assert.Len(t, q3.Values, 3, "该期只应有三个分部")

	// 干扰项一律不得出现,且 IntelligentCloud 不得被交叉维/instant 的值污染
	assert.NotContains(t, q3.Values, "ServerProductsAndCloudServicesMember", "轴不匹配的 context 必须排除")
	assert.NotEqual(t, 12000000000.0, q3.Values["IntelligentCloudMember"], "交叉维 context 必须排除")
	assert.NotEqual(t, 77000000000.0, q3.Values["IntelligentCloudMember"], "instant context 必须排除")
	for _, p := range got {
		assert.NotContains(t, p.Values, "", "无 explicitMember 的合并口径 context 不得产生空 member 键")
	}
}

// functional[3] AD-2:一份 10-Q 同时含本季与去年同季(均 90d),必须产出**两条** SegmentPeriod
// 而非一条 —— plan 原设计的「一报告一期」会让两期按 member 互相覆盖,赢家取决于 XML 顺序。
// 同时验证 274 天累计期被期间过滤丢弃。
func TestFetchSegmentRevenueMultiPeriod(t *testing.T) {
	c := newMiniClient(t, "testdata/instance_mini.xml")

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)
	require.Len(t, got, 2, "一份 10-Q → 本季 + 去年同季两条(274 天累计期被过滤)")

	assert.Equal(t, "2025-01-01", got[0].PeriodStart.Format("2006-01-02"), "按 PeriodEnd 升序")
	assert.Equal(t, "2026-01-01", got[1].PeriodStart.Format("2006-01-02"))

	prior := findPeriod(t, got, "2025-01-01", "2025-03-31")
	assert.InDelta(t, 26751000000.0, prior.Values["IntelligentCloudMember"], 1)
	assert.InDelta(t, 29944000000.0, prior.Values["ProductivityAndBusinessProcessesMember"], 1)

	for _, p := range got {
		assert.NotEqual(t, "2025-07-01", p.PeriodStart.Format("2006-01-02"),
			"274 天累计期必须被期间过滤丢弃")
	}
}

// AD-11 核心断言:instance_mini.xml 把 C_prior_ic 的营收 fact 放在该 context **之前**。
// 若实现边读 fact 边查已收 context(隐含「context 先于 fact」假设),这条会被静默丢弃。
func TestFetchSegmentRevenueFactBeforeContext(t *testing.T) {
	raw := readFixture(t, "testdata/instance_mini.xml")
	factAt := strings.Index(raw, `contextRef="C_prior_ic"`)
	ctxAt := strings.Index(raw, `<context id="C_prior_ic">`)
	require.Greater(t, factAt, 0)
	require.Greater(t, ctxAt, 0)
	require.Less(t, factAt, ctxAt, "fixture 前提:该 fact 必须排在其 context 之前,否则本用例是空洞的")

	c := newMiniClient(t, "testdata/instance_mini.xml")
	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	prior := findPeriod(t, got, "2025-01-01", "2025-03-31")
	assert.InDelta(t, 26751000000.0, prior.Values["IntelligentCloudMember"], 1,
		"单遍收集 + 流末关联:fact 先于 context 出现也必须被关联上")
}

// functional[4] AD-11 单位过滤:同一 (period, member) 上挂了四条营收 fact,只有 U_USD 那条算数。
// 其中 U_UnitedStatesOfAmericaDollarsShare 是真实文档里的 <divide> 单位(USD/share,每股口径),
// 「measure 里含 USD 就算数」的天真判定会把每股值当营收。
func TestFetchSegmentRevenueUnitFilter(t *testing.T) {
	c := newMiniClient(t, "testdata/instance_mini.xml")

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	prior := findPeriod(t, got, "2025-01-01", "2025-03-31")
	v := prior.Values["ProductivityAndBusinessProcessesMember"]
	assert.InDelta(t, 29944000000.0, v, 1, "只有 U_USD 的 fact 计入")
	assert.NotEqual(t, 55555000000.0, v, "U_EUR 必须排除")
	assert.NotEqual(t, 3.24, v, "U_UnitedStatesOfAmericaDollarsShare 是 USD/share,不是营收单位")
	assert.NotEqual(t, 44444000000.0, v, "unitRef 无对应 unit 定义时无法确认是 USD → 保守排除")
}

// 同 (period, member) 多个 revenue tag 并存时取 revenueTags 链上优先级最高者。
// fixture 故意把低优先级的 Revenues 排在前面,「首次出现者胜」会取到 99999B。
func TestFetchSegmentRevenueTagPriority(t *testing.T) {
	c := newMiniClient(t, "testdata/instance_mini.xml")

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	q3 := findPeriod(t, got, "2026-01-01", "2026-03-31")
	assert.InDelta(t, 13192000000.0, q3.Values["MorePersonalComputingMember"], 1,
		"取 RevenueFromContract…(优先级 0)而非文档中先出现的 Revenues(优先级 1)")
}

// functional[2] 期间过滤:两个区间的四个端点各配一对内外样本,另含 366 天闰年 FY。
func TestFetchSegmentRevenuePeriodFilter(t *testing.T) {
	c := newMiniClient(t, "testdata/instance_periods.xml")

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	kept := map[string]bool{}
	for _, p := range got {
		for member := range p.Values {
			kept[member] = true
		}
	}
	for _, tc := range []struct {
		member string
		want   bool
		why    string
	}{
		{"D069Member", false, "69 天 < 单季下界 70"},
		{"D070Member", true, "70 天 = 单季下界,含"},
		{"D100Member", true, "100 天 = 单季上界,含"},
		{"D101Member", false, "101 天 > 单季上界 100"},
		{"D349Member", false, "349 天 < FY 下界 350"},
		{"D350Member", true, "350 天 = FY 下界,含"},
		{"D366Member", true, "366 天闰年 FY 必须落在 350~380 内"},
		{"D380Member", true, "380 天 = FY 上界,含"},
		{"D381Member", false, "381 天 > FY 上界 380"},
	} {
		assert.Equal(t, tc.want, kept[tc.member], "%s: %s", tc.member, tc.why)
	}
}

// submissionsJSON 按 form/reportDate/filingDate 三元组生成 filings.recent 平行数组。
func submissionsJSON(t *testing.T, rows [][3]string) string {
	t.Helper()
	rec := map[string][]string{
		"form": {}, "accessionNumber": {}, "primaryDocument": {}, "reportDate": {}, "filingDate": {},
	}
	for i, r := range rows {
		rec["form"] = append(rec["form"], r[0])
		rec["reportDate"] = append(rec["reportDate"], r[1])
		rec["filingDate"] = append(rec["filingDate"], r[2])
		rec["accessionNumber"] = append(rec["accessionNumber"], fmt.Sprintf("0000950170-25-%06d", i))
		rec["primaryDocument"] = append(rec["primaryDocument"], fmt.Sprintf("doc-%02d.htm", i))
	}
	b, err := json.Marshal(map[string]any{"filings": map[string]any{"recent": rec}})
	require.NoError(t, err)
	return string(b)
}

// boundary[0] 回看上限:单次调用最多处理最近 12 份符合条件的报告
// (since 为零值时 filings.recent 里可达数十份 × ~10MB,不设上限首跑必然超时或触发限频)。
func TestFetchSegmentRevenueLookbackCap(t *testing.T) {
	var rows [][3]string
	docs := map[string]string{}
	for i := 0; i < 20; i++ { // 按 SEC 惯例由新到旧,reportDate 严格递减
		report := fmt.Sprintf("2026-01-%02d", 20-i)
		rows = append(rows, [3]string{"10-Q", report, report})
		docs[fmt.Sprintf("doc-%02d_htm.xml", i)] = "testdata/instance_periods.xml"
	}
	c, _, archRec := newSegmentServers(t, submissionsJSON(t, rows), docs)

	_, err := c.FetchSegmentRevenue("789019", segAxis, time.Time{})
	require.NoError(t, err)
	assert.Len(t, archRec.paths(), maxSegmentFilings, "最多只拉 maxSegmentFilings 份实例文档")
	assert.Equal(t, 12, maxSegmentFilings, "AD-11 定义的回看上限为 12")

	for _, p := range archRec.paths() {
		assert.NotContains(t, p, "doc-12_htm.xml", "超出上限的更旧报告不得被请求")
	}
}

// boundary[0]:同 (period, member) 跨两份报告重复出现 → 取 FilingDate 更晚者(重述以最新披露为准)。
// submissions 按 SEC 惯例把**更晚申报的排在前面**,故「按处理顺序后者胜」会取到旧值。
func TestFetchSegmentRevenueLaterFilingWins(t *testing.T) {
	c, _, _ := newSegmentServers(t,
		submissionsJSON(t, [][3]string{
			{"10-Q", "2027-03-31", "2027-04-28"}, // 更晚申报,含重述后的 2026Q3
			{"10-Q", "2026-03-31", "2026-04-29"}, // 更早申报,含原始 2026Q3
		}),
		map[string]string{
			"doc-00_htm.xml": "testdata/instance_restated.xml",
			"doc-01_htm.xml": "testdata/instance_mini.xml",
		})

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err)

	q3 := findPeriod(t, got, "2026-01-01", "2026-03-31")
	assert.InDelta(t, 34999000000.0, q3.Values["IntelligentCloudMember"], 1,
		"重述以 FilingDate 更晚的披露为准,而非按处理顺序")
	assert.Equal(t, time.Date(2027, 4, 28, 0, 0, 0, 0, time.UTC), q3.FilingDate,
		"该期的 FilingDate 取其数据来源中最新的一份")
}

// error[0]:某份报告的实例文档 404(2019 前老式非 inline XBRL 的 _htm.xml 推导不成立)
// → 跳过该期并可观测,不中断整家。
func TestFetchSegmentRevenueInstanceNotFound(t *testing.T) {
	logs := captureLogs(t)

	c, _, archRec := newSegmentServers(t,
		submissionsJSON(t, [][3]string{
			{"10-Q", "2026-03-31", "2026-04-29"},
			{"10-K", "2019-06-30", "2019-08-01"}, // 老式 filing,archives 未登记 → 404
		}),
		map[string]string{"doc-00_htm.xml": "testdata/instance_mini.xml"})

	got, err := c.FetchSegmentRevenue("789019", segAxis, time.Time{})
	require.NoError(t, err, "单份报告 404 不得让整家失败")
	require.Len(t, archRec.paths(), 2, "两份都尝试过")
	require.NotEmpty(t, got, "可用的那份仍须产出数据")
	assert.Contains(t, logs.String(), "doc-01_htm.xml", "跳过必须可观测(带出错文档名)")
	assert.Contains(t, logs.String(), "404")
}

// error[0]:响应体经 io.LimitReader 限流,超限该期跳过而非 OOM。
// 生产阈值 64MB 用常量断言;截断行为用压低后的阈值验证(否则要造 >64MB 响应,CI 上不现实)。
func TestFetchSegmentRevenueBodyLimit(t *testing.T) {
	assert.Equal(t, int64(64<<20), defaultMaxInstanceBytes, "AD-11 规定的响应体上限为 64MB")

	logs := captureLogs(t)

	c := newMiniClient(t, "testdata/instance_mini.xml")
	c.maxInstanceBytes = 512 // 远小于 fixture,必然被截断

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.NoError(t, err, "超限只跳过该期,不返回错误")
	assert.Empty(t, got, "被截断的文档不产出数据")
	assert.Contains(t, logs.String(), "msft-20260331_htm.xml", "超限必须可观测")
}

// error[0]:实例文档结构损坏 → 跳过该份并记录,不中断整家(与 404 同一条降级路径)。
func TestFetchSegmentRevenueMalformedInstance(t *testing.T) {
	logs := captureLogs(t)

	c, _, _ := newSegmentServers(t,
		submissionsJSON(t, [][3]string{
			{"10-Q", "2026-03-31", "2026-04-29"},
			{"10-Q", "2025-03-31", "2025-04-30"},
		}),
		map[string]string{
			"doc-00_htm.xml": "testdata/instance_malformed.xml",
			"doc-01_htm.xml": "testdata/instance_mini.xml",
		})

	got, err := c.FetchSegmentRevenue("789019", segAxis, time.Time{})
	require.NoError(t, err, "解析失败不得让整家失败")
	require.NotEmpty(t, got, "另一份可解析的报告仍须产出数据")
	assert.Contains(t, logs.String(), "doc-00_htm.xml", "解析失败必须可观测")
}

// boundary:filings.recent 是平行数组,某一列比 form 短(数据不完整)时必须安全截断,
// 不得索引越界 panic。
func TestFetchSegmentRevenueRaggedSubmissions(t *testing.T) {
	body := `{"filings":{"recent":{
		"form":["10-Q","10-Q","10-Q"],
		"accessionNumber":["0000950170-25-000000","0000950170-25-000001"],
		"primaryDocument":["doc-00.htm","doc-01.htm"],
		"reportDate":["2026-03-31","2025-03-31"],
		"filingDate":["2026-04-29"]}}}`
	c, _, archRec := newSegmentServers(t, body,
		map[string]string{"doc-00_htm.xml": "testdata/instance_mini.xml"})

	got, err := c.FetchSegmentRevenue("789019", segAxis, time.Time{})
	require.NoError(t, err)
	assert.Len(t, archRec.paths(), 1, "按最短列(filingDate 只有 1 条)截断,只处理第 1 份")
	assert.NotEmpty(t, got)
}

// error[0]:submissions 非 200 → 返回带 CIK 与 URL 上下文的 error,且不 panic。
func TestFetchSegmentRevenueSubmissionsError(t *testing.T) {
	c, _, archRec := newSegmentServers(t, "", nil)

	got, err := c.FetchSegmentRevenue("789019", segAxis, since2025)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "789019", "错误须带 CIK")
	assert.Contains(t, err.Error(), "/submissions/", "错误须带 URL 上下文")
	assert.Contains(t, err.Error(), "500")
	assert.Empty(t, archRec.paths(), "submissions 失败后不得继续拉实例文档")
}

// non_functional:NewWithBaseURLs 双 host 注入;既有 NewWithBaseURL 语义不变
// (现有 client_test.go 零改动通过即为其行为证明,此处补充构造器本身的断言)。
func TestSegmentClientConstructors(t *testing.T) {
	prod := New("ua (t@e.com)")
	assert.Equal(t, defaultBaseURL, prod.baseURL)
	assert.Equal(t, defaultArchivesURL, prod.archivesURL)
	assert.Equal(t, int64(64<<20), prod.maxInstanceBytes)

	single := NewWithBaseURL("ua (t@e.com)", "http://data.example")
	assert.Equal(t, "http://data.example", single.baseURL)
	assert.Equal(t, "http://data.example", single.archivesURL,
		"单 host 构造器把两个 host 指向同一处,避免测试构造的 client 意外打真实 www.sec.gov")

	dual := NewWithBaseURLs("ua (t@e.com)", "http://data.example", "http://arch.example")
	assert.Equal(t, "http://data.example", dual.baseURL)
	assert.Equal(t, "http://arch.example", dual.archivesURL)
}

// captureLogs 把标准 log 输出重定向到返回的 buffer,用例结束自动恢复。
func captureLogs(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return &buf
}
