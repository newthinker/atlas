package hestia

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// directionAlt 是方向词的正则交替片段，供 T5 的模板族直接嵌入。
//
// **只此一份**：模板方需要方向词时拼这个常量，不要在别处再列一遍词表——
// 两份词表迟早分叉，而分叉的表现是某个字段静默取不到值。
//
// 词表里**没有**「多增」「少增」「少减」，这不是遗漏：它们修饰的是**同比**
// （「同比多增1780亿元」「同比少减1873亿元」），是必须被排除的干扰词。一旦加进来，
// 社融增量那族模板会在「同比」子句上命中，导致重复赋值或取到错的那个数。
//
// 交替顺序在当前词表下**没有语义**（实测：整个顺序颠倒后，四种模板形态的输出
// 逐字节相同）。Go 的 regexp 是 leftmost-first，顺序只在两个分支能在同一起始位置
// 都匹配时才起作用——也就是一个是另一个的**前缀**时。「融资」是「净融资」的
// 后缀而非前缀，所以它排在哪里都不影响结果。真正要守的不变量是「任何一项都不是
// 另一项的前缀」，由 TestNoAlternativeIsPrefixOfAnother 看着；若将来加入「增」
// 这类前缀词，那条测试会转红，届时才需要按长词在前排序。
//
// 增删方向词时同步改 directionSign；TestDirectionSignCoversDirectionAlt 会在
// 两处脱节时转红。
const directionAlt = `净融资|融资|增加|减少|增长|下降`

// directionSign 把方向词映射成符号。
//
// **符号只来自方向词，数字本身永远是正的。** 这是解析器最容易出错也最难发现的
// 一处：漏读方向词会让 deposit_nbfi_ytd 从 −7446 变成 +7446——量级正确、方向
// 相反，而七道闸门检查的是恒等式与量级区间，一道都拦不住。
var directionSign = map[string]float64{
	"净融资": +1, // 社融增量里企业债用它
	"融资":  +1, // 非金融企业境内股票用它
	"增加":  +1,
	"减少":  -1,
	"增长":  +1,
	"下降":  -1,
}

// unitAlt 是单位的正则交替片段，与 directionAlt 一样只此一份、供模板嵌入。
const unitAlt = `万亿美元|万亿元|亿美元|亿元`

// unitScale 把单位换算成以「亿」为基准的倍率。
//
// 美元与人民币共用倍率表：fx_reserve 用「万亿美元」、外币存贷款用「亿美元」，
// 它们各自成列、不与人民币混算，所以只需要量级换算，不需要币种区分。
var unitScale = map[string]float64{
	"万亿元":  10000,
	"亿元":   1,
	"万亿美元": 10000,
	"亿美元":  1,
}

// amount 是一个带符号的量及其原始单位。
//
// 保留原始单位而不在构造时就归一：归一目标由**字段类别**决定（余额类记万亿元、
// 增量类记亿元），而那是调用方才知道的信息。在这里提前归一等于让 amount 猜
// 自己会被用作哪一类。
type amount struct {
	value float64 // 已带符号，尚未换算
	unit  string
}

// newAmount 从正则捕获到的三段构造 amount：方向词决定符号、单位决定倍率。
//
// 三个参数都必须来自**文本捕获**。缺任何一个都不能靠默认值补——默认为正会翻转
// 方向，默认某单位会带来 10000× 误差，而两者都完全无声。
//
// **不要把字面量方向词传进来。** 余额/存量句原文没有方向词，硬编码一个白名单内
// 的词能骗过下面的校验，等于在那些调用点上把方向词防线整个关掉；那类句子请改用
// parsePlainAmount。TestNoLiteralDirectionWordAtProductionCallSites 会静态检出违规。
func newAmount(dir, num, unit string) (amount, error) {
	sign, ok := directionSign[dir]
	if !ok {
		return amount{}, fmt.Errorf(
			"hestia: unknown direction word %q (known: %s): refusing to assume a sign, "+
				"a wrong sign keeps the magnitude right and flips the meaning",
			dir, directionAlt)
	}
	if err := checkUnit(unit); err != nil {
		return amount{}, err
	}

	v, err := parseUnsignedNumber(num)
	if err != nil {
		return amount{}, err
	}
	return amount{value: sign * v, unit: unit}, nil
}

// parsePlainAmount 构造无方向词句式的量，符号恒正。
//
// 余额/存量句的原文里根本没有方向词——「社会融资规模存量为430.22万亿元」
// 「广义货币(M2)余额328.64万亿元」「人民币存款余额…」。这类句子共约 6 处模板、
// 14 处字段赋值，占全部数值赋值的四分之一左右。
//
// 它们必须走这个独立入口，而不是把字面量「增加」传给 newAmount：
//
//   - 硬编码的方向词让白名单在这些调用点上**永远不可能触发**，防线名存实亡，
//     而孤立测 newAmount 全绿，根本看不见喂进去的是常量；
//   - 某期若把余额句改写成「余额较上年末**减少**…」，硬编码的正号会照旧输出
//     正值——符号错恰好发生在防线被关掉的地方；
//   - 走独立入口后，「这里没有方向词可读」变成写在函数名上、可 grep、可枚举的
//     事实，审计时一眼看得出总共有多少处绕过了方向词校验。
func parsePlainAmount(num, unit string) (amount, error) {
	if err := checkUnit(unit); err != nil {
		return amount{}, err
	}
	v, err := parseUnsignedNumber(num)
	if err != nil {
		return amount{}, err
	}
	return amount{value: v, unit: unit}, nil
}

// toYi 归一到「亿元」——增量/新增类字段用它。
func (a amount) toYi() float64 { return a.value * scaleOf(a.unit) }

// toWanYi 归一到「万亿元」——余额/存量类字段用它。
func (a amount) toWanYi() float64 { return a.toYi() / 10000 }

// scaleOf 取单位倍率；单位不在表内即 panic。
//
// 两个构造函数都保证了 unit 在表内，所以这里唯一可能的来源是**零值 amount**——
// 也就是调用方忽略了构造函数返回的 error。此时若按 map 的零值返回 0，就会有一个
// 看起来完全合理的 0 静默流进 Values，而下游查的是恒等式与量级区间，接不住。
// 这属于编码错误而非输入错误，宁可炸在调用点。
func scaleOf(unit string) float64 {
	scale, ok := unitScale[unit]
	if !ok {
		panic(fmt.Sprintf(
			"hestia: amount with unvalidated unit %q: the constructor's error was ignored", unit))
	}
	return scale
}

// parseRatio 解析同比这类百分数：有方向词，无单位。
//
// 走同一张方向词白名单——「同比下降18%」的符号与「减少2043亿元」同源，
// 两处用不同的表迟早会分叉。
func parseRatio(dir, num string) (float64, error) {
	sign, ok := directionSign[dir]
	if !ok {
		return 0, fmt.Errorf(
			"hestia: unknown direction word %q in ratio (known: %s)", dir, directionAlt)
	}
	v, err := parseUnsignedNumber(num)
	if err != nil {
		return 0, err
	}
	return sign * v, nil
}

// parsePlainNumber 解析既无方向词也无单位的数值。
//
// 只有两类这样用：利率（「加权平均利率为1.36%」）与汇率（「1美元兑7.0288元人民币」）。
// 它们句式独立、没有方向词可读——所以这里不走白名单，而**正因为它绕过了那道防线，
// 用它的地方必须少且明确**。
//
// 与另外三个入口不同，这里**不禁止**前导负号：没有方向词可以与之冲突，负号若真的
// 出现在原文里就是那个数本身的一部分。非有限值仍然拒绝。
func parsePlainNumber(num string) (float64, error) {
	return parseFiniteNumber(num)
}

// parseUnsignedNumber 解析一个不得自带符号的数。
//
// 符号只能来自方向词。今天 T5 的 numPat 捕不到前导负号，但那是另一个文件里的另一条
// 约束——2025 年原文注5 的「M1 可比口径回溯」表就含显式负号数字（−0.8%、−1.7%），
// 哪天有人为了够到那张表放宽 numPat，符号就会同时从两个来源进来。把不变量钉在原语
// 上，那一刻会响亮失败而不是静默取到 +0.8。
func parseUnsignedNumber(num string) (float64, error) {
	if strings.HasPrefix(num, "+") || strings.HasPrefix(num, "-") {
		return 0, fmt.Errorf(
			"hestia: number %q carries its own sign: the sign must come from the direction word, "+
				"never from the digits", num)
	}
	return parseFiniteNumber(num)
}

// parseFiniteNumber 把文本解析成有限浮点数。
//
// strconv.ParseFloat 接受 "NaN" / "Inf" / "Infinity" 且不报错；一个 NaN 落进
// Observation.Values 是典型的静默坏值——NaN 参与的比较一律为 false，恒等式与
// 量级区间两类闸门都会直接放行。
func parseFiniteNumber(num string) (float64, error) {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("hestia: cannot parse number %q: %w", num, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf(
			"hestia: number %q is not finite: a NaN or Inf passes every identity and range gate silently", num)
	}
	return v, nil
}

// checkUnit 校验单位在表内。
//
// 只校验存在、不在这里取倍率：归一目标由字段类别决定，取倍率是 toYi / toWanYi 的事。
func checkUnit(unit string) error {
	if _, ok := unitScale[unit]; !ok {
		return fmt.Errorf(
			"hestia: unknown unit %q (known: %s): refusing to assume one, "+
				"guessing between 万亿元 and 亿元 is a 10000x error",
			unit, unitAlt)
	}
	return nil
}
