package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/newthinker/atlas/internal/hestia"
)

// HealthFunc 是 collector 每次抓取时调用的汇总函数；serve 侧包一层 hestia.HealthSummary。
type HealthFunc func(ctx context.Context) (hestia.Health, error)

// HestiaCollector 把 hestia.Health 映射成九个指标（M1.5 的 TASK-004）。
//
// 抓取时现查，不缓存：hestia_runs 一天只有几行，一次三条查询是毫秒级；
// 缓存会让「serve 活着但读不到库」用陈旧值冒充健康。
//
// 时间戳类四项在表为空时**不输出**：0 是 1970 年，hours_since 立刻超阈值假红；
// 告警规则找不到指标时评估为 false（internal/alert/rules.go），正是要的行为。
// HealthSummary 出错时同样不输出事实指标，只让 collect_errors 加一。
//
// now 只从注入的函数取，Collect 里不出现 time.Now()：hours_since 的测试值才可复现，
// 换成 time.Now 会让 hours_since 断言必红（boundary[0] 的变异判据）。
type HestiaCollector struct {
	fetch HealthFunc
	now   func() time.Time

	collectErrors prometheus.Counter

	lastRun, lastIngest, hoursSinceRun, hoursSinceIngest *prometheus.Desc
	runsTotal, blockedTotal                              *prometheus.Desc
	pendingReview, notifyFailures                        *prometheus.Desc
}

// hestiaScrapeTimeout 是单次抓取里那三条查询的上限：抓取路径不该拖住 /metrics。
const hestiaScrapeTimeout = 5 * time.Second

// allOutcomes 让 runs_total 恒输出五个序列：没出现过的 outcome 是 0，不是缺失。
var allOutcomes = []hestia.RunOutcome{
	hestia.RunNoNew, hestia.RunIngested, hestia.RunPending, hestia.RunDuplicate, hestia.RunFailed,
}

// NewHestiaCollector 组装九个指标的描述符；fetch 出错只计 collect_errors，不输出事实指标。
func NewHestiaCollector(fetch HealthFunc, now func() time.Time) *HestiaCollector {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &HestiaCollector{
		fetch: fetch, now: now,
		collectErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hestia_collect_errors_total",
			Help: "Times HealthSummary failed during a scrape (serve alive but hestia db unreadable)",
		}),
		lastRun:          d("hestia_last_run_timestamp", "Unix time of the latest hestia ingest run of any outcome (heartbeat)"),
		lastIngest:       d("hestia_last_ingest_timestamp", "Unix time of the latest run that ingested or pended a period"),
		hoursSinceRun:    d("hestia_hours_since_last_run", "Hours since the latest run; alert rule input"),
		hoursSinceIngest: d("hestia_hours_since_last_ingest", "Hours since the latest ingested/pending run; alert rule input"),
		runsTotal:        d("hestia_runs_total", "Rows in hestia_runs by outcome", "outcome"),
		blockedTotal:     d("hestia_validation_blocked_total", "Pending rows by the first failed check", "check_id"),
		pendingReview:    d("hestia_pending_review", "Rows currently in hestia_pending awaiting a human decision"),
		notifyFailures:   d("hestia_notify_failures_total", "Runs whose Telegram notification failed"),
	}
}

// Describe 实现 prometheus.Collector：九个指标族一处列全。
func (c *HestiaCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.collectErrors.Desc(),
		c.lastRun, c.lastIngest, c.hoursSinceRun, c.hoursSinceIngest,
		c.runsTotal, c.blockedTotal, c.pendingReview, c.notifyFailures,
	} {
		ch <- d
	}
}

// Collect 实现 prometheus.Collector：抓取时现查一次 Health。
func (c *HestiaCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), hestiaScrapeTimeout)
	defer cancel()

	h, err := c.fetch(ctx)
	if err != nil {
		c.collectErrors.Inc()
		c.collectErrors.Collect(ch)
		return
	}
	c.collectErrors.Collect(ch)

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
