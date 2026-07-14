package crisis

// Context Checkpoint: done_criteria → test mapping (dates)
// functional[0] PrevTradingDay 周内日近似 (周一/周日→上周五, 周三→周二) → TestPrevTradingDay
// functional[0] NowStamp 固定宽度 UTC RFC3339 / addYears/addDays/daysBetween → TestDateHelpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPrevTradingDay(t *testing.T) {
	mon := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC) // 周一
	assert.Equal(t, "2026-07-10", PrevTradingDay(mon).Format("2006-01-02"))
	sun := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC) // 周日
	assert.Equal(t, "2026-07-10", PrevTradingDay(sun).Format("2006-01-02"))
	wed := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC) // 周三
	assert.Equal(t, "2026-07-07", PrevTradingDay(wed).Format("2006-01-02"))
}

func TestDateHelpers(t *testing.T) {
	assert.Equal(t, 2, daysBetween("2026-07-01", "2026-07-03"))
	assert.Equal(t, 0, daysBetween("2026-07-03", "2026-07-03"))
	assert.Equal(t, "2021-07-13", addYears("2026-07-13", -5))
	assert.Equal(t, "2026-05-29", addDays("2026-07-13", -45))
	assert.Equal(t, "2026-07-13T02:03:04.000000000Z",
		NowStamp(time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)))
}
