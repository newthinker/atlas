package hestia

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	assert.Equal(t, def.StockContinuityMax, cfg.Thresholds.StockContinuityMax)
	assert.Equal(t, def.DepositSumDriftMax, cfg.Thresholds.DepositSumDriftMax)

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
		{"stock_continuity_max 为 0", head + "thresholds:\n  stock_continuity_max: 0\n", "stock_continuity_max"},
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
