package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/newthinker/atlas/internal/hestia"
)

// HealthFunc 是 collector 每次抓取时调用的汇总函数；serve 侧包一层 hestia.HealthSummary。
type HealthFunc func(ctx context.Context) (hestia.Health, error)

// QueueFunc 读契约队列健康度（serve 侧包一层 hestia.QueueHealthOf）。nil = 不采队列指标。
type QueueFunc func() (hestia.QueueHealth, error)

// HestiaCollector 把 hestia.Health 映射成九个指标（M1.5 的 TASK-004），另加 db_up 与队列指标（TASK-003）。
//
// 抓取时现查，不缓存：hestia_runs 一天只有几行，一次三条查询是毫秒级；
// 缓存会让「serve 活着但读不到库」用陈旧值冒充健康。
//
// 时间戳类四项在表为空时**不输出**：0 是 1970 年，hours_since 立刻超阈值假红；
// 告警规则找不到指标时评估为 false（internal/alert/rules.go），正是要的行为。
// HealthSummary 出错时同样不输出事实指标，只让 collect_errors 加一。
//
// 队列指标（TASK-003）与 DB 指标各自独立采集：Collect 拆成 collectDB 与 collectQueue
// 两个方法（C2），任一侧读失败都只熄灭自己那一组，不带走另一组。每侧另输出一个 up gauge
// （本轮读成功 1 / 失败 0）：*_errors_total 是累计计数，一次瞬时失败后 serve 重启前恒 > 0、
// 恢复也不熄，告警规则因此改用 *_up == 0（2026-09-17 人类裁决）。
//
// now 只从注入的函数取，Collect 里不出现 time.Now()：hours_since 的测试值才可复现，
// 换成 time.Now 会让 hours_since 断言必红（boundary[0] 的变异判据）。
type HestiaCollector struct {
	fetch HealthFunc
	queue QueueFunc
	now   func() time.Time

	collectErrors, queueErrors prometheus.Counter

	lastRun, lastIngest, hoursSinceRun, hoursSinceIngest *prometheus.Desc
	runsTotal, blockedTotal                              *prometheus.Desc
	pendingReview, notifyFailures                        *prometheus.Desc
	dbUp, queueUp                                        *prometheus.Desc
	queueItems, queuePendingAge, queueProcessingAge      *prometheus.Desc
}

// hestiaScrapeTimeout 是单次抓取里那三条查询的上限：抓取路径不该拖住 /metrics。
const hestiaScrapeTimeout = 5 * time.Second

// allOutcomes 让 runs_total 恒输出五个序列：没出现过的 outcome 是 0，不是缺失。
var allOutcomes = []hestia.RunOutcome{
	hestia.RunNoNew, hestia.RunIngested, hestia.RunPending, hestia.RunDuplicate, hestia.RunFailed,
}

// NewHestiaCollector 组装全部指标的描述符；fetch 出错只计 collect_errors 并输出 db_up=0，
// 不输出事实指标。queue 为 nil 时不输出任何 hestia_queue_ 指标。
func NewHestiaCollector(fetch HealthFunc, queue QueueFunc, now func() time.Time) *HestiaCollector {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &HestiaCollector{
		fetch: fetch, queue: queue, now: now,
		collectErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hestia_collect_errors_total",
			Help: "Times HealthSummary failed during a scrape (serve alive but hestia db unreadable)",
		}),
		queueErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hestia_queue_errors_total",
			Help: "Times reading the hestia contract queue failed during a scrape",
		}),
		lastRun:          d("hestia_last_run_timestamp", "Unix time of the latest hestia ingest run of any outcome (heartbeat)"),
		lastIngest:       d("hestia_last_ingest_timestamp", "Unix time of the latest run that ingested or pended a period"),
		hoursSinceRun:    d("hestia_hours_since_last_run", "Hours since the latest run; alert rule input"),
		hoursSinceIngest: d("hestia_hours_since_last_ingest", "Hours since the latest ingested/pending run; alert rule input"),
		runsTotal:        d("hestia_runs_total", "Rows in hestia_runs by outcome", "outcome"),
		blockedTotal:     d("hestia_validation_blocked_total", "Pending rows by the first failed check", "check_id"),
		pendingReview:    d("hestia_pending_review", "Rows currently in hestia_pending awaiting a human decision"),
		notifyFailures:   d("hestia_notify_failures_total", "Runs whose Telegram notification failed"),

		// TASK-003：up gauge 与队列指标。
		dbUp:               d("hestia_db_up", "1 if this scrape read the hestia db, 0 if it failed; alert rule input"),
		queueUp:            d("hestia_queue_up", "1 if this scrape read the contract queue, 0 if it failed; alert rule input"),
		queueItems:         d("hestia_queue_items", "Contract queue items by state directory", "state"),
		queuePendingAge:    d("hestia_queue_pending_age_hours", "Hours the oldest pending contract has waited"),
		queueProcessingAge: d("hestia_queue_processing_age_hours", "Hours the oldest processing contract has been stuck"),
	}
}

// Describe 实现 prometheus.Collector：全部指标族一处列全（queue 为 nil 时声明而不输出，合法）。
func (c *HestiaCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.collectErrors.Desc(),
		c.lastRun, c.lastIngest, c.hoursSinceRun, c.hoursSinceIngest,
		c.runsTotal, c.blockedTotal, c.pendingReview, c.notifyFailures, c.dbUp,
		c.queueErrors.Desc(), c.queueUp, c.queueItems, c.queuePendingAge, c.queueProcessingAge,
	} {
		ch <- d
	}
}

// Collect 实现 prometheus.Collector：DB 与队列两侧各采一次。
//
// 两侧写成两个方法而不是一个函数里的两段：让「一段失败不影响另一段」是结构上的事实，
// 不靠后人记得别在中间加 return。
func (c *HestiaCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectDB(ch)
	c.collectQueue(ch)
}

// collectDB 抓取时现查一次 Health；失败时只输出 collect_errors 与 db_up=0。
func (c *HestiaCollector) collectDB(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), hestiaScrapeTimeout)
	defer cancel()

	h, err := c.fetch(ctx)
	if err != nil {
		c.collectErrors.Inc()
		c.collectErrors.Collect(ch)
		ch <- prometheus.MustNewConstMetric(c.dbUp, prometheus.GaugeValue, 0)
		return
	}
	c.collectErrors.Collect(ch)
	ch <- prometheus.MustNewConstMetric(c.dbUp, prometheus.GaugeValue, 1)

	now := c.now()
	if !h.LastRun.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.lastRun, prometheus.GaugeValue, float64(h.LastRun.Unix()))
		ch <- prometheus.MustNewConstMetric(c.hoursSinceRun, prometheus.GaugeValue, now.Sub(h.LastRun).Hours())
	}
	if !h.LastIngest.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.lastIngest, prometheus.GaugeValue, float64(h.LastIngest.Unix()))
		ch <- prometheus.MustNewConstMetric(c.hoursSinceIngest, prometheus.GaugeValue, now.Sub(h.LastIngest).Hours())
	}
	for _, o := range allOutcomes {
		ch <- prometheus.MustNewConstMetric(c.runsTotal, prometheus.CounterValue, float64(h.RunsByOutcome[o]), string(o))
	}
	for check, n := range h.BlockedByCheck {
		ch <- prometheus.MustNewConstMetric(c.blockedTotal, prometheus.CounterValue, float64(n), check)
	}
	ch <- prometheus.MustNewConstMetric(c.pendingReview, prometheus.GaugeValue, float64(h.PendingReview))
	ch <- prometheus.MustNewConstMetric(c.notifyFailures, prometheus.CounterValue, float64(h.NotifyFailures))
}

// collectQueue 读一次队列。nil ⇒ 什么都不输出；失败 ⇒ 只输出 queue_errors 与 queue_up=0
// （不输出件数：读不到时报 0 件就是用假值冒充「队列空」）；成功 ⇒ 按 ByState() 的口径逐 state
// 输出件数（缺键不补零，理由见函数内）；年龄仅在该目录非空时输出
// （0 小时会让「没有积压」与「刚放进去」同形）。
func (c *HestiaCollector) collectQueue(ch chan<- prometheus.Metric) {
	if c.queue == nil {
		return
	}
	q, err := c.queue()
	if err != nil {
		c.queueErrors.Inc()
		c.queueErrors.Collect(ch)
		ch <- prometheus.MustNewConstMetric(c.queueUp, prometheus.GaugeValue, 0)
		return
	}
	c.queueErrors.Collect(ch)
	ch <- prometheus.MustNewConstMetric(c.queueUp, prometheus.GaugeValue, 1)

	// 状态名单只有 hestia.QueueHealth.ByState() 一份（QA W-2）：这里再列一遍字面量的话，
	// hestia 侧加了状态而这里漏跟时 go test ./... 会全绿，新状态的件数静默消失——实测过。
	//
	// ByState() 对**未接线**的状态不给键（不是给 0），本 collector 照单输出，不补零：
	// 缺键 ⇒ Snapshot 没有该 <name>_<state> 键 ⇒ 告警规则找不到指标、求值为 false，不会假红；
	// 若补成 0，一个 hestia 侧尚未接线的状态会变成「确认为 0 件」的事实指标，把「不知道」
	// 说成「没有」。接线缺口应由 hestia 侧的 TestQueueHealthByStateCoversAllStates 报红，
	// 不该在这里被一个 0 掩盖。
	for state, n := range q.ByState() {
		ch <- prometheus.MustNewConstMetric(c.queueItems, prometheus.GaugeValue, float64(n), state)
	}
	now := c.now()
	if !q.OldestPending.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.queuePendingAge, prometheus.GaugeValue, now.Sub(q.OldestPending).Hours())
	}
	if !q.OldestProcessing.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.queueProcessingAge, prometheus.GaugeValue, now.Sub(q.OldestProcessing).Hours())
	}
}
