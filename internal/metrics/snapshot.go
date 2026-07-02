package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// statusCodeRe matches a 3-digit HTTP status code (100-999) carried in a
// `status` label. Class strings like "5xx" and words like "ok" do not match.
var statusCodeRe = regexp.MustCompile(`^[1-9][0-9]{2}$`)

// Snapshot returns a flat map of every registered metric keyed by name, so
// callers (e.g. the alert evaluator) can reference metrics without touching
// prometheus types. gauge/counter series sum across labels into the base name;
// histograms expand into <name>_count and <name>_sum. Series carrying a 3-digit
// numeric `status` label additionally feed <name>_<class>xx keys bucketed by the
// hundreds digit (AD-13) — the source for http_error_rate.
func (r *Registry) Snapshot() map[string]float64 {
	return snapshot(r)
}

// snapshot does the work against any Gatherer so the error path is testable.
// Gather returns whatever it collected alongside any error; we process the
// partial families and never panic (an empty registry yields an empty map).
func snapshot(g prometheus.Gatherer) map[string]float64 {
	out := make(map[string]float64)

	mfs, _ := g.Gather()
	for _, mf := range mfs {
		name := mf.GetName()
		switch mf.GetType() {
		case dto.MetricType_GAUGE:
			for _, m := range mf.GetMetric() {
				v := m.GetGauge().GetValue()
				out[name] += v
				addStatusClass(out, name, m, v)
			}
		case dto.MetricType_COUNTER:
			for _, m := range mf.GetMetric() {
				v := m.GetCounter().GetValue()
				out[name] += v
				addStatusClass(out, name, m, v)
			}
		case dto.MetricType_HISTOGRAM:
			for _, m := range mf.GetMetric() {
				h := m.GetHistogram()
				out[name+"_count"] += float64(h.GetSampleCount())
				out[name+"_sum"] += h.GetSampleSum()
			}
		}
	}

	return out
}

// addStatusClass adds v to <name>_<class>xx when the series carries a 3-digit
// numeric `status` label, bucketing by the hundreds digit (e.g. 500/502 → _5xx).
func addStatusClass(out map[string]float64, name string, m *dto.Metric, v float64) {
	for _, lp := range m.GetLabel() {
		if lp.GetName() != "status" {
			continue
		}
		if code := lp.GetValue(); statusCodeRe.MatchString(code) {
			out[name+"_"+code[:1]+"xx"] += v
		}
		return
	}
}
