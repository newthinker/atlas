package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
//
// Context Checkpoint: done_criteria → test mapping (TASK-008，追加在同一文件)
// functional[0]     CreateYearTab 四步：duplicateSheet / 2021年 / 2021 年 · / "index":3 → TestCreateYearTabDoesAllFourSteps / TestCreateYearTabSendsIndexZero
// functional[1]     push 接线：duplicateSheet 先于首个写数据请求；模板 2024年；index 按年序 → TestPushCreatesMissingTabsBeforeWriting / TestPushPlacesNewYearTabInOrder
// boundary[0]       CreateSheets 但不缺表 ⇒ 零 duplicateSheet；dry-run 缺表不建表 → TestPushCreateSheetsWithoutMissingTabsDuplicatesNothing / TestPushDryRunReportsMissingTabsWithCreateFlag（007 既有）
// error_handling[0] 模板不存在 ⇒ error 含模板名；duplicateSheet 4xx ⇒ 后续不做 → TestCreateYearTabErrorsWhenTemplateMissing / TestCreateYearTabStopsWhenBatchUpdateFails
// non_functional[0] 守卫登记 sheets.Client.CreateYearTab（review）；006/007 既有测试仍绿 → ../store_test.go

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

// —— TASK-008：缺失年度表的创建 ——

const (
	pathBatchUpdate = "/v4/spreadsheets/sheet-id:batchUpdate"
	pathTitle24     = "/v4/spreadsheets/sheet-id/values/'2024年'!A1"
	templateSheetID = 12345
)

// tabsWithIDs 造 spreadsheets.get 的响应：Tabs 与 CreateYearTab 都从这里拿标题与 sheetId。
func tabsWithIDs(titles ...string) string {
	parts := make([]string, len(titles))
	for i, t := range titles {
		id := 100 + i
		if t == "2024年" {
			id = templateSheetID
		}
		parts[i] = fmt.Sprintf(`{"properties":{"sheetId":%d,"title":"%s","index":%d}}`, id, t, i)
	}
	return `{"sheets":[` + strings.Join(parts, ",") + `]}`
}

// withHeaders 给每张表配一份齐全的表头。录入区不配 ⇒ 服务端回 {} ⇒ 读出来是空的，
// 正好是新建表的样子（录入区全空 ⇒ 它的行全是 WillWrite）。
func withHeaders(m map[string]string, tabs ...string) map[string]string {
	for _, tab := range tabs {
		m["/v4/spreadsheets/sheet-id/values/'"+tab+"'!A3:AI3"] = `{"values":[["月份","发布日期","社融存量","M2余额"]]}`
	}
	return m
}

// tabsResponses：spreadsheets.get 列出 titles（2024年 固定拿 id 12345），模板 A1 是带年份的
// 标题，每张列出的表都配好表头。
func tabsResponses(titles ...string) map[string]string {
	return withHeaders(map[string]string{
		pathTabs:    tabsWithIDs(titles...),
		pathTitle24: `{"values":[["2024 年 · 金融数据追踪"]]}`,
	}, titles...)
}

// templateResponses：表里有 说明 / 2024年（模板）/ 2026年。
func templateResponses() map[string]string {
	return tabsResponses("说明", "2024年", "2026年")
}

func indexOfBodyContaining(rec *recorder, needle string) int {
	for i, b := range rec.bodies {
		if strings.Contains(b, needle) {
			return i
		}
	}
	return -1
}

func countBodiesContaining(rec *recorder, needle string) int {
	n := 0
	for _, b := range rec.bodies {
		if strings.Contains(b, needle) {
			n++
		}
	}
	return n
}

// newTestClientFailingPOST：GET 照常按 resp 查表，任何 POST 都回 status——只让写动作失败，
// 用来证明 duplicateSheet 一失败就停，不会再发第二个写请求。
func newTestClientFailingPOST(t *testing.T, resp map[string]string, status int) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		rec.methods = append(rec.methods, r.Method+" "+r.URL.Path)
		rec.bodies = append(rec.bodies, string(b))
		if r.Method != http.MethodGet {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"fake failure"}}`)
			return
		}
		if body, ok := resp[r.URL.Path]; ok {
			_, _ = io.WriteString(w, body)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), fakeCredentials(t, srv.URL), "sheet-id", WithEndpoint(srv.URL))
	require.NoError(t, err)
	return c, rec
}

// TestCreateYearTabDoesAllFourSteps 钉住四个子步骤都发生了。
//
// 漏掉改名 ⇒ 表叫「2024年 的副本」；漏掉改标题行 ⇒ 表内第 1 行还写着
// 「2024 年 · ……」。**两处都漏的话，表格看起来就像重复了五张 2024**，
// 而每一张的数据都是对的——最难察觉的那种错。
func TestCreateYearTabDoesAllFourSteps(t *testing.T) {
	c, rec := newTestClient(t, templateResponses(), 0)

	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 3))

	joined := strings.Join(rec.bodies, "\n")
	require.Contains(t, joined, "duplicateSheet", "① 复制模板")
	require.Contains(t, joined, "2021年", "② 新表名")
	require.Contains(t, joined, "2021 年 ·", "③ 表内第 1 行标题的年份")
	require.Contains(t, joined, `"index":3`, "④ 按年序排位，别堆在末尾")
	// 四步在**一次** spreadsheets:batchUpdate 里，原子生效；复制的是模板的 sheetId
	require.Len(t, writeBodies(rec), 1)
	require.Equal(t, "POST "+pathBatchUpdate, rec.methods[len(rec.methods)-1])
	require.Contains(t, writeBodies(rec)[0], fmt.Sprintf(`"sourceSheetId":%d`, templateSheetID))
	require.NotContains(t, joined, "2024 年 ·", "标题里的模板年份必须被替换掉")
}

// TestCreateYearTabSendsIndexZero：index 0 是「排最前」，不能被 omitempty 吞掉。
func TestCreateYearTabSendsIndexZero(t *testing.T) {
	c, rec := newTestClient(t, templateResponses(), 0)
	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2019年", 2019, 0))
	require.Contains(t, writeBodies(rec)[0], `"index":0`)
}

// TestCreateYearTabErrorsWhenTemplateMissing：模板不在 ⇒ 报错带模板名，一个写请求都不发。
func TestCreateYearTabErrorsWhenTemplateMissing(t *testing.T) {
	c, rec := newTestClient(t, map[string]string{pathTabs: tabsWithIDs("说明", "2026年")}, 0)
	err := c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "2024年")
	require.Empty(t, writeBodies(rec))
}

// TestCreateYearTabStopsWhenBatchUpdateFails：batchUpdate 4xx ⇒ 报错含状态码，且只发过这一次写请求。
func TestCreateYearTabStopsWhenBatchUpdateFails(t *testing.T) {
	c, rec := newTestClientFailingPOST(t, templateResponses(), http.StatusForbidden)
	err := c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Forbidden")
	require.Len(t, writeBodies(rec), 1, "duplicateSheet 失败后不许再发任何写请求")
}

// TestPushCreatesMissingTabsBeforeWriting：建表必须在写数据之前，
// 否则 range 指向不存在的表，整批写入失败。建完的表要接着 diff 并写它的行。
func TestPushCreatesMissingTabsBeforeWriting(t *testing.T) {
	c, rec := newTestClient(t, withHeaders(templateResponses(), "2023年", "2025年"), 0)
	rows := rowsSpanning(2023, 2026) // 缺 2023年、2025年；2024年/2026年 已有

	res, err := Push(context.Background(), c, rows, sampleLabels(), Options{Apply: true, CreateSheets: true})
	require.NoError(t, err)

	iDup := indexOfBodyContaining(rec, "duplicateSheet")
	iWrite := indexOfBodyContaining(rec, "valueInputOption")
	require.NotEqual(t, -1, iDup)
	require.NotEqual(t, -1, iWrite)
	require.Less(t, iDup, iWrite, "建表必须先于写数据")
	require.Equal(t, 2, countBodiesContaining(rec, "duplicateSheet"), "缺几张建几张")
	require.Equal(t, []string{"2023年", "2025年"}, res.MissingTabs)

	// 新表的行也进了 diff 与写请求：4 行 × 4 列，每行 月份 WillWrite + 3 列 AbsentInDB
	requireInvariant(t, res, rows, sampleLabels())
	require.Equal(t, 4, res.WillWrite)
	var got batchBody
	require.NoError(t, json.Unmarshal([]byte(lastWriteBody(t, rec)), &got))
	require.Len(t, got.Data, res.WillWrite)
	require.Contains(t, strings.Join(rec.bodies, "\n"), "'2023年'!A15")
}

// TestPushPlacesNewYearTabInOrder：index = 现有年份表升序中第一个 > year 的位置。
// {2023年,2024年,2026年} + 新 2025 ⇒ index 2。
func TestPushPlacesNewYearTabInOrder(t *testing.T) {
	m := withHeaders(tabsResponses("2023年", "2024年", "2026年"), "2025年") // 2025年 是待建的那张
	c, rec := newTestClient(t, m, 0)

	_, err := Push(context.Background(), c, rowsSpanning(2025, 2025), sampleLabels(), Options{Apply: true, CreateSheets: true})
	require.NoError(t, err)
	require.Equal(t, 1, countBodiesContaining(rec, "duplicateSheet"))
	require.Contains(t, rec.bodies[indexOfBodyContaining(rec, "duplicateSheet")], `"index":2`)
}

// TestPushCreateSheetsWithoutMissingTabsDuplicatesNothing：给了 --create-sheets 但表都在 ⇒ 不碰表结构。
func TestPushCreateSheetsWithoutMissingTabsDuplicatesNothing(t *testing.T) {
	c, rec := newTestClient(t, tabsAndHeaderResponses(), 0)
	res, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true, CreateSheets: true})
	require.NoError(t, err)
	require.Empty(t, res.MissingTabs)
	require.Equal(t, 0, countBodiesContaining(rec, "duplicateSheet"))
	require.Len(t, writeBodies(rec), 1)
}
