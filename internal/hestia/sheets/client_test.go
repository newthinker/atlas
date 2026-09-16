package sheets

import (
	"context"
	"encoding/json"
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
