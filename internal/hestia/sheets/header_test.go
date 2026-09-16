package sheets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0]     ResolveHeader 标签→列号（含空表头格）；TrimSpace
//                     → TestResolveHeaderMapsLabelsToColumns / TestResolveHeaderSkipsEmptyHeaderCell / TestResolveHeaderTrimsSpace
// functional[1]     ColumnLetter 0→A 25→Z 26→AA 34→AI 53→BB      → TestColumnLetter
// functional[2]     row.go Cell/Row 纯类型（verify_by: review）    → 无测试，见 row.go 注释
// functional[3]     sheets.go 包注释 C2/C7（verify_by: review）    → 无测试，见 sheets.go
// boundary[0]       C3 月份在 0/1 列互换两组夹具                    → TestResolveHeaderMapsLabelsToColumns / TestResolveHeaderFollowsReorder
// boundary[1]       写口守卫登记（verify_by: review）                → ../store_test.go TestPackageExposesNoWriteFunctions
// error_handling[0] 缺标签 ⇒ err 同时含全部缺失标签、不返回部分结果 → TestResolveHeaderRejectsMissingLabel
// non_functional[0] 不 import database/sql / 父包（verify_by: manual）→ go list -deps

func TestResolveHeaderMapsLabelsToColumns(t *testing.T) {
	got, err := ResolveHeader([]string{"月份", "发布日期", "社融存量"}, []string{"社融存量", "月份"})
	require.NoError(t, err)
	require.Equal(t, Columns{"社融存量": 2, "月份": 0}, got)
}

// TestResolveHeaderSkipsEmptyHeaderCell：DoD functional[0] 的第一组夹具——表头中间有空格子，
// 列号仍按实际位置（空格子占位不占标签）。
func TestResolveHeaderSkipsEmptyHeaderCell(t *testing.T) {
	got, err := ResolveHeader([]string{"月份", "", "社融存量"}, []string{"社融存量", "月份"})
	require.NoError(t, err)
	require.Equal(t, Columns{"社融存量": 2, "月份": 0}, got)
}

// TestResolveHeaderRejectsMissingLabel 是本包最重要的一条。
//
// 在分析表里插一列或调顺序是完全正常的动作。按固定偏移写会**静默错位**——
// 数字全在，只是每个都填错格，没人能一眼看出来，还会顺着 AJ–BB 的公式污染
// 整张表的结论。按标签解析时，同样的改动必须得到一条**拒绝执行**的报错。
func TestResolveHeaderRejectsMissingLabel(t *testing.T) {
	got, err := ResolveHeader([]string{"月份", "发布日期"}, []string{"月份", "社融存量", "M2余额"})
	require.Error(t, err)
	// 报出缺哪些，且要全报——只报第一个会让人改一次跑一次
	require.Contains(t, err.Error(), "社融存量")
	require.Contains(t, err.Error(), "M2余额")
	// 整批拒绝：不返回「月份」那一半的部分结果
	require.Nil(t, got)
}

// TestResolveHeaderFollowsReorder：顺序调换后列号必须跟着变，这是「按标签」的本义。
func TestResolveHeaderFollowsReorder(t *testing.T) {
	got, err := ResolveHeader([]string{"社融存量", "月份"}, []string{"月份", "社融存量"})
	require.NoError(t, err)
	require.Equal(t, Columns{"月份": 1, "社融存量": 0}, got)
}

// TestResolveHeaderTrimsSpace：表格里标签常带尾随空格，肉眼看不出来。
func TestResolveHeaderTrimsSpace(t *testing.T) {
	got, err := ResolveHeader([]string{" 月份", "社融存量 "}, []string{"月份", "社融存量"})
	require.NoError(t, err)
	require.Equal(t, Columns{"月份": 0, "社融存量": 1}, got)
}

func TestColumnLetter(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 34: "AI", 53: "BB"} {
		require.Equalf(t, want, ColumnLetter(i), "列号 %d", i)
	}
}
