#!/usr/bin/env python3
# 变异 harness — TASK-008 验证者 test-m2b-b
# 作用对象：隔离 worktree wt-m2b-v008 内的 client.go / push.go（主工作区一个字节不碰）
import subprocess, hashlib, difflib, os

WT = "/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v008"
MAIN = "/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
CLIENT = "internal/hestia/sheets/client.go"
PUSH = "internal/hestia/sheets/push.go"
SHA = {CLIENT: "f000584721af6ce9ef70b19f466995f468fb4d6d5331e817e8537dd5bfa684b0",
       PUSH:   "d15fbc37880532ba307100aaef855dcd958c22aa5fc87eec7afd3a8f4630f88c"}

def rd(f): return open(os.path.join(WT, f), encoding='utf-8').read()
def sha(path): return hashlib.sha256(open(path, 'rb').read()).hexdigest()
def run(cmd, cwd=WT): return subprocess.run(cmd, cwd=cwd, shell=True, capture_output=True, text=True)

ORIG = {CLIENT: rd(CLIENT), PUSH: rd(PUSH)}
for f, h in SHA.items():
    assert hashlib.sha256(ORIG[f].encode()).hexdigest() == h, f"{f} 不是交付版"

def sub(src, old, new, n=1):
    assert src.count(old) == n, f"锚点出现 {src.count(old)} 次，期望 {n}: {old[:60]!r}"
    return src.replace(old, new)

# —— client.go 的四步请求字面量 ——
R_DUP = '''		// ① 复制模板
		{DuplicateSheet: &sheets.DuplicateSheetRequest{SourceSheetId: tplID, NewSheetId: newID}},
'''
R_RENAME = '''		// ② 改名
		{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: newID, Title: newTab},
			Fields:     "title",
		}},
'''
R_TITLE = '''		// ③ 改表内第 1 行标题
		{UpdateCells: &sheets.UpdateCellsRequest{
			Range:  &sheets.GridRange{SheetId: newID, StartRowIndex: 0, EndRowIndex: 1, StartColumnIndex: 0, EndColumnIndex: 1},
			Rows:   []*sheets.RowData{{Values: []*sheets.CellData{{UserEnteredValue: &sheets.ExtendedValue{StringValue: &newTitle}}}}},
			Fields: "userEnteredValue",
		}},
'''
R_INDEX = '''		// ④ 按年序排位。index 0 是「排最前」，omitempty 会把它吞掉，必须 ForceSendFields
		{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: newID, Index: int64(index), ForceSendFields: []string{"Index"}},
			Fields:     "index",
		}},
'''
# —— push.go 的块 ——
BLOCK6 = '''	// 6. 建缺失的年度表（TASK-008），然后对新表补做第 3、4 步：录入区全空 ⇒ 它们的行全是 WillWrite
	if len(missing) > 0 {
		if err := createYearTabs(ctx, c, tabs, missing); err != nil {
			return res, err
		}
		for _, name := range missing {
			changes, err := diffTab(ctx, c, name, wantLabels, byTab[name])
			if err != nil {
				return res, err
			}
			res.Changes = append(res.Changes, changes...)
		}
		toWrite = tally(&res)
	}

'''
BLOCK7 = '''	// 7. 只写 WillWrite
	if err := c.WriteCells(ctx, toWrite); err != nil {
		return res, err
	}
'''
DRYRUN = '''	// 5. dry-run 到此为止：上面只发过 GET
	if !opts.Apply {
		return res, nil
	}

'''
REFILL = '''		for _, name := range missing {
			changes, err := diffTab(ctx, c, name, wantLabels, byTab[name])
			if err != nil {
				return res, err
			}
			res.Changes = append(res.Changes, changes...)
		}
		toWrite = tally(&res)
'''

MUTS = [
    ("N1",  CLIENT, "去掉 ② 改名请求（表会叫「2024年 的副本」）", lambda s: sub(s, R_RENAME, "")),
    ("N2",  CLIENT, "③ 写模板原标题而非新标题（漏掉年份替换；去掉整个请求会因 newTitle 未使用而编译失败，故取这个等价意图）",
     lambda s: sub(sub(s, "StringValue: &newTitle", "StringValue: &title"),
                   "\tif _, err := c.svc.Spreadsheets.BatchUpdate(",
                   "\t_ = newTitle\n\tif _, err := c.svc.Spreadsheets.BatchUpdate(")),
    ("N3",  CLIENT, "去掉 ④ 排位请求（新表堆在末尾）", lambda s: sub(s, R_INDEX, "")),
    ("N4",  CLIENT, "去掉 ① 复制模板请求", lambda s: sub(s, R_DUP, "")),
    ("N5",  CLIENT, "去掉 ForceSendFields（index 0 被 omitempty 吞掉）",
     lambda s: sub(s, ', ForceSendFields: []string{"Index"}', "")),
    ("N6",  CLIENT, "标题恒走拼接分支（不做年份替换）",
     lambda s: sub(s, "if titleYear.MatchString(title) {", "if false {")),
    ("N7",  CLIENT, "新表 id 用 maxID 而不是 maxID+1（与模板/现有表冲突）",
     lambda s: sub(s, "newID := maxID + 1", "newID := maxID")),
    ("N8",  CLIENT, "模板不存在时不报错，照样往下建",
     lambda s: sub(s, "if tplID < 0 {", "if tplID < -1 {")),
    ("N9",  PUSH,   "第 6 步建表挪到第 7 步写数据之后（007 的 M5 等价变异）",
     lambda s: sub(sub(s, BLOCK6, ""), BLOCK7, BLOCK7 + "\n" + BLOCK6)),
    ("N10", PUSH,   "createYearTabs 不把已建的表插进本地表序",
     lambda s: sub(s, "\t\torder = slices.Insert(order, idx, name)\n", "")),
    ("N11", PUSH,   "index 算法 y > year 改成 y >= year",
     lambda s: sub(s, "if y, ok := tabYear(t); ok && y > year {", "if y, ok := tabYear(t); ok && y >= year {")),
    ("N12", PUSH,   "建完表不补做第 3、4 步（新表的行不写）", lambda s: sub(s, REFILL, "")),
    ("N13", PUSH,   "dry-run 短路挪到第 6 步之后（dry-run 会改表结构）",
     lambda s: sub(sub(s, DRYRUN, ""), BLOCK6, BLOCK6 + DRYRUN)),
    ("N14", PUSH,   "第 2 步去掉 CreateSheets 守卫（没给 flag 也建表）",
     lambda s: sub(s, "if len(missing) > 0 && !opts.CreateSheets {", "if false {")),
    # N15 不改被测逻辑，而是检验 non_functional[0] 的那道写口守卫**此刻真的在守**：
    # 往 sheets 包加一个未登记的导出方法，TestPackageExposesNoWriteFunctions 必须变红。
    ("N15", CLIENT, "新增未登记的导出方法 Client.DeleteTab（写口守卫应当变红）",
     lambda s: s + "\n// 变异注入：未在 store_test.go 的 want 名单里登记的导出方法。\nfunc (c *Client) DeleteTab(ctx context.Context, tab string) error { return nil }\n",
     "./internal/hestia/"),
]

def fails(out):
    return sorted({ln.split()[2] for ln in out.splitlines() if ln.startswith("--- FAIL:")})

for entry in MUTS:
    name, tgt, desc, fn = entry[0], entry[1], entry[2], entry[3]
    pkg = entry[4] if len(entry) > 4 else "./internal/hestia/sheets/"
    print("=" * 78)
    print(f"{name}  [{os.path.basename(tgt)}]  {desc}")
    try:
        mutated = fn(ORIG[tgt])
    except AssertionError as e:
        print(f"  ❌ 锚点失配，跳过：{e}")
        continue
    if mutated == ORIG[tgt]:
        print("  ❌ 变异体与原文相同，跳过")
        continue
    open(os.path.join(WT, tgt), 'w', encoding='utf-8').write(mutated)
    d = list(difflib.unified_diff(ORIG[tgt].splitlines(True), mutated.splitlines(True), 'orig', name, n=1))
    print("  --- 变异 diff ---")
    for ln in d[2:]:
        print("   " + ln.rstrip())
    g = run(f"GOTOOLCHAIN=local gofmt -e {tgt} > /dev/null")
    if g.returncode != 0:
        print(f"  ❌ 语法闸命中，早退\n{g.stderr[:400]}")
        run(f"git checkout -- {tgt}"); continue
    v = run(f"GOTOOLCHAIN=local go vet {pkg}")
    print(f"  go vet: exit={v.returncode}{'（有输出）' if v.stderr.strip() else '（无输出）'}")
    t = run(f"GOTOOLCHAIN=local go test {pkg} -count=1 -timeout 300s")
    combined = t.stdout + t.stderr
    f = fails(t.stdout)
    if "build failed" in combined or "[setup failed]" in combined or "cannot " in combined:
        print("  ❌ 有效性闸命中：编译失败，判定无效，早退")
        print("   " + combined[:400])
        run(f"git checkout -- {tgt}"); continue
    if t.returncode != 0 and not f:
        print(f"  ❌ 有效性闸命中：exit={t.returncode} 但零条 --- FAIL，判定无效，早退")
        print("   " + combined[:400])
        run(f"git checkout -- {tgt}"); continue
    print(f"  结果: {'KILLED' if f else '🔴 SURVIVED'}   go test exit={t.returncode}   FAIL 条数={len(f)}")
    for x in f:
        print("    - " + x)
    run(f"git checkout -- {tgt}")
    ok_wt = sha(os.path.join(WT, tgt)) == SHA[tgt]
    ok_main = all(sha(os.path.join(MAIN, k)) == v for k, v in SHA.items())
    print(f"  还原校验: worktree={'OK' if ok_wt else '❌'}  主工作区={'OK' if ok_main else '❌ 被动过'}")

print("=" * 78)
print("收尾：worktree", all(sha(os.path.join(WT, k)) == v for k, v in SHA.items()),
      "| 主工作区", all(sha(os.path.join(MAIN, k)) == v for k, v in SHA.items()))
