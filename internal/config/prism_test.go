package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reflectTag 返回 v 中名为 field 的字段的 mapstructure 标签。
func reflectTag(v any, field string) (string, bool) {
	f, ok := reflect.TypeOf(v).FieldByName(field)
	if !ok {
		return "", false
	}
	return f.Tag.Get("mapstructure"), true
}

// Context Checkpoint: done_criteria → test mapping
// functional[0]  "零值 ApplyDefaults 后 DBPath/LookbackYears/USLookbackYears/LowPct/HighPct 默认值" → TestPrismConfigApplyDefaults
// functional[1]  "Config 含 Prism 字段;PrismInstrument 六字段 mapstructure 标签齐全"               → TestPrismConfigFieldsAndTags
// boundary[0]    "已显式赋值字段不被覆盖,未赋值字段仍补默认"                                        → TestPrismConfigApplyDefaultsKeepsExplicit

func TestPrismConfigApplyDefaults(t *testing.T) {
	c := PrismConfig{}
	c.ApplyDefaults()
	assert.Equal(t, "data/prism.db", c.DBPath)
	assert.Equal(t, 10, c.LookbackYears)
	assert.Equal(t, 5, c.USLookbackYears)
	assert.Equal(t, 10, c.EdgarLookbackYears)
	assert.Equal(t, 15.0, c.LowPct)
	assert.Equal(t, 85.0, c.HighPct)
}

func TestPrismConfigApplyDefaultsKeepsExplicit(t *testing.T) {
	c := PrismConfig{DBPath: "/tmp/x.db", LowPct: 20}
	c.ApplyDefaults()
	assert.Equal(t, "/tmp/x.db", c.DBPath)
	assert.Equal(t, 20.0, c.LowPct)
	assert.Equal(t, 85.0, c.HighPct)
}

// TestPrismConfigFieldsAndTags 校验 Config.Prism 字段存在,且 PrismInstrument
// 六个字段的 mapstructure 标签齐全(functional[1])。
func TestPrismConfigFieldsAndTags(t *testing.T) {
	var cfg Config
	cfg.Prism.Instruments = []PrismInstrument{{
		Symbol: "600519.SH", Name: "贵州茅台", Type: "stock",
		Market: "CN_A", Group: "A股公司", Source: "lixinger",
	}}
	assert.Equal(t, "600519.SH", cfg.Prism.Instruments[0].Symbol)

	prismField, ok := reflectTag(cfg, "Prism")
	assert.True(t, ok)
	assert.Equal(t, "prism", prismField)

	for field, want := range map[string]string{
		"Symbol": "symbol", "Name": "name", "Type": "type",
		"Market": "market", "Group": "group", "Source": "source",
	} {
		tag, ok := reflectTag(PrismInstrument{}, field)
		assert.True(t, ok, "PrismInstrument.%s 应有 mapstructure 标签", field)
		assert.Equal(t, want, tag, "PrismInstrument.%s", field)
	}
}

// Context Checkpoint: akshare 配置默认值与字段 → TestPrismConfigAkshareDefaults
//
//	functional[0] "零值 ApplyDefaults 后 AkshareBaseURL=http://127.0.0.1:8180" → TestPrismConfigAkshareDefaults
//	boundary[0]   "显式 AkshareBaseURL 不被默认值覆盖"                          → TestPrismConfigAkshareDefaults
//	functional[1] "PrismInstrument.FallbackSource mapstructure=fallback_source" → TestPrismInstrumentFallbackSourceTag
func TestPrismConfigAkshareDefaults(t *testing.T) {
	c := PrismConfig{}
	c.ApplyDefaults()
	assert.Equal(t, "http://127.0.0.1:8180", c.AkshareBaseURL)

	c2 := PrismConfig{AkshareBaseURL: "http://127.0.0.1:9999"}
	c2.ApplyDefaults()
	assert.Equal(t, "http://127.0.0.1:9999", c2.AkshareBaseURL, "显式值不被覆盖")
}

func TestPrismInstrumentFallbackSourceTag(t *testing.T) {
	f, ok := reflect.TypeOf(PrismInstrument{}).FieldByName("FallbackSource")
	require.True(t, ok)
	assert.Equal(t, "fallback_source", f.Tag.Get("mapstructure"))
}
