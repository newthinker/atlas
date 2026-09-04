package metrics

// Context Checkpoint: done_criteria → test mapping (M1.5 的 TASK-004)
// functional[0]     九族齐全、值与 Health 对应、五 outcome 恒输出、blocked{deposit_sum}=1、collect_errors=0
//                                          → TestHestiaCollector_FullOutput
// functional[1]     空表四个时间戳类指标不输出、pending_review 仍 0、runs_total 仍五序列
//                                          → TestHestiaCollector_EmptyOmitsTimestamps
// functional[1]     fetch 出错：pending_review 与 runs_total 族都缺席（S7），collect_errors 1→2
//                                          → TestHestiaCollector_ErrorEmitsOnlyCollectErrors
// functional[2]     Snapshot 里 hours_since_last_run=2、runs_total 跨标签求和=13
//                                          → TestHestiaCollector_VisibleInSnapshot
// boundary[0]       BlockedByCheck 空 map ⇒ blocked 族无序列（nil 族不解引用）
//                                          → TestHestiaCollector_EmptyOmitsTimestamps
// boundary[0]       now 只从注入取（变异「换成 time.Now」⇒ hours_since 断言必红）
//                                          → TestHestiaCollector_FullOutput、TestHestiaCollector_VisibleInSnapshot

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/newthinker/atlas/internal/hestia"
)

var fixedNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// gatherFamilies 把 collector 挂到一个干净注册表上抓一次，按名字索引。
func gatherFamilies(t *testing.T, c *HestiaCollector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]*dto.MetricFamily{}
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

// firstMetric 取某族的第一条序列；族缺席时 Fatal 而不是对 nil 切片取 [0] panic
// （boundary[0]：缺席是被测行为，测试自己不能因此崩掉）。
func firstMetric(t *testing.T, fam map[string]*dto.MetricFamily, name string) *dto.Metric {
	t.Helper()
	ms := fam[name].GetMetric()
	if len(ms) == 0 {
		t.Fatalf("metric family %s is absent or has no series", name)
	}
	return ms[0]
}

func gaugeValue(t *testing.T, fam map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	return firstMetric(t, fam, name).GetGauge().GetValue()
}

func counterValue(t *testing.T, fam map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	return firstMetric(t, fam, name).GetCounter().GetValue()
}

func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func fullHealth() hestia.Health {
	return hestia.Health{
		LastRun:    fixedNow.Add(-2 * time.Hour),
		LastIngest: fixedNow.Add(-48 * time.Hour),
		RunsByOutcome: map[hestia.RunOutcome]int{
			hestia.RunNoNew: 10, hestia.RunIngested: 2, hestia.RunPending: 1,
		},
		BlockedByCheck: map[string]int{"deposit_sum": 1},
		PendingReview:  3,
		NotifyFailures: 1,
	}
}

func fixedClock() time.Time { return fixedNow }

func fetchFullHealth(context.Context) (hestia.Health, error) { return fullHealth(), nil }

// 全量输出：九个指标族都在，值与 Health 对应；五个 outcome 恒输出（含 0）。
func TestHestiaCollector_FullOutput(t *testing.T) {
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, fixedClock))

	for _, name := range []string{
		"hestia_last_run_timestamp", "hestia_last_ingest_timestamp",
		"hestia_hours_since_last_run", "hestia_hours_since_last_ingest",
		"hestia_runs_total", "hestia_validation_blocked_total",
		"hestia_pending_review", "hestia_notify_failures_total", "hestia_collect_errors_total",
	} {
		if _, ok := fam[name]; !ok {
			t.Errorf("missing metric family %s", name)
		}
	}
	if got := gaugeValue(t, fam, "hestia_last_run_timestamp"); got != float64(fixedNow.Add(-2*time.Hour).Unix()) {
		t.Errorf("last_run_timestamp = %v", got)
	}
	if got := gaugeValue(t, fam, "hestia_last_ingest_timestamp"); got != float64(fixedNow.Add(-48*time.Hour).Unix()) {
		t.Errorf("last_ingest_timestamp = %v", got)
	}
	if got := gaugeValue(t, fam, "hestia_hours_since_last_run"); got != 2 {
		t.Errorf("hours_since_last_run = %v, want 2", got)
	}
	if got := gaugeValue(t, fam, "hestia_hours_since_last_ingest"); got != 48 {
		t.Errorf("hours_since_last_ingest = %v, want 48", got)
	}
	if got := gaugeValue(t, fam, "hestia_pending_review"); got != 3 {
		t.Errorf("pending_review = %v, want 3", got)
	}
	if got := counterValue(t, fam, "hestia_notify_failures_total"); got != 1 {
		t.Errorf("notify_failures_total = %v, want 1", got)
	}
	runs := map[string]float64{}
	for _, m := range fam["hestia_runs_total"].GetMetric() {
		runs[labelValue(m, "outcome")] = m.GetCounter().GetValue()
	}
	if len(runs) != 5 {
		t.Errorf("runs_total must emit all five outcomes, got %v", runs)
	}
	if runs["no_new"] != 10 || runs["ingested"] != 2 || runs["pending"] != 1 || runs["duplicate"] != 0 || runs["failed"] != 0 {
		t.Errorf("runs_total values = %v", runs)
	}
	blocked := fam["hestia_validation_blocked_total"].GetMetric()
	if len(blocked) != 1 || labelValue(blocked[0], "check_id") != "deposit_sum" || blocked[0].GetCounter().GetValue() != 1 {
		t.Errorf("validation_blocked_total = %v", blocked)
	}
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 0 {
		t.Errorf("collect_errors_total = %v, want 0", got)
	}
}

// 空表：四个时间戳类指标**不输出**——输出 0 会是 1970 年，hours_since 立刻超阈值假红。
// BlockedByCheck 为空 map 时 blocked 族没有序列（Gather 不会产出零序列的族）。
func TestHestiaCollector_EmptyOmitsTimestamps(t *testing.T) {
	c := NewHestiaCollector(func(context.Context) (hestia.Health, error) {
		return hestia.Health{RunsByOutcome: map[hestia.RunOutcome]int{}, BlockedByCheck: map[string]int{}}, nil
	}, fixedClock)
	fam := gatherFamilies(t, c)

	for _, name := range []string{
		"hestia_last_run_timestamp", "hestia_last_ingest_timestamp",
		"hestia_hours_since_last_run", "hestia_hours_since_last_ingest",
	} {
		if _, ok := fam[name]; ok {
			t.Errorf("%s must be omitted when the table is empty", name)
		}
	}
	if _, ok := fam["hestia_pending_review"]; !ok {
		t.Error("pending_review must still be emitted (0)")
	}
	if got := gaugeValue(t, fam, "hestia_pending_review"); got != 0 {
		t.Errorf("pending_review = %v, want 0", got)
	}
	if len(fam["hestia_runs_total"].GetMetric()) != 5 {
		t.Error("runs_total must still emit all five outcomes at 0")
	}
	if mf, ok := fam["hestia_validation_blocked_total"]; ok {
		t.Errorf("validation_blocked_total must have no series for an empty BlockedByCheck, got %v", mf.GetMetric())
	}
}

// fetch 出错：不输出任何 hestia 事实指标（不能用陈旧值冒充），只输出 collect_errors 且递增。
// reviewer S7：runs_total 也必须缺席——否则「出错仍输出五个恒 0 序列」的实现能过。
func TestHestiaCollector_ErrorEmitsOnlyCollectErrors(t *testing.T) {
	c := NewHestiaCollector(func(context.Context) (hestia.Health, error) {
		return hestia.Health{}, errors.New("db locked")
	}, fixedClock)

	fam := gatherFamilies(t, c)
	if _, ok := fam["hestia_pending_review"]; ok {
		t.Error("must not emit facts when HealthSummary fails")
	}
	if mf, ok := fam["hestia_runs_total"]; ok {
		t.Errorf("must not emit runs_total when HealthSummary fails, got %v", mf.GetMetric())
	}
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 1 {
		t.Errorf("collect_errors_total = %v, want 1", got)
	}
	fam = gatherFamilies(t, c) // 第二次抓取，同一个 collector 实例
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 2 {
		t.Errorf("collect_errors_total after 2nd gather = %v, want 2", got)
	}
}

// 告警循环走 Registry.Snapshot()：它按名字求和成单值，hours_since 必须在里面。
func TestHestiaCollector_VisibleInSnapshot(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewHestiaCollector(fetchFullHealth, fixedClock))
	snap := reg.Snapshot()
	if snap["hestia_hours_since_last_run"] != 2 {
		t.Errorf("snapshot hestia_hours_since_last_run = %v, want 2", snap["hestia_hours_since_last_run"])
	}
	if snap["hestia_runs_total"] != 13 {
		t.Errorf("snapshot sums labeled series: hestia_runs_total = %v, want 13", snap["hestia_runs_total"])
	}
}
