package sheets

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Options 控制一次投影。
type Options struct {
	Apply        bool // false = dry-run
	CreateSheets bool // 允许建缺失年度表（TASK-008 用）
}

// Result 是一次投影的全部产出：三类变更、缺表清单与三类计数。
type Result struct {
	Changes     []Change // 全部三类，供调用方排版
	MissingTabs []string // 缺的年度表名
	WillWrite   int
	Same        int
	AbsentInDB  int
	// DroppedCells 是 Diff **少产出**的格数：期望 len(rows)×len(cols)，实得 len(changes)，两者之差。
	//
	// 🔴 它存在的唯一理由是「跳过必须可观测」。Diff 只返回 []Change，越界行被整行跳过之后
	// 没有任何通道能把「我丢了一行」告诉调用方——而「有数据没被比对」与「比对完发现一致」
	// 在 Result 里此前长得一模一样。
	//
	// 判据取**性质**不取成因：算的是「期望 − 实得」，不是「数一数几行越界」。目前唯一的成因
	// 是 Month ∉ [1,12]，但将来若出现第二种让 Diff 少产出的情形，这个字段同样会亮，
	// 不需要再加一个。非零 ⇒ 三类计数之和不再等于格数，而是等于「格数 − DroppedCells」。
	DroppedCells int
}

// tabName 是年度表的命名约定。
func tabName(year int) string { return fmt.Sprintf("%d年", year) }

// templateYearTab 是建新年度表时复制的模板：结构完整、录入区全空的那一张。
// 名字不叫 templateTab，是为了不和 Client.CreateYearTab 的同名形参混淆。
const templateYearTab = "2024年"

// yearTabPattern 匹配年度表名「2021年」并取出年份。
var yearTabPattern = regexp.MustCompile(`^(\d{4})年$`)

// tabYear 从年度表名「2021年」取出年份；不是年度表名 ⇒ ok=false。
func tabYear(name string) (int, bool) {
	m := yearTabPattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	year, _ := strconv.Atoi(m[1]) // 正则已保证是 4 位数字
	return year, true
}

// createYearTabs 按年份升序逐张建缺失的年度表。index 取「现有年份表升序中第一个 > year 的
// 表序位置，无则末尾」；每建一张就插进本地表序，下一张的 index 才算得对。
func createYearTabs(ctx context.Context, c *Client, existing []string, missing []string) error {
	order := slices.Clone(existing)
	for _, name := range missing {
		year, ok := tabYear(name)
		if !ok {
			return fmt.Errorf("hestia sheets: %q 不是年度表名，不知道该怎么建", name)
		}
		idx := len(order)
		for i, t := range order {
			if y, ok := tabYear(t); ok && y > year {
				idx = i
				break
			}
		}
		if err := c.CreateYearTab(ctx, templateYearTab, name, year, idx); err != nil {
			return err
		}
		order = slices.Insert(order, idx, name)
	}
	return nil
}

// diffTab 对一张已存在的年度表走第 3、4 步：读表头 → ResolveHeader → 读录入区 → Diff。
func diffTab(ctx context.Context, c *Client, name string, wantLabels []string, rows []Row) ([]Change, int, error) {
	header, err := c.ReadHeader(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	cols, err := ResolveHeader(header, wantLabels)
	if err != nil {
		return nil, 0, fmt.Errorf("hestia sheets: %s: %w", name, err)
	}
	current, err := c.ReadEntryArea(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	changes := Diff(name, cols, current, rows)
	// 期望与实得的差就是被 Diff 跳过的格数。在这里算而不是在 Diff 里数：
	// Diff 的签名不动（它被别的调用方用），而这一层本来就同时握着 rows、cols 与结果。
	return changes, len(rows)*len(cols) - len(changes), nil
}

// tally 按三类计数并挑出要写的格。每次调用都从 res.Changes 全量重算，
// 所以第 6 步把新表的变更追加进去之后可以原样再调一次。
// ⚠️ tally **不碰 DroppedCells**：那个字段由 diffTab 逐张累加，不是从 res.Changes 重算的
// （被丢掉的格根本不在 Changes 里，重算不出来）。第 6 步之后 tally 会被再调一次，
// 若在这里清零，新表那一轮就会把前面累计的丢格数抹掉。
func tally(res *Result) []Change {
	res.WillWrite, res.Same, res.AbsentInDB = 0, 0, 0
	var toWrite []Change
	for _, ch := range res.Changes {
		switch ch.Kind {
		case WillWrite:
			res.WillWrite++
			toWrite = append(toWrite, ch)
		case Same:
			res.Same++
		case AbsentInDB:
			res.AbsentInDB++
		}
	}
	return toWrite
}

// Push 编排一次投影。Apply=false 时**一个写请求都不发**。
//
// 顺序固定，每一步都有拒绝的机会：
//  1. Tabs             → 算出缺哪些年度表
//  2. 缺表且 !CreateSheets ⇒ 报错返回（一个写请求都没发）
//  3. 逐张年度表 ReadHeader → ResolveHeader（任一标签缺失 ⇒ 报错返回；C3 在这里兑现）
//  4. 逐张 ReadEntryArea → Diff
//  5. !Apply ⇒ 组装 Result 返回（**到此为止只发过 GET**）
//  6. CreateSheets 且有缺表 ⇒ 建表（TASK-008），再对新表补做 3、4
//  7. WriteCells（只含 WillWrite）
//
// 🔴 第 5 步必须在第 6、7 步之前：dry-run 判断若放进 WriteCells 内部，第 6 步的建表会照样
// 执行——dry-run 却改了表结构，是这个设计里最坏的失败。
func Push(ctx context.Context, c *Client, rows []Row, wantLabels []string, opts Options) (Result, error) {
	var res Result
	if c == nil {
		return res, errors.New("hestia sheets: 客户端为 nil（凭据未配置，能力已禁用）")
	}

	// 1. 缺表
	tabs, err := c.Tabs(ctx)
	if err != nil {
		return res, err
	}
	have := make(map[string]bool, len(tabs))
	for _, t := range tabs {
		have[t] = true
	}
	byTab := make(map[string][]Row)
	for _, r := range rows {
		name := tabName(r.Year)
		byTab[name] = append(byTab[name], r)
	}
	var missing, present []string
	for name := range byTab {
		if have[name] {
			present = append(present, name)
		} else {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(present)
	res.MissingTabs = missing

	// 2. 缺表且不许建 ⇒ 拒绝
	if len(missing) > 0 && !opts.CreateSheets {
		return res, fmt.Errorf("hestia sheets: 缺年度表 %s；加 --create-sheets 允许建表，或先手工建好",
			strings.Join(missing, "、"))
	}

	// 3. 表头；4. diff——先只对已有的表
	for _, name := range present {
		changes, dropped, err := diffTab(ctx, c, name, wantLabels, byTab[name])
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, changes...)
		res.DroppedCells += dropped
	}
	toWrite := tally(&res)

	// 5. dry-run 到此为止：上面只发过 GET
	if !opts.Apply {
		return res, nil
	}

	// 6. 建缺失的年度表（TASK-008），然后对新表补做第 3、4 步：录入区全空 ⇒ 它们的行全是 WillWrite
	if len(missing) > 0 {
		if err := createYearTabs(ctx, c, tabs, missing); err != nil {
			return res, err
		}
		for _, name := range missing {
			changes, dropped, err := diffTab(ctx, c, name, wantLabels, byTab[name])
			if err != nil {
				return res, err
			}
			res.Changes = append(res.Changes, changes...)
			res.DroppedCells += dropped
		}
		toWrite = tally(&res)
	}

	// 7. 只写 WillWrite
	if err := c.WriteCells(ctx, toWrite); err != nil {
		return res, err
	}
	return res, nil
}
