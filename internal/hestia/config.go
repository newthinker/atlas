package hestia

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// StorageCfg 是库文件位置。
//
// db_path 用相对路径，按进程 cwd 解析（约束 C8）——plist 的 WorkingDirectory
// 已指向 runtime，手工执行时需自行 cd。status 命令会打印解析后的绝对路径，
// 免得在错误的 cwd 下静默指向一个不存在的库然后报告「0 期」。
type StorageCfg struct {
	DBPath string `mapstructure:"db_path"`
}

// Config 是 configs/hestia.yaml 的内存形态。
//
// 独立文件、独立装载器，照 internal/crisis/config.go 的先例，不并进
// internal/config 的大 Config：方案报告 5.3.1 定的「单一读者原则」——
// 这个文件只有 Atlas 读，Loom 从契约 JSON 的 thresholds 段取值。
type Config struct {
	// ConfigVersion 不参与逻辑，只是让「这期用的是哪版配置」在契约里一眼可见
	// （方案报告 5.3.2）。改配置时手工递增，用日期串。
	ConfigVersion string      `mapstructure:"config_version"`
	Storage       StorageCfg  `mapstructure:"storage"`
	Discover      DiscoverCfg `mapstructure:"discover"`
	Thresholds    Thresholds  `mapstructure:"thresholds"`
}

// LoadConfig 读配置文件并立即校验。
//
// 预填 DefaultThresholds 再 Unmarshal —— mapstructure 只覆盖 YAML 里出现的键，
// 没写的保持预填值。这一步不能省：deposit_sum_tolerance 静默变成 0 会让每一期
// 都因残差超容差进 pending（实测残差本就有 7.6–9.1%），整条管线停摆，
// 而现场看起来像「数据全都有问题」。
func LoadConfig(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("hestia: reading config %s: %w", path, err)
	}

	cfg := Config{Thresholds: DefaultThresholds()}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("hestia: parsing config %s: %w", path, err)
	}
	// 必须查 v（用户显式写了什么）而不是 cfg（merge 之后的结果）——理由见函数注释。
	if err := checkStockContinuityComplete(v); err != nil {
		return Config{}, fmt.Errorf("hestia: invalid config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("hestia: invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// stockContinuityKey 是分档表在配置文件里的完整键路径。
const stockContinuityKey = "thresholds.stock_continuity_max"

// checkStockContinuityComplete 要求：**显式写了**分档表，就必须每个 period_type
// 都写（M1c-2 的 TASK-001）。
//
// # 为什么非查不可
//
// LoadConfig 先预填 DefaultThresholds 再 Unmarshal，而 mapstructure 的 ZeroFields
// 默认 false ⇒ **merge 不是 replace**。于是「写了 map 但漏一档」会让那一档**静默
// 继承默认值**：配置里写的和实际生效的不一致，而两者都不报错。M1c-3 拿标定结果
// 重填阈值时最可能干的正是这个 —— 改窄 monthly、顺手删一档，那条序列的闸门实质
// 放宽了 7.5 倍，没有任何东西会说一声。
//
// # 为什么只在 IsSet 为 true 时查
//
// 完全不写这张表是**正常路径**（用默认值，与另外四个阈值一样）。一律要求齐全会把
// 它也判成配置错误。这两种情形成对，缺一不可——见 config_test.go 里那两条用例。
//
// ⚠️ 拿 viper 的 GetStringMap 取**用户显式写的键**，不能拿 Unmarshal 后的 cfg：
// 后者已经 merge 过默认值，五档永远齐全，查不出任何东西。
//
// ⚠️ 旧格式标量（stock_continuity_max: 0.02）**到不了这里** —— 预填的是 map，
// mapstructure 塞不进 float64，Unmarshal 就先失败了。所以这里不为标量写分支：
// 写了也永远进不去，只是让人以为有守卫。
func checkStockContinuityComplete(v *viper.Viper) error {
	if !v.IsSet(stockContinuityKey) {
		return nil
	}
	written := v.GetStringMap(stockContinuityKey)
	// 遍历 periodTypeList() 而不是硬编码五个键：加第六种 period_type 时这里自动
	// 要求配置也补上一档。顺序也由它定——map 迭代序随机，不定序的话同一份错配置
	// 每次报出的档序都不同，排查变成猜谜。
	var missing []string
	for _, pt := range periodTypeList() {
		if _, ok := written[pt]; !ok {
			missing = append(missing, pt)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s 缺 period_type: %s"+
		"（显式写了这张表就必须每档都写：漏掉的那档会静默继承默认值，"+
		"配置里写的和实际生效的不一致，而两者都不报错）",
		stockContinuityKey, strings.Join(missing, ", "))
}

// validate 检查配置自洽。装载即校验，不给「先读进来再说」留口子。
func (c Config) validate() error {
	switch {
	case c.Storage.DBPath == "":
		return errors.New("storage.db_path is required")
	case c.Discover.IndexURL == "":
		return errors.New("discover.index_url is required")
	case c.Discover.MaxPages < 1:
		return errors.New("discover.max_pages must be >= 1")
	case c.Discover.Timeout <= 0:
		return errors.New("discover.timeout must be > 0")
	}

	// 第二道防线：预填只挡住「没写」，**挡不住「写了 0」**。有人显式写
	// deposit_sum_tolerance: 0 时，Unmarshal 会如实覆盖掉预填的默认值，
	// 而 0 容差让每一期都超差进 pending —— 与漏写的后果完全一样。
	//
	// MagnitudeRanges **不在此检查** —— 它有意为空（区间要等 M1c 用回填分布标定，
	// 表为空时 magnitude_sanity 记 skipped{not_calibrated}）。把「非空」当成
	// 合法性要求会让默认配置自己就装载不了。
	t := c.Thresholds
	switch {
	case t.DepositSumTolerance <= 0:
		return errors.New("thresholds.deposit_sum_tolerance must be > 0")
	case t.DepositSumDriftMax <= 0:
		return errors.New("thresholds.deposit_sum_drift_max must be > 0")
	case t.CorpLoanTolerance <= 0:
		return errors.New("thresholds.corp_loan_tolerance must be > 0")
	case t.YoYSanityMax <= 0:
		return errors.New("thresholds.yoy_sanity_max must be > 0")
	}

	// stock_continuity_max 是一张分档表，**逐档**查（M1c-2 的 TASK-001）。
	// 只查「表非空」的话，把某一档写成 0 会让那条序列的每一期都超标进 pending ——
	// 与另外四个阈值写 0 是同一族后果，只是现在有五个地方能写坏。
	//
	// 错误点名是哪一档：只说「stock_continuity_max 非正」的话，配置的人得自己把
	// 五个数看一遍。排序遍历是因为 map 迭代序随机——同一份错配置每次报出的档不同
	// 会让排查变成猜谜。
	for _, pt := range slices.Sorted(maps.Keys(t.StockContinuityMax)) {
		if t.StockContinuityMax[pt] <= 0 {
			return fmt.Errorf("thresholds.stock_continuity_max[%s] must be > 0", pt)
		}
	}
	return t.validate() // 豁免校验（含 M1b-4a 的 TASK-006 补的 PeriodTypes 必填非空）
}
