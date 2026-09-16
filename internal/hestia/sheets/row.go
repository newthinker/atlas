package sheets

// Cell 是一个要写的格。
//
// 🔴 Value 的**动态类型决定单元格类型**：RAW 模式下 JSON 数字存成数字、
// JSON 字符串存成文本。数值列必须给 float64——给 string 会让那格变成文本，
// 而 AJ–BB 那 19 个公式拿文本没办法，**表面上数字还都在**。
type Cell struct {
	Label string // 表头标签，如「社融存量」
	Value any    // float64（数值列）或 string（月份、发布日期）
}

// Row 是一个年度表里的一行。
//
// 🔴 **库里缺的字段不出现在 Cells 里。** 这是 C4「逐格写、不写空」的结构化表达——
// 让规则长在类型上，而不是散在某个 if 里等人忘记。
type Row struct {
	Year  int // 选哪张年度表
	Month int // 1–12，决定行号（行 = Month + 3）
	Cells []Cell
}
