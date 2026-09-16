package sheets

import (
	"fmt"
	"sort"
	"strings"
)

// Columns 是「表头标签 → 0 基列号」。
type Columns map[string]int

// ResolveHeader 把一行表头解析成列号表，want 里任何一个标签缺失就整体报错。
//
// 逐张年度表各自解析，不读一张套全部：分年结构的好处就是列布局可以分化
// （新增字段只在新年份加列，不回改历史），所以每张表的表头必须各自解析。
func ResolveHeader(headerRow, want []string) (Columns, error) {
	idx := make(map[string]int, len(headerRow))
	for i, label := range headerRow {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		// 重复标签取**最左**一个：右边那个多半是后加的笔记列，写左边更接近人的预期
		if _, dup := idx[label]; !dup {
			idx[label] = i
		}
	}

	cols := make(Columns, len(want))
	var missing []string
	for _, label := range want {
		i, ok := idx[strings.TrimSpace(label)]
		if !ok {
			missing = append(missing, label)
			continue
		}
		cols[label] = i
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		// 全报而不是只报第一个：只报一个会让人「改一次跑一次」，而表头改动
		// 往往是成片的。
		return nil, fmt.Errorf("表头缺 %d 个标签：%s（表头共 %d 列，请核对年度表第 3 行）",
			len(missing), strings.Join(missing, "、"), len(headerRow))
	}
	return cols, nil
}

// ColumnLetter 把 0 基列号转成 A1 记法的列字母：0→A、25→Z、26→AA、53→BB。
func ColumnLetter(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}
