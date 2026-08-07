package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-002 Classify)
//
// functional[0]     四种 Verdict 各自触发，每种失败可单独定位
//                   → TestClassify（表驱动 + t.Run，每种 Verdict 独占一个子测试名）
// functional[1]     Verdict.String() 可读 + 未定义值的兜底分支
//                   → TestVerdictString / TestVerdictString 中的 Verdict(9) 一条
// boundary[0]       同一业务键 1 个 / 3 个版本时判定随之变化，
//                   Duplicate 与 OutOfOrder 各有独立断言
//                   → TestClassifyAcrossVersions（每个版本数一个子测试）
// boundary[1]       相邻边界（相等 vs 仅小一天）各自独立取证
//                   → TestClassifyAdjacentBoundary
// error_handling[0] Exists=false 时判定与 LatestRevision 取值无关
//                   → TestClassifyAbsentIgnoresRevision

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		state    State
		incoming string
		want     Verdict
	}{
		{"业务键首次出现", State{}, "2026-07-15", New},
		{"同键同 revision——站点迁移换了 URL", State{Exists: true, LatestRevision: "2026-07-15"}, "2026-07-15", Duplicate},
		{"同键更新的 revision——央行修订重发", State{Exists: true, LatestRevision: "2026-07-15"}, "2026-08-20", Revision},
		{"同键更旧的 revision——回填乱序", State{Exists: true, LatestRevision: "2026-07-15"}, "2020-07-10", OutOfOrder},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Classify(tc.state, tc.incoming))
		})
	}
}

// TestClassifyAcrossVersions 覆盖同一业务键累积 0 / 1 / 3 个版本时判定随之变化。
// 三个版本的场景下 LatestRevision 是其中最大者，比它旧、与它相等、比它新分别
// 落在三个独立子测试里——任一判定错都能凭子测试名单独定位。
func TestClassifyAcrossVersions(t *testing.T) {
	cases := []struct {
		name     string
		state    State
		incoming string
		want     Verdict
	}{
		{"零个版本——空库一律新增", State{}, "2020-07-10", New},
		{"一个版本——更晚的是修订", State{Exists: true, LatestRevision: "2020-07-10"}, "2021-01-12", Revision},
		{"三个版本——比最大者旧的是乱序", State{Exists: true, LatestRevision: "2026-01-15"}, "2021-01-12", OutOfOrder},
		{"三个版本——与最大者相等的是重复", State{Exists: true, LatestRevision: "2026-01-15"}, "2026-01-15", Duplicate},
		{"三个版本——比最大者新的是修订", State{Exists: true, LatestRevision: "2026-01-15"}, "2026-07-15", Revision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Classify(tc.state, tc.incoming))
		})
	}
}

// TestClassifyAdjacentBoundary 紧贴边界取证：与库中最新 revision 相等（Duplicate）
// 和仅小一天（OutOfOrder）是相邻的两个判定，各占一个子测试，不共用断言。
func TestClassifyAdjacentBoundary(t *testing.T) {
	s := State{Exists: true, LatestRevision: "2026-07-15"}
	t.Run("相等——重复", func(t *testing.T) {
		assert.Equal(t, Duplicate, Classify(s, "2026-07-15"))
	})
	t.Run("小一天——乱序", func(t *testing.T) {
		assert.Equal(t, OutOfOrder, Classify(s, "2026-07-14"))
	})
	t.Run("大一天——修订", func(t *testing.T) {
		assert.Equal(t, Revision, Classify(s, "2026-07-16"))
	})
}

// TestClassifyAbsentIgnoresRevision 守住 Classify 的早返回：业务键不存在时，
// LatestRevision 本应为空串，但即便被填成任意值（含比 incoming 更大的值），
// 判定也必须是 New——绝不能去比较 revision。
func TestClassifyAbsentIgnoresRevision(t *testing.T) {
	for _, latest := range []string{"", "2020-01-01", "2026-07-15", "9999-12-31"} {
		t.Run("LatestRevision="+latest, func(t *testing.T) {
			s := State{Exists: false, LatestRevision: latest}
			assert.Equal(t, New, Classify(s, "2026-07-15"))
		})
	}
}

func TestVerdictString(t *testing.T) {
	assert.Equal(t, "New", New.String())
	assert.Equal(t, "Duplicate", Duplicate.String())
	assert.Equal(t, "Revision", Revision.String())
	assert.Equal(t, "OutOfOrder", OutOfOrder.String())
	// 未定义值走兜底分支：日志里要看得出是哪个数，而不是空串。
	assert.Equal(t, "Verdict(9)", Verdict(9).String())
}
