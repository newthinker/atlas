package hestia

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hestia.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

// YAML 里没写的阈值必须保持 DefaultThresholds 的值，**不能是零值**。
//
// 这不是便利性问题：deposit_sum_tolerance 若静默变成 0，每一期都会因残差
// 超容差而进 pending —— 而实测残差本就有 7.6–9.1%。一个漏写的键会让整条
// 管线停摆，且现场看起来像「数据全都有问题」。
func TestLoadConfigKeepsDefaultsForOmittedThresholds(t *testing.T) {
	p := writeConfig(t, `
config_version: "2026-08-12"
storage:
  db_path: data/hestia.db
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
thresholds:
  yoy_sanity_max: 60
`)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)

	def := DefaultThresholds()
	assert.Equal(t, 60.0, cfg.Thresholds.YoYSanityMax, "写了的要被覆盖")
	assert.Equal(t, def.DepositSumTolerance, cfg.Thresholds.DepositSumTolerance,
		"没写的必须保持默认，不能是 0")
	assert.Equal(t, def.CorpLoanTolerance, cfg.Thresholds.CorpLoanTolerance)
	assert.Equal(t, def.DepositSumDriftMax, cfg.Thresholds.DepositSumDriftMax)

	// M1c-2 的 TASK-001：这条与 TestLoadConfigRejects 的「显式写了 map 但漏一档」
	// **成对**。那格要求「写了这张表就必须五档齐全」；缺了本条的话，一个「一律要求
	// 五档齐全」的实现会把「完全不写、用默认值」这条正常路径也判成配置错误。
	assert.Equal(t, def.StockContinuityMax, cfg.Thresholds.StockContinuityMax)
	assert.Len(t, cfg.Thresholds.StockContinuityMax, len(validPeriodTypes),
		"完全不写时必须拿到齐全的默认分档表")

	// 覆盖值必须真的**不同于**默认值，否则上面那条 Equal 在「什么都没覆盖」的
	// 实现下也成立 —— 60 与默认的 50 不同，这条把「覆盖确实发生了」钉死。
	require.NotEqual(t, def.YoYSanityMax, 60.0,
		"用例前提：60 必须与默认值不同，否则「写了的要被覆盖」那条平凡为真")
}

// 完整配置能装载，豁免连同 period_types 一起读出来。
func TestLoadConfigFull(t *testing.T) {
	p := writeConfig(t, `
config_version: "2026-08-12"
storage:
  db_path: data/hestia.db
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
thresholds:
  deposit_sum_tolerance: 0.12
  caliber_exemptions:
    - version: "2025-01"
      period: "2025-01"
      period_types: [monthly, h1, annual]
      skip_checks: [deposit_sum]
      reason: "M1 口径纳入个人活期存款"
`)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)

	assert.Equal(t, "2026-08-12", cfg.ConfigVersion)
	assert.Equal(t, "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html", cfg.Discover.IndexURL)
	assert.Equal(t, 3, cfg.Discover.MaxPages)
	assert.Equal(t, 30*time.Second, cfg.Discover.Timeout,
		"30s 要被解析成 duration 而不是字符串或 30 纳秒")

	require.Len(t, cfg.Thresholds.CaliberExemptions, 1)
	ex := cfg.Thresholds.CaliberExemptions[0]
	assert.Equal(t, "2025-01", ex.Version)
	assert.Equal(t, "2025-01", ex.Period)
	assert.Equal(t, []string{"monthly", "h1", "annual"}, ex.PeriodTypes)
	assert.Equal(t, []string{"deposit_sum"}, ex.SkipChecks)
	assert.Equal(t, "M1 口径纳入个人活期存款", ex.Reason)
}

// 五种非法配置各自报错，且错误里含对应的键名。
//
// ⚠️ 前置断言不是装饰：没有它，下面每条用例都可能因**别的原因**报错而测试照样绿
// —— 它们只断言「有 error 且串里有某几个字」。这与 thresholds_test.go 的
// TestThresholdsRejectMalformedExemptions 用的是同一道绊线。
//
// ⚠️ 含 caliber_exemptions 的夹具都写了 period_types（除了那条**故意**缺它的）：
// T6 起 PeriodTypes 必填非空，且这道检查在 validate() 里排在 Reason 之前会抢先返回
// —— 漏写会让用例红在 PeriodTypes 上，而不是它真正想测的那一项。
func TestLoadConfigRejects(t *testing.T) {
	// storage 段与 discover 一样是每条夹具的「合法底座」：db_path 必填且排在
	// validate() 第一位，缺它会让每条用例红在 db_path 上而不是它想测的那一项
	// —— 与下面 period_types 那道注释是同一种抢先返回。
	const storage = `
storage:
  db_path: data/hestia.db
`
	const head = storage + `
discover:
  index_url: https://example.com/index.html
  max_pages: 3
  timeout: 30s
`
	_, err := LoadConfig(writeConfig(t, head))
	require.NoError(t, err, "base 必须合法，否则下面的用例证明不了任何事")

	tests := []struct {
		name, body, want string
	}{
		{"缺 index_url", storage + "discover:\n  max_pages: 3\n  timeout: 30s\n", "index_url"},
		{"max_pages 为 0", storage + "discover:\n  index_url: https://x/i.html\n  max_pages: 0\n  timeout: 30s\n", "max_pages"},
		{"timeout 为 0", storage + "discover:\n  index_url: https://x/i.html\n  max_pages: 3\n  timeout: 0s\n", "timeout"},
		{"显式把容差写成 0", head + "thresholds:\n  deposit_sum_tolerance: 0\n", "deposit_sum_tolerance"},
		// 另外四个阈值同族：DoD 只点名了 deposit_sum_tolerance，但那四条检查
		// **写了却没人测** —— 删掉任一条都不会有东西变红（实测 validate 覆盖率
		// 只有 66.7%）。第二道防线要么每条都守，要么它就只是看起来有五条。
		{"drift_max 为 0", head + "thresholds:\n  deposit_sum_drift_max: 0\n", "deposit_sum_drift_max"},
		{"corp_loan_tolerance 为 0", head + "thresholds:\n  corp_loan_tolerance: 0\n", "corp_loan_tolerance"},
		// —— M1c-2 的 TASK-001：stock_continuity_max 的三种失效，三个 want ——
		//
		// ⚠️ 三条的 want **必须互不相同、且互不包含**：字段名 stock_continuity_max
		// 在三者的错误里都出现，拿它当 want 一条都区分不开 —— 那样把三种失效实现成
		// 同一条分支（或干脆漏掉其中两条）测试照样全绿。
		// 验收方式：把任一格的 want 换成另一格的，该格必须变红。
		//
		// 第三种（旧格式标量）在 TestLoadConfigRejectsLegacyScalarStockContinuity
		// ——它不在这张表里，因为它要断言的不止「错误串含某几个字」，还有**错误出自
		// 哪一层**，而本表的循环体只做 Contains。
		{
			// 显式写了这张表却漏一档：**这才是真正静默的那个**。
			// LoadConfig 先预填 DefaultThresholds 再 Unmarshal，而 mapstructure 的
			// ZeroFields 默认 false ⇒ merge 不是 replace，漏掉的 annual 会**悄悄保留
			// 默认的 0.15**。有人改窄 monthly 时顺手删一档，那条序列的闸门就实质
			// 放宽了 7.5 倍，而没有任何东西会说一声。
			"显式写了 map 但漏一档",
			head + `thresholds:
  stock_continuity_max:
    monthly: 0.008
    q1: 0.15
    h1: 0.15
    q1_q3: 0.15
`,
			"缺 period_type: annual",
		},
		{
			// 值非正：与另外四个阈值同族的第二道防线，只是现在要逐档查。
			// 错误必须点名是哪一档 —— 只说「stock_continuity_max 非正」的话，
			// 配置的人得自己把五个数看一遍。
			"某一档写成 0",
			head + `thresholds:
  stock_continuity_max:
    monthly: 0
    q1: 0.15
    h1: 0.15
    q1_q3: 0.15
    annual: 0.15
`,
			"stock_continuity_max[monthly] must be > 0",
		},
		{"yoy_sanity_max 为 0", head + "thresholds:\n  yoy_sanity_max: 0\n", "yoy_sanity_max"},
		// 负数与 0 同样要拒：写 -1 的人多半想表达「关掉这道闸」，而阈值是
		// 「超过就拦」的上限，负数会让每一期都超标 —— 与写 0 的后果同族。
		{"容差写成负数", head + "thresholds:\n  deposit_sum_tolerance: -1\n", "deposit_sum_tolerance"},
		{
			"豁免缺 period_types",
			head + `thresholds:
  caliber_exemptions:
    - version: "2025-01"
      period: "2025-01"
      skip_checks: [deposit_sum]
      reason: "忘了写 period_types"
`,
			"PeriodTypes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			// 校验错误也必须被 %w 包住：只看错误串分不出 %w 与 %v（两者打印结果
			// 一模一样），而调用方要靠 Unwrap 拿到底层判因。
			require.NotNil(t, errors.Unwrap(err), "校验失败的错误必须包住底层 err")
		})
	}
}

// 旧格式（stock_continuity_max 写成标量）必须响亮失败，且**失败在 Unmarshal 层**。
//
// # 这条测试要钉的是「守卫在哪一层」，不只是「有没有报错」
//
// 上游计划在四处反复陈述「旧格式不会报错——viper 把标量塞进 map 得到空 map，
// 一道闸门无声消失」，并据此要求实现里写一段 `if len(...)==0 { …"scalar"… }`。
// **实跑证伪**：LoadConfig 先 `cfg := Config{Thresholds: DefaultThresholds()}`
// 预填、再 Unmarshal，预填的是 map ⇒ mapstructure 塞不进 float64，当场报错。
// ⇒ 那段守卫在 LoadConfig 路径上**不可达**，写了只是一段假装守卫存在的死分支。
//
// 所以这里断言的是**真实的**那一层：错误出自 `parsing config`（不是 `invalid
// config`），文案是 mapstructure 的，且点名了是哪个键。谁哪天真去加了那段死分支
// 并让它抢先返回，这三条会一起红——而只断「有 error」的话不会。
func TestLoadConfigRejectsLegacyScalarStockContinuity(t *testing.T) {
	p := writeConfig(t, `
storage:
  db_path: data/hestia.db
discover:
  index_url: https://x/i.html
  max_pages: 3
  timeout: 30s
thresholds:
  stock_continuity_max: 0.02
`)
	_, err := LoadConfig(p)
	require.Error(t, err, "旧格式标量必须报错，不得被当成空 map 静默放过")

	assert.Contains(t, err.Error(), "parsing config",
		"错误必须出自 Unmarshal 层：换成 invalid config 说明有人加了那段不可达的守卫分支")
	assert.Contains(t, err.Error(), "expected type 'map[string]float64'",
		"要说清期望的形状，配置的人才知道该怎么改")
	assert.Contains(t, err.Error(), "stock_continuity_max",
		"要点名是哪个键——thresholds 段有五个阈值")
	require.NotNil(t, errors.Unwrap(err), "必须包住 mapstructure 的底层错误")
}

// YAML 结构对不上类型时必须报错，不能把那个键当成没写而静默用默认值。
//
// 这是 LoadConfig 三条错误出口里的第二条（另两条是「文件读不了」与「校验不过」），
// 原本零覆盖。危害与漏写不同也更隐蔽：`max_pages: [1, 2]` 是**写了**的，写的人
// 认为自己配了 —— 若被静默忽略，跑起来用的却是别的值。
func TestLoadConfigRejectsMalformedYAML(t *testing.T) {
	p := writeConfig(t, `
storage:
  db_path: data/hestia.db
discover:
  index_url: https://x/i.html
  max_pages: [1, 2]
  timeout: 30s
`)
	_, err := LoadConfig(p)
	require.Error(t, err, "类型对不上必须报错，不得当成没写")
	assert.Contains(t, err.Error(), "parsing config")
	require.NotNil(t, errors.Unwrap(err), "必须包住 mapstructure 的底层错误")
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err, "文件不存在必须报错，不得当成空配置装载默认值")
	assert.Contains(t, err.Error(), "nope.yaml", "错误要带出是哪个文件")

	// 断言链路完整而不只是「有 error」：4b 的 cmd 层要区分「配置文件还没建」
	// （首次部署，可提示用户创建）与「配置写错了」（必须修）。%w 改成 %v 时
	// 错误串一模一样，只有这两条分得出来。
	require.NotNil(t, errors.Unwrap(err), "必须包住底层 err")
	assert.ErrorIs(t, err, fs.ErrNotExist, "调用方要能判出这是「文件不存在」")
}

// db_path 是 4b 才需要的：4a 不开库。没有它，cmd 层不知道该打开哪个文件。
func TestLoadConfigReadsStorage(t *testing.T) {
	p := writeConfig(t, `
storage:
  db_path: data/hestia.db
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.Equal(t, "data/hestia.db", cfg.Storage.DBPath)
}

// 缺 db_path 要立刻报错，而不是等到开库时报一个「打不开空路径」。
func TestLoadConfigRequiresDBPath(t *testing.T) {
	p := writeConfig(t, `
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)
	_, err := LoadConfig(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db_path", "错误要点名是哪个键缺了")
	require.NotNil(t, errors.Unwrap(err), "校验失败的错误必须包住底层 err")
}

// **写了但写成空串**与漏写同样要拒。
//
// 这条不与上一条重复：漏写走的是「mapstructure 没覆盖，字段保持零值」，
// 显式空串走的是「Unmarshal 如实覆盖成 ""」—— 与 thresholds 那组
// 「预填只挡住『没写』，挡不住『写了 0』」是同一族。只按「键是否出现」
// 判必填的实现能过上一条，过不了这一条。
func TestLoadConfigRejectsEmptyDBPath(t *testing.T) {
	p := writeConfig(t, `
storage:
  db_path: ""
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)
	_, err := LoadConfig(p)
	require.Error(t, err, "显式空串必须与漏写同样被拒")
	assert.Contains(t, err.Error(), "db_path")
	require.NotNil(t, errors.Unwrap(err), "校验失败的错误必须包住底层 err")
}

// 相对 db_path 装载后必须**原样**留着，validate 不得把它解析成绝对路径。
//
// 约束 C8：按**进程 cwd** 解析，而解析发生在 cmd 层（status 命令负责把解析后的
// 绝对路径打出来）。这里钉住两种会静默改变语义的实现：
//   - 在 validate 里 filepath.Abs —— 那是按**测试进程的 cwd** 解析，与 cmd 层
//     真正运行时的 cwd 未必相同；
//   - 相对**配置文件所在目录**解析 —— 看起来更「贴心」，但 plist 的
//     WorkingDirectory 指向 runtime 而配置文件在别处，两者会指向不同的库。
//
// 三条断言不是三道独立的闸：真正杀掉上面两种实现的是第一条 Equal，另两条被它
// 蕴含（等于 "data/hestia.db" 就必然不是绝对路径、也必然不含临时目录）。留着是
// 为了让「被排除的是哪两种实现」在失败信息里直接读得到，别当成三重保险。
func TestLoadConfigKeepsDBPathRelative(t *testing.T) {
	p := writeConfig(t, `
storage:
  db_path: data/hestia.db
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)

	assert.Equal(t, "data/hestia.db", cfg.Storage.DBPath, "必须一字不改")
	assert.False(t, filepath.IsAbs(cfg.Storage.DBPath), "装载不得把相对路径解析掉")
	assert.NotContains(t, cfg.Storage.DBPath, filepath.Dir(p),
		"更不得相对配置文件所在目录解析")
}

// Context Checkpoint: done_criteria → test mapping（M1c-3b 的 TASK-005）
// functional[4] 读仓库真配置断言四件事 → TestShippedConfigLoadsAndIsCalibrated

// 读**仓库里那份真配置**，不是临时夹具。
//
// # 为什么必须有这条（阻断级缺口 A-4）
//
// 实测 `grep -rn "configs/hestia.yaml" --include="*.go" .` **零命中** —— 那 54 项
// 区间是**人手填的**，而 TASK-004 的三类校验（未知字段名 / 区间倒置 / 缺 unit）
// 只在 `LoadConfig` 被调用时生效，**CI 里没有任何测试调用真配置**。
//
// 没有这条，打错一个字段名（如把 `m2` 写成 `m_2`）会让 `gateMagnitudeSanity`
// 对该字段**完全不设防**（validate.go 对未知键 `continue`），而缺陷要等**运维
// 真跑回填**才暴露——那时库已经建好了。
//
// ⚠️ 它替代的是原计划里「跑一次 hestia status」那个**手动动作**：手动步骤无产物、
// 不可事后核，且没人能证明上一次跑过。
func TestShippedConfigLoadsAndIsCalibrated(t *testing.T) {
	cfg, err := LoadConfig("../../configs/hestia.yaml")
	require.NoError(t, err, "仓库自带的配置必须能被自己的 LoadConfig 接受")

	require.Len(t, cfg.Thresholds.MagnitudeRanges, len(fieldOrder),
		"区间表必须覆盖全部 %d 个字段：漏填的字段其幅度闸静默消失（未知键 continue）",
		len(fieldOrder))
	assert.Equal(t,
		slices.Sorted(slices.Values(fieldOrder)),
		slices.Sorted(maps.Keys(cfg.Thresholds.MagnitudeRanges)),
		"键集合必须与 fieldOrder **逐项相等**：只比长度的话，打错一个字段名"+
			"（多一个未知键、少一个真字段）两边都还是 54")

	assert.Equal(t, "2026-08-31", cfg.ConfigVersion,
		"填了区间表就改变了这份配置的行为，config_version 必须跟着走——"+
			"否则「这期用的是哪版配置」在契约里查不出来")

	assert.Equal(t, DefaultThresholds().StockContinuityMax, cfg.Thresholds.StockContinuityMax,
		"真配置里的五档必须与代码默认值一致：两处分叉时，跑起来用的是 YAML 那份，"+
			"而读代码的人看到的是默认值那份")
}
