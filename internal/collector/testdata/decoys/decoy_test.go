// 本文件是假阳性夹具的第三种形态：**测试文件里的结构体字段**。
//
// DoD boundary[0] 要求「扫描范围只覆盖生产源码，不误报测试文件中出于验证目的
// 出现的同名标识符」—— 各 collector 的 gate_test.go 里确实会为了验证节流而提到
// 这些名字（如 yahoo/gate_test.go:291 的注释「把 yahoo 域的 lastReq 置为刚刚」）。
package decoys

import "time"

type inTestOnly struct {
	lastReq     time.Time
	minInterval time.Duration
}
