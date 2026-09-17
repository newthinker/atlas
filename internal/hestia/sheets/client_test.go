package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-006)
// functional[0]     NewClient/WithEndpoint/Tabs/ReadHeader(第 3 行)/ReadEntryArea(行 4–15、A–AI)/WriteCells
//                     → TestTabsListsSheetTitles / TestReadHeaderReadsRowThree / TestReadEntryAreaReadsRows4To15 / TestWriteCellsSendsRawAndOneBatch
// functional[1]     C5+C11：RAW、Data 2 条、range 'sheet'!C9 / T9、一次请求、值是 JSON 数字 → TestWriteCellsSendsRawAndOneBatch
// boundary[0]       C9 凭据留空 ⇒ (nil,nil)；range 表名无条件单引号、列字母走 ColumnLetter → TestNewClientDisabledWhenNoCredentials / TestWriteCellsQuotesSheetNameAndUsesColumnLetter
// error_handling[0] 4xx/5xx ⇒ error 含状态码；凭据文件不存在 ⇒ error → TestWriteCellsReportsHTTPStatus / TestNewClientErrorsWhenCredentialsFileMissing
// non_functional[0] 无空 Transport{}；守卫登记（review）           → TestClientDoesNotUseBareTransport + ../store_test.go

// recorder 记下服务端真正收到了什么。
//
// 🔴 用 httptest 而不是把 client 抽成 interface 再断言「Update 被调用了」。
// 那种测试永远绿，却证明不了 range 字符串对不对、valueInputOption 是不是 RAW——
// 这些错误全都发生在「调用了正确的方法之后」。
type recorder struct {
	methods []string
	bodies  []string
	// queries 记下每个请求的原始查询串。methods 只留方法与路径，而 valueRenderOption
	// 走的是查询参数——不记就断言不了「读用的是 UNFORMATTED_VALUE」（QA round2 CRITICAL-1）。
	queries []string
}

// fakeCredentials 把 testdata/fake-sa.json 复制到临时目录，并把 token_uri 指向 srv：
// 假密钥只要结构合法即可，鉴权请求打的是 httptest，不出网。
func fakeCredentials(t *testing.T, srvURL string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "fake-sa.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	doc["token_uri"] = srvURL + "/token"
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(p, out, 0o600))
	return p
}

// newTestClient 起一个真 HTTP 服务并把 Client 指过去，返回它与记下服务端收到什么的 recorder。
//
// 服务端：/token 发一枚假 access token（服务账号鉴权走这里，不出网，也**不记录**——
// 它不是 Sheets API 调用）；status 非 0 时全部 API 路径都回该状态码，用来测错误路径；
// 其余路径按 resp 查表，查不到回 {}。
func newTestClient(t *testing.T, resp map[string]string, status int) (*Client, *recorder) {
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
		if status != 0 {
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
	require.NotNil(t, c)
	return c, rec
}

// batchBody 是 values:batchUpdate 请求体里本包关心的那几个字段。
type batchBody struct {
	ValueInputOption string `json:"valueInputOption"`
	Data             []struct {
		Range  string  `json:"range"`
		Values [][]any `json:"values"`
	} `json:"data"`
}

// TestWriteCellsSendsRawAndOneBatch 断言**真实请求体**的三件事。
func TestWriteCellsSendsRawAndOneBatch(t *testing.T) {
	c, rec := newTestClient(t, nil, 0)

	err := c.WriteCells(context.Background(), []Change{
		{Sheet: "2025年", Row: 9, Col: 2, Want: 462.06},
		{Sheet: "2025年", Row: 9, Col: 19, Want: 9715.0},
	})
	require.NoError(t, err)

	// 一次请求，不是两次——回填是一次提交而不是几百次半途而废
	require.Len(t, rec.bodies, 1)
	require.Equal(t, "POST /v4/spreadsheets/sheet-id/values:batchUpdate", rec.methods[0])

	var got batchBody
	require.NoError(t, json.Unmarshal([]byte(rec.bodies[0]), &got))

	require.Equal(t, "RAW", got.ValueInputOption)
	require.Len(t, got.Data, 2)
	// range 必须带引号包住中文表名，否则 A1 记法解析不了
	require.Equal(t, "'2025年'!C9", got.Data[0].Range)
	require.Equal(t, "'2025年'!T9", got.Data[1].Range)
	// 值必须是 JSON 数字，不是字符串——字符串会让那格变成文本，公式全失效
	require.IsType(t, float64(0), got.Data[0].Values[0][0])
	require.Equal(t, 462.06, got.Data[0].Values[0][0])
}

// TestWriteCellsQuotesSheetNameAndUsesColumnLetter：表名**无条件**单引号（纯 ASCII 表名也包），
// 列字母走 003 的 ColumnLetter（AI=34、BB=53 这种两字母列不能手拼）。
func TestWriteCellsQuotesSheetNameAndUsesColumnLetter(t *testing.T) {
	c, rec := newTestClient(t, nil, 0)

	require.NoError(t, c.WriteCells(context.Background(), []Change{
		{Sheet: "Sheet1", Row: 4, Col: 0, Want: "1月"},
		{Sheet: "2026年", Row: 15, Col: 34, Want: 7.1},
	}))
	var got batchBody
	require.NoError(t, json.Unmarshal([]byte(rec.bodies[0]), &got))
	require.Equal(t, "'Sheet1'!A4", got.Data[0].Range)
	require.Equal(t, "'2026年'!AI15", got.Data[1].Range)
	require.IsType(t, "", got.Data[0].Values[0][0]) // 月份列是 string，原样发
}

// TestWriteCellsReportsHTTPStatus：4xx/5xx 不能被吞，错误里要带状态码。
func TestWriteCellsReportsHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		c, _ := newTestClient(t, nil, status)
		err := c.WriteCells(context.Background(), []Change{{Sheet: "2025年", Row: 9, Col: 2, Want: 1.0}})
		require.Error(t, err, "status %d", status)
		require.Contains(t, err.Error(), http.StatusText(status), "status %d", status)
	}
}

// TestNewClientDisabledWhenNoCredentials：C9 —— 留空 = 能力禁用，不报错。
func TestNewClientDisabledWhenNoCredentials(t *testing.T) {
	c, err := NewClient(context.Background(), "", "sheet-id")
	require.NoError(t, err)
	require.Nil(t, c, "凭据留空应返回 nil client，由调用方跳过投影而不是报错")
}

// TestNewClientErrorsWhenCredentialsFileMissing：文件不存在是配置错误，要与 C9 的「留空」区分。
func TestNewClientErrorsWhenCredentialsFileMissing(t *testing.T) {
	c, err := NewClient(context.Background(), filepath.Join(t.TempDir(), "nope.json"), "sheet-id")
	require.Error(t, err)
	require.Nil(t, c)
	require.Contains(t, err.Error(), "nope.json")
}

// TestTabsListsSheetTitles：008 判缺表用。
func TestTabsListsSheetTitles(t *testing.T) {
	c, rec := newTestClient(t, map[string]string{
		"/v4/spreadsheets/sheet-id": `{"sheets":[{"properties":{"title":"2024年"}},{"properties":{"title":"2025年"}},{"properties":{"title":"说明"}}]}`,
	}, 0)
	got, err := c.Tabs(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"2024年", "2025年", "说明"}, got)
	require.Len(t, rec.methods, 1)
	require.True(t, strings.HasPrefix(rec.methods[0], "GET /v4/spreadsheets/sheet-id"), rec.methods[0])
}

// TestReadHeaderReadsRowThree：表头在第 3 行，读 A3:AI3。
func TestReadHeaderReadsRowThree(t *testing.T) {
	c, rec := newTestClient(t, map[string]string{
		"/v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3": `{"range":"'2025年'!A3:AI3","values":[["月份","发布日期","社融存量"]]}`,
	}, 0)
	got, err := c.ReadHeader(context.Background(), "2025年")
	require.NoError(t, err)
	require.Equal(t, []string{"月份", "发布日期", "社融存量"}, got)
	require.Len(t, rec.methods, 1)
	require.Equal(t, "GET /v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3", rec.methods[0])
}

// TestReadEntryAreaReadsRows4To15：录入区行 4–15、列 A–AI（C6 读侧：不碰 AJ 之后的计算区）。
// 返回按行的 [][]any，索引 0 = 1 月；Sheets API 截掉的尾行/尾列原样保留为短行。
func TestReadEntryAreaReadsRows4To15(t *testing.T) {
	c, rec := newTestClient(t, map[string]string{
		"/v4/spreadsheets/sheet-id/values/'2025年'!A4:AI15": `{"range":"'2025年'!A4:AI15","values":[["1月","2025-02-14",412.5],["2月"]]}`,
	}, 0)
	got, err := c.ReadEntryArea(context.Background(), "2025年")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []any{"1月", "2025-02-14", 412.5}, got[0])
	require.Equal(t, []any{"2月"}, got[1])
	require.Equal(t, "GET /v4/spreadsheets/sheet-id/values/'2025年'!A4:AI15", rec.methods[0])
}

// TestClientDoesNotUseBareTransport：C7——出网走默认 transport（ProxyFromEnvironment），
// 不许照抄 fetch.go 的空 &http.Transport{}。用源码断言，因为「用了哪个 transport」在行为上
// 只有到境外才看得出来。
func TestClientDoesNotUseBareTransport(t *testing.T) {
	src, err := os.ReadFile("client.go")
	require.NoError(t, err)
	require.NotContains(t, string(src), "http.Transport{")
}

// —— TASK-006 返工（QA round2 CRITICAL-1 / CRITICAL-2）——

// TestReadUsesUnformattedValue 是 CRITICAL-1 的**根因守卫**。
//
// 🔴 pinned 模块 sheets-gen.go:11695 明写 values.get 默认 FORMATTED_VALUE ⇒ 返回格式化
// 文本（千分位、百分号、会计负数括号）。不显式要 UNFORMATTED_VALUE 的话，读回来的
// 「462.06」是字符串而库里是 float64，diff 每次都判不一致 ⇒ 幂等失效、每次 apply 全量重写。
//
// 判据落在**真实请求的查询串**上（C11：断言真实请求，不是断言 mock 被调用）。
func TestReadUsesUnformattedValue(t *testing.T) {
	c, rec := newTestClient(t, map[string]string{
		"/v4/spreadsheets/sheet-id/values/'2025年'!A4:AI15": `{"values":[["1月","2025-02-14",412.5]]}`,
		"/v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3":  `{"values":[["月份","发布日期","社融存量"]]}`,
	}, 0)

	_, err := c.ReadEntryArea(context.Background(), "2025年")
	require.NoError(t, err)
	_, err = c.ReadHeader(context.Background(), "2025年")
	require.NoError(t, err)

	require.Len(t, rec.queries, 2)
	for i, q := range rec.queries {
		require.Contains(t, q, "valueRenderOption=UNFORMATTED_VALUE",
			"第 %d 个读请求没要 UNFORMATTED_VALUE（实际查询串 %q）—— 默认 FORMATTED_VALUE 会回文本", i, q)
	}
}

// readTitle 走的是同一条坑：模板标题要拿原值做年份替换，格式化过的标题会让替换错位。
func TestCreateYearTabReadsTitleUnformatted(t *testing.T) {
	c, rec := newTestClient(t, templateResponses(), 0)
	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 3))

	var titleQueries []string
	for i, m := range rec.methods {
		if strings.Contains(m, "/values/") {
			titleQueries = append(titleQueries, rec.queries[i])
		}
	}
	require.NotEmpty(t, titleQueries, "读模板标题这一步必须发生")
	for _, q := range titleQueries {
		require.Contains(t, q, "valueRenderOption=UNFORMATTED_VALUE")
	}
}

// TestCreateYearTabClearsNewTabEntryArea 是 CRITICAL-2 的核心。
//
// 🔴 模板表与数据表是**同一张**：templateYearTab 是 "2024年"，而 tabName(2024) 也是
// "2024年"。真库 2024 年有 10 期数据 ⇒ 首次 apply 会把「模板」填满；明年 1 月跨年建表时
// 复制的就是一张带数据的表，新年度表 2–12 月带着 2024 年的数字，而库里没有对应月份的行
// **不会被 Diff 触碰**（Diff 只遍历 rows）。ingest 固定 Apply+CreateSheets ⇒ 自动触发、无告警。
//
// ⚠️ 清的是**刚复制出来的新表**（sheetId = 新表的 id），不动模板、不删任何人工数据。
func TestCreateYearTabClearsNewTabEntryArea(t *testing.T) {
	c, rec := newTestClient(t, templateResponses(), 0)
	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 3))

	bodies := writeBodies(rec)
	require.Len(t, bodies, 1, "四步 + 清空仍在同一次 batchUpdate 里")
	body := bodies[0]
	require.Contains(t, body, "updateCells", "清空录入区用 updateCells + 空 rows")

	// 新表 sheetId = 现有最大 + 1；templateResponses 里 2024年 是 12345 且为最大值
	var got batchUpdateBody
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	newID := int64(templateSheetID + 1)

	var clear *gridRange
	iDup, iClear := -1, -1
	for i, r := range got.Requests {
		switch {
		case r.DuplicateSheet != nil:
			iDup = i
		// 清空请求的特征：范围覆盖录入区整块（12 行 × 35 列）且不带 rows
		case r.UpdateCells != nil && r.UpdateCells.Range != nil &&
			r.UpdateCells.Range.EndRowIndex == 15 && len(r.UpdateCells.Rows) == 0:
			clear, iClear = r.UpdateCells.Range, i
		}
	}
	require.NotNil(t, clear, "必须有一条清空新表录入区的请求；请求体：%s", body)
	require.Equal(t, newID, clear.SheetID, "清的必须是新表，不是模板（模板 id=%d）", templateSheetID)
	require.Equal(t, int64(3), clear.StartRowIndex, "录入区首行是第 4 行（0 基 3）")
	require.Equal(t, int64(15), clear.EndRowIndex, "录入区末行是第 15 行")
	require.Equal(t, int64(0), clear.StartColumnIndex, "从 A 列起")
	require.Equal(t, int64(35), clear.EndColumnIndex, "到 AI 列止（0 基 34，半开区间 35）")

	// 清空必须排在 duplicate **之后**：顺序反了就是清模板的录入区——那才是真的删人工数据。
	require.NotEqual(t, -1, iDup)
	require.Less(t, iDup, iClear, "清空必须在复制之后，否则清的是模板")
}

// fix_items[5] 的端到端形态：**模板表里本来就有数据**时，新表的录入区仍被清空。
//
// 这正是 CRITICAL-2 的真实场景——首次 --apply 把「2024年」填满之后，它既是数据表
// 又是模板。⚠️ 受限于 httptest 替身不建模服务端状态，这里能证的是「清空请求被发出
// 且覆盖整块录入区」，证不了「服务端执行后真的空了」；后者属 spec §10 判据五，
// 要对真表跑（留给 TASK-012 的人执行项）。
func TestCreateYearTabClearsEvenWhenTemplateHasData(t *testing.T) {
	resp := templateResponses()
	// 模板的录入区塞满 12 个月的数据——首次 apply 之后 2024年 就是这个样子
	rows := make([]string, 0, 12)
	for m := 1; m <= 12; m++ {
		rows = append(rows, fmt.Sprintf(`["%d月","2024-%02d-15",%d.5]`, m, m, 400+m))
	}
	resp["/v4/spreadsheets/sheet-id/values/'2024年'!A4:AI15"] = `{"values":[` + strings.Join(rows, ",") + `]}`
	c, rec := newTestClient(t, resp, 0)

	require.NoError(t, c.CreateYearTab(context.Background(), "2024年", "2021年", 2021, 3))

	var got batchUpdateBody
	require.NoError(t, json.Unmarshal([]byte(writeBodies(rec)[0]), &got))
	cleared := false
	for _, r := range got.Requests {
		if r.UpdateCells != nil && r.UpdateCells.Range != nil &&
			r.UpdateCells.Range.EndRowIndex == 15 && len(r.UpdateCells.Rows) == 0 &&
			r.UpdateCells.Range.SheetID == int64(templateSheetID+1) {
			cleared = true
		}
	}
	require.True(t, cleared, "模板带数据时更要清空新表，否则新年度表 2–12 月会带着上一年的数字")
}

// batchUpdateBody / gridRange 只解析本轮断言要的那几个字段，不复刻整个 API schema。
type batchUpdateBody struct {
	Requests []struct {
		DuplicateSheet *struct {
			SourceSheetID int64 `json:"sourceSheetId"`
			NewSheetID    int64 `json:"newSheetId"`
		} `json:"duplicateSheet"`
		UpdateCells *struct {
			Range *gridRange        `json:"range"`
			Rows  []json.RawMessage `json:"rows"`
		} `json:"updateCells"`
	} `json:"requests"`
}

type gridRange struct {
	SheetID          int64 `json:"sheetId"`
	StartRowIndex    int64 `json:"startRowIndex"`
	EndRowIndex      int64 `json:"endRowIndex"`
	StartColumnIndex int64 `json:"startColumnIndex"`
	EndColumnIndex   int64 `json:"endColumnIndex"`
}
