package hestia

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M2a 的 TASK-003）
// functional[0] 字段映射 / URL 拼接 / absent_fields 与 data 按 fieldOrder 序 / validation 三态 / thresholds 快照
//                                                    → TestBuildContractMapsFields
// functional[0] 修订字段与显式 SourceURL              → TestBuildContractRevisionAndExplicitURL
// functional[0] 回放 generated_by 与空数组             → TestBuildContractReplay
// functional[0] JSON 确定性 + 合法 JSON + null         → TestContractJSONIsDeterministic
// functional[0] 文件名带 period_type                   → TestContractFileName
// functional[1] Store.Current / PriorPublishedAt       → store_test.go TestStoreCurrent / TestStorePriorPublishedAt
// functional[2] 守卫 reflect 14 / AST 32               → store_test.go 两条守卫
// boundary[0]   顶层 17 键序                            → TestContractJSONTopLevelKeyOrder
// boundary[1]   PriorPublishedAt/Current 不跨 period_type → store_test.go TestStorePriorPublishedAt 子例
// error_handling[0] 关库后错误前缀                       → store_test.go TestStoreCurrentAndPriorErrorsCarryPrefix

func contractObs() Observation {
	return Observation{
		Meta: Meta{Period: "2026-08", PeriodType: "monthly", PublishedAt: "2026-09-12",
			ArticleID: "2026091412345678901", CaliberVersion: "2025-01", Extractor: "rule-monthly@v2",
			IngestedAt: "2026-09-12T15:31:02+08:00"},
		Values: map[string]float64{
			FieldM2: 356.71, FieldM1: 118.48, FieldTSFStock: 462.06,
			FieldDepositFlowMoM: 447, FieldLoanHHMLTMoM: 1486,
		},
	}
}

func contractCfg() Config {
	return Config{ConfigVersion: "2026-09-06", Signals: DefaultSignals()}
}

func f(v float64) *float64 { return &v }

// 字段映射：Meta 原样、URL 由 article_id 拼、absent_fields 与 data 按 fieldOrder 序、_mom 入 data。
func TestBuildContractMapsFields(t *testing.T) {
	rep := ValidationReport{Passed: true, Checks: []Check{
		{ID: "monetary_hierarchy", Status: CheckPassed},
		{ID: "deposit_sum", Status: CheckPassed, Value: f(0.0765)},
		{ID: "stock_continuity", Status: CheckSkipped, Reason: "no_prior_period"},
	}}
	c := BuildContract(ContractInput{Obs: contractObs(), Report: rep}, contractCfg())

	assert.Equal(t, "1.0", c.SchemaVersion)
	assert.Equal(t, "2026-08", c.Period)
	assert.Equal(t, "monthly", c.PeriodType)
	assert.Equal(t, "2026-09-12", c.PublishedAt)
	assert.Equal(t, "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2026091412345678901/index.html", c.SourceURL)
	assert.Equal(t, "2026091412345678901", c.ArticleID)
	assert.Equal(t, "2025-01", c.CaliberVersion)
	assert.False(t, c.IsRevision)
	assert.Nil(t, c.SupersedesPublishedAt)
	assert.Equal(t, "2026-09-12T15:31:02+08:00", c.ExtractedAt)
	assert.Equal(t, "rule-monthly@v2", c.Extractor)
	assert.Equal(t, "contract@v1", c.GeneratedBy)
	assert.Equal(t, map[string]string{"balance": "万亿元", "flow": "亿元", "ratio": "百分数"}, c.Units)

	// absent_fields = fieldOrder − Values 的键，且保持 fieldOrder 顺序
	assert.Len(t, c.AbsentFields, len(fieldOrder)-5)
	assert.NotContains(t, c.AbsentFields, FieldM2)
	assert.Contains(t, c.AbsentFields, FieldM0)
	assert.Equal(t, orderedSubset(fieldOrder, c.AbsentFields), c.AbsentFields, "absent_fields 必须按 fieldOrder 序")

	// data 按 fieldOrder 序，_mom 原样进
	keys := make([]string, 0, len(c.Data))
	for _, kv := range c.Data {
		keys = append(keys, kv.Key)
	}
	assert.Equal(t, orderedSubset(fieldOrder, keys), keys)
	assert.Contains(t, keys, FieldDepositFlowMoM)

	// validation 三态原样；Value nil ⇒ null
	require.Len(t, c.Validation.Checks, 3)
	assert.Nil(t, c.Validation.Checks[0].Value)
	assert.Equal(t, 0.0765, *c.Validation.Checks[1].Value)
	assert.Equal(t, "no_prior_period", c.Validation.Checks[2].Reason)

	// thresholds 快照
	assert.Equal(t, "2026-09-06", c.Thresholds.ConfigVersion)
	assert.Equal(t, "0-4", c.Thresholds.TempScale)
	assert.Equal(t, -2.0, c.Thresholds.Signals.ScissorsSink)
}

// orderedSubset 把 want 按 order 的顺序重排，供断言「保持 fieldOrder 序」。
func orderedSubset(order, want []string) []string {
	set := map[string]bool{}
	for _, w := range want {
		set[w] = true
	}
	var out []string
	for _, o := range order {
		if set[o] {
			out = append(out, o)
		}
	}
	return out
}

// 修订：is_revision 为真、supersedes 写被取代的 published_at；SourceURL 显式给了就用给的。
func TestBuildContractRevisionAndExplicitURL(t *testing.T) {
	c := BuildContract(ContractInput{
		Obs: contractObs(), Report: ValidationReport{Passed: true},
		IsRevision: true, Supersedes: "2026-09-11", SourceURL: "https://example.test/x",
	}, contractCfg())
	assert.True(t, c.IsRevision)
	require.NotNil(t, c.SupersedesPublishedAt)
	assert.Equal(t, "2026-09-11", *c.SupersedesPublishedAt)
	assert.Equal(t, "https://example.test/x", c.SourceURL)
}

// 回放：generated_by 带 /replay，checks 为空数组而不是 null。
func TestBuildContractReplay(t *testing.T) {
	c := BuildContract(ContractInput{Obs: contractObs(), Report: ValidationReport{Passed: true}, Replay: true}, contractCfg())
	assert.Equal(t, "contract@v1/replay", c.GeneratedBy)
	b, err := c.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(b), `"checks": []`)
	assert.Contains(t, string(b), `"absent_fields": [`)
}

// 同一观测两次生成逐字节相同——done/ 里的文件能与重放结果 cmp。
func TestContractJSONIsDeterministic(t *testing.T) {
	in := ContractInput{Obs: contractObs(), Report: ValidationReport{Passed: true}}
	a, err := BuildContract(in, contractCfg()).JSON()
	require.NoError(t, err)
	b, err := BuildContract(in, contractCfg()).JSON()
	require.NoError(t, err)
	assert.Equal(t, a, b)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(a, &parsed), "输出必须是合法 JSON")
	assert.Equal(t, "2026-08", parsed["period"])
	assert.Nil(t, parsed["supersedes_published_at"], "非修订写 null")
	data := parsed["data"].(map[string]any)
	assert.Equal(t, 447.0, data[FieldDepositFlowMoM])
}

func TestContractFileName(t *testing.T) {
	c := BuildContract(ContractInput{Obs: contractObs(), Report: ValidationReport{Passed: true}}, contractCfg())
	assert.Equal(t, "2026-08-monthly.json", c.FileName(), "带 period_type：12 月月报与年报不能撞名")
}

// 顶层键序守卫（spec §2.2；M2a 的 TASK-003 boundary）。Contract 是结构体，字段一旦被重排，
// MarshalIndent 会静默跟着变，而 Loom 与 done/ 里的 cmp 都依赖这个顺序。判据落在 JSON()
// 的字节上而不是反射字段序：反射序对 `json:"-"` 或嵌入字段会与输出不一致。
//
// 扫缩进恰为两空格的 `"<key>":` 行：顶层键在两空格，嵌套键在四空格及以上，data/units/
// validation/thresholds 段里的键不会混进来。
func TestContractJSONTopLevelKeyOrder(t *testing.T) {
	b, err := BuildContract(ContractInput{Obs: contractObs(), Report: ValidationReport{Passed: true}}, contractCfg()).JSON()
	require.NoError(t, err)
	var got []string
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, `  "`) {
			continue
		}
		rest := line[3:]
		end := strings.Index(rest, `":`)
		if end < 0 {
			continue
		}
		got = append(got, rest[:end])
	}
	assert.Equal(t, []string{
		"schema_version", "period", "period_type", "published_at", "source_url", "article_id",
		"caliber_version", "is_revision", "supersedes_published_at", "extracted_at", "extractor",
		"generated_by", "absent_fields", "units", "validation", "thresholds", "data",
	}, got, "契约顶层 17 键的顺序是 spec §2.2 定的，改结构体字段顺序会静默改它")
}
