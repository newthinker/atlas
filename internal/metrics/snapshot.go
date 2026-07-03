package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// statusCodeRe matches a `status` label value that denotes an HTTP status
// class: either a 3-digit code (e.g. "500") or a "[1-5]xx" class string (the
// form RecordRequest stores via statusToString). Words like "ok" do not match.
// Both forms share a leading class digit, so bucketing keys off value[:1]
// folds them into the same <name>_<N>xx key (AD-13a).
var statusCodeRe = regexp.MustCompile(`^([1-5]xx|[1-9][0-9]{2})$`)

// Snapshot returns a flat map of every registered metric keyed by name, so
// callers (e.g. the alert evaluator) can reference metrics without touching
// prometheus types. gauge/counter series sum across labels into the base name;
// histograms expand into <name>_count and <name>_sum. Series carrying a status
// label that is a 3-digit code or a "[1-5]xx" class string additionally feed
// <name>_<class>xx keys bucketed by the leading class digit (AD-13/AD-13a) —
// the source for http_error_rate.
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

// addStatusClass adds v to <name>_<class>xx when the series carries a status
// label that statusCodeRe recognises (3-digit code or "[1-5]xx" string),
// bucketing by the leading class digit (e.g. 500, 502 and "5xx" all → _5xx).
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
