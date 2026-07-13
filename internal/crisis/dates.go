package crisis

import "time"

const (
	dateLayout = "2006-01-02"
	// timeLayout matches internal/storage/signal: fixed-width UTC RFC3339 so
	// lexicographic order on TEXT columns equals chronological order.
	timeLayout = "2006-01-02T15:04:05.000000000Z07:00"
)

// NowStamp renders t as the fixed-width UTC timestamp for fetched_at/eval_at.
func NowStamp(t time.Time) string { return t.UTC().Format(timeLayout) }

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// PrevTradingDay returns the last weekday strictly before t. Weekday ≈ trading
// day: holidays are accepted noise (design §4.3 relies on idempotency).
func PrevTradingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, -1)
	for isWeekend(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func addYears(date string, years int) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.AddDate(years, 0, 0).Format(dateLayout)
}

func addDays(date string, days int) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format(dateLayout)
}

// daysBetween returns whole calendar days from `from` to `to` (0 when equal
// or unparseable).
func daysBetween(from, to string) int {
	f, errF := time.Parse(dateLayout, from)
	t, errT := time.Parse(dateLayout, to)
	if errF != nil || errT != nil {
		return 0
	}
	return int(t.Sub(f).Hours() / 24)
}
