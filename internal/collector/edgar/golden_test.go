package edgar

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   [TASK-001] error_handling[0] AD-18 golden 回归 → TestFetchCompanyFactsGolden
//     既有全部 fixture 的 FetchCompanyFacts 输出(期数 + FiscalPeriod 标签 + M2 既有 8 个字段)
//     在 tag 链改造前后必须逐季逐字段完全一致。基线文件 testdata/golden/*.txt 由改造**之前**
//     的实现生成(git 历史可查),重点覆盖 companyfacts_split / _double_split / _nonsplit_jump ——
//     detectSplits 的输入由单 tag 改为 tag 链后,拆股比例投票与生效日聚簇一旦变化就会漂移
//     EPS/股本归一化,进而波及下游 PE/PB/PS 全序列;既有测试只断言各自关注的少数字段,
//     捕获不到这种全局漂移。
//
// 基线只覆盖 M3 之前就存在的 8 个字段 —— M3 新增的 5 个主干流字段在基线生成时尚不存在,
// 无「改动前」可比,由 TestFetchCompanyFactsMainFlow 等用例单独覆盖。

var updateGolden = flag.Bool("update-golden", false,
	"regenerate testdata/golden/*.txt from the current implementation")

// goldenDump renders facts as one deterministic line per quarter.
//
// 精度取 12 位有效数字:派生 Q4 = FY − Σ三季 的求和次序取决于 map 迭代序(既有实现,
// 非本任务引入),同一输入的末位会有 ~1e-16 相对抖动。12 位既在该噪声之上,又远高于
// 任何真实漂移的量级(拆股归一化变化是 2/4/10 倍级)。
func goldenDump(facts []QuarterlyFact) string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }
	var b strings.Builder
	fmt.Fprintf(&b, "quarters=%d\n", len(facts))
	for _, f := range facts {
		fmt.Fprintf(&b, "%s|%s|%s|eps=%s|rev=%s|ni=%s|eq=%s|shares=%s\n",
			f.FiscalPeriod, f.PeriodEnd.Format("2006-01-02"), f.FilingDate.Format("2006-01-02"),
			num(f.EPSDiluted), num(f.Revenue), num(f.NetIncome), num(f.Equity), num(f.DilutedShares))
	}
	return b.String()
}

// goldenFixtures 是 M3 tag 链改造**之前**就存在的全部 9 个 fixture ——只有它们有「改动前」
// 基线可比。写死清单(而非 glob)以免本任务新增的 fixture 混进来稀释 AD-18 的语义。
var goldenFixtures = []string{
	"companyfacts_double_split",
	"companyfacts_mini",
	"companyfacts_nonsplit_jump",
	"companyfacts_partial_fy",
	"companyfacts_q4guard",
	"companyfacts_rev_fallback",
	"companyfacts_rev_fpfy",
	"companyfacts_shares_noq4",
	"companyfacts_split",
}

func TestFetchCompanyFactsGolden(t *testing.T) {
	for _, name := range goldenFixtures {
		t.Run(name, func(t *testing.T) {
			srv, _ := factsFileServer(t, filepath.Join("testdata", name+".json"),
				"/api/xbrl/companyfacts/CIK0001045810.json")
			defer srv.Close()
			c := NewWithBaseURL("ua (t@e.com)", srv.URL)
			facts, err := c.FetchCompanyFacts("1045810")
			require.NoError(t, err)
			got := goldenDump(facts)

			path := filepath.Join("testdata", "golden", name+".txt")
			if *updateGolden {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "缺少 golden 基线,先用 -update-golden 在改动前生成")
			assert.Equal(t, string(want), got, "%s 输出相对改动前的基线发生漂移(AD-18)", name)
		})
	}
}
