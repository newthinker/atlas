package hestia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: TASK-006 done_criteria → test mapping
// functional[0]   断点续抓：已抓的不重发请求        → TestBackfillFetchSkipsAlreadyFetchedArticles
// functional[1]   index/search 页存盘               → TestBackfillFetchSavesIndexAndSearchSnapshots
//                                                     TestBackfillSearchSlugsCoverAllKeywords
// functional[2]   SearchSkippedReason 落盘          → TestBackfillFetchRecordsSearchSkippedReason
//                                                     TestBackfillFetchOmitsSearchSkippedReasonWhenSearchWorks
// error_handling[0] 单篇失败 → failed[] + 继续 + 非零 → TestBackfillFetchRecordsFailedArticleAndContinues
// error_handling[1] 落盘失败 → 立刻中止              → TestBackfillFetchAbortsOnDiskFailure
// non_functional[0] sleep 次数 == 请求次数 − 1       → TestBackfillFetchSleepsBetweenEveryRequest
// boundary[0]     manifest 为字面量 null ⇒ 报错     → TestBackfillFetchRejectsNullManifest
//                                                     TestLoadManifestRejectsJSONNull
// boundary[1]     交集取 index 侧 URL/Published     → TestCrossCheckBackfillIntersectionTakesIndexSideFields

// —— 合成站点 ——
//
// 一份 index（1 页）+ 三个关键词各 1 页搜索结果 + 每篇文章一页正文。
// 请求总数 = 1 + 3 + 待抓篇数，这个等式是 sleep 计数用例的基准。

type bfArticle struct{ ID, Title, Date string }

func bfArticleURL(id string) string {
	return "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + id + "/index.html"
}

func bfArticleHTML(id string) []byte {
	return []byte("<html><body>正文 " + id + "</body></html>")
}

func bfConfig(t *testing.T, out string) BackfillConfig {
	t.Helper()
	return BackfillConfig{
		IndexURL: testIndexURL,
		From:     backfillDate(t, "2020-01-01"),
		To:       backfillDate(t, "2020-12-31"),
		Out:      out,
		MaxPages: 200,
	}
}

// bfSite 造站点：arts 既是 index 上的报告条目，也各有一页正文。
// 搜索侧三个关键词都只索到 arts[0]，于是它 Source=both、其余 Source=index
// ——交集与差集同时非空，别让用例落在「两侧完全一致」这种退化输入上。
func bfSite(t *testing.T, cfg BackfillConfig, arts ...bfArticle) *fakeFetcher {
	t.Helper()
	items := make([]string, 0, len(arts))
	for _, a := range arts {
		items = append(items, backfillReportItem(a.ID, a.Title, a.Date))
	}
	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: backfillIndexPageHTML(1, items...)}}
	for _, kw := range backfillSearchKeywords {
		f.pages[backfillSearchURL(kw, cfg.From, cfg.To, 1)] = searchPageHTML(1, 1,
			searchItemHTML(bfArticleURL(arts[0].ID), arts[0].Title, "2020年01月10日"))
	}
	for _, a := range arts {
		f.pages[bfArticleURL(a.ID)] = bfArticleHTML(a.ID)
	}
	return f
}

func bfTwoArticles() []bfArticle {
	return []bfArticle{
		{ID: "9001", Title: "2020年1月金融统计数据报告", Date: "2020-01-10"},
		{ID: "9002", Title: "2020年2月金融统计数据报告", Date: "2020-01-10"},
	}
}

// bfFailOn 让内层 Fetcher 对含某个子串的 URL 报错，其余照常。
// 用子串而不是全等：三个关键词的搜索 URL 各不相同，但都含同一个 base。
type bfFailOn struct {
	inner    Fetcher
	contains string
	err      error
}

func (f bfFailOn) Get(ctx context.Context, url string) ([]byte, error) {
	if strings.Contains(url, f.contains) {
		if f.err != nil {
			return nil, f.err
		}
		return nil, fmt.Errorf("bfFailOn: %s", url)
	}
	return f.inner.Get(ctx, url)
}

// recordingSleeper 记录每次 sleep 的时长。计数等式是 non_functional[0] 的判据，
// 所以记的是**每一次**的时长而不只是次数。
type recordingSleeper struct{ calls []time.Duration }

func (r *recordingSleeper) sleep(d time.Duration) { r.calls = append(r.calls, d) }

func bfRun(t *testing.T, f Fetcher, cfg BackfillConfig) (*recordingSleeper, error) {
	t.Helper()
	rec := &recordingSleeper{}
	cfg.Report = io.Discard // 既有用例不看对账报告，别让它刷进测试输出
	err := runBackfill(context.Background(), f, rec.sleep, cfg)
	return rec, err
}

// TestBackfillFetchSkipsAlreadyFetchedArticles 对应 functional[0]。
//
// 判据是**那些 URL 根本没进 fetcher**，不是「manifest 没变多」——重发一次拿到相同内容时，
// 后者照样绿，而 400 次请求已经白发了。
func TestBackfillFetchSkipsAlreadyFetchedArticles(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)

	// 预置：9001 已抓过（manifest 有记录 + 文件在盘上），9002 没有。
	seed, err := loadManifest(out)
	if err != nil {
		t.Fatalf("预置 loadManifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(out, "articles"), 0o755); err != nil {
		t.Fatalf("建 articles 目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(out, articleFile(arts[0].ID)), bfArticleHTML(arts[0].ID), 0o644); err != nil {
		t.Fatalf("预置文章文件: %v", err)
	}
	if err := seed.AppendArticle(Article{ID: arts[0].ID, File: articleFile(arts[0].ID), URL: bfArticleURL(arts[0].ID)}); err != nil {
		t.Fatalf("预置 manifest: %v", err)
	}

	f := bfSite(t, cfg, arts...)
	if _, err := bfRun(t, f, cfg); err != nil {
		t.Fatalf("runBackfill: %v", err)
	}

	if slices.Contains(f.calls, bfArticleURL(arts[0].ID)) {
		t.Errorf("已抓过的 %s 不该再发请求，实际请求了\n全部请求: %v", arts[0].ID, f.calls)
	}
	if !slices.Contains(f.calls, bfArticleURL(arts[1].ID)) {
		t.Errorf("没抓过的 %s 必须请求，实际没有\n全部请求: %v", arts[1].ID, f.calls)
	}

	got := readManifestFile(t, out)
	if len(got.Articles) != 2 {
		t.Errorf("manifest 应含两篇（一篇沿用、一篇新抓），实得 %d 篇", len(got.Articles))
	}
}

// TestBackfillFetchSavesIndexAndSearchSnapshots 对应 functional[1]。
//
// index 快照名按设计附录 §A3 用「序号 + 日期区间」而不是页码——页码随新文章上架而漂移，
// 重跑时同一个文件名会对应不同内容的页。
func TestBackfillFetchSavesIndexAndSearchSnapshots(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	if _, err := bfRun(t, f, cfg); err != nil {
		t.Fatalf("runBackfill: %v", err)
	}

	idx, err := os.ReadDir(filepath.Join(out, "index"))
	if err != nil {
		t.Fatalf("index 快照目录不存在: %v", err)
	}
	if len(idx) != 1 {
		t.Fatalf("应存 1 页 index 快照，实得 %d 个: %v", len(idx), dirNames(idx))
	}
	if want := "001-2020-01-10_2020-01-10.html"; idx[0].Name() != want {
		t.Errorf("index 快照命名应为 %q（§A3：序号-最新_最老），实得 %q", want, idx[0].Name())
	}
	raw, err := os.ReadFile(filepath.Join(out, "index", idx[0].Name()))
	if err != nil {
		t.Fatalf("读 index 快照: %v", err)
	}
	if string(raw) != string(f.pages[testIndexURL]) {
		t.Error("index 快照内容与站点返回的原始响应体不一致")
	}

	sr, err := os.ReadDir(filepath.Join(out, "search"))
	if err != nil {
		t.Fatalf("search 快照目录不存在: %v", err)
	}
	if len(sr) != len(backfillSearchKeywords) {
		t.Fatalf("应存 %d 页搜索快照（每关键词 1 页），实得 %d 个: %v",
			len(backfillSearchKeywords), len(sr), dirNames(sr))
	}
	for _, e := range sr {
		if !strings.HasSuffix(e.Name(), "-p01.html") {
			t.Errorf("搜索快照名应带页码后缀 -pNN.html，实得 %q", e.Name())
		}
	}
}

func dirNames(es []os.DirEntry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// TestBackfillSearchSlugsCoverAllKeywords：每个关键词都要有自己的快照文件名片段，
// 且互不相同。两个关键词落到同一个文件名 ⇒ 后写的静默覆盖先写的，而快照的全部用途
// 就是事后离线核对。
func TestBackfillSearchSlugsCoverAllKeywords(t *testing.T) {
	seen := map[string]string{}
	for i, kw := range backfillSearchKeywords {
		s := backfillSearchSlug(kw, i)
		if s == "" {
			t.Errorf("关键词 %q 没有 slug", kw)
			continue
		}
		if prev, dup := seen[s]; dup {
			t.Errorf("slug %q 被两个关键词共用: %q 与 %q", s, prev, kw)
		}
		seen[s] = kw
		if strings.ContainsAny(s, `/\ `) {
			t.Errorf("slug %q 含路径分隔符或空格，不能直接做文件名", s)
		}
	}
}

// TestBackfillFetchRecordsSearchSkippedReason 对应 functional[2]。
//
// 搜索侧失效 ⇒ fail-open：主路径照常抓完（返回 nil），但**必须在 manifest 里留下痕迹**。
// 这个字段的全部意义是让「这次没做校验」与「校验通过」在读者看来不一样。
func TestBackfillFetchRecordsSearchSkippedReason(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	fail := bfFailOn{inner: f, contains: backfillSearchBase}

	if _, err := bfRun(t, fail, cfg); err != nil {
		t.Fatalf("搜索侧失效必须 fail-open（主路径照常完成），实得错误: %v", err)
	}

	got := readManifestFile(t, out)
	if got.SearchSkippedReason == "" {
		t.Fatal("搜索侧失效时 search_skipped_reason 必须非空——否则「没做校验」与「校验通过」在 manifest 里长得一样")
	}
	if len(got.Articles) != len(arts) {
		t.Errorf("fail-open 后主路径应照常抓完 %d 篇，实得 %d 篇", len(arts), len(got.Articles))
	}
	// 差集必须留空：宣称「搜索没索到这几篇」是谎报，事实是根本没问过搜索。
	if len(got.OnlyInIndex) != 0 || len(got.OnlyInSearch) != 0 {
		t.Errorf("跳过校验时两个差集必须留空，实得 only_in_index=%v only_in_search=%v",
			got.OnlyInIndex, got.OnlyInSearch)
	}
}

// TestBackfillFetchOmitsSearchSkippedReasonWhenSearchWorks：正常完成时该字段
// 不出现在 JSON 里（omitempty）。与上一条成对——只有上一条时，实现把字段写成常量
// 非空也能绿。
func TestBackfillFetchOmitsSearchSkippedReasonWhenSearchWorks(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	if _, err := bfRun(t, f, cfg); err != nil {
		t.Fatalf("runBackfill: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatalf("读 manifest: %v", err)
	}
	if strings.Contains(string(raw), "search_skipped_reason") {
		t.Errorf("交叉校验正常完成时不该出现 search_skipped_reason\n实际内容: %s", raw)
	}
}

// TestBackfillFetchRecordsFailedArticleAndContinues 对应 error_handling[0]：
// 单篇抓取失败是**外部世界**的事 ⇒ 记 failed[]、继续抓后面的、跑完返回非零。
func TestBackfillFetchRecordsFailedArticleAndContinues(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	fail := bfFailOn{inner: f, contains: "/" + arts[0].ID + "/"}

	if _, err := bfRun(t, fail, cfg); err == nil {
		t.Error("有单篇失败时跑完必须返回非零")
	}

	got := readManifestFile(t, out)
	if len(got.Failed) != 1 || got.Failed[0].ID != arts[0].ID {
		t.Fatalf("失败那篇必须记进 failed[]，实得 %+v", got.Failed)
	}
	if got.Failed[0].Error == "" {
		t.Error("failed[] 条目必须带错误原因")
	}
	// 「继续抓后面的」——这是与 error_handling[1] 处置相反的那一半。
	if !slices.Contains(f.calls, bfArticleURL(arts[1].ID)) {
		t.Errorf("单篇失败后必须继续抓后面的，实际没请求 %s\n全部请求: %v", arts[1].ID, f.calls)
	}
	if len(got.Articles) != 1 || got.Articles[0].ID != arts[1].ID {
		t.Errorf("成功那篇应进 articles[]，实得 %+v", got.Articles)
	}
	if _, err := os.Stat(filepath.Join(out, articleFile(arts[1].ID))); err != nil {
		t.Errorf("成功那篇的正文应已落盘: %v", err)
	}
}

// TestBackfillFetchAbortsOnDiskFailure 对应 error_handling[1]：
// 落盘失败是**本机**的事 ⇒ 立刻中止，不继续抓。
//
// 判据必须是「**后面那篇根本没被请求**」——只断言返回错误的话，「记 failed 继续抓」
// 那种实现照样绿，而两条 DoD 的处置刻意相反正是这一点。
func TestBackfillFetchAbortsOnDiskFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("以 root 运行时挡不住写入，构造不出落盘失败")
	}
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	// articles 是个**普通文件** ⇒ 往 articles/<id>.html 落盘必然失败。
	if err := os.WriteFile(filepath.Join(out, "articles"), []byte("x"), 0o644); err != nil {
		t.Fatalf("构造不可写的 articles 路径: %v", err)
	}

	f := bfSite(t, cfg, arts...)
	if _, err := bfRun(t, f, cfg); err == nil {
		t.Fatal("落盘失败必须返回错误")
	}

	fetched := 0
	for _, a := range arts {
		if slices.Contains(f.calls, bfArticleURL(a.ID)) {
			fetched++
		}
	}
	if fetched != 1 {
		t.Errorf("落盘失败后必须立刻中止：应只请求到第 1 篇就停，实际请求了 %d 篇\n全部请求: %v",
			fetched, f.calls)
	}
}

// TestBackfillFetchSleepsBetweenEveryRequest 对应 non_functional[0]。
//
// 判据是**计数等式** `sleep 次数 == 请求次数 − 1`，不是「两次请求之间调用了 sleep」
// ——后者字面上一次 sleep 就满足（1 条 sleep + 365 条请求也算数）。
func TestBackfillFetchSleepsBetweenEveryRequest(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	rec, err := bfRun(t, f, cfg)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}

	// 1 页 index + 3 页搜索 + 2 篇文章 = 6 次请求。写死期望值是为了让「请求数」本身
	// 也被钉住——只比两个观测值相等的话，实现少发一半请求时等式照样成立。
	if len(f.calls) != 6 {
		t.Fatalf("本用例应发 6 次请求（1 index + 3 search + 2 文章），实得 %d: %v", len(f.calls), f.calls)
	}
	if len(rec.calls) != len(f.calls)-1 {
		t.Errorf("sleep 次数必须等于请求次数 − 1，实得 sleep=%d 请求=%d", len(rec.calls), len(f.calls))
	}
	for i, d := range rec.calls {
		if d != backfillInterval {
			t.Errorf("第 %d 次 sleep 时长应为 %v，实得 %v", i+1, backfillInterval, d)
		}
	}
	if backfillInterval != time.Second {
		t.Errorf("backfillInterval 应为 1s（自我约束的限速），实得 %v", backfillInterval)
	}
}

// TestBackfillFetchRejectsNullManifest 对应 boundary[0]。
//
// `null` 是**合法 JSON**，所以它走的是 Unmarshal 的成功路径、留下一个零值 Manifest。
// 静默当空的后果不是少一条记录，是断点续抓退化成**全量重抓 400+ 次请求且不报错**。
// 判据除了「报错」，还要断言**一次请求都没发**——否则「先重抓再说」的实现也能绿。
func TestBackfillFetchRejectsNullManifest(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), []byte("null\n"), 0o644); err != nil {
		t.Fatalf("写 null manifest: %v", err)
	}
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)

	if _, err := bfRun(t, f, cfg); err == nil {
		t.Fatal("manifest 内容为字面量 null 时必须报错，不得当成空 manifest")
	}
	if len(f.calls) != 0 {
		t.Errorf("manifest 不可用时不该发任何请求，实得 %d 次: %v", len(f.calls), f.calls)
	}
}

// TestLoadManifestRejectsJSONNull 是上一条在读取层的单元版本：`null` 与「文件不存在」
// 必须分开——后者是首跑的正常路径，前者是一份坏掉的 manifest。
func TestLoadManifestRejectsJSONNull(t *testing.T) {
	for _, content := range []string{"null", " null ", "null\n"} {
		t.Run(fmt.Sprintf("%q", content), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(content), 0o644); err != nil {
				t.Fatalf("写 manifest: %v", err)
			}
			s, err := loadManifest(dir)
			if err == nil {
				t.Fatalf("内容为 null 时必须报错，实得 nil error（store=%+v）", s)
			}
			if s != nil {
				t.Errorf("报错时不该同时返回 store，实得 %+v", s)
			}
		})
	}
}

// TestCrossCheckBackfillIntersectionTakesIndexSideFields 对应 boundary[1]。
//
// TASK-005 的设计决定是「交集那条取 index 侧字段」，但它的用例只让两侧的 **Title** 不同
// ⇒ 把 URL / Published 改成取搜索侧，没有任何测试会红（实测两个 SURVIVED）。
//
// 风险由抓取层引入：index 侧 URL 是相对路径经 resolveURL(base, href) 补全，而 base 由
// 本层传入；搜索侧本就是绝对 URL。`http` vs `https`、有无末尾斜杠，任一不同即分叉——
// 分叉那天这条设计会**零信号失效**，而拿去下载的正是这个 URL。
func TestCrossCheckBackfillIntersectionTakesIndexSideFields(t *testing.T) {
	const id = "9001"
	index := []backfillItem{{
		ArticleID: id,
		URL:       "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/9001/index.html",
		Title:     "2020年1月金融统计数据报告",
		Published: "2020-02-10",
	}}
	search := []backfillSearchHit{{
		ArticleID: id,
		URL:       "http://www.pbc.gov.cn/goutongjiaoliu/113456/113469/9001/index.html/", // scheme 与末尾斜杠都不同
		Title:     "2020年1月金融统计数据报告（搜索侧重建）",
		Published: "2020-02-11", // 日期也不同
	}}

	got := crossCheckBackfill(index, search, nil)
	if len(got.Fetch) != 1 {
		t.Fatalf("两侧同一篇应合成 1 条，实得 %d 条: %+v", len(got.Fetch), got.Fetch)
	}
	c := got.Fetch[0]
	if c.URL != index[0].URL {
		t.Errorf("交集条目的 URL 必须取 index 侧\n实得: %q\n期望: %q", c.URL, index[0].URL)
	}
	if c.Published != index[0].Published {
		t.Errorf("交集条目的 Published 必须取 index 侧\n实得: %q\n期望: %q", c.Published, index[0].Published)
	}
	if c.Title != index[0].Title {
		t.Errorf("交集条目的 Title 必须取 index 侧\n实得: %q\n期望: %q", c.Title, index[0].Title)
	}
	if c.Source != backfillSourceBoth {
		t.Errorf("两侧都有的条目 Source 应为 %q，实得 %q", backfillSourceBoth, c.Source)
	}
}

// TestBackfillFetchEntryPointWiresRealFetcher 覆盖导出入口本身（TASK-007 接 cobra 用的那一个）。
//
// 用**已取消的 ctx**：请求在进网络栈之前就被 ctx 掐断，于是这条用例既不碰网络、也不依赖
// 站点可达，同时证明了 BackfillFetch 确实接上了真实 Fetcher 并把错误原样透传。
func TestBackfillFetchEntryPointWiresRealFetcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := bfConfig(t, t.TempDir())
	cfg.Timeout = time.Second
	if err := BackfillFetch(ctx, cfg); err == nil {
		t.Fatal("ctx 已取消时必须返回错误")
	}
}

// TestBackfillFetchWritesManifestMetadata：把扫描规模与来源差异落进 manifest。
// 下游 M1c-2/-3 只读这份文件，字段空着等于这次回填没留下可核对的记录。
func TestBackfillFetchWritesManifestMetadata(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	if _, err := bfRun(t, f, cfg); err != nil {
		t.Fatalf("runBackfill: %v", err)
	}

	got := readManifestFile(t, out)
	if got.From != "2020-01" {
		t.Errorf("from 应为 %q，实得 %q", "2020-01", got.From)
	}
	if got.PagesScanned != 1 {
		t.Errorf("pages_scanned 应为 1，实得 %d", got.PagesScanned)
	}
	if got.SearchPagesScanned != len(backfillSearchKeywords) {
		t.Errorf("search_pages_scanned 应为 %d，实得 %d", len(backfillSearchKeywords), got.SearchPagesScanned)
	}
	if got.ScannedAt == "" {
		t.Error("scanned_at 不该为空")
	}
	// 搜索侧只索到 arts[0] ⇒ arts[1] 落进 only_in_index；两侧都有的那条 Source=both。
	if len(got.OnlyInIndex) != 1 || got.OnlyInIndex[0] != arts[1].ID {
		t.Errorf("only_in_index 应恰为 [%s]，实得 %v", arts[1].ID, got.OnlyInIndex)
	}
	var sources []string
	for _, a := range got.Articles {
		sources = append(sources, a.ID+"="+a.Source)
	}
	want := []string{arts[0].ID + "=" + backfillSourceBoth, arts[1].ID + "=" + backfillSourceIndex}
	if !slices.Equal(sources, want) {
		t.Errorf("Source 标记不对\n实得: %v\n期望: %v", sources, want)
	}
	for _, a := range got.Articles {
		if a.SHA256 != articleSHA256(bfArticleHTML(a.ID)) {
			t.Errorf("%s 的 sha256 应是正文内容的哈希，实得 %q", a.ID, a.SHA256)
		}
		if a.FetchedAt == "" {
			t.Errorf("%s 缺 fetched_at", a.ID)
		}
	}
}

// TestBackfillFetchAbortsWhenIndexScanFails：index 页抓取失败是硬失败
// （少一页 index = 静默少 15 条候选），不记 failed 继续。
func TestBackfillFetchAbortsWhenIndexScanFails(t *testing.T) {
	out := t.TempDir()
	arts := bfTwoArticles()
	cfg := bfConfig(t, out)
	f := bfSite(t, cfg, arts...)
	fail := bfFailOn{inner: f, contains: "113469/index.html"}

	if _, err := bfRun(t, fail, cfg); err == nil {
		t.Fatal("index 侧抓取失败必须直接返回错误")
	}
	for _, a := range arts {
		if slices.Contains(f.calls, bfArticleURL(a.ID)) {
			t.Errorf("index 扫描失败后不该继续抓文章，实际请求了 %s", a.ID)
		}
	}
}

func TestBackfillManifestSearchSkippedReasonRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	s.Manifest.SearchSkippedReason = "search side failed, cross-check skipped: boom"
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("重新 loadManifest: %v", err)
	}
	if back.Manifest.SearchSkippedReason != s.Manifest.SearchSkippedReason {
		t.Errorf("search_skipped_reason 往返不一致\n实得: %q\n期望: %q",
			back.Manifest.SearchSkippedReason, s.Manifest.SearchSkippedReason)
	}

	var top map[string]json.RawMessage
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("读 manifest: %v", err)
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := top["search_skipped_reason"]; !ok {
		t.Errorf("非空时 JSON 必须含 search_skipped_reason\n实际内容: %s", raw)
	}
}

// ============================================================================
// M1c-1 的 TASK-009：把对账层接进管线
//
// Context Checkpoint: done_criteria → test mapping
//
//	functional[0]      端到端可达（不是 mock 计数）→ TestRunBackfillReconcileIsReachedEndToEnd
//	functional[1]      四个可见面全部输出          → TestRunBackfillReportsAllFourFaces
//	boundary[0]        零期次 vs 全部齐全可分      → TestRunBackfillEmptyDiffersFromComplete
//	boundary[1]        缺篇不是失败                → TestRunBackfillMissingPeriodsIsNotFailure
//	error_handling[0]  Violations ⇒ 非零 + 具体内容 → TestRunBackfillViolationsCauseNonZeroExit
//	error_handling[1]  抓取失败与对账违规互不吞没  → TestRunBackfillFailedAndViolationsBothVisible
// ============================================================================

// bfRunReport 跑一次 runBackfill，把对账报告收进 buffer 后连同 error 一起交出。
//
// ⚠️ 端到端：走的是真的 runBackfill（fake 网络 + 临时目录），**不是** mock 计数。
// 「对账被调用了一次」这种间接证据对「**调用了、但结果被丢弃**」同样为真，
// 而那正是本任务要堵的缺口的下一个变种（test-agent-28 指出）。
func bfRunReport(t *testing.T, f Fetcher, cfg BackfillConfig) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cfg.Report = &buf
	err := runBackfill(context.Background(), f, func(time.Duration) {}, cfg)
	return buf.String(), err
}

// 🔴 functional[0]：对账在生产路径上**真的被执行**，且它的**具体产出**到得了输出。
//
// 缺口的形态：reconcileBackfill 有 12 个测试、12 个变异全 KILLED，而 runBackfill
// 一次都不碰它 —— **单元层每条断言都有鉴别力，而整层的可达性没有任何断言在管**。
// 这是「守卫在场但不可达」在**集成层**的版本。
func TestRunBackfillReconcileIsReachedEndToEnd(t *testing.T) {
	cfg := bfConfig(t, t.TempDir())

	report, err := bfRunReport(t, bfSite(t, cfg, bfTwoArticles()...), cfg)
	require.NoError(t, err)

	// 具体产出，不是「非空」：两个期次各自成行，篇数与规则都要对得上。
	assert.Contains(t, report, backfillReconcileHeader)
	assert.Contains(t, report, "2020-01", "期次行必须出现")
	assert.Contains(t, report, "2020-02")
	assert.Contains(t, report, backfillRuleV1, "规则版本要写在行里")
	assert.Contains(t, report, "1/3", "实得/应有：每期只抓到金融统计一篇")

	// 「只调一次」：报告头在整份输出里**恰好出现一次**。
	// 用输出计数而不是 mock 计数 —— mock 被调了一次但结果被丢弃时，mock 断言照样绿。
	assert.Equal(t, 1, strings.Count(report, backfillReconcileHeader),
		"对账只该跑一次；出现两次说明循环里也调了一遍")
}

// 🔴 functional[1]：`Rows` / `MissingPeriods` / `Unclassified` / `Warnings` **四个面都要输出**。
//
// ⚠️ 逐个断言**具体内容**，不许只断言「输出非空」—— 后者对「只打印了 Rows」同样为真。
//
// Unclassified 的造法：给搜索侧一条**只有它索到**的条目，标题用语义非法的期次
// （央行不发单季报，`backfillPeriodOf` 会拒它）。它经交叉校验进抓取集、进 manifest，
// 于是对账时落进 Unclassified —— 这条链路本身也顺带证明了对账吃的是 manifest 而不是 index 侧。
func TestRunBackfillReportsAllFourFaces(t *testing.T) {
	cfg := bfConfig(t, t.TempDir())
	arts := bfTwoArticles()

	const oddID, oddTitle = "9009", "2020年三季度金融统计数据报告" // 语义非法 ⇒ Unclassified
	f := bfSite(t, cfg, arts...)
	for _, kw := range backfillSearchKeywords {
		f.pages[backfillSearchURL(kw, cfg.From, cfg.To, 1)] = searchPageHTML(2, 1,
			searchItemHTML(bfArticleURL(arts[0].ID), arts[0].Title, "2020年01月10日"),
			searchItemHTML(bfArticleURL(oddID), oddTitle, "2020年10月20日"))
	}
	f.pages[bfArticleURL(oddID)] = bfArticleHTML(oddID)

	report, err := bfRunReport(t, f, cfg)
	require.NoError(t, err)

	assert.Contains(t, report, "2020-01", "① Rows：期次行")
	assert.Contains(t, report, "2020-02", "① Rows：另一期")
	assert.Contains(t, report, backfillKindStock, "① Rows：缺的种类要点名")

	assert.Contains(t, report, backfillReconcileMissingLabel, "② MissingPeriods 段")

	assert.Contains(t, report, backfillReconcileUnclassifiedLabel, "③ Unclassified 段")
	assert.Contains(t, report, oddTitle, "③ 认不出的标题原文要出现，否则人无从判断站点是不是改了表述")

	assert.Contains(t, report, backfillReconcileWarningLabel, "④ Warnings 段")
	assert.Contains(t, report, "推算值", "④ 未显式传期望值 ⇒ 与推算值不符的告警")
}

// 🔴 boundary[0]：「一期都没有」与「全部齐全」必须能从输出区分开。
//
// 两者的 `MissingPeriods` **都是空的**（caller_contract 第 3 条），所以只断言
// 「两份输出不相同」还不够 —— 那对「其中一份多了个无关字符」也为真。
// 各自钉住自己那句特征文本，将来有人把两条文案改成一样时才会红。
func TestRunBackfillEmptyDiffersFromComplete(t *testing.T) {
	// A：index 上有列表项但一条报告都没有 ⇒ 抓取集为空 ⇒ 对账零期次。
	cfgA := bfConfig(t, t.TempDir())
	fA := &fakeFetcher{pages: map[string][]byte{
		testIndexURL: backfillIndexPageHTML(1, backfillItemsOn("910", "2020-03-25", "2020-03-11")...),
	}}
	for _, kw := range backfillSearchKeywords {
		fA.pages[backfillSearchURL(kw, cfgA.From, cfgA.To, 1)] = searchPageHTML(0, 1)
	}
	reportEmpty, err := bfRunReport(t, fA, cfgA)
	require.NoError(t, err, "一条报告都没有不是失败")

	// B：某一期三种齐全 ⇒ 无缺篇。
	cfgB := bfConfig(t, t.TempDir())
	full := []bfArticle{
		{ID: "8001", Title: "2020年1月金融统计数据报告", Date: "2020-02-10"},
		{ID: "8002", Title: "2020年1月社会融资规模存量统计数据报告", Date: "2020-02-10"},
		{ID: "8003", Title: "2020年1月社会融资规模增量统计数据报告", Date: "2020-02-10"},
	}
	reportFull, err := bfRunReport(t, bfSite(t, cfgB, full...), cfgB)
	require.NoError(t, err)

	require.NotEqual(t, reportEmpty, reportFull, "前置锚点：两份输出必须不同")

	assert.Contains(t, reportEmpty, backfillReconcileEmptyLabel, "零期次要有自己的特征文本")
	assert.NotContains(t, reportEmpty, backfillReconcileCompleteLabel)

	assert.Contains(t, reportFull, backfillReconcileCompleteLabel, "全部齐全也要有自己的特征文本")
	assert.NotContains(t, reportFull, backfillReconcileEmptyLabel)
}

// 🔴 boundary[1]：`MissingPeriods` 非空、`Violations` 为空 ⇒ **退出码 0**。
//
// 缺篇是**要报告的事实**不是失败（caller_contract 第 1 条）：历史上本来就有期次不齐，
// 把它判成失败会让一次正常的回填无法收工。与 error_handling[0] 是**一对**——
// 一条钉「该失败时失败」，一条钉「不该失败时不失败」，缺任一条另一条就可能被一个
// 「恒失败」或「恒成功」的实现满足。
func TestRunBackfillMissingPeriodsIsNotFailure(t *testing.T) {
	cfg := bfConfig(t, t.TempDir()) // 不设 ExpectPeriods/ExpectArticles ⇒ 走告警路径
	report, err := bfRunReport(t, bfSite(t, cfg, bfTwoArticles()...), cfg)

	require.NoError(t, err, "缺篇不构成失败")
	require.Contains(t, report, backfillReconcileMissingLabel, "前置锚点：这份输入确实缺篇，否则本条平凡通过")
	assert.Contains(t, report, "2020-01")
}

// 🔴 error_handling[0]：`Violations` 非空 ⇒ **非零退出**，且 error 文本含**具体的 violation 内容**。
//
// 「笼统的『对账失败』」不够：跑完一趟约 9 分钟、400+ 请求，人看到的第一行就该说清
// 是期数不符还是篇数不符、期望多少实得多少。
func TestRunBackfillViolationsCauseNonZeroExit(t *testing.T) {
	cfg := bfConfig(t, t.TempDir())
	cfg.ExpectPeriods = 99 // 显式传入且不符 ⇒ 硬失败

	report, err := bfRunReport(t, bfSite(t, cfg, bfTwoArticles()...), cfg)

	require.Error(t, err, "显式期望值不符必须非零退出")
	assert.Contains(t, err.Error(), "99", "error 里要有期望值")
	assert.Contains(t, err.Error(), "期数", "要说清是哪一项不符，不是笼统的『对账失败』")
	assert.Contains(t, report, backfillReconcileViolationLabel, "报告里也要有，别只在 error 里")
}

// 🔴 error_handling[1]：抓取失败与对账违规**同时发生**时，两者的信息**都要能看到**。
//
// 既有的 `failed > 0` 非零退出行为不变。⚠️ 一个「先判 failed 就 return」的实现会把对账
// 违规**整个吞掉**，而那趟跑已经花了 9 分钟——人拿到的错误信息里少一半。
func TestRunBackfillFailedAndViolationsBothVisible(t *testing.T) {
	cfg := bfConfig(t, t.TempDir())
	cfg.ExpectPeriods = 99
	arts := bfTwoArticles()
	f := bfFailOn{inner: bfSite(t, cfg, arts...), contains: arts[1].ID}

	_, err := bfRunReport(t, f, cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "抓取失败", "① 既有的抓取失败信息不能丢")
	assert.Contains(t, err.Error(), "期数", "② 对账违规信息也不能丢")
}

// 🔴 `Report` 为 nil 时默认写 **os.Stdout**，不是 io.Discard。
//
// 这条钉的是**默认值的方向**：忘记设 Report 是一定会发生的，而默认丢弃会让这个
// 「防静默」的机制**以静默的方式**失效。改成 io.Discard 时本条会红，而全包其余用例
// 全都自己传了 writer ⇒ 不会有任何反应。
func TestBackfillReportWriterDefaultsToStdout(t *testing.T) {
	assert.Same(t, os.Stdout, backfillReportWriter(nil), "nil ⇒ 默认可见，不是默认丢弃")

	var buf bytes.Buffer
	assert.Same(t, &buf, backfillReportWriter(&buf), "显式传入时原样用调用方给的")
}

// 🔴 `cfg.Cutover` 必须**真的被用**，不是在 runBackfill 里写死一份。
//
// DoD functional[0] 明写「cutover 经 BackfillConfig 由调用方传入，不在 runBackfill 里
// 写死一份」（caller_contract 第 4 条）。⚠️ 这条**原本没有守卫**：变异 S9（把
// `cmp.Or(cfg.Cutover, backfillCutover)` 换成 `backfillCutover`）**存活** ——
// 全包没有任何用例传过一个与默认值不同的 cutover。
//
// 判据造法：同一批输入（2020-01 只有金融统计一篇），默认 cutover(2025-09) 下它是 v1
// ⇒ 应有 3 篇 ⇒ 缺篇；把 cutover 挪到 2020-01 之后它变 v2 ⇒ 应有 1 篇 ⇒ 不缺。
// **同一份输入、两种结论**，写死实现给不出后者。
func TestRunBackfillHonorsConfiguredCutover(t *testing.T) {
	arts := []bfArticle{{ID: "7001", Title: "2020年1月金融统计数据报告", Date: "2020-02-10"}}

	outDefault := t.TempDir()
	cfgDefault := bfConfig(t, outDefault)
	reportDefault, err := bfRunReport(t, bfSite(t, cfgDefault, arts...), cfgDefault)
	require.NoError(t, err)
	require.Contains(t, reportDefault, backfillRuleV1, "前置锚点：默认切换点下 2020-01 是 v1")
	assert.Contains(t, reportDefault, backfillReconcileMissingLabel, "v1 期次只有 1 篇 ⇒ 缺篇")

	outEarly := t.TempDir()
	cfgEarly := bfConfig(t, outEarly)
	cfgEarly.Cutover = "2020-01" // 调用方传入 ⇒ 该期起按 v2 判
	reportEarly, err := bfRunReport(t, bfSite(t, cfgEarly, arts...), cfgEarly)
	require.NoError(t, err)

	assert.Contains(t, reportEarly, backfillRuleV2, "cfg.Cutover 必须真的生效")
	assert.Contains(t, reportEarly, backfillReconcileCompleteLabel, "v2 期次一篇即齐全 ⇒ 不缺")
	assert.NotContains(t, reportEarly, backfillReconcileMissingLabel)
}

// 🔴 对账吃的是 **manifest 里已抓到的篇目**，不是交叉校验的抓取集 `cc.Fetch`。
//
// 两者在「有篇目抓取失败」时**结论相反**：`cc.Fetch` 是**打算抓**的清单，用它对账会把
// 抓失败的那篇算成「有」⇒ 报告说这期齐全，而磁盘上根本没有它。对账要回答的是
// 「**手上这份产物完不完整**」，抓取失败造成的缺口同样要算进来。
//
// ⚠️ 这条**原本也没有守卫**：变异 S10（入参换成 `backfillReconcileItemsFromCandidates(cc.Fetch)`）
// **存活** —— 全包没有任何用例让「抓取失败」与「对账结论」同时可观测。
// 这正是我在实现注释里写了、却没人钉住的那句话（本 sprint 第三次撞到同一形状）。
func TestRunBackfillReconcilesFetchedNotPlanned(t *testing.T) {
	out := t.TempDir()
	cfg := bfConfig(t, out)
	// 2020-01 期三种齐全 —— 若三篇都抓到，本期不缺。
	arts := []bfArticle{
		{ID: "7101", Title: "2020年1月金融统计数据报告", Date: "2020-02-10"},
		{ID: "7102", Title: "2020年1月社会融资规模存量统计数据报告", Date: "2020-02-10"},
		{ID: "7103", Title: "2020年1月社会融资规模增量统计数据报告", Date: "2020-02-10"},
	}
	// 让「增量」那篇抓取失败 ⇒ 它进 failed[]，**不进** manifest.Articles。
	f := bfFailOn{inner: bfSite(t, cfg, arts...), contains: "7103"}

	report, err := bfRunReport(t, f, cfg)
	require.Error(t, err, "有篇目抓取失败 ⇒ 非零退出（既有行为）")

	assert.Contains(t, report, backfillReconcileMissingLabel,
		"抓失败的那篇不在产物里 ⇒ 这一期就是缺的；用 cc.Fetch 对账会说它齐全")
	assert.Contains(t, report, backfillKindFlow, "缺的正是抓失败的那一种")
	assert.NotContains(t, report, backfillReconcileCompleteLabel)
}

// 🔴 functional[2](a)：**逐条钉住写盘失败路径**，不是只钉一条。
//
// 验证者实测：`AbortsOnDiskFailure` 只钉住 articles/ 那一条，其余写盘点**实现全对但
// 无断言** —— 改坏它们全包不会红。表驱动跑满可构造的每一格。
//
// 构造法全部靠**文件系统**，不动实现（DoD：这是「补背书」不是「改行为」）：
//   - 把某个本该是目录的路径预置成**普通文件** ⇒ 那一格的 MkdirAll 必然失败
//   - 把产物根目录 chmod 成只读、但**预先建好** index/ 与 search/ 子目录
//     ⇒ 前两格照常写入（写的是子目录），而 manifest 的 os.CreateTemp(根目录) 失败
//
// ⚠️ **`AppendFailed` / `AppendArticle` 内部那两次 Save 不在本表内**：它们发生在
// 第 114 行那次 Save **之后**，与它写同一个目录 ⇒ 静态的文件系统构造没法让前者成功、
// 后者失败；而要注入就得改 `backfill_manifest.go`，那**不在本任务的 writes 里**。
// 已向 Leader 申报，未擅自扩大范围。
func TestBackfillFetchAbortsOnEveryDiskFailurePath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("以 root 运行时挡不住写入，构造不出落盘失败")
	}
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, out string)
		want  string // 错误里应出现的片段 —— 钉住「红在哪一格」，不只是「红了」
		//
		// 🔴 **子测试名里不许出现 want 的字面量**：`t.TempDir()` 的路径**包含子测试名**，
		// 于是 `Contains(err, "manifest")` 会被临时目录路径满足 —— 断言看起来钉住了
		// 失败来源，实际匹配的是自己造的路径。我第一版就是这么写的（子测试叫
		// 「manifest 落盘失败」），消融「忽略第 114 行 Save 的错误」时**四格全绿**，
		// 是「看哪一格红」把它揪出来的：那一格当时报的其实是 articles 的错。
	}{
		{
			name:  "第一步：栏目页快照落盘失败",
			setup: func(t *testing.T, out string) { mustWriteBlocker(t, filepath.Join(out, "index")) },
			want:  "/index:",
		},
		{
			name: "第二步：检索页快照落盘失败",
			setup: func(t *testing.T, out string) {
				require.NoError(t, os.MkdirAll(filepath.Join(out, "index"), 0o755)) // 让 index 那格先过
				mustWriteBlocker(t, filepath.Join(out, "search"))
			},
			want: "/search:",
		},
		{
			name:  "第三步：文章正文落盘失败",
			setup: func(t *testing.T, out string) { mustWriteBlocker(t, filepath.Join(out, "articles")) },
			want:  "/articles:",
		},
		{
			name: "第四步：清单落盘失败（根目录不可写）",
			setup: func(t *testing.T, out string) {
				// index/ 与 search/ 预先建好 ⇒ 它们的写入只需子目录可写；
				// 而 manifest 的 os.CreateTemp 写在**根目录**上，必然失败。
				require.NoError(t, os.MkdirAll(filepath.Join(out, "index"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(out, "search"), 0o755))
				require.NoError(t, os.Chmod(out, 0o555))
				t.Cleanup(func() { _ = os.Chmod(out, 0o755) }) // 否则 t.TempDir 清理失败
			},
			want: "manifest.json",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			cfg := bfConfig(t, out)
			arts := bfTwoArticles()
			tt.setup(t, out)

			_, err := bfRun(t, bfSite(t, cfg, arts...), cfg)

			require.Error(t, err, "落盘失败必须中止，不能继续抓")
			assert.Contains(t, err.Error(), tt.want,
				"错误要点名是哪一格落盘失败了——只断言「有错」时，任何一格坏了这条都绿")
		})
	}
}

// mustWriteBlocker 把 path 造成一个**普通文件**，于是任何以它为目录的写入必然失败。
func mustWriteBlocker(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
}

// 🔴 functional[2](b)：两侧 URL **不分叉** —— 在 `runBackfill` 层证伪，不在 crossCheck 层。
//
// index 侧给的是**站内相对路径** href，搜索侧给的是**绝对 URL**，两者指向同一个
// article_id。只有 `resolveURL(cfg.IndexURL, href)` 的产出与搜索侧**逐字相同**时，
// 交叉校验才会把它们认成同一篇（Source=both）、抓取才会命中 fake 里注册的那个 URL。
//
// 🔴 **为什么必须在 runBackfill 层**（DoD 已订正过一次）：在 `crossCheckBackfill` 层断言
// 「两侧同值时输出等于它」是**恒真**的 —— 那个函数收的是**已构造好**的两侧条目，
// **它看不到 base**；两侧同值时「取哪一侧」在输出上不可观测，取 index 也对、取 search
// 也对、随机取也对。放到这一层之后，把 `cfg.IndexURL` 写成 `http://` 或多一个末尾斜杠，
// 它立刻红（我实测过，见 discovery 的消融记录）。
func TestRunBackfillIndexAndSearchURLsDoNotDiverge(t *testing.T) {
	out := t.TempDir()
	cfg := bfConfig(t, out)
	arts := bfTwoArticles()
	// bfSite 的 index 用相对 href（backfillListItemHTML），搜索用绝对 URL（bfArticleURL），
	// 且搜索侧只索到 arts[0] ⇒ 那一篇是两侧都有的那个交集元素。
	f := bfSite(t, cfg, arts...)

	_, err := bfRun(t, f, cfg)
	require.NoError(t, err, "两侧 URL 一致时不该有抓取失败")

	st, err := loadManifest(out)
	require.NoError(t, err)
	got := map[string]Article{}
	for _, a := range st.Manifest.Articles {
		got[a.ID] = a
	}

	both := got[arts[0].ID]
	require.NotEmpty(t, both.ID, "前置锚点：交集那一篇必须真的进了 manifest")
	assert.Equal(t, bfArticleURL(arts[0].ID), both.URL,
		"index 侧相对 href 经 resolveURL 后必须与搜索侧绝对 URL 逐字相同")
	assert.Equal(t, backfillSourceBoth, both.Source,
		"两侧认成同一篇才会是 both——分叉时会变成两条 index/search 各一份")

	// 阴性对照：只有 index 侧索到的那一篇，URL 同样由 resolveURL 产出。
	onlyIndex := got[arts[1].ID]
	require.NotEmpty(t, onlyIndex.ID)
	assert.Equal(t, bfArticleURL(arts[1].ID), onlyIndex.URL)
	assert.Equal(t, backfillSourceIndex, onlyIndex.Source)
}
