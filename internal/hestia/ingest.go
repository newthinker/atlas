package hestia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// IngestDeps 是一次抓取所需的全部外部依赖。
//
// 编排层放在 internal/hestia 而不是 cmd/atlas：它是业务编排（发现→抓→解析→
// 校验→入库），和 Discover / Parse / Validate / Save 同层；而且测试要用同包的
// testdata 快照，放 cmd 就得写 ../../internal/hestia/testdata 或复制一份。
type IngestDeps struct {
	Store *Store
	Fetch Fetcher
	Cfg   Config
	Out   io.Writer // nil 等价于 io.Discard
	Force bool      // 绕过一级幂等键，用于改了阈值后重跑
}

// Ingest 跑一轮发现与入库。
//
// 单期失败**不中断整批** —— 一期解析失败不该阻止其它期入库。逐期收集，最后
// 汇总返回非零：launchd 的 err.log 会留痕，launchctl list 也能看到。一期反复
// 失败会让每次唤起都非零，那本来就该被注意到。
//
// 汇总错误用 errors.Join 把各期的错误**包进去**，而不是只拼一句人话：调用方
// （以及测试）要能 errors.Is / errors.As 出底层判因，只留字符串等于把判因扔了。
func Ingest(ctx context.Context, d IngestDeps) error {
	if d.Out == nil {
		d.Out = io.Discard
	}

	cands, err := Discover(ctx, d.Fetch, d.Store, d.Cfg.Discover)
	if err != nil {
		// 此刻还没有任何期次，能给的定位上下文只有 index URL。
		return fmt.Errorf("hestia ingest: discover %s: %w", d.Cfg.Discover.IndexURL, err)
	}
	if len(cands) == 0 {
		fmt.Fprintln(d.Out, "no new reports")
		return nil
	}

	// 按期次**升序**处理。Discover 给的是「最近的排在前面」，顺着跑会让
	// stock_continuity / deposit_sum 的漂移检测一次都不真正执行（每一期都成了
	// 「首期」⇒ 恒 no_prior_period），而数据照进权威表、报告照样 Passed=true、
	// **零告警**——skipped 不拉低 Passed（validate.go:101）。
	//
	// 用排序而不是把切片倒过来：倒序依赖「Discover 一定按时间倒序返回」这个
	// 外部约定，而排序直接断言我们要的性质本身。period 是 YYYY-MM 定宽格式，
	// 字典序即时间序。稳定排序 ⇒ 同期次的不同 period_type（12 月月报与年报的
	// period 都是 YYYY-12）保持 Discover 给的相对顺序，它们本就是独立序列。
	slices.SortStableFunc(cands, func(a, b Candidate) int {
		return strings.Compare(a.Period, b.Period)
	})

	var failedPeriods []string
	var errs []error
	for _, c := range cands {
		if err := d.ingestOne(ctx, c); err != nil {
			fmt.Fprintf(d.Out, "%s FAILED: %v\n", c.Period, err)
			failedPeriods = append(failedPeriods, c.Period)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hestia ingest: %d/%d 期失败 (%s): %w",
			len(errs), len(cands), strings.Join(failedPeriods, ", "), errors.Join(errs...))
	}
	return nil
}

// ingestOne 处理一条候选。返回的错误一律带「期次 (article_id)」前缀——汇总之后
// 调用方看到的是一串错误，不指名是哪一期的话没法排障。
func (d IngestDeps) ingestOne(ctx context.Context, c Candidate) error {
	// 出错时统一加定位上下文。期次与 article_id 都给：期次是人认的，article_id
	// 是能直接拼回 URL 的那个。
	wrap := func(stage string, err error) error {
		return fmt.Errorf("hestia ingest %s/%s (%s): %s: %w",
			c.Period, c.PeriodType, c.ArticleID, stage, err)
	}

	if !d.Force {
		seen, err := d.Store.HasArticle(ctx, c.ArticleID)
		if err != nil {
			return wrap("has article", err)
		}
		if seen {
			fmt.Fprintf(d.Out, "%s already ingested (%s)\n", c.Period, c.ArticleID)
			return nil
		}
	}

	raw, err := d.Fetch.Get(ctx, c.URL)
	if err != nil {
		return wrap("fetch "+c.URL, err)
	}
	obs, err := Parse(raw)
	if err != nil {
		return wrap("parse", err)
	}

	// 接缝 ①：Parse 只拿到 raw bytes，看不到 URL —— 让它编造一个 ArticleID 才是
	// 缺陷（parse.go 的原话）。这是两个包之间唯一的手工装配点。
	obs.Meta.ArticleID = c.ArticleID

	// 接缝 ②：标题与正文各解析出一次期次，本该一致。
	//
	// 不一致意味着央行把链接挂错了，或某一侧的解析有 bug —— 两种都该拦下而不是
	// 入库。这是两次独立解析白捡的校验，与 Parse 已有的「PubDate 与正文交叉校验」
	// 同一思路：同一事实的两个独立来源，对不上就是信号。
	//
	// 不拦的代价不只是「一期数据错了」：按正文键入库、按候选键问判停，Discover
	// 下次仍认为那期没入库 ⇒ **静默的永久循环**（Sprint 036 W6 实测）。
	if obs.Meta.Period != c.Period || obs.Meta.PeriodType != c.PeriodType {
		return fmt.Errorf("hestia ingest %s/%s (%s): 期次不一致：标题说 %s/%s，正文说 %s/%s（%s）",
			c.Period, c.PeriodType, c.ArticleID,
			c.Period, c.PeriodType, obs.Meta.Period, obs.Meta.PeriodType, c.URL)
	}

	rep, err := Validate(ctx, obs, d.Store, d.Cfg.Thresholds)
	if err != nil {
		return wrap("validate", err)
	}
	out, err := d.Store.Save(ctx, obs, rep)
	if err != nil {
		return wrap("save", err)
	}

	// Verdict 与 Table 都打出来。Table 是当下就必须区分的（入权威表 vs 落 pending
	// 对运维的含义相反）；Verdict 当前恒为 New（经 Discover 过滤的候选必然不在
	// 当前行里），打它是为了将来有人放开那条路时，Duplicate/Revision 自己会显形，
	// 而不是悄悄混在「已入库」里。
	fmt.Fprintf(d.Out, "%s %s → %s\n", obs.Meta.Period, out.Verdict, out.Table)
	return nil
}
