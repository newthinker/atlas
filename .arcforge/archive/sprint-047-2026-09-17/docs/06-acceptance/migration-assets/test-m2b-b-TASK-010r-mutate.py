#!/usr/bin/env python3
# 返工复验变异 harness — TASK-010 review_fix 第 1 轮 · 验证者 test-m2b-b
# 作用对象：隔离 worktree wt-m2b-r010 的 ingest.go / sheets_project.go（主工作区不碰）
import subprocess, hashlib, difflib, os

WT = "/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-r010"
MAIN = "/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
IG = "internal/hestia/ingest.go"
SP = "internal/hestia/sheets_project.go"
PKG = "./internal/hestia/"

def rd(f): return open(os.path.join(WT, f), encoding='utf-8').read()
def sha(p): return hashlib.sha256(open(p,'rb').read()).hexdigest()
def run(c, cwd=WT): return subprocess.run(c, cwd=cwd, shell=True, capture_output=True, text=True)

ORIG = {IG: rd(IG), SP: rd(SP)}
SHA = {f: hashlib.sha256(s.encode()).hexdigest() for f, s in ORIG.items()}
print("起始 sha:", {os.path.basename(k): v[:12] for k, v in SHA.items()})

def sub(s, old, new, n=1):
    assert s.count(old) == n, f"锚点 {s.count(old)} 次，期望 {n}: {old[:60]!r}"
    return s.replace(old, new)

RECOVER_BODY = '''	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return f()
'''
BUILD_WRAPPED = '''			var rows []sheets.Row
			berr := recoverPanic(func() (err error) {
				rows, err = buildSheetRows(ctx, d.Store)
				return err
			})
'''
PROJ_WRAPPED = '''			} else if perr := recoverPanic(func() error { return d.ProjectSheets(ctx, rows) }); perr != nil {
'''
NIL_GUARD = '''		if d.ProjectSheets != nil {
'''
PERIOD_ERR1 = '''		return 0, 0, fmt.Errorf("hestia sheets: period %q 不是 YYYY-MM 形态，无法定位年度表与行号", period)
'''
PERIOD_ERR2 = '''		return 0, 0, fmt.Errorf("hestia sheets: period %q 的年或月不是合法数字（月份须 01–12）", period)
'''
MONTH_RANGE = '''	if yerr != nil || merr != nil || month < 1 || month > 12 {
'''
ASSEMBLE = '''		row, err := buildRow(obs)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
'''
WRITE_HISTORY = '''		if _, err := WriteHistory(d.Cfg.Queue.Dir, hist); err != nil {
			return fail("contract", contractError{err: err})
		}
'''
PROJ_BLOCK = '''		// 投影在契约之后（M2b 的 TASK-010）：契约有下游在等，投影没有。'''
GLOB_EXCLUDE = None  # 在测试文件里，不在本 harness 的变异面

MUTS = [
 ("R1", IG, "recoverPanic 吞掉 panic（err 不赋值）",
  lambda s: sub(s, RECOVER_BODY, '''	defer func() {
		_ = recover()
	}()
	return f()
''')),
 ("R2", IG, "组装那步去掉 recoverPanic（恢复直接调用）",
  lambda s: sub(s, BUILD_WRAPPED, '''			rows, berr := buildSheetRows(ctx, d.Store)
''')),
 ("R3", IG, "投影那步去掉 recoverPanic",
  lambda s: sub(s, PROJ_WRAPPED, '''			} else if perr := d.ProjectSheets(ctx, rows); perr != nil {
''')),
 ("R4", SP, "periodYearMonth 形态错时折 0 不报错（QA 字面读法）",
  lambda s: sub(s, PERIOD_ERR1, '''		return 0, 0, nil
''')),
 ("R5", SP, "periodYearMonth 数字错时折 0 不报错",
  lambda s: sub(s, PERIOD_ERR2, '''		return 0, 0, nil
''')),
 ("R6", SP, "periodYearMonth 去掉月份 1–12 范围检查",
  lambda s: sub(s, MONTH_RANGE, '''	if yerr != nil || merr != nil {
''')),
 ("R7", SP, "assembleRows 忽略 buildRow 的 error",
  lambda s: sub(s, ASSEMBLE, '''		row, _ := buildRow(obs)
		rows = append(rows, row)
''')),
 # 🔴 R8 首版锚点定错：只把 `var rows` 声明挪出去，buildSheetRows 调用仍在 if 内 ⇒ 等价变异。
 # 订正为把**整个组装块**（含 recoverPanic 调用）挪到 nil 判断之外。
 ("R8", IG, "M11：整个组装块挪到 nil 判断之外（能力禁用时仍调 buildSheetRows）",
  lambda s: sub(s, '''		if d.ProjectSheets != nil {
			var rows []sheets.Row
			berr := recoverPanic(func() (err error) {
				rows, err = buildSheetRows(ctx, d.Store)
				return err
			})
			if berr != nil {''',
                '''		var rows []sheets.Row
		berr := recoverPanic(func() (err error) {
			rows, err = buildSheetRows(ctx, d.Store)
			return err
		})
		if d.ProjectSheets != nil {
			if berr != nil {''')),
 ("R9", IG, "顺序：投影块挪到写契约之前（= 首验的 M7，fix_items[4] 要能杀它）",
  lambda s: _move_before_contract(s)),
 ("R10", IG, "侧车不再写出（模拟「夹具不产侧车」，验失效告警能不能响）",
  lambda s: sub(s, WRITE_HISTORY, '''		_ = hist
''')),
]

def _move_before_contract(s):
    i = s.index(PROJ_BLOCK)
    j = s.index('\t\ttemp = Evaluate(obs, d.Cfg.Signals)', i)
    block = s[i:j]
    s2 = s[:i] + s[j:]
    anchor = '\t\tpath, err := WriteContract(d.Cfg.Queue.Dir, BuildContract(in, d.Cfg))\n'
    assert s2.count(anchor) == 1
    return s2.replace(anchor, block + anchor)

def fails(out):
    return sorted({l.split()[2] for l in out.splitlines() if l.startswith("--- FAIL:")})

for name, tgt, desc, fn in MUTS:
    print("=" * 78)
    print(f"{name}  [{os.path.basename(tgt)}]  {desc}")
    try:
        mut = fn(ORIG[tgt])
    except Exception as e:
        print(f"  ❌ 锚点失配，跳过：{e}"); continue
    if mut == ORIG[tgt]:
        print("  ❌ 与原文相同，跳过"); continue
    open(os.path.join(WT, tgt), 'w', encoding='utf-8').write(mut)
    d = list(difflib.unified_diff(ORIG[tgt].splitlines(True), mut.splitlines(True), 'orig', name, n=1))
    print("  --- diff ---")
    for l in d[2:]:
        print("   " + l.rstrip())
    g = run(f"GOTOOLCHAIN=local gofmt -e {tgt} > /dev/null")
    if g.returncode != 0:
        print("  ❌ 语法闸命中，早退"); run(f"git checkout -- {tgt}"); continue
    v = run(f"GOTOOLCHAIN=local go vet {PKG}")
    t = run(f"GOTOOLCHAIN=local go test {PKG} -count=1 -timeout 300s")
    comb = t.stdout + t.stderr
    f = fails(t.stdout)
    if any(k in comb for k in ("build failed", "[setup failed]", "declared and not used", "cannot ")):
        print("  ❌ 有效性闸：编译失败，判无效，早退"); print("   " + comb[:300])
        run(f"git checkout -- {tgt}"); continue
    if t.returncode != 0 and not f:
        print(f"  ❌ 有效性闸：exit={t.returncode} 零条 FAIL，判无效，早退"); print("   " + comb[:300])
        run(f"git checkout -- {tgt}"); continue
    print(f"  go vet exit={v.returncode}   结果: {'KILLED' if f else '🔴 SURVIVED'}   FAIL 条数={len(f)}")
    for x in f: print("    - " + x)
    run(f"git checkout -- {tgt}")
    print(f"  还原: wt={'OK' if sha(os.path.join(WT,tgt))==SHA[tgt] else '❌'}  "
          f"main={'OK' if all(sha(os.path.join(MAIN,k))==v2 for k,v2 in SHA.items()) else '❌ 主工作区被动过'}")

print("=" * 78)
print("收尾：wt", all(sha(os.path.join(WT,k))==v for k,v in SHA.items()),
      "| main", all(sha(os.path.join(MAIN,k))==v for k,v in SHA.items()))
