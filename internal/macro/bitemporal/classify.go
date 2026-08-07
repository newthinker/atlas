package bitemporal

import "fmt"

// State 是某个业务键在库中的现状，由 Lookup 填充。
type State struct {
	Exists         bool
	LatestRevision string // Exists 为 false 时为空串
}

// Verdict 是一次待写入相对库中现状的定性。
type Verdict int

const (
	New        Verdict = iota // 业务键首次出现
	Duplicate                 // 同键、同 revision
	Revision                  // 同键、更新的 revision
	OutOfOrder                // 同键、更旧的 revision
)

func (v Verdict) String() string {
	switch v {
	case New:
		return "New"
	case Duplicate:
		return "Duplicate"
	case Revision:
		return "Revision"
	case OutOfOrder:
		return "OutOfOrder"
	}
	// 兜底：未定义取值也要在日志里看得出是哪个数，而不是空串。
	return fmt.Sprintf("Verdict(%d)", int(v))
}

// Classify 判定一次待写入是新增、重复、修订还是乱序。
//
// 为什么这个判断值得单独成函数：央行会迁移站点，同一篇报告换新 URL 重新出现，
// 此时发布日不变——必须判为 Duplicate 而不写观测行。若把它埋进 Lookup 的
// if 分支，一次站点迁移就会让全部历史期次重复入库、且每期都被误判为修订。
//
// revision 按字符串比较，前提是取值为 ISO 8601 日期或时间戳（YYYY-MM-DD…），
// 其字典序与时间序一致。本包的两个使用者都满足：published_at 是发布日，
// fetched_at 是 RFC3339 时间戳。
func Classify(s State, incoming string) Verdict {
	// 业务键不存在时一律新增，绝不比较 revision：调用方若把 LatestRevision
	// 填成了残值，比较会把一次新增误判成重复或乱序，直接丢数据。
	if !s.Exists {
		return New
	}
	switch {
	case incoming == s.LatestRevision:
		return Duplicate
	case incoming > s.LatestRevision:
		return Revision
	default:
		return OutOfOrder
	}
}
