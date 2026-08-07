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

// TestClassifyComparesLexicographically 钉住 Classify 的比较方式是【字典序】。
//
// 上面所有用例的 revision 都是 10 字符零填充的 ISO 日期，在那组取值上字典序与
// 时间序完全一致——于是任何按时间序比较的实现（time.Parse 后比 Before）都会
// 存活。QA 实证：把 Classify 换成时间序比较，全包 125 条无一转红。
//
// 本条改用一组【两种序不一致】的取值把假设钉住。这不是主张字典序更好，而是：
// Lookup 侧取最大值走 SQL 的 MAX()（字典序），Classify 必须用同一种序，否则
// 接缝两侧静默错位——而错位不会被任何单侧测试发现。要改成时间序，两侧必须
// 一起改；本条会强制那次讨论发生，而不是让它悄悄溜过去。
//
// 两组取值都是**合法的 RFC3339**、能被 time.Parse 解析——这是刻意的：若用
// "2026-7-15" 这种解析不了的形态，按时间序的实现会退回字典序兜底，变异反而
// 存活，这条测试就白写了。
func TestClassifyComparesLexicographically(t *testing.T) {
	cases := []struct {
		name        string
		state       State
		incoming    string
		want        Verdict
		ifTimeOrder string // 若改成时间序会得到什么——本条正是要挡住它
	}{
		{
			"不同时区写法：字典序更大，时间上更早",
			State{Exists: true, LatestRevision: "2026-07-15T05:00:00Z"},
			// 08:00+09:00 == 2026-07-14T23:00Z，时间上比库中的更早
			"2026-07-15T08:00:00+09:00",
			Revision,
			"按时间序会判 OutOfOrder",
		},
		{
			"带小数秒：字典序更小，时间上更晚",
			State{Exists: true, LatestRevision: "2026-07-15T10:00:00Z"},
			// '.'(0x2E) < 'Z'(0x5A)，故字典序上它更小；时间上它更晚
			"2026-07-15T10:00:00.500Z",
			OutOfOrder,
			"按时间序会判 Revision",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Classify(tc.state, tc.incoming),
				"Classify 必须按字典序判定（与 Lookup 侧 SQL MAX() 同序）；%s", tc.ifTimeOrder)
		})
	}
}
