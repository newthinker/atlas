package sheets

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
}

// tabName 是年度表的命名约定。
func tabName(year int) string { return fmt.Sprintf("%d年", year) }

// createTabs 是 TASK-008 的建表挂点：Push 在 dry-run 短路之后、WriteCells 之前调用它。
// 本任务不实现建表，默认报错——建表失败时已有表的格也不写，写一半比不写更难收拾。
var createTabs = func(ctx context.Context, c *Client, names []string) error {
	return fmt.Errorf("hestia sheets: 建缺失年度表（%s）尚未实现，见 TASK-008", strings.Join(names, "、"))
}

// Push 编排一次投影。Apply=false 时**一个写请求都不发**。
//
// 顺序固定，每一步都有拒绝的机会：
//  1. Tabs             → 算出缺哪些年度表
//  2. 缺表且 !CreateSheets ⇒ 报错返回（一个写请求都没发）
//  3. 逐张年度表 ReadHeader → ResolveHeader（任一标签缺失 ⇒ 报错返回；C3 在这里兑现）
//  4. 逐张 ReadEntryArea → Diff
//  5. !Apply ⇒ 组装 Result 返回（**到此为止只发过 GET**）
//  6. CreateSheets 且有缺表 ⇒ 建表（TASK-008）
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

	// 3. 表头；4. diff——只对已有的表
	for _, name := range present {
		header, err := c.ReadHeader(ctx, name)
		if err != nil {
			return res, err
		}
		cols, err := ResolveHeader(header, wantLabels)
		if err != nil {
			return res, fmt.Errorf("hestia sheets: %s: %w", name, err)
		}
		current, err := c.ReadEntryArea(ctx, name)
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, Diff(name, cols, current, byTab[name])...)
	}
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

	// 5. dry-run 到此为止：上面只发过 GET
	if !opts.Apply {
		return res, nil
	}

	// 6. 建表（TASK-008）
	if len(missing) > 0 {
		if err := createTabs(ctx, c, missing); err != nil {
			return res, err
		}
	}

	// 7. 只写 WillWrite
	if err := c.WriteCells(ctx, toWrite); err != nil {
		return res, err
	}
	return res, nil
}
