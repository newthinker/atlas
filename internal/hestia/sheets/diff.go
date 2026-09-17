package sheets

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ChangeKind 是一格比对后的三种去向。
type ChangeKind int

const (
	WillWrite  ChangeKind = iota // 表中无值或不同 ⇒ 要写
	Same                         // 已一致 ⇒ 跳过
	AbsentInDB                   // 库缺 ⇒ 保留表中现值
)

// Change 是一格的比对结果。
type Change struct {
	Sheet   string // 年度表名，如「2025年」
	Row     int    // 1 基行号
	Col     int    // 0 基列号
	Label   string
	Current any // 表中现值；无则 nil
	Want    any // 将写值；AbsentInDB 时为 nil
	Kind    ChangeKind
}

// entryRowOffset 是录入区首行（1 月）在表里的 1 基行号减 1：表头占前 3 行，1 月落第 4 行。
const entryRowOffset = 3

// Diff 比对一张年度表的现值与待写行，产出三类变更。
//
// **Month ∈ [1,12] 的行**，每行 × cols 每列恰产出一条；**Month 越界的行不产出任何 Change**
// （理由见下面循环里的钳制注释）。所以恒等式是：
//
//	len(out) == len(合法行) × len(cols)    而**不是** len(rows) × len(cols)
//
// 🔴 这条不变量在 M2b 的行号钳制（006 返工第 2 轮）之后被**收窄**过一次，而当时这段注释
// 没跟着改，于是它有一轮时间在逐字描述一件已经不成立的事——「少一条就有一格无声消失」
// 恰好变成了对新代码的准确描述。⇒ 收窄不变量时，**宣称它的那段文字和依赖它的那段代码
// 必须一起改**，否则文档本身会变成下一条假溯源。
//
// dry-run 的「将写 / 一致 / 库缺」三者之和因此**不再等于格数**，而是等于
// `格数 − Result.DroppedCells`。跳过由 `Push` 记进 `Result.DroppedCells` 并在
// dry-run 输出里打印——**跳过必须可观测**，这正是本函数只返回切片、无法自己报告的那半。
// 输出顺序：rows 的输入顺序，行内按列号升序。行号只由 Row.Month 决定（Month+3），
// 不依赖 rows 的顺序。
//
// current 是从表里读回的录入区（行 4–15 共 12 行），索引 0 对应 1 月。
// 越界与短行都按「表中无值」处理——Sheets API 对尾部空格会截断行。
//
// Cell.Label 不在 cols 里的格**不校验、不产出**：C3 由 push 层的 ResolveHeader 兑现，
// 这里重复做只会让同一个错误报两遍。
func Diff(sheetName string, cols Columns, current [][]any, rows []Row) []Change {
	order := make([]string, 0, len(cols))
	for label := range cols {
		order = append(order, label)
	}
	sort.Slice(order, func(i, j int) bool { return cols[order[i]] < cols[order[j]] })

	out := make([]Change, 0, len(rows)*len(cols))
	for _, r := range rows {
		// 🔴 行号钳制（QA round2 [7]）：`Row = Month + entryRowOffset` 而
		// `entryRowOffset = 3` **恰好等于 client.go 的 headerRow** ⇒ Month=0 会把
		// 「0月」的格写进**表头行**，破坏 C3 依赖的表头本身，此后所有投影都会因
		// 表头解析失败而永久停摆；Month=13 写到第 16 行，落在录入区 4–15 之外。
		//
		// 越界就**不产出任何 Change**——产出了就有机会被 WriteCells 写出去。
		// 这是最后一道防线：正常路径上 buildRow 的 periodYearMonth 已经拦过一次
		// （TASK-010），但 Diff 是导出函数、也被别的调用方用，不该假设入参已校验。
		if r.Month < 1 || r.Month > 12 {
			continue
		}
		want := make(map[string]any, len(r.Cells))
		for _, c := range r.Cells {
			want[c.Label] = c.Value
		}
		rowIdx := r.Month - 1
		for _, label := range order {
			col := cols[label]
			ch := Change{
				Sheet:   sheetName,
				Row:     r.Month + entryRowOffset,
				Col:     col,
				Label:   label,
				Current: cellAt(current, rowIdx, col),
			}
			v, ok := want[label]
			switch {
			case !ok:
				ch.Kind = AbsentInDB
			case ch.Current != nil && sameValue(ch.Current, v):
				ch.Want, ch.Kind = v, Same
			default:
				ch.Want, ch.Kind = v, WillWrite
			}
			out = append(out, ch)
		}
	}
	return out
}

// cellAt 取表中现值；行或列越界、或格为空串都按无值（nil）。
func cellAt(current [][]any, row, col int) any {
	if row < 0 || row >= len(current) || col < 0 || col >= len(current[row]) {
		return nil
	}
	v := current[row][col]
	if s, ok := v.(string); ok && s == "" {
		return nil
	}
	return v
}

// sameValue 判两个格的值是否已经一致。
//
// 🔴 **current 的类型取决于调用方用的 valueRenderOption**（QA round2 CRITICAL-1）：
// values.get 默认 FORMATTED_VALUE ⇒ 数值以**格式化文本**回来；要 UNFORMATTED_VALUE
// 才回 JSON 数字。本包的 Client 已显式要后者，但本函数是纯函数、也被别的路径调用，
// 所以 toFloat 同时认字符串形态——两头都兜住，而不是把这个假设压在调用方身上。
//
// 数值用**相对容差**而不是 ==：浮点表示噪声（实测契约里出现过 64400.00000000001）
// 会让某些格永远报不一致，于是幂等永远达不成，每次 apply 都在写同一个值。
// 1e-9 的相对容差远小于本业务任何一个字段的有效精度（金额到亿元、同比到 0.1%），
// 不会盖住真实差异。
//
// 🔴 **容差有一个例外，方向单开**：库值是可精确表示的整数、而表中值不是时判不一致。
//
// 上面那句 64400.00000000001 不是举例，是真发生过的事——它来自 amount.toYi 早期的
// 裸浮点乘法，脏值入了库又经本包投影进了线上表格。库侧修好之后，表里那 44 格却
// **不会**跟着恢复：容差恰好把它们判成「一致」，push 永远跳过。容差于是从「防幂等
// 失效」变成了「冻住我们自己写进去的陈旧噪声」，而这是它最不该做的事。
//
// 例外只朝一个方向开口，因为两个方向的含义不同：库值整数而表中带尾巴 ⇒ 尾巴是
// 历史噪声，该纠正；库值自身带尾巴（同比 8.7 那族 float64 固有不可表示的值）⇒ 那是
// float64 的性质、纠不了也不该纠，仍走容差。**幂等不受影响**：纠正写入后两边都是
// 整数、精确相等，下一轮即判 Same。
func sameValue(current, want any) bool {
	curF, curOK := toFloat(current)
	wantF, wantOK := toFloat(want)
	if curOK && wantOK {
		if wantF == math.Trunc(wantF) && curF != math.Trunc(curF) {
			return false
		}
		scale := max(1, math.Abs(curF), math.Abs(wantF))
		return math.Abs(curF-wantF) <= 1e-9*scale
	}
	return fmt.Sprint(current) == fmt.Sprint(want)
}

// toFloat 把表里可能出现的各种数值表示统一成 float64；非数值返回 false。
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		return parseSheetNumber(x)
	}
	return 0, false
}

// parseSheetNumber 把表里读回来的**文本形态数值**解析成 float64。
//
// 两种来源，都真实存在：
//  1. 调用方漏了 valueRenderOption ⇒ API 回 FORMATTED_VALUE（千分位、百分号、
//     会计负数括号）。本包的 client 已显式要 UNFORMATTED_VALUE，但 diff 是纯函数、
//     不该假设唯一的调用方永远记得这件事。
//  2. 表里那一格**真的被人存成了文本**。这种格该被纠正成数字（AJ–BB 的 19 个公式
//     拿文本没办法），所以这里只负责「看懂它的值」，判 Same 还是 WillWrite 由
//     sameValue 比完值再定——同值 ⇒ Same，不同值 ⇒ WillWrite 写回数字。
//
// 解析不出来 ⇒ ok=false，退回 fmt.Sprint 的字符串比较（「暂无」这类占位文字仍按
// 文本比，不会被误当成 0）。
func parseSheetNumber(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, false
	}
	neg := false
	// 会计负数：(933) == -933
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg, t = true, strings.TrimSuffix(strings.TrimPrefix(t, "("), ")")
	}
	// 百分号只去符号、**不除以 100**：本表的同比/占比/利率列存的就是不带符号的百分数
	// （见 hestia.SheetColumns 的口径），除一次会把 10.7 变成 0.107。
	t = strings.TrimSuffix(strings.TrimSpace(t), "%")
	t = strings.ReplaceAll(t, ",", "") // 千分位
	f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil {
		return 0, false
	}
	if neg {
		f = -f
	}
	return f, true
}
