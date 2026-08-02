// Package prism orchestrates the daily Prism valuation refresh (design doc
// docs/prism/atlas_prism_design.md §9 M1).
package prism

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/tushare"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/newthinker/atlas/internal/valuation"
)

// Store is the subset of *prismstore.Store used by Refresh and, from M3, by the
// segment refresh and the sankey service.
type Store interface {
	UpsertInstrument(inst prismstore.Instrument) (int64, error)
	LatestDate(instrumentID int64) (string, error)
	UpsertValuations(instrumentID int64, rows []prismstore.ValuationRow) error
	UpsertFundamentals(instrumentID int64, rows []prismstore.FundamentalRow) error
	Series(symbol, from string) (*prismstore.SeriesData, error)
	// M3 新增
	UpsertPrices(instrumentID int64, closes []prismstore.PriceRow) error
	UpsertSegments(instrumentID int64, rows []prismstore.SegmentRow) error
	SegmentRows(instrumentID int64) ([]prismstore.SegmentRow, error)
	LatestSegmentPeriodEnd(instrumentID int64) (string, error)
	QuarterlyFundamentals(instrumentID int64) ([]prismstore.FundamentalRow, error)
}

// EdgarClient is the subset of *edgar.Client used by the EDGAR refresh path.
type EdgarClient interface {
	FetchCompanyFacts(cik string) ([]edgar.QuarterlyFact, error)
}

// LixingerClient is the subset of *lixinger.Lixinger used by Refresh.
type LixingerClient interface {
	FetchValuationSeries(symbol string, start, end time.Time) ([]lixinger.ValuationPoint, error)
}

// AkshareClient is the subset of *akshare.Client used by Refresh.
type AkshareClient interface {
	FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error)
	FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error)
}

// USClient is the subset of *yahoo.Yahoo used by the US engine path (Task 6).
type USClient interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}

// TushareClient is the subset of *tushare.Client used by the M3.5a A/H fallback
// hops (spec §2). index_dailybasic 无权限(TASK-001 live 探针 40203),故指数与港股
// 链尾只取价格,不取估值。
type TushareClient interface {
	FetchDailyBasic(symbol string, start, end time.Time) ([]tushare.ValuationPoint, error)
	FetchIndexDaily(symbol string, start, end time.Time) ([]tushare.PricePoint, error)
	FetchHKDaily(symbol string, start, end time.Time) ([]tushare.PricePoint, error)
}

// TwelvedataClient is the subset of *twelvedata.Client used as the US price
// second hop. 只补价格,EPS 链路不变。
type TwelvedataClient interface {
	FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error)
}

// errFallbackNoData:兜底源返回零行。兜底跳只在主源已失败时触发,此时「零行」无法与
// 「符号形态不被上游接受」区分——实测 tushare hk_daily 对 ts_code="0700.HK" 返回
// code=0 且 items 为空(非报错)。判成功会天天上报「fallback ok」却零写入,把真实缺口
// 静默吞掉;故兜底跳一律把零行当失败,让缺口显性化。
var errFallbackNoData = errors.New("fallback source returned no data")

// Report summarizes one refresh run. Partial failures do not abort the run.
type Report struct {
	Refreshed int
	Failed    []string // "SYMBOL: error" 摘要
	Degraded  []string // 主源失败但兜底成功的标的(可观测降级,进告警提示不算失败)
}

// degrade records observable degradations; empty strings mean "none".
func (r *Report) degrade(msgs ...string) {
	for _, msg := range msgs {
		if msg != "" {
			r.Degraded = append(r.Degraded, msg)
		}
	}
}

// Refresh updates every configured instrument: lixinger-sourced instruments
// fetch incrementally (理杏豆计费), engine-sourced US stocks rebuild via yahoo.
// ts/td 为 nil 表示该备源未配置:此时行为与 M3.5a 之前完全一致,且不发「备源未配置」
// 提示(ADR#9——未配置是永久状态,天天提示违反 spec §2「不得天天降级」)。
func Refresh(cfg config.PrismConfig, store Store, lix LixingerClient, us USClient, ak AkshareClient, ed EdgarClient,
	ts TushareClient, td TwelvedataClient, now time.Time) Report {
	var rep Report
	for _, inst := range cfg.Instruments {
		var err error
		switch inst.Source {
		case "lixinger":
			err = refreshLixinger(cfg, store, lix, inst, now)
			if err != nil && inst.FallbackSource == "akshare" {
				if fbErr := refreshAkshare(cfg, store, ak, inst, now); fbErr == nil {
					rep.Degraded = append(rep.Degraded,
						fmt.Sprintf("%s: lixinger failed (%v), akshare fallback ok", inst.Symbol, err))
					err = nil
				} else {
					err = fmt.Errorf("lixinger: %v; akshare fallback: %v", err, fbErr)
					// 链尾第三跳(spec §2 A股指数·估值):tushare 仅价格。
					if ts != nil {
						degs, tsErr := tushareFallback(cfg, store, ts, inst, now, fbErr, "lixinger+akshare")
						rep.degrade(degs...)
						if tsErr == nil {
							err = nil
						} else {
							err = fmt.Errorf("%v; tushare fallback: %v", err, tsErr)
						}
					}
				}
			}
		case "engine":
			var degs []string
			degs, err = refreshEngine(cfg, store, us, td, inst, now)
			rep.degrade(degs...)
		case "akshare":
			err = refreshAkshare(cfg, store, ak, inst, now)
			// 二跳(spec §2 A股公司·估值 / 港股·估值):A 股公司取 daily_basic 官方
			// 口径估值,港股与指数只取价格。
			if err != nil && ts != nil {
				degs, tsErr := tushareFallback(cfg, store, ts, inst, now, err, "akshare")
				rep.degrade(degs...)
				if tsErr == nil {
					err = nil
				} else {
					err = fmt.Errorf("akshare: %v; tushare fallback: %v", err, tsErr)
				}
			}
		case "edgar":
			var degs []string
			degs, err = refreshEdgar(cfg, store, ed, us, td, inst, now)
			rep.degrade(degs...)
			if err != nil && inst.FallbackSource == "engine" {
				fbDegs, fbErr := refreshEngine(cfg, store, us, td, inst, now)
				rep.degrade(fbDegs...)
				if fbErr == nil {
					rep.Degraded = append(rep.Degraded,
						fmt.Sprintf("%s: edgar failed (%v), engine fallback ok", inst.Symbol, err))
					err = nil
				} else {
					err = fmt.Errorf("edgar: %v; engine fallback: %v", err, fbErr)
				}
			}
		default:
			err = fmt.Errorf("unknown source %q", inst.Source)
		}
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", inst.Symbol, err))
			continue
		}
		rep.Refreshed++
	}
	return rep
}

func upsertMeta(store Store, inst config.PrismInstrument) (int64, error) {
	return store.UpsertInstrument(prismstore.Instrument{
		Symbol: inst.Symbol, Type: inst.Type, Market: inst.Market,
		Name: inst.Name, Group: inst.Group, Source: inst.Source,
	})
}

// incrementalStart 计算增量拉取起点并处理「已是最新」的日历日零请求守卫,供
// lixinger/akshare 两路复用。返回 (start, skip, err);skip=true 表示已是最新、
// 应零请求直接成功。首次(latest="")从 now-lookbackYears 回填。
//
// 日历日语义比较(ISO 日期串按字典序即时间序,与宿主时区无关):增量起点已达今天
// 或更新 → 零请求。避免西经宿主机把 latest 的 UTC 午夜与本地 now 做绝对时刻比较,
// 产生 startDate>endDate 的倒挂请求。
func incrementalStart(store Store, id int64, lookbackYears int, now time.Time) (time.Time, bool, error) {
	latest, err := store.LatestDate(id)
	if err != nil {
		return time.Time{}, false, err
	}
	start := now.AddDate(-lookbackYears, 0, 0)
	if latest != "" {
		d, perr := time.Parse("2006-01-02", latest)
		if perr != nil {
			return time.Time{}, false, fmt.Errorf("bad latest date %q: %w", latest, perr)
		}
		start = d.AddDate(0, 0, 1) // 增量:从 latest 的次日开始
		if start.Format("2006-01-02") >= now.Format("2006-01-02") {
			return time.Time{}, true, nil
		}
	}
	return start, false, nil
}

func refreshLixinger(cfg config.PrismConfig, store Store, lix LixingerClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	pts, err := lix.FetchValuationSeries(inst.Symbol, start, now)
	if err != nil {
		return err
	}
	rows := make([]prismstore.ValuationRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, prismstore.ValuationRow{
			D:     p.Date.Format("2006-01-02"),
			PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM,
			Pctl5Y: p.Pctl5Y, Pctl10Y: p.Pctl10Y,
		})
	}
	return store.UpsertValuations(id, rows)
}

// currentEPS returns the EPS of the latest-dated point (current TTM EPS),
// mirroring valuation.ReconstructPEPercentile's currentEPS. ok is false for an
// empty slice. Does not assume eps is sorted.
func currentEPS(eps []core.EPSPoint) (float64, bool) {
	var latest core.EPSPoint
	found := false
	for _, p := range eps {
		if !found || p.Date.After(latest.Date) {
			latest, found = p, true
		}
	}
	return latest.EPS, found
}

// rollingMinPoints: 滚动分位窗口内最少样本数(约 1 年交易日)。
const rollingMinPoints = 252

// upsertPrices 把已取得的行情落 price_daily,返回一条降级说明("" 表示成功)。
//
// 价格对估值链路是**附属产物**(供 M3 桑基/基本面页叠加股价),写失败不应让当日
// 估值刷新整条判负 —— 故只返回可观测的降级说明交由调用方记入 Report.Degraded,
// 不作为 error 上抛。
func upsertPrices(store Store, id int64, symbol string, closes []core.OHLCV) string {
	rows := make([]prismstore.PriceRow, len(closes))
	for i, c := range closes {
		rows[i] = prismstore.PriceRow{D: c.Time.Format("2006-01-02"), Close: c.Close}
	}
	if err := store.UpsertPrices(id, rows); err != nil {
		return fmt.Sprintf("%s: price_daily upsert failed (%v), valuation unaffected", symbol, err)
	}
	return ""
}

// refreshEngine 每日全量重算美股公司近 us_lookback_years 年 PE 序列并整段 upsert
// (幂等,无增量状态)。yahoo 路径 M1 口径:无 PB/PSTTM,仅 5Y 滚动分位。
// 返回的首个值是降级说明("" 表示无降级),见 upsertPrices。
func refreshEngine(cfg config.PrismConfig, store Store, us USClient, td TwelvedataClient,
	inst config.PrismInstrument, now time.Time) ([]string, error) {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return nil, err
	}
	start := now.AddDate(-cfg.USLookbackYears, 0, 0)
	// EPS 需要比价格多回看 1 年,保证首个交易日有可对齐的 EPS 点
	eps, err := us.FetchEPSHistory(inst.Symbol, start.AddDate(-1, 0, 0), now)
	if err != nil {
		return nil, fmt.Errorf("eps history: %w", err)
	}
	closes, fbDeg, err := fetchCloses(us, td, inst.Symbol, start, now)
	if err != nil {
		return nil, err
	}
	// 价格是原始事实,紧随取得即落库:后续估值环节(熔断/重建)失败时行情不应一并丢失。
	deg := []string{fbDeg, upsertPrices(store, id, inst.Symbol, closes)}
	// 熔断:当前 EPS(TTM)<=0(真实亏损)→ 不入库,记入 Report.Failed,语义对齐
	// valuation.ReconstructPEPercentile 的 ErrNonPositiveEPS。否则历史有 >=8 正 EPS
	// 季但最新亏损的标的会静默入库截断到最后盈利日的过期序列,误导 board 展示。
	if e, ok := currentEPS(eps); ok && e <= 0 {
		return deg, valuation.ErrNonPositiveEPS
	}
	dates, pe, err := valuation.ReconstructPESeries(closes, eps)
	if err != nil {
		return deg, fmt.Errorf("reconstruct: %w", err)
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)

	rows := make([]prismstore.ValuationRow, len(dates))
	for i := range dates {
		rows[i] = prismstore.ValuationRow{
			D: dates[i].Format("2006-01-02"), PETTM: pe[i],
			PB: math.NaN(), PSTTM: math.NaN(), // yahoo 路径 M1 无 PB/PS(设计 §9 M1 口径标注)
			Pctl5Y: p5[i], Pctl10Y: math.NaN(), // 仅 5Y 数据,10Y 分位无意义
		}
	}
	return deg, store.UpsertValuations(id, rows)
}

// fetchCloses 取美股价格:yahoo 失败且 td 已配置时,切 twelvedata 重取同一段
// (spec §2 美股·价格二跳)。EPS 链路不受影响,twelvedata 只补价格。
// td 为 nil 或兜底也失败时,错误保持既有的 "price history:" 前缀不变。
func fetchCloses(us USClient, td TwelvedataClient, symbol string, start, end time.Time) ([]core.OHLCV, string, error) {
	closes, err := us.FetchHistory(symbol, start, end, "1d")
	if err == nil {
		return closes, "", nil
	}
	if td == nil {
		return nil, "", fmt.Errorf("price history: %w", err)
	}
	fb, fbErr := td.FetchHistory(symbol, start, end)
	if fbErr == nil && len(fb) == 0 {
		fbErr = errFallbackNoData
	}
	if fbErr != nil {
		return nil, "", fmt.Errorf("price history: %v; twelvedata fallback: %v", err, fbErr)
	}
	return fb, fmt.Sprintf("%s: yahoo price failed (%v), twelvedata fallback ok", symbol, err), nil
}

// refreshAkshare 每日增量拉取 akshare(经 aktools)公司/指数估值,并在本地计算滚动
// 分位——akshare 无官方分位,故读回 store 已存全历史与新拉取点合并后整段算 5Y/10Y
// 分位,但只为新拉取的日期写行(历史行不回写)。
func refreshAkshare(cfg config.PrismConfig, store Store, ak AkshareClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	var pts []akshare.ValuationPoint
	if inst.Type == "index" {
		pts, err = ak.FetchIndexValuationSeries(inst.Symbol, start, now)
	} else {
		pts, err = ak.FetchStockValuationSeries(inst.Symbol, inst.Market, start, now)
	}
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return nil // 无新数据,零写入
	}
	vps := make([]valPoint, len(pts))
	for i, p := range pts {
		vps[i] = valPoint{Date: p.Date, PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM}
	}
	return upsertWithLocalPercentile(store, id, inst.Symbol, vps)
}

// valPoint 是估值三件套的来源无关形态,让 akshare(百度)与 tushare(daily_basic)
// 两条 A 股估值链共用同一套本地分位实现,避免两份会各自漂移的算法。
type valPoint struct {
	Date             time.Time
	PETTM, PB, PSTTM float64
}

// upsertWithLocalPercentile 读回库内全历史与新点合并(同日新值覆盖),整段算 5Y/10Y
// 滚动分位,但只为新点写行(历史行不回写)。上游数据源无官方分位时共用此路径。
func upsertWithLocalPercentile(store Store, id int64, symbol string, pts []valPoint) error {
	if len(pts) == 0 {
		return nil
	}
	// 本地分位:历史(剔除 NaN PE)+ 新点合并(同日新值覆盖),升序后整段算 5Y/10Y 分位。
	hist, err := store.Series(symbol, "")
	if err != nil {
		return fmt.Errorf("read back series: %w", err)
	}
	peByDay := make(map[string]float64, len(hist.Dates)+len(pts))
	for i, ds := range hist.Dates {
		if !math.IsNaN(hist.PETTM[i]) {
			peByDay[ds] = hist.PETTM[i]
		}
	}
	newDays := make(map[string]valPoint, len(pts))
	for _, p := range pts {
		ds := p.Date.Format("2006-01-02")
		newDays[ds] = p
		if !math.IsNaN(p.PETTM) {
			peByDay[ds] = p.PETTM
		}
	}
	days := make([]string, 0, len(peByDay))
	for ds := range peByDay {
		days = append(days, ds)
	}
	sort.Strings(days)
	dates := make([]time.Time, len(days))
	pe := make([]float64, len(days))
	idx := make(map[string]int, len(days))
	for i, ds := range days {
		d, perr := time.Parse("2006-01-02", ds)
		if perr != nil {
			return fmt.Errorf("bad stored date %q: %w", ds, perr)
		}
		dates[i], pe[i], idx[ds] = d, peByDay[ds], i
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)
	p10 := valuation.RollingPercentile(dates, pe, 10, rollingMinPoints)

	rows := make([]prismstore.ValuationRow, 0, len(newDays))
	for ds, p := range newDays {
		row := prismstore.ValuationRow{
			D: ds, PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM,
			Pctl5Y: math.NaN(), Pctl10Y: math.NaN(),
		}
		if i, ok := idx[ds]; ok { // PE 为 NaN 的新点不在分位序列中,保持 NaN
			row.Pctl5Y, row.Pctl10Y = p5[i], p10[i]
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].D < rows[j].D })
	return store.UpsertValuations(id, rows)
}

// tushareFallback 按「市场×类型」分派 A/H 兜底跳(spec §2 总表):
// 港股与 A 股指数只取价格(index_dailybasic 无权限,TASK-001 live 探针),
// A 股公司取 daily_basic 官方口径估值。chain 是已失败的上游链名,用于降级文案。
func tushareFallback(cfg config.PrismConfig, store Store, ts TushareClient, inst config.PrismInstrument,
	now time.Time, mainErr error, chain string) ([]string, error) {
	// priceFetch 非 nil 即「仅价格」跳,同时决定走哪个 api;nil 表示 A 股公司估值跳。
	var priceFetch func(symbol string, start, end time.Time) ([]tushare.PricePoint, error)
	switch {
	case inst.Market == "HK":
		priceFetch = ts.FetchHKDaily
	case inst.Type == "index":
		priceFetch = ts.FetchIndexDaily
	}
	priceOnly := priceFetch != nil

	var degs []string
	var err error
	if priceOnly {
		var deg string
		deg, err = refreshTusharePrices(cfg, store, inst, now, priceFetch)
		degs = append(degs, deg)
	} else {
		err = refreshTushareValuation(cfg, store, ts, inst, now)
	}
	if err != nil {
		// 永久性错误(积分/权限)不是临时故障:点明配置性问题,避免运维当抖动反复观察。
		// 本跳只调用一次,永不重试(spec §2「永久不覆盖不重试」)。
		if errors.Is(err, tushare.ErrNoPermission) {
			degs = append(degs, fmt.Sprintf("%s: tushare 跳不可用(权限不足,配置性问题,不重试): %v",
				inst.Symbol, err))
		}
		return degs, err
	}
	msg := fmt.Sprintf("%s: %s failed (%v), tushare fallback ok", inst.Symbol, chain, mainErr)
	if priceOnly {
		msg += "(仅价格,估值缺失)"
	}
	return append(degs, msg), nil
}

// refreshTushareValuation A 股公司估值二跳:daily_basic 官方口径 pe_ttm/pb/ps_ttm,
// 分位与 akshare 路径共用同一套本地滚动窗口算法。
func refreshTushareValuation(cfg config.PrismConfig, store Store, ts TushareClient,
	inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	pts, err := ts.FetchDailyBasic(inst.Symbol, start, now)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return errFallbackNoData
	}
	vps := make([]valPoint, len(pts))
	for i, p := range pts {
		vps[i] = valPoint{Date: p.Date, PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM}
	}
	return upsertWithLocalPercentile(store, id, inst.Symbol, vps)
}

// refreshTusharePrices 是港股与 A 股指数链尾的「仅价格」跳:只写 price_daily。
//
// **刻意不写估值行**:LatestDate 取 MAX(d) FROM valuation_daily,是增量拉取的水位。
// 若为这些天写下估值列全 NaN 的行,水位会被推到今天,主源恢复后 incrementalStart 判定
// 「已是最新」直接 skip,这段窗口的真实估值将永久不回填。缺行与 NaN 行在 Series 视图上
// 同样呈现为缺失,但只有缺行不会毒化水位。
func refreshTusharePrices(cfg config.PrismConfig, store Store, inst config.PrismInstrument, now time.Time,
	fetch func(symbol string, start, end time.Time) ([]tushare.PricePoint, error)) (string, error) {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return "", err
	}
	pts, err := fetch(inst.Symbol, now.AddDate(-cfg.LookbackYears, 0, 0), now)
	if err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", errFallbackNoData
	}
	closes := make([]core.OHLCV, len(pts))
	for i, p := range pts {
		closes[i] = core.OHLCV{Time: p.Date, Close: p.Close}
	}
	return upsertPrices(store, id, inst.Symbol, closes), nil
}

// maxTTMWindowDays 是 4 季 TTM 窗口首尾 period_end 的合理跨度上限。连续 4 季约跨
// 9 个月(实测 273~275 天);整季缺失(fundamental_q 无该季行)时窗口跨缺口求和,
// 首尾跨度跃升至约 365 天(单季缺口)。取 330 天:高于正常跨度+财季不规则边际,
// 低于单季缺口跨度,据此剔除跨缺口窗口防 TTM 失真(QA WARNING-2)。
const maxTTMWindowDays = 330

// ttmPoints 通用 4 季滑窗:val 取单季值,4 季齐、皆非 NaN 且窗口季度连续(首尾
// period_end 跨度 <= maxTTMWindowDays)才产 TTM 阶梯点(EPSPoint 携带 FilingDate,
// 防前视经 valuation 生效日逻辑)。
func ttmPoints(facts []edgar.QuarterlyFact, val func(edgar.QuarterlyFact) float64) []core.EPSPoint {
	var pts []core.EPSPoint
	for i := 3; i < len(facts); i++ {
		span := facts[i].PeriodEnd.Sub(facts[i-3].PeriodEnd).Hours() / 24
		if span > maxTTMWindowDays {
			continue // 整季缺失,窗口跨缺口 → 不产点
		}
		sum := 0.0
		ok := true
		for j := i - 3; j <= i; j++ {
			v := val(facts[j])
			if math.IsNaN(v) {
				ok = false
				break
			}
			sum += v
		}
		if ok {
			pts = append(pts, core.EPSPoint{Date: facts[i].PeriodEnd,
				FilingDate: facts[i].FilingDate, EPS: sum})
		}
	}
	return pts
}

// perSharePoints 单季即点(BVPS 等 instant 派生值);缺失或零值不产点。
func perSharePoints(facts []edgar.QuarterlyFact, val func(edgar.QuarterlyFact) float64) []core.EPSPoint {
	var pts []core.EPSPoint
	for _, f := range facts {
		if v := val(f); !math.IsNaN(v) && v != 0 {
			pts = append(pts, core.EPSPoint{Date: f.PeriodEnd, FilingDate: f.FilingDate, EPS: v})
		}
	}
	return pts
}

// sharesAt 返回 period_end 对应季度的 DilutedShares,缺失/未命中返回 NaN。
func sharesAt(facts []edgar.QuarterlyFact, periodEnd time.Time) float64 {
	for _, f := range facts {
		if f.PeriodEnd.Equal(periodEnd) {
			return f.DilutedShares
		}
	}
	return math.NaN()
}

// rpsPoints 构造 RPS_TTM(营收 TTM / 当季稀释股本)阶梯,股本缺失的点置 NaN
// (交由 ReconstructPESeries 的正值门槛过滤,PB/PS 尽力而为)。
func rpsPoints(facts []edgar.QuarterlyFact) []core.EPSPoint {
	pts := ttmPoints(facts, func(f edgar.QuarterlyFact) float64 { return f.Revenue })
	for i := range pts {
		shares := sharesAt(facts, pts[i].Date)
		if math.IsNaN(shares) || shares == 0 {
			pts[i].EPS = math.NaN()
		} else {
			pts[i].EPS /= shares
		}
	}
	return pts
}

// seriesByDate 用 ReconstructPESeries 对齐并转 date→ratio 查找表;失败(科目缺失/
// 门槛不满足)返回 nil,调用方 lookup 得 NaN(整列 NaN)。
func seriesByDate(closes []core.OHLCV, pts []core.EPSPoint) map[string]float64 {
	dates, ratio, err := valuation.ReconstructPESeries(closes, pts)
	if err != nil {
		return nil
	}
	m := make(map[string]float64, len(dates))
	for i := range dates {
		m[dates[i].Format("2006-01-02")] = ratio[i]
	}
	return m
}

// refreshEdgar 全量重拉 EDGAR 季度事实,落 fundamental_q,并以 filing date 生效的
// EPS_TTM 阶梯重建 PE 序列;PB(Close/BVPS)、PS(Close/RPS_TTM)同构对齐并入行。
// 价格仍走 yahoo。CIK 空 → 报错提示配置。
// 返回的首个值是降级说明("" 表示无降级),见 upsertPrices。
func refreshEdgar(cfg config.PrismConfig, store Store, ed EdgarClient, us USClient, td TwelvedataClient,
	inst config.PrismInstrument, now time.Time) ([]string, error) {
	if inst.CIK == "" {
		return nil, fmt.Errorf("edgar source requires cik in config")
	}
	id, err := upsertMeta(store, inst)
	if err != nil {
		return nil, err
	}
	facts, err := ed.FetchCompanyFacts(inst.CIK)
	if err != nil {
		return nil, fmt.Errorf("companyfacts: %w", err)
	}
	frows := make([]prismstore.FundamentalRow, len(facts))
	for i, f := range facts {
		frows[i] = prismstore.FundamentalRow{
			FiscalPeriod: f.FiscalPeriod, PeriodEnd: f.PeriodEnd.Format("2006-01-02"),
			FilingDate: f.FilingDate.Format("2006-01-02"),
			Revenue:    f.Revenue, NetIncome: f.NetIncome, EPSDiluted: f.EPSDiluted,
			Equity: f.Equity, DilutedShares: f.DilutedShares,
			GrossProfit: f.GrossProfit, RnD: f.RnD, SGnA: f.SGnA,
			OperatingIncome: f.OperatingIncome, IncomeTax: f.IncomeTax,
			Source: "edgar",
		}
	}
	if err := store.UpsertFundamentals(id, frows); err != nil {
		return nil, err
	}

	closes, fbDeg, err := fetchCloses(us, td, inst.Symbol, now.AddDate(-cfg.EdgarLookbackYears, 0, 0), now)
	if err != nil {
		return nil, err
	}
	deg := []string{fbDeg, upsertPrices(store, id, inst.Symbol, closes)}
	epsPts := ttmPoints(facts, func(f edgar.QuarterlyFact) float64 { return f.EPSDiluted })
	dates, pe, err := valuation.ReconstructPESeries(closes, epsPts)
	if err != nil {
		return deg, fmt.Errorf("reconstruct: %w", err)
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)
	p10 := valuation.RollingPercentile(dates, pe, 10, rollingMinPoints)

	pbByDate := seriesByDate(closes, perSharePoints(facts, func(f edgar.QuarterlyFact) float64 {
		if math.IsNaN(f.Equity) || math.IsNaN(f.DilutedShares) || f.DilutedShares == 0 {
			return math.NaN()
		}
		return f.Equity / f.DilutedShares // BVPS
	}))
	psByDate := seriesByDate(closes, rpsPoints(facts))

	rows := make([]prismstore.ValuationRow, len(dates))
	for i := range dates {
		d := dates[i].Format("2006-01-02")
		rows[i] = prismstore.ValuationRow{D: d, PETTM: pe[i],
			PB: lookupRatio(pbByDate, d), PSTTM: lookupRatio(psByDate, d),
			Pctl5Y: p5[i], Pctl10Y: p10[i]}
	}
	return deg, store.UpsertValuations(id, rows)
}

// lookupRatio 返回 m[d],m 为 nil 或未命中时返回 NaN。
func lookupRatio(m map[string]float64, d string) float64 {
	if v, ok := m[d]; ok {
		return v
	}
	return math.NaN()
}
