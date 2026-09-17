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
// 每行 × cols 每列恰产出一条（len == len(rows)×len(cols)）：这是 dry-run 计数
// 「将写 / 一致 / 库缺」三者之和等于格数的根据，少一条就有一格无声消失。
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
func sameValue(a, b any) bool {
	fa, aok := toFloat(a)
	fb, bok := toFloat(b)
	if aok && bok {
		scale := max(1, math.Abs(fa), math.Abs(fb))
		return math.Abs(fa-fb) <= 1e-9*scale
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
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
