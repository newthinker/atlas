package metrics

// Context Checkpoint: done_criteria → test mapping (M1.5 的 TASK-004)
// functional[0]     九族齐全、值与 Health 对应、五 outcome 恒输出、blocked{deposit_sum}=1、collect_errors=0
//                                          → TestHestiaCollector_FullOutput
// functional[1]     空表四个时间戳类指标不输出、pending_review 仍 0、runs_total 仍五序列
//                                          → TestHestiaCollector_EmptyOmitsTimestamps
// functional[1]     fetch 出错：pending_review 与 runs_total 族都缺席（S7），collect_errors 1→2
//                                          → TestHestiaCollector_DBErrorEmitsOnlyCollectErrorsAndDBUp（TASK-003 改名，加 db_up=0）
// functional[2]     Snapshot 里 hours_since_last_run=2、runs_total 跨标签求和=13
//                                          → TestHestiaCollector_VisibleInSnapshot
// boundary[0]       BlockedByCheck 空 map ⇒ blocked 族无序列（nil 族不解引用）
//                                          → TestHestiaCollector_EmptyOmitsTimestamps
// boundary[0]       now 只从注入取（变异「换成 time.Now」⇒ hours_since 断言必红）
//                                          → TestHestiaCollector_FullOutput、TestHestiaCollector_VisibleInSnapshot

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0]     DB+queue 成功：items 四序列 2/1/3/1、两个 age 3/1、queue_up/db_up=1、queue_errors=0
//                                          → TestHestiaCollector_QueueFullOutput
// functional[1]     注册进 Registry 后 Snapshot 含 hestia_queue_items_failed=1、hestia_queue_up=1
//                                          → TestHestiaCollector_QueueVisibleInSnapshot
// functional[2]     Collect 只顺序调用 collectDB / collectQueue → review
// boundary[0]       四目录空：items 四序列全 0、两个 age 不输出、queue_up=1 → TestHestiaCollector_QueueEmptyOmitsAges
// boundary[1]       queue nil：无任何 hestia_queue_ 前缀指标，DB 指标与 db_up 照常 → TestHestiaCollector_QueueNilSkipsQueue
// error_handling[0] C2 双向隔离 → TestHestiaCollector_QueueFailureKeepsDBMetrics / TestHestiaCollector_DBFailureKeepsQueueMetrics
// error_handling[1] 可恢复：第二次成功 up 回 1、errors 仍 1（queue 与 DB 各一例）
//                                          → TestHestiaCollector_QueueUpRecovers / TestHestiaCollector_DBUpRecovers
// non_functional[0] PedanticRegistry 三形态 Gather 无 error → TestHestiaCollector_PedanticRegistry；
//                   go build ./... 与 go test ./internal/metrics ./cmd/atlas 由交付前实跑证明
// functional[0]+    state 标签集合 == q.ByState() 键集合（QA W-2，单一口径）
//                                            → TestHestiaCollector_QueueItemsFollowByState

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, nil, fixedClock))

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
	}, nil, fixedClock)
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

// fetch 出错：不输出任何 hestia 事实指标（不能用陈旧值冒充），只输出 collect_errors 且递增，
// 以及 hestia_db_up=0（TASK-003：原名 ErrorEmitsOnlyCollectErrors，DB 失败现在多输出 db_up）。
// reviewer S7：runs_total 也必须缺席——否则「出错仍输出五个恒 0 序列」的实现能过。
func TestHestiaCollector_DBErrorEmitsOnlyCollectErrorsAndDBUp(t *testing.T) {
	c := NewHestiaCollector(func(context.Context) (hestia.Health, error) {
		return hestia.Health{}, errors.New("db locked")
	}, nil, fixedClock)

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
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 0 {
		t.Errorf("db_up = %v, want 0", got)
	}
	fam = gatherFamilies(t, c) // 第二次抓取，同一个 collector 实例
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 2 {
		t.Errorf("collect_errors_total after 2nd gather = %v, want 2", got)
	}
}

// 告警循环走 Registry.Snapshot()：它按名字求和成单值，hours_since 必须在里面。
func TestHestiaCollector_VisibleInSnapshot(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewHestiaCollector(fetchFullHealth, nil, fixedClock))
	snap := reg.Snapshot()
	if snap["hestia_hours_since_last_run"] != 2 {
		t.Errorf("snapshot hestia_hours_since_last_run = %v, want 2", snap["hestia_hours_since_last_run"])
	}
	if snap["hestia_runs_total"] != 13 {
		t.Errorf("snapshot sums labeled series: hestia_runs_total = %v, want 13", snap["hestia_runs_total"])
	}
}

// --- TASK-003：队列指标与 up gauge ---

func fullQueue() (hestia.QueueHealth, error) {
	return hestia.QueueHealth{
		PendingCount: 2, ProcessingCount: 1, DoneCount: 3, FailedCount: 1,
		OldestPending:    fixedNow.Add(-3 * time.Hour),
		OldestProcessing: fixedNow.Add(-1 * time.Hour),
	}, nil
}

func failQueue() (hestia.QueueHealth, error) {
	return hestia.QueueHealth{}, errors.New("permission denied")
}

func failHealth(context.Context) (hestia.Health, error) {
	return hestia.Health{}, errors.New("db locked")
}

// queueItems 按 state label 取 hestia_queue_items 的各序列值。
func queueItems(fam map[string]*dto.MetricFamily) map[string]float64 {
	out := map[string]float64{}
	for _, m := range fam["hestia_queue_items"].GetMetric() {
		out[labelValue(m, "state")] = m.GetGauge().GetValue()
	}
	return out
}

func TestHestiaCollector_QueueFullOutput(t *testing.T) {
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, fullQueue, fixedClock))

	items := queueItems(fam)
	want := map[string]float64{"pending": 2, "processing": 1, "done": 3, "failed": 1}
	if len(fam["hestia_queue_items"].GetMetric()) != 4 || len(items) != 4 {
		t.Fatalf("hestia_queue_items must have exactly four series, got %v", items)
	}
	for state, v := range want {
		if items[state] != v {
			t.Errorf("queue_items{state=%q} = %v, want %v", state, items[state], v)
		}
	}
	if got := gaugeValue(t, fam, "hestia_queue_pending_age_hours"); got != 3 {
		t.Errorf("pending_age_hours = %v, want 3", got)
	}
	if got := gaugeValue(t, fam, "hestia_queue_processing_age_hours"); got != 1 {
		t.Errorf("processing_age_hours = %v, want 1", got)
	}
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 1 {
		t.Errorf("queue_up = %v, want 1", got)
	}
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 1 {
		t.Errorf("db_up = %v, want 1", got)
	}
	if got := counterValue(t, fam, "hestia_queue_errors_total"); got != 0 {
		t.Errorf("queue_errors_total = %v, want 0", got)
	}
	if got := gaugeValue(t, fam, "hestia_hours_since_last_run"); got != 2 {
		t.Errorf("DB metrics must be unaffected: hours_since_last_run = %v, want 2", got)
	}
}

// 端到端到快照：TASK-002 的 state 展开在真实指标上生效。
func TestHestiaCollector_QueueVisibleInSnapshot(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewHestiaCollector(fetchFullHealth, fullQueue, fixedClock))
	snap := reg.Snapshot()
	if snap["hestia_queue_items_failed"] != 1 {
		t.Errorf("snapshot hestia_queue_items_failed = %v, want 1", snap["hestia_queue_items_failed"])
	}
	if snap["hestia_queue_up"] != 1 {
		t.Errorf("snapshot hestia_queue_up = %v, want 1", snap["hestia_queue_up"])
	}
}

// 四个目录都空：件数照常输出 0，年龄不输出（「没有最旧那份」不是 0 小时）。
func TestHestiaCollector_QueueEmptyOmitsAges(t *testing.T) {
	c := NewHestiaCollector(fetchFullHealth, func() (hestia.QueueHealth, error) {
		return hestia.QueueHealth{}, nil
	}, fixedClock)
	fam := gatherFamilies(t, c)

	items := queueItems(fam)
	if len(items) != 4 {
		t.Fatalf("hestia_queue_items must still emit four series, got %v", items)
	}
	for state, v := range items {
		if v != 0 {
			t.Errorf("queue_items{state=%q} = %v, want 0", state, v)
		}
	}
	for _, name := range []string{"hestia_queue_pending_age_hours", "hestia_queue_processing_age_hours"} {
		if _, ok := fam[name]; ok {
			t.Errorf("%s must be omitted for an empty queue", name)
		}
	}
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 1 {
		t.Errorf("queue_up = %v, want 1", got)
	}
}

// 未配置队列：整组 hestia_queue_ 都不输出（含 up 与 errors_total），DB 侧照常。
func TestHestiaCollector_QueueNilSkipsQueue(t *testing.T) {
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, nil, fixedClock))

	for name := range fam {
		if strings.HasPrefix(name, "hestia_queue_") {
			t.Errorf("queue == nil must not emit %s", name)
		}
	}
	if got := gaugeValue(t, fam, "hestia_hours_since_last_run"); got != 2 {
		t.Errorf("hours_since_last_run = %v, want 2", got)
	}
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 1 {
		t.Errorf("db_up = %v, want 1", got)
	}
}

// C2：队列读失败不能带走 DB 指标。
func TestHestiaCollector_QueueFailureKeepsDBMetrics(t *testing.T) {
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, failQueue, fixedClock))

	if got := gaugeValue(t, fam, "hestia_last_run_timestamp"); got != float64(fixedNow.Add(-2*time.Hour).Unix()) {
		t.Errorf("last_run_timestamp = %v, DB metrics must survive a queue failure", got)
	}
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 1 {
		t.Errorf("db_up = %v, want 1", got)
	}
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 0 {
		t.Errorf("queue_up = %v, want 0", got)
	}
	if got := counterValue(t, fam, "hestia_queue_errors_total"); got != 1 {
		t.Errorf("queue_errors_total = %v, want 1", got)
	}
	for _, name := range []string{"hestia_queue_items", "hestia_queue_pending_age_hours", "hestia_queue_processing_age_hours"} {
		if _, ok := fam[name]; ok {
			t.Errorf("%s must be absent when the queue read fails (no fake zeros)", name)
		}
	}
}

// C2 反方向：DB 读失败不能带走队列指标。
func TestHestiaCollector_DBFailureKeepsQueueMetrics(t *testing.T) {
	fam := gatherFamilies(t, NewHestiaCollector(failHealth, fullQueue, fixedClock))

	if items := queueItems(fam); len(items) != 4 || items["pending"] != 2 {
		t.Errorf("queue_items must survive a DB failure, got %v", items)
	}
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 0 {
		t.Errorf("db_up = %v, want 0", got)
	}
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 1 {
		t.Errorf("collect_errors_total = %v, want 1", got)
	}
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 1 {
		t.Errorf("queue_up = %v, want 1", got)
	}
}

// up 是「本轮读成功」而非累计：失败一次后恢复要熄灭，计数器照常累计。
func TestHestiaCollector_QueueUpRecovers(t *testing.T) {
	calls := 0
	c := NewHestiaCollector(fetchFullHealth, func() (hestia.QueueHealth, error) {
		calls++
		if calls == 1 {
			return failQueue()
		}
		return fullQueue()
	}, fixedClock)

	fam := gatherFamilies(t, c)
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 0 {
		t.Errorf("1st collect queue_up = %v, want 0", got)
	}
	fam = gatherFamilies(t, c)
	if got := gaugeValue(t, fam, "hestia_queue_up"); got != 1 {
		t.Errorf("2nd collect queue_up = %v, want 1 (recovered)", got)
	}
	if got := counterValue(t, fam, "hestia_queue_errors_total"); got != 1 {
		t.Errorf("2nd collect queue_errors_total = %v, want 1 (cumulative)", got)
	}
}

func TestHestiaCollector_DBUpRecovers(t *testing.T) {
	calls := 0
	c := NewHestiaCollector(func(ctx context.Context) (hestia.Health, error) {
		calls++
		if calls == 1 {
			return failHealth(ctx)
		}
		return fullHealth(), nil
	}, nil, fixedClock)

	fam := gatherFamilies(t, c)
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 0 {
		t.Errorf("1st collect db_up = %v, want 0", got)
	}
	fam = gatherFamilies(t, c)
	if got := gaugeValue(t, fam, "hestia_db_up"); got != 1 {
		t.Errorf("2nd collect db_up = %v, want 1 (recovered)", got)
	}
	if got := counterValue(t, fam, "hestia_collect_errors_total"); got != 1 {
		t.Errorf("2nd collect collect_errors_total = %v, want 1 (cumulative)", got)
	}
}

// PedanticRegistry 校验 Collect 输出的每个指标都在 Describe 里声明过、且类型与 help 一致。
func TestHestiaCollector_PedanticRegistry(t *testing.T) {
	tests := []struct {
		name  string
		fetch HealthFunc
		queue QueueFunc
	}{
		{"success", fetchFullHealth, fullQueue},
		{"queue failure", fetchFullHealth, failQueue},
		{"db failure", failHealth, fullQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewPedanticRegistry()
			if err := reg.Register(NewHestiaCollector(tt.fetch, tt.queue, fixedClock)); err != nil {
				t.Fatalf("register: %v", err)
			}
			if _, err := reg.Gather(); err != nil {
				t.Errorf("pedantic gather: %v", err)
			}
		})
	}
}

// TestHestiaCollector_QueueItemsFollowByState 堵 QA W-2 实证的形态：hestia 侧加了第五个
// 状态、collector 不动 ⇒ 此前 go test ./... 全绿而新状态的件数静默消失。
//
// 🔴 断言的是**集合相等**而不是「包含这四个」：后者在 ByState 多出一个键时恒绿，正是要堵的那种绿。
// 期望值取自 q.ByState()（hestia 侧的单一口径），测试里不列第二份状态名字面量——列了就等于
// 把同一个失效搬进测试。
func TestHestiaCollector_QueueItemsFollowByState(t *testing.T) {
	q, err := fullQueue()
	if err != nil {
		t.Fatalf("fullQueue: %v", err)
	}
	fam := gatherFamilies(t, NewHestiaCollector(fetchFullHealth, fullQueue, fixedClock))

	got := queueItems(fam)
	want := map[string]float64{}
	for state, n := range q.ByState() {
		want[state] = float64(n)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hestia_queue_items 的 state 集合与值 = %v, want %v（须与 ByState() 逐项相等）", got, want)
	}
}
