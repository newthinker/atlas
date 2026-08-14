package hestia

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// 回填抓取层（M1c-1 的 TASK-006，design-spec §7）。
//
// 本文件是整条回填链路的编排者：index 扫描（TASK-002）→ 站内搜索（TASK-004）→
// 交叉校验（TASK-005）→ 逐篇下载并落进 manifest（TASK-003）。上游各层都是纯逻辑或
// 单页取回，**限速、落盘、断点续抓、两类错误的不同处置全在这里**。

// backfillInterval 是两次请求之间的最小间隔。
//
// 央行站点没有公开的速率限制，1 req/s 是**自我约束**：回填是一次性动作，
// 约 480 次请求跑 9 分钟完全可以接受，没有理由更快。
const backfillInterval = time.Second

// backfillLimiter 给任意 Fetcher 套上「每次请求前先等一秒」。
//
// 做成 Fetcher 装饰器而不是在每个调用点插 sleep，是为了让
// **「sleep 次数 == 请求次数 − 1」由构造保证**：index 翻页与搜索翻页的循环都在上游
// 各自的包里，逐个调用点去插 sleep 既漏得掉、也没法在测试里数清楚。
//
// sleep 可注入（`func(time.Duration)`），测试注入记录器；**不引入 fake clock** ——
// 被测的是「调了几次、每次多久」，不是时间本身。
type backfillLimiter struct {
	inner Fetcher
	sleep func(time.Duration)
	n     int // 已发出的请求数
}

func (l *backfillLimiter) Get(ctx context.Context, url string) ([]byte, error) {
	if l.n > 0 { // 第一次不等：等在请求**之间**，不是请求之前
		l.sleep(backfillInterval)
	}
	l.n++
	return l.inner.Get(ctx, url)
}

// BackfillConfig 是一次回填的参数。
//
// Out 是产物根目录（design-spec §8：manifest.json + index/ + search/ + articles/），
// 必须是仓库外的绝对路径——那三个理由是 worktree 拆掉产物仍在、不可能误提交、
// 会话外备份直接 cp -r。
type BackfillConfig struct {
	IndexURL string
	From     time.Time // 语义是**发布日期**不是期次（设计附录 §A1）
	To       time.Time // 搜索侧的区间上界；零值时用 time.Now()
	Out      string
	MaxPages int           // <= 0 时用 backfillMaxPages
	Timeout  time.Duration // 仅 BackfillFetch 用：单次 HTTP 的超时
}

// BackfillFetch 跑一次完整回填。**本包对外的唯一入口**（cmd/atlas 接 cobra 用）。
//
// 返回非 nil 不等于「什么都没抓到」：单篇失败会记进 manifest 的 failed[] 并继续，
// 跑完才返回非零 —— 产物仍然可用，调用方据此决定要不要重跑补齐。
func BackfillFetch(ctx context.Context, cfg BackfillConfig) error {
	return runBackfill(ctx, NewPBOCFetcher(cfg.Timeout), time.Sleep, cfg)
}

// runBackfill 是 BackfillFetch 的可注入版本：Fetcher 与 sleep 都由调用方给。
//
// # 两类错误的处置刻意相反（design-spec §7.2）
//
//   - **单篇文章抓取失败**（网络抖动、单篇 404）⇒ 记进 failed[]、**继续抓后面的**、
//     跑完返回非零。这是外部世界的事，一篇抓不到不该让另外两百篇也白跑。
//   - **落盘失败**（磁盘满、权限）⇒ **立刻中止**。这是本机的事——继续抓只会浪费请求，
//     而且后面每一篇都会失败。
//
// index 侧抓取/解析失败同样是硬失败（少一页 index = 静默少十几条候选）；
// 搜索侧失败则 fail-open（ADR-M1c1-02），由 crossCheckBackfill 退化处理。
func runBackfill(ctx context.Context, f Fetcher, sleep func(time.Duration), cfg BackfillConfig) error {
	store, err := loadManifest(cfg.Out)
	if err != nil {
		return err // 含「manifest 坏了」——绝不当成首跑，那会退化成全量重抓
	}
	to := cfg.To
	if to.IsZero() {
		to = time.Now()
	}
	lim := &backfillLimiter{inner: f, sleep: sleep}

	scan, err := scanBackfillIndex(ctx, lim, backfillIndexCfg{
		IndexURL: cfg.IndexURL, From: cfg.From, MaxPages: cfg.MaxPages,
	})
	if err != nil {
		return fmt.Errorf("index 侧扫描: %w", err)
	}
	if err := saveBackfillIndexPages(cfg.Out, scan.Pages); err != nil {
		return err
	}

	hits, searchPages, searchErr := fetchBackfillSearchAll(ctx, lim, cfg.Out, cfg.From, to)
	if searchErr != nil && errors.Is(searchErr, errBackfillDisk) {
		return searchErr // 落盘失败不 fail-open：那是本机故障，不是检索服务的事
	}

	cc := crossCheckBackfill(scan.Reports, hits, searchErr)

	store.Manifest.From = cfg.From.Format("2006-01")
	store.Manifest.ScannedAt = time.Now().UTC().Format(time.RFC3339)
	store.Manifest.PagesScanned = len(scan.Pages)
	store.Manifest.SearchPagesScanned = searchPages
	store.Manifest.OnlyInIndex = cc.OnlyInIndex
	store.Manifest.OnlyInSearch = cc.OnlyInSearch
	store.Manifest.SearchSkippedReason = cc.SearchSkippedReason
	if err := store.Save(); err != nil {
		return err
	}

	var failed int
	for _, c := range cc.Fetch {
		// 「已抓」= manifest 有记录 **且** 文件在盘上。断点续抓的整个价值就在这一句：
		// 命中就**根本不发请求**，而不是发了再丢掉。
		if store.HasArticle(c.ArticleID) {
			continue
		}
		body, err := lim.Get(ctx, c.URL)
		if err != nil {
			failed++
			if err := store.AppendFailed(Failed{ID: c.ArticleID, URL: c.URL, Error: err.Error()}); err != nil {
				return err // 记不下失败本身就是落盘失败 ⇒ 中止
			}
			continue
		}
		rel := articleFile(c.ArticleID)
		if err := writeBackfillFile(filepath.Join(cfg.Out, rel), body); err != nil {
			return err
		}
		if err := store.AppendArticle(Article{
			ID:        c.ArticleID,
			Title:     c.Title,
			Published: c.Published,
			URL:       c.URL,
			File:      rel,
			SHA256:    articleSHA256(body),
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
			Source:    c.Source,
		}); err != nil {
			return err
		}
	}

	if failed > 0 {
		return fmt.Errorf("回填完成但有 %d 篇抓取失败（详见 %s 的 failed[]）", failed, store.Path())
	}
	return nil
}

// errBackfillDisk 标记「本机落盘故障」，用来把它与搜索侧的取回/解析失败区分开：
// 后者 fail-open，前者必须中止。
var errBackfillDisk = errors.New("落盘失败")

// fetchBackfillSearchAll 遍历三个关键词、逐页取回搜索结果，顺带存快照。
//
// 返回的 error 一律交给 crossCheckBackfill 走 fail-open（ADR-M1c1-02：不让一个不可控的
// 第三方检索服务有权否决主路径交付），**唯独** errBackfillDisk 例外，由调用方中止。
//
// 这里不用 fetchBackfillSearchPage：那个函数把原始 HTML 丢了，而 design-spec §7.3 要求
// 搜索页也存盘（同样不可再生）。取回 → 解析 → 区间校验 → 栏目筛的**顺序与它逐字一致**，
// 尤其区间校验必须在栏目筛**之前**（某页若恰好全是调统司栏目，筛后为空会让守卫平凡通过）。
func fetchBackfillSearchAll(ctx context.Context, f Fetcher, out string, from, to time.Time) ([]backfillSearchHit, int, error) {
	var all []backfillSearchHit
	pages := 0
	for i, kw := range backfillSearchKeywords {
		total := 1
		for page := 1; page <= total; page++ {
			html, err := f.Get(ctx, backfillSearchURL(kw, from, to, page))
			if err != nil {
				return nil, pages, fmt.Errorf("搜索 %q 第 %d 页: %w", kw, page, err)
			}
			pages++
			name := fmt.Sprintf("%s-p%02d.html", backfillSearchSlug(kw, i), page)
			if err := writeBackfillFile(filepath.Join(out, "search", name), html); err != nil {
				return nil, pages, err
			}
			hits, totalPages, err := parseBackfillSearchPage(html, page)
			if err != nil {
				return nil, pages, fmt.Errorf("搜索 %q 第 %d 页: %w", kw, page, err)
			}
			if err := checkBackfillSearchDateRange(hits, from, to); err != nil {
				return nil, pages, fmt.Errorf("搜索 %q 第 %d 页: %w", kw, page, err)
			}
			all = append(all, filterBackfillSearchHits(hits)...)
			total = totalPages
		}
	}
	return all, pages, nil
}

// backfillSearchSlug 是关键词在快照文件名里的短名。
//
// 用 ASCII 短名而不是关键词原文：文件名要能被人和工具直接用，而中文文件名在
// macOS 上还会被规范化成 NFD。序号兜底保证**任何**关键词都有唯一名字——
// 表漏了一项也只是名字难看，不会让两个关键词写进同一个文件（那会静默互相覆盖）。
func backfillSearchSlug(keyword string, i int) string {
	if s, ok := backfillSearchSlugs[keyword]; ok {
		return s
	}
	return fmt.Sprintf("kw%d", i+1)
}

var backfillSearchSlugs = map[string]string{
	"金融统计数据报告":       "finstats",
	"社会融资规模存量统计数据报告": "tsf-stock",
	"社会融资规模增量统计数据报告": "tsf-flow",
}

// saveBackfillIndexPages 存 index 快照。
//
// 命名按设计附录 §A3 `<抓取序号>-<该页最新条目日期>_<该页最老条目日期>.html`：
// **页码不作唯一键**，因为页码随新文章上架而漂移，重跑时同一个文件名会对应不同内容的页
// ——而存快照的全部目的正是「重跑与 M1c-2 的核对都能离线做」。
func saveBackfillIndexPages(out string, pages []backfillIndexPage) error {
	for i, p := range pages {
		name := fmt.Sprintf("%03d-%s_%s.html", i+1, p.Newest, p.Oldest)
		if err := writeBackfillFile(filepath.Join(out, "index", name), p.HTML); err != nil {
			return err
		}
	}
	return nil
}

// writeBackfillFile 建目录并写文件，错误一律裹上 errBackfillDisk。
func writeBackfillFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: 建目录 %s: %v", errBackfillDisk, filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("%w: 写 %s: %v", errBackfillDisk, path, err)
	}
	return nil
}
