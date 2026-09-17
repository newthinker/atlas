#!/usr/bin/env python3
# 变异 harness — TASK-010 验证者 test-m2b-b
# 作用对象：隔离 worktree wt-m2b-v010 内的 internal/hestia/ingest.go（主工作区一个字节不碰）
# 每个变异：落盘 → gofmt 语法闸 → go vet → 打印 diff → 跑全包测试 → git checkout 还原 → sha256 校验
import subprocess, sys, hashlib, difflib, os

WT = "/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v010"
MAIN = "/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
TGT = "internal/hestia/ingest.go"
ORIG_SHA = "20a8db13aa08d09af6682945499dc3bf1b67e5f869a6a5c1508391b4ed5f2d0e"
MAIN_SHA = ORIG_SHA

BLOCK = '''		// 投影在契约之后（M2b 的 TASK-010）：契约有下游在等，投影没有。
		// ⚠️ 两个分支都**只打印不返回**：写成 return err 或并进 errors.Join 都违反 C8。
		if d.ProjectSheets != nil {
			rows, berr := buildSheetRows(ctx, d.Store)
			if berr != nil {
				fmt.Fprintf(d.Out, "sheets: 组装失败（不影响入库）: %v\\n", berr)
			} else if perr := d.ProjectSheets(ctx, rows); perr != nil {
				fmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\\n", perr)
			}
		}
'''
CONTRACT_PRINT = '''		fmt.Fprintf(d.Out, "%s contract → %s\\n", obs.Meta.Period, path)\n'''
WRITE_CONTRACT = '''		path, err := WriteContract(d.Cfg.Queue.Dir, BuildContract(in, d.Cfg))\n'''
PROJ_LINE = '''				fmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\\n", perr)\n'''
BUILD_LINE = '''				fmt.Fprintf(d.Out, "sheets: 组装失败（不影响入库）: %v\\n", berr)\n'''
ELSE_IF = '''			} else if perr := d.ProjectSheets(ctx, rows); perr != nil {\n'''

def sub(src, old, new, n=1):
    assert src.count(old) == n, f"锚点出现 {src.count(old)} 次，期望 {n}：{old[:70]!r}"
    return src.replace(old, new)

def m_ret_proj(s):   # M1 投影失败改成 return
    return sub(s, PROJ_LINE, PROJ_LINE + '''				return fail("sheets", perr)\n''')
def m_ret_build(s):  # M2 组装失败改成 return
    return sub(s, BUILD_LINE, BUILD_LINE + '''				return fail("sheets", berr)\n''')
def m_word(s):       # M3 文案改一个字
    return sub(s, "投影失败（不影响入库）", "投影错误（不影响入库）")
def m_paren(s):      # M4 全角括号 → 半角
    return sub(s, "投影失败（不影响入库）", "投影失败(不影响入库)")
def m_space(s):      # M5 冒号后空格删掉
    return sub(s, '''投影失败（不影响入库）: %v''', '''投影失败（不影响入库）:%v''')
def m_nonil(s):      # M6 去掉 nil 判断
    return sub(s, "if d.ProjectSheets != nil {", "if true {")
def m_order(s):      # M7 投影挪到写契约之前（顺序颠倒）
    s = sub(s, BLOCK, "")
    return sub(s, WRITE_CONTRACT, BLOCK + WRITE_CONTRACT)
def m_noelse(s):     # M8 else if → if（组装失败后仍尝试投影）
    return sub(s, ELSE_IF, '''			}\n			if perr := d.ProjectSheets(ctx, rows); perr != nil {\n''')
def m_noerr(s):      # M9 错误详情丢失（%v 填固定串）
    return sub(s, PROJ_LINE, '''				fmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\\n", "see logs")\n''')
def m_outside(s):    # M10 投影挪出 New/Revision 块（Duplicate 也投影）
    s = sub(s, BLOCK, "")
    anchor = '''		temp = Evaluate(obs, d.Cfg.Signals)\n	}\n'''
    return sub(s, anchor, anchor + BLOCK)

def m_build_outside(s):  # M11 nil 判断只包投影调用，组装挪到判断之外（C9「不尝试投影」的边界）
    old = """		if d.ProjectSheets != nil {
			rows, berr := buildSheetRows(ctx, d.Store)
			if berr != nil {"""
    new = """		rows, berr := buildSheetRows(ctx, d.Store)
		if d.ProjectSheets != nil {
			if berr != nil {"""
    return sub(s, old, new)
def m_p1(s):         # M12 投影失败时发 P1 告警（C8「不发 P1」那条断言的靶子）
    return sub(s, PROJ_LINE, PROJ_LINE + """				if d.Notify != nil {
					_ = d.Notify.SendText("[P1] sheets 投影失败")
				}\n""")

MUTS = [
    ("M1", "C8 投影失败支改成 return fail(...)", m_ret_proj),
    ("M2", "C8 组装失败支改成 return fail(...)", m_ret_build),
    ("M3", "文案改一个字：投影失败 → 投影错误", m_word),
    ("M4", "文案全角括号 → 半角括号", m_paren),
    ("M5", "文案冒号后空格删掉", m_space),
    ("M6", "去掉 d.ProjectSheets != nil 判断（C9 失效）", m_nonil),
    ("M7", "投影块挪到 WriteContract 之前（顺序颠倒）", m_order),
    ("M8", "else if → if（组装失败后仍尝试投影）", m_noelse),
    ("M9", "%v 参数换成固定串（丢失错误详情）", m_noerr),
    ("M10", "投影块挪出 New/Revision 块（Duplicate 也投影）", m_outside),
    ("M11", "组装挪到 nil 判断之外（C9 时仍调 BuildSheetRows）", m_build_outside),
    ("M12", "投影失败时发 [P1] 告警", m_p1),
]

def run(cmd, cwd=WT):
    return subprocess.run(cmd, cwd=cwd, shell=True, capture_output=True, text=True)

def sha(path):
    return hashlib.sha256(open(path,'rb').read()).hexdigest()

def fails(output):
    # 仪器订正：`--- FAIL: TestName (0.01s)` 的测试名是第 3 个字段，不是最后一个
    # （首版取 [-1] 得到的是耗时 "(0.01s)"）。
    return sorted({ln.split()[2] for ln in output.splitlines() if ln.startswith("--- FAIL:")})

orig = open(os.path.join(WT, TGT), encoding='utf-8').read()
assert hashlib.sha256(orig.encode()).hexdigest() == ORIG_SHA, "起始文件不是交付版"

for name, desc, fn in MUTS:
    print("=" * 78)
    print(f"{name}  {desc}")
    try:
        mutated = fn(orig)
    except AssertionError as e:
        print(f"  ❌ 锚点失配，跳过：{e}")
        continue
    if mutated == orig:
        print("  ❌ 变异体与原文相同（sha 未变），跳过")
        continue
    open(os.path.join(WT, TGT), 'w', encoding='utf-8').write(mutated)
    # 语义闸：打印 diff 供逐字核对
    d = list(difflib.unified_diff(orig.splitlines(True), mutated.splitlines(True),
                                  'orig', name, n=1))
    print("  --- 变异 diff ---")
    for ln in d[2:]:
        print("   " + ln.rstrip())
    # 语法闸
    g = run(f"GOTOOLCHAIN=local gofmt -e {TGT} > /dev/null")
    if g.returncode != 0:
        print(f"  ❌ 语法闸命中（gofmt -e 非零），该变异体无效，早退\n{g.stderr[:400]}")
        run(f"git checkout -- {TGT}")
        continue
    v = run("GOTOOLCHAIN=local go vet ./internal/hestia/")
    print(f"  go vet: exit={v.returncode} {'（有输出）' if v.stderr.strip() else '（无输出）'}")
    if v.returncode != 0:
        print("   vet stderr 前 400 字：" + v.stderr[:400])
    t = run("GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -timeout 300s")
    f = fails(t.stdout)
    combined = t.stdout + t.stderr
    # 有效性闸：编译失败 / 无任何测试跑起来 ⇒ 既不是 KILLED 也不是 SURVIVED，立即早退
    if "build failed" in combined or "[setup failed]" in combined or "cannot " in combined:
        print("  ❌ 有效性闸命中：该变异体编译失败，判定无效（不计 KILLED/SURVIVED），早退")
        print("   编译输出前 600 字：" + combined[:600])
        run(f"git checkout -- {TGT}")
        continue
    if t.returncode != 0 and not f:
        print(f"  ❌ 有效性闸命中：go test 退出码 {t.returncode} 但零条 --- FAIL，判定无效，早退")
        print("   输出前 600 字：" + combined[:600])
        run(f"git checkout -- {TGT}")
        continue
    verdict = "KILLED" if f else "🔴 SURVIVED"
    print(f"  结果: {verdict}   go test exit={t.returncode}   FAIL 条数={len(f)}")
    for x in f:
        print("    - " + x)
    run(f"git checkout -- {TGT}")
    back = sha(os.path.join(WT, TGT))
    print(f"  还原校验: worktree sha={'OK' if back == ORIG_SHA else '❌ ' + back}"
          f"  主工作区 sha={'OK' if sha(os.path.join(MAIN, TGT)) == MAIN_SHA else '❌ 主工作区被动过'}")

print("=" * 78)
print("收尾：worktree", sha(os.path.join(WT, TGT)) == ORIG_SHA,
      "| 主工作区", sha(os.path.join(MAIN, TGT)) == MAIN_SHA)
