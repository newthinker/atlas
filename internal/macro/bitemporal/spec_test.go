package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-001 spec)
// functional[0]     NewSpec 接受合法形状（多列键 / 单列键）；零值 Spec 被 zero() 识别 → TestNewSpecAcceptsValidShapes
// functional[1]     NewSpec 复制 businessKey 切片，调用方后续修改不影响 Spec     → TestNewSpecCopiesBusinessKey
// boundary[0]       correlate 只产出 `col = alias.col` 形式、无字面值            → TestCorrelate
// error_handling[0] 非法标识符被拒（注入/空串/数字开头/空格/引号/反引号 × 三入口）→ TestNewSpecRejectsInvalidIdentifiers
// error_handling[1] 注入用例断言 error 指名被拒的那个标识符（契约 T2）          → TestNewSpecRejectsInvalidIdentifiers（wantLabel + %q 断言）
// error_handling[2] 键形状被拒（业务键为空 / 重复 / 与 revision 列重名）          → TestNewSpecRejectsBadKeyShape
// error_handling[3] checkKey 拒绝键集不匹配（缺键 / 多键 / 键名不同）            → TestCheckKey
//
// 注：boundary[0] 是纯函数字符串断言，按契约 T1 它不能单独构成守护——
// 「Query 真的用了 correlate」由 TASK-004 的行为侧测试证明，本文件只保证纯函数本身正确。

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpecAcceptsValidShapes(t *testing.T) {
	multi, err := NewSpec("hestia_observations", []string{"period", "period_type"}, "published_at")
	require.NoError(t, err)
	assert.False(t, multi.zero())

	single, err := NewSpec("t", []string{"ts"}, "fetched_at")
	require.NoError(t, err)
	assert.False(t, single.zero())

	// 下划线开头、含数字的标识符是合法的——identRE 只禁数字开头，不禁数字本身
	underscored, err := NewSpec("_t2", []string{"_col1"}, "_rev9")
	require.NoError(t, err)
	assert.False(t, underscored.zero())

	// 零值 Spec 必须能被识别——它是唯一绕过校验的途径
	assert.True(t, Spec{}.zero())
}

// TestNewSpecRejectsInvalidIdentifiers 覆盖六类非法标识符 × 三个入口。
//
// 每个用例除了断言「有 error」，还断言 error 指名了**被拒的那个入口和那个标识符**。
// 只断言 require.Error 是不够的：任何原因的 error 都能让它绿——若将来 NewSpec 因
// 别的原因（比如键形状）先行返回错误，注入这条会在实际防线已失效的情况下照样通过。
// 断言 `<入口标签> <标识符的 %q 形式>` 把测试钉死在「正是标识符校验拦下的」这件事上。
func TestNewSpecRejectsInvalidIdentifiers(t *testing.T) {
	const (
		labelTable = "invalid table name"
		labelKey   = "invalid business key column"
		labelRev   = "invalid revision column"
	)
	// bad 是本用例中预期被拒的那个标识符原值，用于构造期望的 error 片段；
	// 用 %q 而非原串拼接，因为实现也用 %q 格式化——注入串里的引号会被转义。
	cases := map[string]struct {
		table     string
		key       []string
		rev       string
		wantLabel string
		bad       string
	}{
		"table 注入":   {`x"; DROP TABLE y; --`, []string{"period"}, "published_at", labelTable, `x"; DROP TABLE y; --`},
		"table 空串":   {"", []string{"period"}, "published_at", labelTable, ""},
		"table 数字开头": {"1table", []string{"period"}, "published_at", labelTable, "1table"},
		"table 含空格":  {"my table", []string{"period"}, "published_at", labelTable, "my table"},
		"table 含引号":  {`ta'ble`, []string{"period"}, "published_at", labelTable, `ta'ble`},
		"table 含反引号": {"ta`ble", []string{"period"}, "published_at", labelTable, "ta`ble"},

		"key 注入":   {"t", []string{"period; DELETE FROM t"}, "published_at", labelKey, "period; DELETE FROM t"},
		"key 空串":   {"t", []string{""}, "published_at", labelKey, ""},
		"key 数字开头": {"t", []string{"1period"}, "published_at", labelKey, "1period"},
		"key 含空格":  {"t", []string{"per iod"}, "published_at", labelKey, "per iod"},
		"key 含引号":  {"t", []string{`per'iod`}, "published_at", labelKey, `per'iod`},
		"key 含反引号": {"t", []string{"per`iod"}, "published_at", labelKey, "per`iod"},

		// 多列键时非法列排在合法列之后，确认校验遍历整个键而不是只看第一列
		"key 尾列非法": {"t", []string{"period", "per iod"}, "published_at", labelKey, "per iod"},

		"revision 注入":   {"t", []string{"period"}, `published_at"; DROP TABLE y; --`, labelRev, `published_at"; DROP TABLE y; --`},
		"revision 空串":   {"t", []string{"period"}, "", labelRev, ""},
		"revision 数字开头": {"t", []string{"period"}, "1published", labelRev, "1published"},
		"revision 含空格":  {"t", []string{"period"}, "published at", labelRev, "published at"},
		"revision 含引号":  {"t", []string{"period"}, `pub'lished`, labelRev, `pub'lished`},
		"revision 含反引号": {"t", []string{"period"}, "pub`lished", labelRev, "pub`lished"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, err := NewSpec(tc.table, tc.key, tc.rev)
			require.Error(t, err)
			require.ErrorContains(t, err, fmt.Sprintf("%s %q", tc.wantLabel, tc.bad))
			// 出错时必须返回零值 Spec：调用方若忽略 error，拿到的东西也得能被 zero() 拦下
			assert.True(t, s.zero())
		})
	}
}

func TestNewSpecRejectsBadKeyShape(t *testing.T) {
	_, err := NewSpec("t", nil, "published_at")
	require.Error(t, err, "业务键为空")

	_, err = NewSpec("t", []string{}, "published_at")
	require.Error(t, err, "业务键为空切片")

	_, err = NewSpec("t", []string{"period", "period"}, "published_at")
	require.Error(t, err, "业务键重复")

	_, err = NewSpec("t", []string{"period", "published_at"}, "published_at")
	require.Error(t, err, "revision 列不得同时是业务键")
}

// TestNewSpecCopiesBusinessKey 证明 NewSpec 复制了调用方的切片。
// 若实现改成直接赋值（businessKey: businessKey），调用方后面这一行 tamper
// 就会改到 Spec 内部，correlate 随之产出被篡改的列名——本测试转红。
func TestNewSpecCopiesBusinessKey(t *testing.T) {
	key := []string{"period", "period_type"}
	s, err := NewSpec("t", key, "published_at")
	require.NoError(t, err)

	key[0] = "tampered" // 调用方改自己的切片，不应影响已构造的 Spec
	assert.Equal(t, "period = o.period AND period_type = o.period_type", s.correlate("o"))
	// checkKey 走的是同一份内部切片，一并确认它没被污染
	require.NoError(t, s.checkKey(Key{"period": "2026-06", "period_type": "h1"}))
}

func TestCheckKey(t *testing.T) {
	s, err := NewSpec("t", []string{"period", "period_type"}, "published_at")
	require.NoError(t, err)

	require.NoError(t, s.checkKey(Key{"period": "2026-06", "period_type": "h1"}))

	require.Error(t, s.checkKey(Key{"period": "2026-06"}), "缺键")
	require.Error(t, s.checkKey(Key{"period": "2026-06", "period_type": "h1", "extra": "x"}), "多键")
	// 键名不符：长度与业务键相同，只有靠逐列存在性检查才能拦下
	require.Error(t, s.checkKey(Key{"period": "2026-06", "wrong": "h1"}), "键名不符")
	require.Error(t, s.checkKey(nil), "空 Key")
}

// TestCorrelate 断言 correlate 的确切输出：只有 `col = alias.col` 与 AND，
// 没有任何字面值。这里是本包唯一断言 SQL 字符串字面量的地方。
func TestCorrelate(t *testing.T) {
	s, err := NewSpec("t", []string{"ts", "indicator"}, "fetched_at")
	require.NoError(t, err)
	assert.Equal(t, "ts = o.ts AND indicator = o.indicator", s.correlate("o"))

	// 单列键不应带 AND，也不应有多余空格
	single, err := NewSpec("t", []string{"ts"}, "fetched_at")
	require.NoError(t, err)
	assert.Equal(t, "ts = x.ts", single.correlate("x"))
}
