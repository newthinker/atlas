package tiingo

import (
	"os"
	"testing"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// testDefaultGate：零策略闸门（不缓存、不节流、不计配额）。临时 SetDefault 的用例
// 必须在 t.Cleanup 里恢复到它，而不是 nil——nil 会懒构造带 40/h 配额的内置表闸门。
var testDefaultGate *policy.Gate

func TestMain(m *testing.M) {
	testDefaultGate = gateWith(policy.Policy{}, nil)
	policy.SetDefault(testDefaultGate)
	os.Exit(m.Run())
}

func gateWith(p policy.Policy, q policy.QuotaStore) *policy.Gate {
	tbl := policy.NewTable()
	p.Domain = "tiingo"
	tbl.Set(topicDaily, p)
	return policy.New(tbl, q)
}
