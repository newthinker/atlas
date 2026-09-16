package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-007)
// functional[0]     Options/Result/Push 签名无 *Store；表名 "%d年"      → 编译即校验；TestPushApplyWritesOnlyWillWrite 断 range 带 '2026年'
// functional[1]     判据三：Apply=false 全 GET 且 WillWrite>0            → TestPushDryRunSendsNoWriteRequest
// functional[2]     Apply=true：len(Data)==WillWrite；不变量 rows×labels → TestPushApplyWritesOnlyWillWrite / TestPushDryRunSendsNoWriteRequest
// error_handling[0] 表头缺标签 ⇒ error 且 writeBodies 空                → TestPushRejectsUnknownLabel
// error_handling[1] 缺年度表且 !CreateSheets ⇒ error 含表名与 --create-sheets、MissingTabs 列全、零写 → TestPushRefusesMissingTabs
// （补）            缺表 + CreateSheets + dry-run ⇒ 不报错、MissingTabs 列全、全 GET → TestPushDryRunReportsMissingTabsWithCreateFlag
// （补）            缺表 + CreateSheets + Apply ⇒ 建表挂点在写之前、本任务未实现 ⇒ error 提 TASK-008、零写 → TestPushCreateSheetsHookRunsBeforeWrite
// non_functional[0] 守卫登记 sheets.Push（review）                        → ../store_test.go TestPackageExposesNoWriteFunctions

const (
	pathTabs     = "/v4/spreadsheets/sheet-id"
	pathHeader26 = "/v4/spreadsheets/sheet-id/values/'2026年'!A3:AI3"
	pathEntry26  = "/v4/spreadsheets/sheet-id/values/'2026年'!A4:AI15"
)

func sampleLabels() []string { return []string{"月份", "发布日期", "社融存量", "M2余额"} }

// sampleRows：2026 年 6 月一行，库里有 月份/发布日期/社融存量，M2余额 缺。
func sampleRows() []Row {
	return []Row{{Year: 2026, Month: 6, Cells: []Cell{
		{Label: "月份", Value: "6月"},
		{Label: "发布日期", Value: "2026-07-15"},
		{Label: "社融存量", Value: 462.06},
	}}}
}

// rowsSpanning：每年 12 月一行，只带月份一格。
func rowsSpanning(from, to int) []Row {
	var rows []Row
	for y := from; y <= to; y++ {
		rows = append(rows, Row{Year: y, Month: 12, Cells: []Cell{{Label: "月份", Value: "12月"}}})
	}
	return rows
}

// tabsAndHeaderResponses：表里只有 2026年；表头齐全；6 月行 月份/发布日期 已一致、社融存量 是旧值、M2余额 有人工值。
// ⇒ 月份 Same、发布日期 Same、社融存量 WillWrite、M2余额 AbsentInDB：三类各有、WillWrite>0。
func tabsAndHeaderResponses() map[string]string {
	return map[string]string{
		pathTabs:     `{"sheets":[{"properties":{"title":"2026年"}}]}`,
		pathHeader26: `{"values":[["月份","发布日期","社融存量","M2余额"]]}`,
		pathEntry26:  `{"values":[["1月"],["2月"],["3月"],["4月"],["5月"],["6月","2026-07-15",999.0,300.0]]}`,
	}
}

func onlyOneTabResponse() map[string]string {
	return map[string]string{pathTabs: `{"sheets":[{"properties":{"title":"2026年"}}]}`}
}

func headerMissingOneLabel() map[string]string {
	m := tabsAndHeaderResponses()
	m[pathHeader26] = `{"values":[["月份","发布日期","社融存量"]]}` // 缺 M2余额
	return m
}

// writeBodies 取服务端收到的全部非 GET 请求体：dry-run 与拒绝路径都要求它为空。
func writeBodies(rec *recorder) []string {
	var out []string
	for i, m := range rec.methods {
		if !strings.HasPrefix(m, "GET ") {
			out = append(out, rec.bodies[i])
		}
	}
	return out
}

// requireOnlyGETs 逐条断言服务端收到的每一个请求都是 GET——dry-run 的「没动表」
// 只能从服务端看，所以这条断言按请求逐条落，不是看个总数。
func requireOnlyGETs(t *testing.T, rec *recorder) {
	t.Helper()
	for _, m := range rec.methods {
		require.Truef(t, strings.HasPrefix(m, "GET "), "dry-run 发出了非 GET 请求：%s", m)
	}
}

func lastWriteBody(t *testing.T, rec *recorder) string {
	t.Helper()
	bodies := writeBodies(rec)
	require.NotEmpty(t, bodies, "没有任何写请求")
	return bodies[len(bodies)-1]
}

func requireInvariant(t *testing.T, res Result, rows []Row, labels []string) {
	t.Helper()
	require.Equal(t, len(rows)*len(labels), res.WillWrite+res.Same+res.AbsentInDB, "三类之和必须等于格数")
	require.Len(t, res.Changes, len(rows)*len(labels))
}

// TestPushDryRunSendsNoWriteRequest 是本任务唯一不可妥协的一条。
//
// 检查代码里有没有 if !opts.Apply { return } 是**自证**；检查线上有没有发出
// 写请求才是**他证**。dry-run 的全部价值就是「它真的没动表」，而这件事只能
// 从服务端看。
func TestPushDryRunSendsNoWriteRequest(t *testing.T) {
	c, rec := newTestClient(t, tabsAndHeaderResponses(), 0)

	res, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: false})
	require.NoError(t, err)
	require.Greater(t, res.WillWrite, 0, "夹具要造出至少一个待写格，否则这条测试是空跑")

	require.NotEmpty(t, rec.methods)
	requireOnlyGETs(t, rec)
	require.Empty(t, writeBodies(rec))

	requireInvariant(t, res, sampleRows(), sampleLabels())
	require.Equal(t, 1, res.WillWrite)
	require.Equal(t, 2, res.Same)
	require.Equal(t, 1, res.AbsentInDB)
	require.Empty(t, res.MissingTabs)
}

// TestPushApplyWritesOnlyWillWrite：一致的与库缺的都不能进写请求。
func TestPushApplyWritesOnlyWillWrite(t *testing.T) {
	c, rec := newTestClient(t, tabsAndHeaderResponses(), 0)

	res, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.NoError(t, err)
	requireInvariant(t, res, sampleRows(), sampleLabels())

	require.Len(t, writeBodies(rec), 1, "全部变更一次 batchUpdate")
	var got batchBody // client_test.go 里的同一个请求体形状，不另抄一份
	require.NoError(t, json.Unmarshal([]byte(lastWriteBody(t, rec)), &got))
	require.Len(t, got.Data, res.WillWrite, "写请求里的 range 数必须恰好等于 WillWrite")
	require.Equal(t, "'2026年'!C9", got.Data[0].Range) // 年度表名 "%d年"，6 月 ⇒ 第 9 行，社融存量 ⇒ C
	require.Equal(t, 462.06, got.Data[0].Values[0][0])
	require.Equal(t, "RAW", got.ValueInputOption)
}

// TestPushRefusesMissingTabs：缺表且没给 --create-sheets ⇒ 拒绝执行，一个写请求都没发。
func TestPushRefusesMissingTabs(t *testing.T) {
	c, rec := newTestClient(t, onlyOneTabResponse(), 0) // 表里只有 2026年

	res, err := Push(context.Background(), c, rowsSpanning(2019, 2026), sampleLabels(),
		Options{Apply: true, CreateSheets: false})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2019年")
	require.Contains(t, err.Error(), "--create-sheets")
	require.Equal(t, []string{"2019年", "2020年", "2021年", "2022年", "2023年", "2024年", "2025年"}, res.MissingTabs)
	require.Empty(t, writeBodies(rec))
}

// TestPushDryRunReportsMissingTabsWithCreateFlag：给了 --create-sheets 但是 dry-run ⇒ 不建表、不报错，
// 只把缺表列出来；已有表照常 diff；全程只有 GET（第 5 步在第 6 步之前）。
func TestPushDryRunReportsMissingTabsWithCreateFlag(t *testing.T) {
	c, rec := newTestClient(t, tabsAndHeaderResponses(), 0)
	rows := append(rowsSpanning(2025, 2025), sampleRows()...)

	res, err := Push(context.Background(), c, rows, sampleLabels(), Options{Apply: false, CreateSheets: true})
	require.NoError(t, err)
	require.Equal(t, []string{"2025年"}, res.MissingTabs)
	require.Equal(t, 1, res.WillWrite) // 2026 年那行的社融存量；缺表的行不在 Changes 里
	requireOnlyGETs(t, rec)
}

// TestPushCreateSheetsHookRunsBeforeWrite：Apply + CreateSheets + 缺表 ⇒ 走到建表挂点。
// 建表是 TASK-008 的活，本任务的挂点默认报错提示 TASK-008；报错时已有表的格也不写——
// 建表失败还写一半，比什么都不写更难收拾。
func TestPushCreateSheetsHookRunsBeforeWrite(t *testing.T) {
	c, rec := newTestClient(t, tabsAndHeaderResponses(), 0)
	rows := append(rowsSpanning(2025, 2025), sampleRows()...)

	_, err := Push(context.Background(), c, rows, sampleLabels(), Options{Apply: true, CreateSheets: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2025年")
	require.Contains(t, err.Error(), "TASK-008")
	require.Empty(t, writeBodies(rec), "建表挂点报错后不许再写")
}

// TestPushRejectsUnknownLabel：任一表头标签缺失 ⇒ 整批拒绝，不是跳过那一列。
func TestPushRejectsUnknownLabel(t *testing.T) {
	c, rec := newTestClient(t, headerMissingOneLabel(), 0)

	_, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "M2余额")
	require.Empty(t, writeBodies(rec), "表头不认识时一个格都不许写")
}

// TestPushNilClient：C9 关掉能力时 NewClient 返回 nil，Push 收到 nil 要明确报错，不 panic。
func TestPushNilClient(t *testing.T) {
	var res Result
	var err error
	require.NotPanics(t, func() { res, err = Push(context.Background(), nil, sampleRows(), sampleLabels(), Options{}) })
	require.Error(t, err)
	require.Empty(t, res.Changes)
}
