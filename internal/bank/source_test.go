package bank

// Context Checkpoint: done_criteria → test mapping
// functional[0]     httptest 回放样本：路径/query、6 期升序、最新期取值避开三个诱饵、MissingFields 空、尾斜杠 → TestEMSourceFetchParsesFixture
// boundary[0]       字符串数值/null/缺键/坏日期行跳过/MissingFields 只列全缺键且按固定顺序         → TestParseEMRowsTolerant, TestParseEMRowsMissingFieldsOrder
// boundary[1]       全 null 列不算缺失、值全为 NaN                                             → TestParseEMRowsAllNullColumnNotMissing
// boundary[2]       非数值（"-" "" "Inf" "-Infinity" "NaN" 布尔 对象）⇒ NaN                     → TestToFloatNonNumericIsNaN
// error_handling[0] HTTP 500 / [] / null / 全坏日期 / 非 JSON / 连接失败；错误含代码且为合法 UTF-8 → TestEMSourceFetchErrors, TestEMSourceFetchConnectionRefused, TestEMSourceFetchErrorBodyValidUTF8
// non_functional[0] 超时 60s                                                                  → TestNewEMSourceTimeout（字段名/禁用字段 grep 见 discovery）
// non_functional[1] go vet / go test / 覆盖率                                                → 命令行核验

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSymbol = "600036.SH"

func TestEMSourceFetchParsesFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/em_600036_sample.json")
	require.NoError(t, err)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assert.Equal(t, testSymbol, r.URL.Query().Get("symbol"))
		assert.Equal(t, "按报告期", r.URL.Query().Get("indicator"))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s, err := NewEMSource(srv.URL + "/").Fetch(testSymbol)
	require.NoError(t, err)
	assert.Equal(t, "/api/public/stock_financial_analysis_indicator_em", gotPath, "尾斜杠不产生 //api")
	require.Len(t, s.Obs, 6)
	for i := 1; i < len(s.Obs); i++ {
		assert.True(t, s.Obs[i-1].Period.Before(s.Obs[i].Period), "升序 @%d", i)
	}
	assert.Equal(t, day("2025-03-31"), s.Obs[0].Period)
	last := s.Obs[5]
	assert.Equal(t, day("2026-06-30"), last.Period)
	assert.Equal(t, 0.94, last.Values[IndNPL])
	assert.Equal(t, 385.1, last.Values[IndCoverage], "取 BLDKBBL 而非拨贷比 LOAN_PROVISION_RATIO")
	assert.Equal(t, 14.07, last.Values[IndCET1], "取 HXYJBCZL 而非 NEWCAPITALADER / FIRST_ADEQUACY_RATIO")
	assert.Empty(t, s.MissingFields)
}

func TestParseEMRowsTolerant(t *testing.T) {
	rows := []map[string]any{
		{"REPORT_DATE": "2026-06-30 00:00:00", "NONPERLOAN": "1.23", "BLDKBBL": nil},
		{"REPORT_DATE": "bad", "NONPERLOAN": 9.9},
		{"REPORT_DATE": "2026-02-30 00:00:00", "NONPERLOAN": 9.9}, // 长度够但日期非法
		{"REPORT_DATE": "2026-03-31 00:00:00", "NONPERLOAN": 1.2, "BLDKBBL": 200.0},
	}
	s, err := parseEMRows(rows)
	require.NoError(t, err)
	require.Len(t, s.Obs, 2, "日期异常的行跳过")
	assert.Equal(t, day("2026-03-31"), s.Obs[0].Period)
	assert.Equal(t, 1.2, s.Obs[0].Values[IndNPL])
	assert.Equal(t, 200.0, s.Obs[0].Values[IndCoverage])
	assert.Equal(t, 1.23, s.Obs[1].Values[IndNPL], "字符串数值可解析")
	assert.True(t, math.IsNaN(s.Obs[1].Values[IndCoverage]), "null → NaN")
	assert.True(t, math.IsNaN(s.Obs[1].Values[IndCET1]), "缺键 → NaN")
	assert.Equal(t, []string{"HXYJBCZL"}, s.MissingFields, "所有行都缺的键才算字段缺失；null 不算")
}

func TestParseEMRowsMissingFieldsOrder(t *testing.T) {
	s, err := parseEMRows([]map[string]any{{"REPORT_DATE": "2026-06-30 00:00:00"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"NONPERLOAN", "BLDKBBL", "HXYJBCZL"}, s.MissingFields)
}

func TestParseEMRowsAllNullColumnNotMissing(t *testing.T) {
	rows := []map[string]any{
		{"REPORT_DATE": "2026-06-30 00:00:00", "NONPERLOAN": 0.9, "BLDKBBL": 300.0, "HXYJBCZL": nil},
		{"REPORT_DATE": "2026-03-31 00:00:00", "NONPERLOAN": 0.9, "BLDKBBL": 300.0, "HXYJBCZL": nil},
	}
	s, err := parseEMRows(rows)
	require.NoError(t, err)
	assert.Empty(t, s.MissingFields, "键在每行都存在、只是值为 null，不算字段缺失")
	for _, o := range s.Obs {
		assert.True(t, math.IsNaN(o.Values[IndCET1]))
	}
}

func TestToFloatNonNumericIsNaN(t *testing.T) {
	cases := map[string]any{
		"横线": "-", "空串": "", "Inf": "Inf", "-Infinity": "-Infinity", "NaN串": "NaN",
		"布尔": true, "对象": map[string]any{"v": 1.0}, "nil": nil,
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			assert.True(t, math.IsNaN(toFloat(v)), "%#v ⇒ NaN", v)
		})
	}
	assert.Equal(t, 1.23, toFloat("1.23"))
	assert.Equal(t, 0.0, toFloat(0.0), "真 0 仍是 0")
}

func TestEMSourceFetchErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"非200":  {http.StatusInternalServerError, "boom", "HTTP 500"},
		"空数组":   {http.StatusOK, "[]", "无数据"},
		"null":  {http.StatusOK, "null", "无数据"},
		"无有效期":  {http.StatusOK, `[{"REPORT_DATE": "x"}, {"REPORT_DATE": null}, {"REPORT_DATE": "2026-13-01 00:00:00"}]`, "无可解析报告期"},
		"非JSON": {http.StatusOK, "<html>", "decode"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := NewEMSource(srv.URL).Fetch(testSymbol)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), testSymbol, "错误带代码，报告里可定位")
			assert.True(t, utf8.ValidString(err.Error()))
		})
	}
}

func TestEMSourceFetchConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := NewEMSource(url).Fetch(testSymbol)
	require.Error(t, err)
	assert.Contains(t, err.Error(), testSymbol)
}

func TestEMSourceFetchErrorBodyValidUTF8(t *testing.T) {
	// 3 字节汉字 × 100 = 300 字节；截断点若按字节落在字符中间，会切坏最后一个字。
	body := strings.Repeat("错", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	_, err := NewEMSource(srv.URL).Fetch(testSymbol)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
	assert.True(t, utf8.ValidString(err.Error()), "截断后的错误体须是合法 UTF-8: %q", err.Error())
	assert.Less(t, len(err.Error()), len(body), "错误体被截断")
}

func TestNewEMSourceTimeout(t *testing.T) {
	assert.Equal(t, 60*time.Second, NewEMSource("http://x").hc.Timeout)
}
