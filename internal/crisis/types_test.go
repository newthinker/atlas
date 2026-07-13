package crisis

// Context Checkpoint: done_criteria → test mapping (types helpers)
// functional[* 契约] severity/maxStatus/isColor 按契约排序（非色彩状态 -1，退出共振） → TestStatusHelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusHelpers(t *testing.T) {
	// severity: GREEN=0 AMBER=1 RED=2；非色彩状态 -1。
	assert.Equal(t, 0, severity(StatusGreen))
	assert.Equal(t, 1, severity(StatusAmber))
	assert.Equal(t, 2, severity(StatusRed))
	assert.Equal(t, -1, severity(StatusStale))
	assert.Equal(t, -1, severity(StatusNoData))

	// maxStatus 取严重度更高者；非色彩不会胜出。
	assert.Equal(t, StatusRed, maxStatus(StatusAmber, StatusRed))
	assert.Equal(t, StatusAmber, maxStatus(StatusAmber, StatusGreen))
	assert.Equal(t, StatusGreen, maxStatus(StatusGreen, StatusStale))

	// isColor 仅三色为真。
	assert.True(t, isColor(StatusGreen))
	assert.True(t, isColor(StatusRed))
	assert.False(t, isColor(StatusStale))
	assert.False(t, isColor(StatusSuppressed))
}
