package bank

import "time"

// day 是本包测试共享的日期构造辅助（AD-3：其他测试文件不得重复定义）。
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
