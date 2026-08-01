package sankey

// Context Checkpoint: done_criteria → test mapping
// functional[0] 合法模板解析出 company/cik/segment_axis/version 与完整 segments 列表（逐字段断言） → TestLoadTemplatesValid
// functional[1] 校验分支：key 重复 / xbrl_member 为空 / company 为空 → error                     → TestLoadTemplatesValidation
// functional[1](a) segments[].key 含大写字符 → 加载期 error                                      → TestLoadTemplatesValidation/upper_case_segment_key
// functional[1](b) 两个模板文件声明同一 company → 加载期 error（含两个文件名）                    → TestLoadTemplatesDuplicateCompany
// functional[2] AD-16 三种目录情况区分（不存在 / 无 yaml / 非法 YAML 含文件名）                   → TestLoadTemplatesDirCases
// functional[3] LoadManualSegments 两层 map / 文件不存在空 map / fiscal_period 键格式校验         → TestLoadManualSegments, TestLoadManualSegmentsBadPeriodKey
// functional[3](a) YAML 语法错误 / schema 与两层 map 不匹配 → error（含文件名）                    → TestLoadManualSegmentsUnparsableFile
// functional[4] 仓库真实模板冒烟（configs/prism/templates 全量 + msft 三个 xbrl_member）          → TestLoadTemplatesFromRepoDir
// boundary[0]   返回 map 的 key 统一为大写 symbol；cik 非空且纯数字                               → TestLoadTemplatesValid, TestLoadTemplatesValidation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTemplatesValid(t *testing.T) {
	got, err := LoadTemplates(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	require.Len(t, got, 1)

	// boundary[0]: 文件里 company 是小写 acme，返回 map 的 key 必须是大写 symbol。
	tmpl, ok := got["ACME"]
	require.True(t, ok, "map key should be the upper-cased symbol, got %v", keysOf(got))

	assert.Equal(t, "acme", tmpl.Company)
	assert.Equal(t, "0000123456", tmpl.CIK)
	assert.Equal(t, "StatementBusinessSegmentsAxis", tmpl.SegmentAxis)
	assert.Equal(t, 2, tmpl.Version)
	assert.Equal(t, 2020, tmpl.SinceFY)

	require.Len(t, tmpl.Segments, 2, "viper 必须能把 YAML 列表映射成 []Segment 且保序")
	assert.Equal(t, Segment{Key: "cloud", NameZH: "云业务", NameEN: "Cloud", Member: "CloudMember"}, tmpl.Segments[0])
	assert.Equal(t, Segment{Key: "devices", NameZH: "硬件设备", NameEN: "Devices", Member: "DevicesMember"}, tmpl.Segments[1])
}

func TestLoadTemplatesSegmentAxisDefault(t *testing.T) {
	dir := writeTemplate(t, "noaxis.yaml", `
company: NOAXIS
cik: "42"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`)
	got, err := LoadTemplates(dir)
	require.NoError(t, err)
	require.Contains(t, got, "NOAXIS")
	assert.Equal(t, DefaultSegmentAxis, got["NOAXIS"].SegmentAxis)
}

func TestLoadTemplatesValidation(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "duplicate segment key",
			yaml: `
company: A
cik: "1"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
  - {key: cloud, name_zh: 云二, name_en: Cloud2, xbrl_member: CloudTwoMember}
`,
			wantErr: "duplicate segment key",
		},
		{
			name: "duplicate xbrl_member",
			yaml: `
company: A
cik: "1"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
  - {key: other, name_zh: 其他, name_en: Other, xbrl_member: CloudMember}
`,
			wantErr: "duplicate xbrl_member",
		},
		{
			name: "empty xbrl_member",
			yaml: `
company: A
cik: "1"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: ""}
`,
			wantErr: "xbrl_member",
		},
		{
			name: "missing xbrl_member field",
			yaml: `
company: A
cik: "1"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud}
`,
			wantErr: "xbrl_member",
		},
		{
			name: "empty segment key",
			yaml: `
company: A
cik: "1"
segments:
  - {key: "", name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`,
			wantErr: "key",
		},
		{
			name: "empty company",
			yaml: `
cik: "1"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`,
			wantErr: "company",
		},
		// 2026-08-01 语义变更:CIK 允许为空(A 股 akshare 模板无 CIK,EDGAR 路径
		// 在编排层守卫)——原「empty cik 拒绝」用例由 TestTemplateEmptyCIKAllowed 取代。
		{
			name: "non numeric cik",
			yaml: `
company: A
cik: "78901X"
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`,
			wantErr: "cik",
		},
		{
			// viper 把 LoadManualSegments 的 map key 小写化，大写模板 key 只会静默查出 0 条，
			// 必须在加载期就报错。
			name: "upper case segment key",
			yaml: `
company: A
cik: "1"
segments:
  - {key: Cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`,
			wantErr: "lower case",
		},
		{
			name: "segments is not a list",
			yaml: `
company: A
cik: "1"
segments: notalist
`,
			wantErr: "parsing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeTemplate(t, "case.yaml", tc.yaml)
			got, err := LoadTemplates(dir)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Contains(t, err.Error(), "case.yaml", "错误信息必须指明出错的文件")
		})
	}
}

// AD-16：三种目录情况必须区分，非法 YAML 不得静默吞成空 map（否则 sankey 全站 404 且日志无痕）。
func TestLoadTemplatesDirCases(t *testing.T) {
	t.Run("dir missing", func(t *testing.T) {
		got, err := LoadTemplates(filepath.Join(t.TempDir(), "nope"))
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got, "未配置模板是合法状态，应返回可用的空 map")
	})

	t.Run("dir without yaml", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("no templates here"), 0o644))
		got, err := LoadTemplates(dir)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got, "未配置模板是合法状态，应返回可用的空 map")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		got, err := LoadTemplates(filepath.Join("testdata", "bad_yaml"))
		require.Error(t, err, "非法 YAML 必须报错，不得静默返回空 map")
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "broken.yaml", "错误信息必须含出错的文件名")
	})

	t.Run("invalid yaml among valid ones", func(t *testing.T) {
		dir := t.TempDir()
		copyFile(t, filepath.Join("testdata", "valid", "valid.yaml"), filepath.Join(dir, "valid.yaml"))
		copyFile(t, filepath.Join("testdata", "bad_yaml", "broken.yaml"), filepath.Join(dir, "broken.yaml"))
		got, err := LoadTemplates(dir)
		require.Error(t, err)
		assert.Nil(t, got, "一个文件坏掉即整体失败，不得返回部分结果掩盖故障")
		assert.Contains(t, err.Error(), "broken.yaml")
	})

	t.Run("duplicate key file", func(t *testing.T) {
		got, err := LoadTemplates(filepath.Join("testdata", "dup_key"))
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "dup_key.yaml")
	})
}

// 两个文件声明同一 company 时，后加载者会静默覆盖先加载者，赢家取决于 os.ReadDir 顺序
// （跨平台不保证稳定）——必须在加载期报错，且错误信息要同时点出两个文件，否则用户还得
// 自己翻目录找另一半。
func TestLoadTemplatesDuplicateCompany(t *testing.T) {
	got, err := LoadTemplates(filepath.Join("testdata", "dup_company"))
	require.Error(t, err, "同名 company 不得静默覆盖")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "company")
	// 大小写不同（DUPCO / dupco）也算撞车：返回 map 的 key 统一大写。
	assert.Contains(t, err.Error(), "DUPCO")
	assert.Contains(t, err.Error(), "a.yaml", "错误必须点出两个冲突文件之一")
	assert.Contains(t, err.Error(), "b.yaml", "错误必须点出两个冲突文件之二")
}

func TestLoadManualSegments(t *testing.T) {
	dir := filepath.Join("testdata", "segments")

	t.Run("file exists", func(t *testing.T) {
		got, err := LoadManualSegments(dir, "ACME")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, map[string]float64{"cloud": 29940000000, "devices": 13020000000.5}, got["2025Q1"])
		assert.Equal(t, map[string]float64{"cloud": 120000000000}, got["FY2025"])
	})

	t.Run("lower case symbol resolves same file", func(t *testing.T) {
		got, err := LoadManualSegments(dir, "acme")
		require.NoError(t, err)
		assert.Contains(t, got, "2025Q1")
	})

	t.Run("file missing", func(t *testing.T) {
		got, err := LoadManualSegments(dir, "NOSUCH")
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got)
	})

	t.Run("dir missing", func(t *testing.T) {
		got, err := LoadManualSegments(filepath.Join(t.TempDir(), "nope"), "ACME")
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// 非法 fiscal_period 必须报错，而不是静默产生永远 JOIN 不上的数据。
func TestLoadManualSegmentsBadPeriodKey(t *testing.T) {
	for _, bad := range []string{"2025Q5", "2025-Q1", "FY25", "2025", "Q1"} {
		t.Run(bad, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "acme.yaml"),
				[]byte(bad+":\n  cloud: 1\n"), 0o644))
			got, err := LoadManualSegments(dir, "ACME")
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), "acme.yaml")
		})
	}

}

// 文件读不出 period → key → amount 两层 map 时（YAML 语法错误，或语法合法但 schema
// 不匹配——用户手写这类文件时的常见错误）必须报错并点名文件，不得静默返回空 map。
func TestLoadManualSegmentsUnparsableFile(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"invalid yaml", "2025Q1:\n  cloud: [1\n"},
		{"period value is scalar", "2025Q1: 123\n"},
		{"amount is not a number", "2025Q1:\n  cloud: notanumber\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "acme.yaml"), []byte(tc.body), 0o644))
			got, err := LoadManualSegments(dir, "ACME")
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), "acme.yaml")
		})
	}
}

// 冒烟测试直接加载仓库内的正式模板目录，保证 yaml 与 struct 永不脱节。
func TestLoadTemplatesFromRepoDir(t *testing.T) {
	got, err := LoadTemplates(filepath.Join("..", "..", "..", "configs", "prism", "templates"))
	require.NoError(t, err)
	require.NotEmpty(t, got, "仓库应至少包含 msft.yaml")

	msft, ok := got["MSFT"]
	require.True(t, ok, "configs/prism/templates 应包含 MSFT 模板，实际 %v", keysOf(got))
	assert.Equal(t, "789019", msft.CIK)
	assert.Equal(t, "StatementBusinessSegmentsAxis", msft.SegmentAxis)
	assert.Equal(t, 1, msft.Version)

	members := make([]string, 0, len(msft.Segments))
	keys := make([]string, 0, len(msft.Segments))
	for _, s := range msft.Segments {
		members = append(members, s.Member)
		keys = append(keys, s.Key)
		assert.NotEmpty(t, s.NameZH, "segment %s 缺中文名", s.Key)
		assert.NotEmpty(t, s.NameEN, "segment %s 缺英文名", s.Key)
	}
	// member 名已由 live XBRL 实例文档验证，改动必须同步真实数据。
	assert.Equal(t, []string{
		"ProductivityAndBusinessProcessesMember",
		"IntelligentCloudMember",
		"MorePersonalComputingMember",
	}, members)
	assert.Equal(t, []string{"productivity", "intelligent_cloud", "personal_computing"}, keys)
}

func writeTemplate(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	return dir
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, b, 0o644))
}

func keysOf(m map[string]*Template) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}


// writeTpl 写一个模板文件到 dir(A 股模板测试辅助)。
func writeTpl(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// Context Checkpoint: A 股模板支持(2026-08-01):
//   functional[0] aliases 解析且 KeyByName 含别名   → TestTemplateAliasesKeyByName
//   boundary[0]   别名与既有 member/别名冲突拒绝     → TestTemplateAliasDuplicateRejected
//   boundary[1]   CIK 允许为空(A 股无 CIK)           → TestTemplateEmptyCIKAllowed

func TestTemplateAliasesKeyByName(t *testing.T) {
	dir := t.TempDir()
	writeTpl(t, dir, "600519.sh.yaml", `
company: 600519.SH
version: 1
segments:
  - {key: series_liquor, name_zh: 系列酒, name_en: Series, xbrl_member: 系列酒, aliases: [其他系列酒]}
`)
	tpls, err := LoadTemplates(dir)
	require.NoError(t, err)
	tpl := tpls["600519.SH"]
	require.NotNil(t, tpl)
	m := tpl.KeyByName()
	assert.Equal(t, "series_liquor", m["系列酒"])
	assert.Equal(t, "series_liquor", m["其他系列酒"], "别名必须命中同一 key")
}

func TestTemplateAliasDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	writeTpl(t, dir, "x.yaml", `
company: X
version: 1
segments:
  - {key: a, name_zh: 甲, name_en: A, xbrl_member: MA}
  - {key: b, name_zh: 乙, name_en: B, xbrl_member: MB, aliases: [MA]}
`)
	_, err := LoadTemplates(dir)
	require.Error(t, err, "别名撞既有 member 必须拒绝")
}

func TestTemplateEmptyCIKAllowed(t *testing.T) {
	dir := t.TempDir()
	writeTpl(t, dir, "y.yaml", `
company: 000423.SZ
version: 1
segments:
  - {key: ejiao, name_zh: 阿胶, name_en: Ejiao, xbrl_member: 阿胶及系列产品}
`)
	tpls, err := LoadTemplates(dir)
	require.NoError(t, err, "A 股模板无 CIK 应合法")
	assert.Equal(t, "", tpls["000423.SZ"].CIK)
}
