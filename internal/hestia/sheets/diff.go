package sheets

import (
	"fmt"
	"math"
	"sort"
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
	}
	return 0, false
}
