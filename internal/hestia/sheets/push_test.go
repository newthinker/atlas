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
// （补）            缺表 + CreateSheets + Apply ⇒ 建表挂点在写之前 → ⚠️ 原测试
//                   TestPushCreateSheetsHookRunsBeforeWrite **已在 TASK-008 的 b119a5a 删除**
//                   （它断言 error 含 "TASK-008"，真接线做完后按设计必然失效），
//                   由 TestPushCreatesMissingTabsBeforeWriting 接替。全仓已无此函数。
// non_functional[0] 守卫登记 sheets.Push（review）                        → ../store_test.go TestPackageExposesNoWriteFunctions
//
// Context Checkpoint: done_criteria → test mapping (TASK-008，追加在同一文件)
// functional[0]     CreateYearTab 四步：duplicateSheet / 2021年 / 2021 年 · / "index":3 → TestCreateYearTabDoesAllFourSteps / TestCreateYearTabSendsIndexZero
// functional[1]     push 接线：duplicateSheet 先于首个写数据请求；模板 2024年；index 按年序 → TestPushCreatesMissingTabsBeforeWriting / TestPushPlacesNewYearTabInOrder
// boundary[0]       CreateSheets 但不缺表 ⇒ 零 duplicateSheet；dry-run 缺表不建表 → TestPushCreateSheetsWithoutMissingTabsDuplicatesNothing / TestPushDryRunReportsMissingTabsWithCreateFlag（007 既有）
// error_handling[0] 模板不存在 ⇒ error 含模板名；duplicateSheet 4xx ⇒ 后续不做 → TestCreateYearTabErrorsWhenTemplateMissing / TestCreateYearTabStopsWhenBatchUpdateFails
// non_functional[0] 守卫登记 sheets.Client.CreateYearTab（review）；006/007 既有测试仍绿 → ../store_test.go

// —— 🔴 本文件的替身与真实系统的形状差异（QA round2 [11]）——
//
// 共同形状只有一句：**替身比真实系统仁慈——它从不失败，也从不返回意外形状。**
// 每条都真实咬过人或差点咬人，列在这里是为了让下一个加测试的人知道自己站在什么地基上。
//
//  1. **渲染形态**：替身回 JSON 数字，真 API 默认回 FORMATTED_VALUE 的**格式化文本**
//     （千分位、百分号、会计负数括号）。CRITICAL-1 就是这么漏过去的——该性质在原有
//     替身下**结构上不可观测**。现由 TestReadUsesUnformattedValue 与
//     TestDiffTreatsStringNumbersAsSame 钉住。
//  2. **写入结果**：替身曾对 values:batchUpdate 回 `{}`，真 API 回 totalUpdatedCells。
//     「200 但一格都没写」因此不可能被发现（CRITICAL-2 之后的 [8]）。现在替身从请求体
//     数 data 长度算出该字段——**不写死**，写死就等于让它永远说「我全写成功了」。
//  3. **失败层次**：替身的失败永远是 HTTP 状态码（googleapi.Error），而生产最常见的是
//     **网络层**失败（代理没起、DNS、超时），那条路径拿不到任何状态码。round2 的
//     CRITICAL-3 的生产形态走的正是后者。现由 TestWrapErrHandlesNonGoogleAPIError 覆盖。
//  4. **表结构**：替身的年度表是空的；真表的「模板」2024年 有 10 期数据，且模板与数据表
//     **是同一张**。CRITICAL-2 的自污染由此而来。
//  5. **并发与配额**：替身无速率限制、无 429、无部分成功；真 API 三者都有。目前**没有**
//     任何测试覆盖 429 与部分成功——这是已知缺口，不是已解决问题。
//  6. **服务端状态**：替身**不建模状态**——建表之后再读同一张表，读到的仍是夹具里写死的
//     那份。所以「清空录入区之后它真的空了」这类性质在本文件里**证不了**，只能证明
//     请求发出去了。真正的验证属 spec §10 判据五，要对真表跑。
//
// ⇒ 加新测试时先问：我要验的性质，会不会正好落在上面某一条的盲区里？

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
		rec.queries = append(rec.queries, r.URL.RawQuery)
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
	// fix_items[7]：原来这里写的是 `require.Len(writeBodies(rec), 1, "duplicateSheet
	// 失败后不许再发任何写请求")`——**那是恒真的**：四步在同一个 batchUpdate 里，写请求
	// 恒为 1，任何实现都满足它，包括「失败后继续写数据」的实现。
	//
	// 「失败后不再写」这条性质由**四步同批**的结构保证（API 对 batchUpdate 原子），
	// 不需要也无法由请求条数来证。这里改成能区分的断言：失败时不许出现写数据请求。
	require.Equal(t, 0, countBodiesContaining(rec, "valueInputOption"),
		"CreateYearTab 报错后不许再发写数据请求")
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

// —— TASK-006 返工（QA round2 fix_items[6][7][8]）——

// newTestClientFailPath：按**路径**注入状态码，比既有两种粒度细一档。
//
// 既有只有「全路径失败」（newTestClient 的 status）与「全 POST 失败」
// （newTestClientFailingPOST）两种，**读路径的 4xx 从未被走过** ⇒ push.go 里
// 「建表之后、写数据之前那次 diff 失败要立刻返回」这条分支零覆盖。
func newTestClientFailPath(t *testing.T, resp map[string]string, failPath map[string]int) (*Client, *recorder) {
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
		rec.queries = append(rec.queries, r.URL.RawQuery)
		rec.bodies = append(rec.bodies, string(b))
		if r.URL.Path == pathWriteValues {
			if code, ok := failPath[r.URL.Path]; ok {
				w.WriteHeader(code)
				_, _ = io.WriteString(w, `{"error":{"message":"fake failure"}}`)
				return
			}
			// 与 newTestClient 同口径：回真实形状的 totalUpdatedCells
			var req struct {
				Data []json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal(b, &req)
			fmt.Fprintf(w, `{"totalUpdatedCells":%d}`, len(req.Data))
			return
		}
		if code, ok := failPath[r.URL.Path]; ok {
			w.WriteHeader(code)
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

// fix_items[6] N7：新表 id 必须由**现有最大 id** 推，不是由模板 id 推。
//
// ⚠️ 这条我没有照抄验证者的夹具——它把模板设成 777，而它造的其余表是 10/12，
// **模板仍然是最大值**，于是「用 tplID+1 代替 maxID+1」这个变异在它那儿同样不红。
// 这里直接把性质本身钉死：让模板**不是**最大 id，两个算法就分叉了。
func TestCreateYearTabDerivesNewIDFromMaxNotTemplate(t *testing.T) {
	resp := withHeaders(map[string]string{
		pathTabs: `{"sheets":[` +
			`{"properties":{"sheetId":10,"title":"说明","index":0}},` +
			`{"properties":{"sheetId":777,"title":"2024年","index":1}},` +
			`{"properties":{"sheetId":9000,"title":"2026年","index":2}}]}`,
		pathTitle24: `{"values":[["2024 年 · 金融数据追踪"]]}`,
	}, "说明", "2024年", "2026年")
	c, rec := newTestClient(t, resp, 0)

	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 2))

	body := writeBodies(rec)[0]
	require.Contains(t, body, `"newSheetId":9001`,
		"新表 id 必须是现有最大(9000)+1；出现 778 说明它是从模板 id(777) 推的")
	require.NotContains(t, body, `"newSheetId":778`)
	require.Contains(t, body, `"sourceSheetId":777`, "复制的仍然是模板")
}

// fix_items[6] N10：连建两张时，后一张的 index 必须把前一张已插入的位置算进去。
//
// 现有 {说明, 2024年, 2026年}，缺 {2023年, 2025年}：
//
//	2023 ⇒ 第一个 >2023 的是 2024年（位置 1）⇒ index 1，插入后表序 [说明,2023年,2024年,2026年]
//	2025 ⇒ 第一个 >2025 的是 2026年（**此时**在位置 3）⇒ index 3
//
// 实现若忘了把前一张插进本地表序，第二张会算成 2——dev 首轮只测了单张，漏了这条。
func TestPushPlacesTwoNewTabsCumulatively(t *testing.T) {
	c, rec := newTestClient(t, withHeaders(templateResponses(), "2023年", "2025年"), 0)

	_, err := Push(context.Background(), c, rowsSpanning(2023, 2026), sampleLabels(),
		Options{Apply: true, CreateSheets: true})
	require.NoError(t, err)

	var indexes []int64
	for _, b := range rec.bodies {
		if !strings.Contains(b, "duplicateSheet") {
			continue
		}
		var got batchIndexBody
		require.NoError(t, json.Unmarshal([]byte(b), &got))
		for _, r := range got.Requests {
			if r.UpdateSheetProperties != nil && r.UpdateSheetProperties.Fields == "index" {
				indexes = append(indexes, r.UpdateSheetProperties.Properties.Index)
			}
		}
	}
	require.Equal(t, []int64{1, 3}, indexes,
		"第二张的 index 必须把第一张已插入的位置算进去（忘了就会是 2）")
}

type batchIndexBody struct {
	Requests []struct {
		UpdateSheetProperties *struct {
			Fields     string `json:"fields"`
			Properties struct {
				Index int64 `json:"index"`
			} `json:"properties"`
		} `json:"updateSheetProperties"`
	} `json:"requests"`
}

// fix_items[8]：建表失败必须在 WriteCells 之前返回，否则往不存在的表写。
//
// 这正是 fix_items[7] 那条恒真断言自称在守、实际没守的性质。用按路径注入的 4xx
// 让**建表之后那次读表头**失败——它落在 push.go 第 6 步与第 7 步之间。
func TestPushStopsWhenNewTabDiffFails(t *testing.T) {
	resp := withHeaders(templateResponses(), "2025年")
	c, rec := newTestClientFailPath(t, resp, map[string]int{
		"/v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3": http.StatusForbidden,
	})

	_, err := Push(context.Background(), c, rowsSpanning(2025, 2025), sampleLabels(),
		Options{Apply: true, CreateSheets: true})
	require.Error(t, err, "新表 diff 失败必须整批返回，不能往下写")
	require.Equal(t, 1, countBodiesContaining(rec, "duplicateSheet"), "建表本身发生过")
	require.Equal(t, 0, countBodiesContaining(rec, "valueInputOption"),
		"一个写数据请求都不许发——否则就是往刚建好但还没验过表头的表里写")
}

// —— TASK-008 返工（QA round2 fix_items[3]：push.go 其余零覆盖的 error 传播分支）——
//
// Context Checkpoint: fix_items → test mapping (TASK-008 review_fix 第 1 轮)
// [0] CRITICAL-2 模板自污染 —— 已在 TASK-006 的 5af1701 做掉，本任务不重做
// [1] 恒真断言替换 + push.go 第 6 步「补做 diff 失败」那条 —— 已在 5af1701 做掉
//     （TestPushStopsWhenNewTabDiffFails）；而「建表本身失败」那条由**本任务 ba6c589** 的
//     TestPushStopsWhenCreateYearTabFails 兑现。⚠️ 此处原先写成「push.go:177-179 已在
//     5af1701 做掉（TestPushStopsWhenNewTabDiffFails）」，**两处都错**，且与下方
//     TestPushStopsWhenCreateYearTabFails 上方的注释直接矛盾。变异实测（006 返工第 2 轮）：
//     让 createYearTabs 的 err 不返回 ⇒ 只红 TestPushStopsWhenCreateYearTabFails；
//     让补做 diff 的 err 不返回 ⇒ 只红 TestPushStopsWhenNewTabDiffFails。
// [2] N7 / N10 夹具         —— 已在 5af1701 做掉（TestCreateYearTabDerivesNewIDFromMaxNotTemplate /
//                              TestPushPlacesTwoNewTabsCumulatively）
// [3] 其余 error 传播分支   —— 本文件以下各条
//
// 🔴 这些分支**不是形式主义的覆盖率填空**：真实世界里最高频的失败恰是读
// （403 权限没给对、400 Unable to parse range、404 表被改名），而它们全都落在这几行上。
// 一条都没走过，意味着「读失败时会不会往下写」这件事此前从未被验证过。

// pathWriteCells 是 WriteCells 用的 values:batchUpdate（与建表的 :batchUpdate 不同路径）。
const pathWriteCells = "/v4/spreadsheets/sheet-id/values:batchUpdate"

// Tabs 失败 ⇒ 第 1 步就返回，后面什么都不做。
func TestPushStopsWhenTabsFails(t *testing.T) {
	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		pathTabs: http.StatusForbidden,
	})

	res, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.ErrorContains(t, err, "列工作表")
	require.Empty(t, res.Changes, "第 1 步就失败，不该有任何比对结果")
	require.Empty(t, writeBodies(rec), "一个写请求都不许发")
}

// 读表头失败 ⇒ 整批返回。表头是 C3 的入口，读不到就无从定位列。
func TestPushStopsWhenReadHeaderFails(t *testing.T) {
	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		pathHeader26: http.StatusBadRequest,
	})

	_, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Empty(t, writeBodies(rec), "读失败时不许往下写")
}

// 读录入区失败 ⇒ 整批返回（diffTab 的第二条错误路径，与读表头是两行）。
func TestPushStopsWhenReadEntryAreaFails(t *testing.T) {
	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		pathEntry26: http.StatusForbidden,
	})

	_, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Empty(t, writeBodies(rec), "读不到现值就写，等于拿空比对结果覆盖真表")
}

// 建表失败 ⇒ 第 6 步返回，且不进第 7 步。
//
// 与 TestPushStopsWhenNewTabDiffFails 是**相邻但不同**的两行：那条是建表**成功之后**
// 补做 diff 时失败（push.go:182-184），这条是建表本身失败（:177-179）。
func TestPushStopsWhenCreateYearTabFails(t *testing.T) {
	c, rec := newTestClientFailPath(t, withHeaders(templateResponses(), "2025年"), map[string]int{
		pathBatchUpdate: http.StatusForbidden,
	})

	_, err := Push(context.Background(), c, rowsSpanning(2025, 2025), sampleLabels(),
		Options{Apply: true, CreateSheets: true})
	require.Error(t, err)
	require.Equal(t, 0, countBodiesContaining(rec, "valueInputOption"),
		"建表失败后一个写数据请求都不许发——否则就是往不存在的表里写")
}

// 写格失败 ⇒ 错误原样带出去。写失败必须响亮，不能像投影失败那样只记日志：
// 这一层不知道调用方是 ingest（C8 允许降级）还是 CLI（人在等结果）。
func TestPushPropagatesWriteCellsFailure(t *testing.T) {
	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		pathWriteCells: http.StatusForbidden,
	})

	res, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Positive(t, res.WillWrite, "前置锚点：真的有格要写，否则 WriteCells 根本不会被调到")
	require.Equal(t, 1, countBodiesContaining(rec, "valueInputOption"), "写请求发过一次并失败了")
}

// createYearTabs 收到不是年度表名的条目 ⇒ 报错且**一个建表请求都不发**。
//
// 经 Push 到不了这里（missing 由 tabName(r.Year) 生成，恒为 "NNNN年"），所以直接调
// 包内函数。这条守的是「将来有人换了 missing 的来源」——那时这行是唯一的拦截点。
func TestCreateYearTabsRejectsNonYearTabName(t *testing.T) {
	c, rec := newTestClient(t, templateResponses(), 0)

	err := createYearTabs(context.Background(), c, []string{"说明", "2024年"}, []string{"说明表"})
	require.ErrorContains(t, err, "不是年度表名")
	require.ErrorContains(t, err, "说明表", "错误里要点名是哪一个，否则缺表一多就没法查")
	require.Empty(t, writeBodies(rec), "名字都不认识，不许动表结构")
}

// —— TASK-006 返工第 3 轮（QA [1]：钳制不能静默）——
//
// 🔴 上一轮的钳制本身对（位置、反空洞对照、三个变异全 KILLED），**未达标的是它静默**：
// `Diff` 只返回 `[]Change`，越界行被整行跳过之后，**结构上没有任何通道能把「我丢了一行」
// 告诉调用方**；而 `Diff` 的文档注释还在宣称「每行 × 每列恰产出一条…少一条就有一格
// 无声消失」——那句话逐字描述了新代码的行为，且它挂在一个导出函数上。
//
// 本轮给跳过加可观测出口：`Result.DroppedCells`。判据不是「月份越界」这个**成因**，
// 而是「期望格数 ≠ 实得格数」这个**性质**——将来若出现第二种让 Diff 少产出的成因，
// 这条同样会亮，不需要再加一个字段。
func TestPushReportsDroppedCellsWhenMonthOutOfRange(t *testing.T) {
	c, _ := newTestClient(t, tabsAndHeaderResponses(), 0)
	rows := append(sampleRows(),
		Row{Year: 2026, Month: 0, Cells: []Cell{{Label: "社融存量", Value: 1.0}}},
		Row{Year: 2026, Month: 13, Cells: []Cell{{Label: "社融存量", Value: 2.0}}},
	)

	res, err := Push(context.Background(), c, rows, sampleLabels(), Options{})
	require.NoError(t, err, "越界行不该让整批失败——它只是没被比对")

	// 2 行越界 × 4 列 = 8 格未被比对
	require.Equal(t, 8, res.DroppedCells,
		"越界行被跳过这件事必须能从 Result 看出来，否则调用方无从知道有数据没比对")

	// 🔴 反空洞：合法输入下必须恒为 0，否则上面那条会被一个「永远报非零」的实现满足
	res2, err := Push(context.Background(), c, sampleRows(), sampleLabels(), Options{})
	require.NoError(t, err)
	require.Zero(t, res2.DroppedCells, "全部行合法时不许报丢格")

	// 🔴 那条不变量现在的正确形式：三类计数之和 + 丢掉的格数 = 期望格数
	require.Equal(t, len(rows)*len(sampleLabels()),
		res.WillWrite+res.Same+res.AbsentInDB+res.DroppedCells,
		"三类之和不再等于格数——它现在等于「格数 − DroppedCells」，这正是要让人看见的")
}
