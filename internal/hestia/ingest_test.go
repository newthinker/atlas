package hestia

// Context Checkpoint: done_criteria → test mapping (ingest)
// functional[0]     IngestDeps{Store,Fetch,Cfg,Out,Force} + Ingest(ctx,d)，端到端用 syntheticIndex
//                                                   → TestIngestEndToEnd、TestIngestSkipsSeenArticleUnlessForce
// functional[1]     接缝①：ArticleID 由 ingest 补     → TestIngestFillsArticleID
// functional[2]     接缝②：期次交叉校验拦下不入库     → TestIngestRejectsPeriodMismatch
// boundary[0]       单期失败不中断整批，最后汇总非零   → TestIngestContinuesAfterOneFailure
// error_handling[0] 五个错误出口各一用例，%w 包住带上下文
//                                                   → TestIngestWrapsStageErrors 的五个子测试
// non_functional[0] 候选按期次升序处理（肯定式断言：Out 的期次序列升序）
//                                                   → TestIngestProcessesOldestFirst
// non_functional[1] 新增导出物 Ingest 登记进导出面守卫
//                                                   → TestPackageExposesNoWriteFunctions（store_test.go）
//
// leader 追加要求（不在 DoD 里，但要进实现）：Out 必须能区分「入了权威表」与「落了 pending」
//                                                   → TestIngestReportsVerdictAndTable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexEntry 是合成 index 页上的一条链接。
type indexEntry struct{ articleID, title string }

// syntheticIndex 造一个最小 index 页：一个分页控件 + 若干条报告链接。
//
// 不用真实快照：p2 里的候选是 2026-06，而能解析的文章快照是 2020-06 / 2025-12，
// 期次对不上会撞进交叉校验（那是 TestIngestRejectsPeriodMismatch 要的用例，
// 不是主路径要的）。真实快照的价值在 discover 的解析测试里，端到端要的是「接线通不通」。
//
// 总页数写 1 —— Discover 只扫一页就停，省掉构造第 2 页。
//
// 条目按传入顺序排布，调用方**照真实站点的样子把最近的排在前面**：本包要断言的
// 恰恰是 ingest 会把这个顺序倒过来处理。
func syntheticIndex(t *testing.T, entries ...indexEntry) []byte {
	t.Helper()
	require.NotEmpty(t, entries, "合成 index 至少要有一条链接")

	var b strings.Builder
	b.WriteString(`<html><body>` + "\n")
	b.WriteString(`<a onclick="jumpTo(this,'1','1','/goutongjiaoliu/113456/113469/11040-%1.html')">尾页</a>` + "\n")
	for _, e := range entries {
		fmt.Fprintf(&b, `<a href="/goutongjiaoliu/113456/113469/%s/index.html" title="true">%s</a>`+"\n",
			e.articleID, e.title)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// articleURL 拼出候选文章的绝对 URL，与 scanPage 的 resolveURL 结果一致。
func articleURL(articleID string) string {
	return "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + articleID + "/index.html"
}

// 两份**能被 Parse 接受**的正文快照。
//
// ⚠️ 季报那两份（pboc-2026-03-q1 / pboc-2025-09-q3）**不能用在 ingest 链路上**：
// TASK-001 放宽的是 discover.go 的 reportTitleRE（index 标题侧），而 parse.go 有
// 自己独立的 titleRE（parse.go:21），没跟着放宽 —— 实测 Parse 对这两份直接报
// "unrecognized report title"。正文侧归 TASK-004，届时它们才进得来。
const (
	annualID    = "2026011509294440745" // 2025 年报的真实 id
	annualTitle = "2025年金融统计数据报告"
	annualFile  = "pboc-2025-12-annual.html"

	h1ID    = "2025092212550713215" // 2020 上半年报的真实 id（站点迁移时间戳）
	h1Title = "2020年上半年金融统计数据报告"
	h1File  = "pboc-2020-06-h1.html"
)

// ingestCfg 是各用例的公共配置：合成 index 只有一页，MaxPages 取多少都只扫一页。
func ingestCfg() Config {
	return Config{
		Discover:   DiscoverCfg{IndexURL: testIndexURL, MaxPages: 3},
		Thresholds: DefaultThresholds(),
	}
}

// annualFetcher 造出主路径用的 fetcher：index 页只挂 2025 年报这一条，
// 外加它那份能被 Parse 接受的正文快照。多数用例只关心「接线通不通」，
// 一条候选就够。
func annualFetcher(t *testing.T) *fakeFetcher {
	t.Helper()
	return &fakeFetcher{pages: map[string][]byte{
		testIndexURL:         syntheticIndex(t, indexEntry{annualID, annualTitle}),
		articleURL(annualID): readTestdata(t, annualFile),
	}}
}

// outPeriodRE 认出 Out 里以期次开头的行。每条候选无论成败都会打一行，
// 且都以 period 开头 —— "no new reports" 这类行不匹配，自然被排除。
var outPeriodRE = regexp.MustCompile(`(?m)^(\d{4}-\d{2})\b`)

// outPeriods 按打印顺序取出 Out 里的期次序列。
func outPeriods(out string) []string {
	var got []string
	for _, m := range outPeriodRE.FindAllStringSubmatch(out, -1) {
		got = append(got, m[1])
	}
	return got
}

// 端到端：发现 → 抓 → 解析 → 校验 → 入库。
func TestIngestEndToEnd(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	var out bytes.Buffer
	require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: ingestCfg()}))

	has, err := s.HasPeriod(ctx, "2025-12", "annual")
	require.NoError(t, err)
	assert.True(t, has, "2025 年报应当已入观测表")
	assert.Contains(t, out.String(), "2025-12")
}

// 接缝 ①：ArticleID 由 ingest 补 —— Parse 只拿到 raw bytes、看不到 URL，
// 而 parse.go 写着「让 Parse 编造一个 ArticleID 才是缺陷」。
//
// 这是两个包之间唯一的手工装配点。漏填**不会**静默（Meta.validate 要求非空），
// 但填**错**（比如填成 URL 全串）会静默入库 —— 所以断言钉的是「填进去的**就是**
// 候选的 ArticleID」，不是「非空」。
//
// Out 在本例里刻意留 nil：实现必须容忍（io.Discard）。
func TestIngestFillsArticleID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Cfg: ingestCfg()}))

	// 用 Preceding 把入库的那行读回来（2025-12 < 2026-01，同 annual 序列）
	prior, err := s.Preceding(ctx, "2026-01", "annual", 1)
	require.NoError(t, err)
	require.Len(t, prior, 1)
	assert.Equal(t, annualID, prior[0].Meta.ArticleID,
		"填错（比如填成整个 URL）会静默入库，所以要钉住等值而不是非空")
}

// 接缝 ②：index 标题与正文的期次对不上要拦下。
//
// 真实场景：央行把链接挂错，或某一侧的解析有 bug。两种都不该入库 —— 入了就是
// 一期张冠李戴的数据，而它在库里看起来完全正常。
//
// ⚠️ MaxPages 必须是 **2** 而不是计划写的 3：twoPageFetcher 只备两页，而空库下
// HasPeriod 恒 false ⇒ Discover 不会提前返回，会一路翻到 MaxPages（discover_test.go
// 已就同一个坑留过注释）。
func TestIngestRejectsPeriodMismatch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f, _ := twoPageFetcher(t)                         // 真实的两页 index 快照
	target := targetOnPage2(t)                        // 第 2 页上那条报告（2026-06/h1）
	f.pages[target.URL] = readTestdata(t, annualFile) // 正文却是 2025 年报

	var out bytes.Buffer
	err := Ingest(ctx, IngestDeps{
		Store: s, Fetch: f, Out: &out,
		Cfg: Config{
			Discover:   DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2},
			Thresholds: DefaultThresholds(),
		},
	})

	require.Error(t, err, "期次对不上必须失败")
	assert.Contains(t, out.String(), "期次不一致")

	for _, tc := range []struct{ period, periodType string }{
		{target.Period, target.PeriodType}, // 标题说的那一期
		{"2025-12", "annual"},              // 正文说的那一期
	} {
		has, herr := s.HasPeriod(ctx, tc.period, tc.periodType)
		require.NoError(t, herr)
		assert.Falsef(t, has, "拦下之后 %s/%s 都不该有行入库", tc.period, tc.periodType)
	}
}

// 单期失败不中断整批（Global Constraint）：一期解析失败不该阻止其它期入库，
// 但整批必须汇总返回非零 —— launchd 的 err.log 才会留痕。
//
// 顺序是刻意的：合成 index 按站点的样子把**较新的 2025-12 排在前面**，而实现
// 按期次升序处理 ⇒ **先跑的是会失败的 2020-06**。于是本例同时钉住「第一条失败、
// 第二条仍被处理」。
//
// 失败源用一份合成的垃圾正文，**不用**季报快照：季报现在解析失败是 parse.go 还没
// 跟上 discover.go（TASK-004 的活），拿它当失败源会让本用例在 TASK-004 修好那天
// 变红，而红的理由与本用例要守的东西无关。
func TestIngestContinuesAfterOneFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f := &fakeFetcher{pages: map[string][]byte{
		testIndexURL: syntheticIndex(t,
			indexEntry{annualID, annualTitle}, // 较新的排前面，与真实站点一致
			indexEntry{h1ID, h1Title},
		),
		articleURL(annualID): readTestdata(t, annualFile),
		articleURL(h1ID):     []byte(`<html><body>这不是一篇报告</body></html>`),
	}}

	var out bytes.Buffer
	err := Ingest(ctx, IngestDeps{Store: s, Fetch: f, Out: &out, Cfg: ingestCfg()})

	require.Error(t, err, "有期次失败时整批必须返回非零")
	assert.Contains(t, err.Error(), "2020-06", "汇总错误要指名是哪一期失败")

	// 钉住「失败的那条**排在前面**」，否则本用例的名字（continues after one failure）
	// 与它实际证明的东西对不上：不钉顺序时，即使失败的那条排在最后，上面那些断言
	// 也全都成立 —— 实测（消融 M1，去掉排序）确认过这一点。
	assert.Equal(t, []string{"2020-06", "2025-12"}, outPeriods(out.String()),
		"先跑失败的那条，再跑成功的那条 —— 这才是「不中断」")

	has, herr := s.HasPeriod(ctx, "2025-12", "annual")
	require.NoError(t, herr)
	assert.True(t, has, "第一条失败之后，第二条仍然要被处理并入库")

	has, herr = s.HasPeriod(ctx, "2020-06", "h1")
	require.NoError(t, herr)
	assert.False(t, has, "失败的那期不得留下任何行")
}

// 候选必须按期次**升序**处理。
//
// Discover 文档写明「最近的排在前面」，直接顺着 for range 会让 stock_continuity /
// deposit_sum 的漂移检测一次都没真正执行（首期恒 no_prior_period），而数据照进
// 权威表、报告照样 Passed=true、**零告警**（skipped 不拉低 Passed，validate.go:101）。
//
// 🔴 断言是**肯定式**的：钉「Out 的期次序列升序」，而不是「较新那期的
// stock_continuity 不应是 no_prior_period」。后者是纯否定式，而 gateStockContinuity
// 有四种 skip 理由 —— 该期缺 tsf_stock 时返回 absent_field:tsf_stock，**断言照样
// 绿而顺序仍然是错的**（reviewer C1 实测）。
//
// ⚠️ 「构造同 period_type 的两期来看闸门真的跑了」在当前 testdata 下**做不到**：
// 能被 Parse 接受的只有 2025-12/annual 与 2020-06/h1，是两条独立序列；而新增的
// 两份季报快照 parse.go 还认不出（见文件头部常量那段注释）。所以这里断言的是
// **顺序本身**，它不经过任何闸门，也就不会被闸门的 skip 理由蒙混过去。
func TestIngestProcessesOldestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f := &fakeFetcher{pages: map[string][]byte{
		testIndexURL: syntheticIndex(t,
			indexEntry{annualID, annualTitle}, // 2025-12：站点上排在前面
			indexEntry{h1ID, h1Title},         // 2020-06：更旧，排在后面
		),
		articleURL(annualID): readTestdata(t, annualFile),
		articleURL(h1ID):     readTestdata(t, h1File),
	}}

	var out bytes.Buffer
	require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: f, Out: &out, Cfg: ingestCfg()}))

	got := outPeriods(out.String())
	require.Len(t, got, 2, "两条候选都要打一行，否则下面的顺序断言会平凡为真")
	assert.Equal(t, []string{"2020-06", "2025-12"}, got,
		"必须按期次升序处理：Discover 给的是倒序，顺着 for range 会让漂移检测永远拿不到前期")
	assert.True(t, slices.IsSorted(got), "期次序列必须升序")
}

// Out 必须能区分「入了权威表」与「落了 pending」。
//
// 只看 err == nil 的话，一期过闸入库与一期没过闸落 pending **在日志里完全同形**，
// 而两者对运维的含义相反：前者不用管，后者要人看一眼。
//
// 用「把某个阈值收紧到几乎必然超差」来构造落 pending，不依赖任何快照的具体数值 ——
// 依赖具体数值的构造会在换快照那天以无关的理由变红。
//
// ⚠️ 这里刻意**不**断言 Verdict 恒为 New：经 Ingest 的候选都已通过 Discover 的
// HasPeriod 过滤（discover.go:303-318）⇒ Save 的 Lookup 必然查不到当前行 ⇒ Verdict
// 结构上恒为 New，Duplicate/Revision 当前不可达。把这条钉成断言等于把当前的局限
// 焊死，将来有人放开这条路时红的理由是反的。Verdict 仍然打进 Out，是为了那一天
// 到来时它自己会显形。
func TestIngestReportsVerdictAndTable(t *testing.T) {
	ctx := context.Background()

	t.Run("过闸的入权威表", func(t *testing.T) {
		s := newTestStore(t)

		var out bytes.Buffer
		require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: ingestCfg()}))
		assert.Contains(t, out.String(), TableObservations)
		assert.NotContains(t, out.String(), TablePending)
	})

	t.Run("没过闸的落 pending 且说得出来", func(t *testing.T) {
		s := newTestStore(t)

		cfg := ingestCfg()
		cfg.Thresholds.DepositSumTolerance = 1e-9 // >0（cfg.validate 要求），但必然超差

		var out bytes.Buffer
		require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: cfg}),
			"落 pending 不是失败：pending 的设计意图是「人看一眼再决定」")
		assert.Contains(t, out.String(), TablePending,
			"落 pending 必须在 Out 里说出来，否则它与正常入库同形")

		has, err := s.HasPeriod(ctx, "2025-12", "annual")
		require.NoError(t, err)
		assert.False(t, has, "pending 不算已入库")
	})
}

// 一级幂等键挡住重复处理，Force 绕过它。
//
// 这条路径**恰好**只在「上次落了 pending」时可达，而那正是 TASK-003 定案的形状：
// HasPeriod 不认 pending（所以 Discover 仍把这期当候选交出来），HasArticle 认
// pending（所以这里挡住）。两者方向相反而都对。
func TestIngestSkipsSeenArticleUnlessForce(t *testing.T) {
	ctx := context.Background()

	// 先让这一期落 pending：article 进了 pending，但 period 不算已入库。
	seed := func(t *testing.T) *Store {
		t.Helper()
		s := newTestStore(t)
		out, err := s.Save(ctx, Observation{
			Meta: Meta{
				Period: "2025-12", PeriodType: "annual", PublishedAt: "2026-01-15",
				ArticleID: annualID, CaliberVersion: "2025-01", Extractor: extractorV2,
			},
			Values: map[string]float64{FieldM2: 300},
		}, failing())
		require.NoError(t, err)
		require.Equal(t, TablePending, out.Table, "前置条件：这一期必须落 pending")
		return s
	}
	t.Run("默认跳过已处理过的文章", func(t *testing.T) {
		s := seed(t)
		var out bytes.Buffer
		require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: ingestCfg()}))
		assert.Contains(t, out.String(), "already ingested")

		has, err := s.HasPeriod(ctx, "2025-12", "annual")
		require.NoError(t, err)
		assert.False(t, has, "跳过意味着没有重新入库")
	})

	t.Run("Force 绕过一级幂等键", func(t *testing.T) {
		s := seed(t)
		var out bytes.Buffer
		require.NoError(t, Ingest(ctx, IngestDeps{
			Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: ingestCfg(), Force: true,
		}))
		assert.NotContains(t, out.String(), "already ingested")

		has, err := s.HasPeriod(ctx, "2025-12", "annual")
		require.NoError(t, err)
		assert.True(t, has, "Force 之后这一期应当真的被重新处理并入库")
	})
}

// errBoom 是注入的哨兵，用来证明错误链**一路通到底层**而不是半路被截断。
var errBoom = errors.New("boom")

// urlFailFetcher 只在一个 URL 上失败，其余照常 —— 用来把故障精确摆在取正文那一步
// （fakeFetcher 的 err 字段是全局的，连 index 页都取不到，那会失败在 Discover）。
type urlFailFetcher struct {
	inner  *fakeFetcher
	failOn string
}

func (f urlFailFetcher) Get(ctx context.Context, url string) ([]byte, error) {
	if url == f.failOn {
		return nil, errBoom
	}
	return f.inner.Get(ctx, url)
}

// annualCandidate 是 2025 年报那条候选，供直测 ingestOne 用。
func annualCandidate() Candidate {
	return Candidate{
		ArticleID: annualID, URL: articleURL(annualID), Title: annualTitle,
		Period: "2025-12", PeriodType: "annual",
	}
}

// ingestOneDeps 组装直测 ingestOne 用的依赖：不经 Discover，只备年报那一篇正文，
// 由调用方决定喂进去的是真快照还是垃圾。Out 走 io.Discard —— 直测这层看的是
// 返回的错误，不是打印。
func ingestOneDeps(s *Store, cfg Config, body []byte) IngestDeps {
	return IngestDeps{
		Store: s, Cfg: cfg, Out: io.Discard,
		Fetch: &fakeFetcher{pages: map[string][]byte{articleURL(annualID): body}},
	}
}

// requireWrappedStageError 是直测 ingestOne 那三条（Parse / Validate / Save）的共同判据：
// 错误既要带得定位的上下文（期次 + article_id），又要用 %w 包住底层 err。
//
// 这一层没有汇总兜底（见下面 TestIngestWrapsStageErrors 的注释），所以
// errors.Unwrap 非 nil 才真的在守 %w，不是平凡为真。
func requireWrappedStageError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.ErrorContains(t, err, "2025-12")
	assert.ErrorContains(t, err, annualID)
	require.NotNil(t, errors.Unwrap(err), "必须用 %w 包住底层 err")
}

// 五个错误出口各一条，都要**响亮**：错误信息带得定位的上下文，且用 %w 包住。
//
// 🔴 **两层用两种判据，这不是随意的**。汇总错误自己就是 `fmt.Errorf(..., %w,
// errors.Join(...))`，所以在 Ingest 这一层，`errors.Unwrap(err)` **必然非 nil**
// ——哪怕每个 stage 的包裹全写成 `%v`。照搬 TASK-003 的 NotNil 写法到这里就是
// 一条新形状的**平凡为真**（同 F8 的病，换了个载体）。故：
//
//   - Discover / Fetch：经 Ingest 测，注入哨兵 errBoom 后断言 **errors.Is 找得到它**
//     —— 链路要穿过「汇总的 %w → errors.Join → stage 的 %w」三层，任一层写成 %v
//     就断了。Fetch 那条因此**同时**证明了汇总没有截断链。
//   - Parse / Validate / Save：底层错误产自被调包内部，拿不到哨兵；改为**直测
//     ingestOne**——那一层没有汇总兜底，`errors.Unwrap` 非 nil 才真的在守 %w。
//
// ⚠️ 「包住」一律不用 NotErrorIs：它在不包裹时 Unwrap 返回 nil、errors.Is(nil, err)
// 恒 false ⇒ 平凡为真（Sprint 035 的 F8，跨 Sprint 存活一整轮才被修）。
func TestIngestWrapsStageErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("Discover 失败", func(t *testing.T) {
		s := newTestStore(t)
		f := &fakeFetcher{err: errBoom}

		err := Ingest(ctx, IngestDeps{Store: s, Fetch: f, Cfg: ingestCfg()})
		require.Error(t, err)
		assert.ErrorContains(t, err, testIndexURL, "此刻还没有期次，上下文只能是 index URL")
		assert.ErrorIs(t, err, errBoom, "%w 断在任何一层，这条就找不到哨兵")
	})

	t.Run("Fetch 失败", func(t *testing.T) {
		s := newTestStore(t)
		f := urlFailFetcher{
			inner: &fakeFetcher{pages: map[string][]byte{
				testIndexURL: syntheticIndex(t, indexEntry{annualID, annualTitle}),
			}},
			failOn: articleURL(annualID),
		}

		err := Ingest(ctx, IngestDeps{Store: s, Fetch: f, Cfg: ingestCfg()})
		require.Error(t, err)
		assert.ErrorContains(t, err, "2025-12")
		assert.ErrorContains(t, err, annualID)
		assert.ErrorIs(t, err, errBoom,
			"这条同时证明汇总（errors.Join）没有把底层判因截断")
	})

	t.Run("Parse 失败", func(t *testing.T) {
		d := ingestOneDeps(newTestStore(t), ingestCfg(),
			[]byte(`<html><body>这不是一篇报告</body></html>`))

		requireWrappedStageError(t, d.ingestOne(ctx, annualCandidate()))
	})

	t.Run("Validate 失败", func(t *testing.T) {
		// Validate 最前面调 Thresholds.validate()，而它查的是 CaliberExemptions
		// 的自洽（**不是** Config.validate() 那几条 >0 —— 那是 LoadConfig 的活，
		// 实测：把 StockContinuityMax 设成 0 Validate 照样通过）。
		// PeriodTypes 留空是它明确拒绝的一种：留空不等于「全部」。
		cfg := ingestCfg()
		cfg.Thresholds.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01", Reason: "test",
			SkipChecks: []string{"m1_caliber_break"},
		}}
		d := ingestOneDeps(newTestStore(t), cfg, readTestdata(t, annualFile))

		requireWrappedStageError(t, d.ingestOne(ctx, annualCandidate()))
	})

	// Save 失败在 ingest 链路上**不是随手能构造的**：任何 DB 级故障都会先撞上
	// HasArticle 或 Validate（两者都查库且排在 Save 前面），任何数据级问题都会
	// 先被 Parse 拦下（它要求 PubDate 形态、标题形态、已知版式）——实测：连
	// 「正文为空 ⇒ 0 个字段 ⇒ Save 的 checkPassedHasValues 拒」这条路都不通，
	// Parse 自己会先报 "unrecognized layout: 0 sections"。
	//
	// 所以用 rawDB 造一个「Store 管不到的库状态」——一条只挡 INSERT 的触发器。
	// SELECT 不受影响 ⇒ HasArticle 与 Validate 照常通过，故障精确落在 Save 上。
	t.Run("Save 失败", func(t *testing.T) {
		s, path := newTestStoreAt(t)
		_, err := rawDB(t, path).ExecContext(ctx,
			`CREATE TRIGGER block_insert BEFORE INSERT ON `+TableObservations+`
			 BEGIN SELECT RAISE(ABORT, 'blocked by test trigger'); END;`)
		require.NoError(t, err, "前置条件：触发器要建得上")

		d := ingestOneDeps(s, ingestCfg(), readTestdata(t, annualFile))

		requireWrappedStageError(t, d.ingestOne(ctx, annualCandidate()))
	})
}
