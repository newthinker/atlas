package sankey

// Context Checkpoint: done_criteria → test mapping
// functional[0] 合法模板解析出 company/cik/segment_axis/version 与完整 segments 列表（逐字段断言） → TestLoadTemplatesValid
// functional[1] 校验分支：key 重复 / xbrl_member 为空 / company 为空 → error                     → TestLoadTemplatesValidation
// functional[2] AD-16 三种目录情况区分（不存在 / 无 yaml / 非法 YAML 含文件名）                   → TestLoadTemplatesDirCases
// functional[3] LoadManualSegments 两层 map / 文件不存在空 map / fiscal_period 键格式校验         → TestLoadManualSegments, TestLoadManualSegmentsBadPeriodKey
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
		{
			name: "empty cik",
			yaml: `
company: A
segments:
  - {key: cloud, name_zh: 云, name_en: Cloud, xbrl_member: CloudMember}
`,
			wantErr: "cik",
		},
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

	t.Run("invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "acme.yaml"), []byte("2025Q1:\n  cloud: [1\n"), 0o644))
		got, err := LoadManualSegments(dir, "ACME")
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "acme.yaml")
	})
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
