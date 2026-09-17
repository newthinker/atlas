#!/usr/bin/env python3
"""
TASK-008 返工复验的全部变异 —— 验证者 test-m2b-b 事后重建。

⚠️ 为什么是「事后重建」：这一轮我是用内联 python 跑的，脚本随命令消失，
   而 final-report 引用的 N7 对照表（下面的 T1/T2）正来自它。
   报告里的「复现命令」只覆盖跑测试，不覆盖跑变异 ⇒ 那张表原本不可重放。
   本文件把当时的每一个变异原样固化，使结论可独立复核。

用法（在主仓库执行；它自己建/拆 worktree，主工作区一个字节不碰）：
    python3 test-m2b-b-TASK-008r-mutate.py

覆盖：
  T1/T2  N7 对照实验 —— 同一变异下 dev 新夹具 vs 验证者旧夹具（final-report 那张表）
  U1–U7  push.go 六条 error 传播分支 + client.go 的清空新表录入区那步
  N10    createYearTabs 连建两张时本地表序累积
  W1     「手写字面量与夹具脱钩」——测 dev 那条理由（实测证伪：会红，不是静默）
  X1     「报错但仍写」——测零写断言的**独立**鉴别力（六个 U 变异让两条断言同时红，分不清谁在守）
"""
import subprocess, hashlib, os, sys, shutil

BASE = "042c739d93c2c875f246487d63454f7886d651ac"   # TASK-008 返工的 verify_baseline.head
HERE = os.path.abspath(os.path.dirname(__file__))          # 旧夹具与本文件同目录
# 🔴 仓库根不能用脚本所在目录推：本文件会被搬进 .arcforge/docs/06-acceptance/migration-assets/，
#    那时 __file__ 的目录不是仓库根。用 git 自己回答「我在哪棵树里」。
REPO = subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=HERE,
                      capture_output=True, text=True).stdout.strip() \
       or subprocess.run(["git", "rev-parse", "--show-toplevel"],
                         capture_output=True, text=True).stdout.strip()
WT   = None
PU   = "internal/hestia/sheets/push.go"
CL   = "internal/hestia/sheets/client.go"
PT   = "internal/hestia/sheets/push_test.go"
OLD_FIXTURE = "test-m2b-b-TASK-008-fixture.go.txt"   # 与本文件同目录

def sh(c, cwd=None):
    return subprocess.run(c, cwd=cwd or WT, shell=True, capture_output=True, text=True)

def fails(out):
    return sorted({l.split()[2] for l in out.splitlines() if l.startswith("--- FAIL:")})

def sha(p):
    return hashlib.sha256(open(p, 'rb').read()).hexdigest()

def sub(src, old, new, n=1):
    assert src.count(old) == n, f"锚点出现 {src.count(old)} 次（期望 {n}）：{old[:70]!r}"
    return src.replace(old, new)

# ── push.go 的 error 传播块（把 return 换成吞掉继续走）──────────────────────
def swallow(anchor):
    return lambda s: sub(s, anchor, anchor.replace("return res, err", "_ = err"))

A_TABS = '''	tabs, err := c.Tabs(ctx)
	if err != nil {
		return res, err
	}'''
A_DIFF_PRESENT = '''		changes, err := diffTab(ctx, c, name, wantLabels, byTab[name])
		if err != nil {
			return res, err
		}'''
A_CREATE = '''		if err := createYearTabs(ctx, c, tabs, missing); err != nil {
			return res, err
		}'''
A_DIFF_NEW = '''			changes, err := diffTab(ctx, c, name, wantLabels, byTab[name])
			if err != nil {
				return res, err
			}'''
A_WRITE = '''	if err := c.WriteCells(ctx, toWrite); err != nil {
		return res, err
	}'''

def m_nonyear(s):
    return sub(s, '''		year, ok := tabYear(name)
		if !ok {
			return fmt.Errorf("hestia sheets: %q 不是年度表名，不知道该怎么建", name)
		}''', '''		year, _ := tabYear(name)''')

def m_n10(s):
    return sub(s, "\t\torder = slices.Insert(order, idx, name)\n", "")

def m_x1(s):
    """报错但仍写：error 断言保持满足，只有零写断言能发现。"""
    return sub(s, A_TABS, A_TABS.replace(
        "\t\treturn res, err",
        '\t\t_ = c.WriteCells(ctx, []Change{{Sheet: "2026年", Row: 4, Col: 0, Want: "x"}})\n\t\treturn res, err'))

def m_clear_step(s):
    """删掉 CreateYearTab 的第 5 步（清空新表录入区，TASK-006 返工加的）。"""
    import re
    return re.sub(r'\n\t\t// ⑤[^\n]*\n(?:\t\t[^\n]*\n)*?\t\t\{UpdateCells: &sheets\.UpdateCellsRequest\{\n(?:[^\n]*\n)*?\t\t\}\},\n',
                  '\n', s, count=1)

def m_w1(s):
    """把 failPath 的键换成与夹具表名脱钩的手写字面量（dev 说这会静默，实测会红）。"""
    return sub(s, '''	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		pathHeader26: http.StatusBadRequest,
	})''', '''	c, rec := newTestClientFailPath(t, tabsAndHeaderResponses(), map[string]int{
		"/v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3": http.StatusBadRequest,
	})''')

MUTS = [
    ("U1",  PU, "Tabs 失败不返回",                         swallow(A_TABS),          None),
    ("U2",  PU, "已有表 diffTab 失败不返回",                swallow(A_DIFF_PRESENT),  None),
    ("U3",  PU, "🔴 建表失败不返回（fix_items[1] 核心）",    swallow(A_CREATE),        None),
    ("U4",  PU, "新表 diffTab 失败不返回",                  swallow(A_DIFF_NEW),      None),
    ("U5",  PU, "WriteCells 失败不返回",                    swallow(A_WRITE),         None),
    ("U6",  PU, "createYearTabs 收到非年度表名不报错",       m_nonyear,                None),
    ("U7",  CL, "删掉清空新表录入区那步（CRITICAL-2）",      m_clear_step,             None),
    ("N10", PU, "createYearTabs 不更新本地表序",            m_n10,                    None),
    ("X1",  PU, "报错但仍写（测零写断言的独立鉴别力）",       m_x1,  "TestPushStopsWhenTabsFails"),
    ("W1",  PT, "failPath 键与夹具脱钩（测 dev 那条理由）",   m_w1,  "TestPushStopsWhenReadHeaderFails"),
]

# ── N7 对照实验：同一变异，dev 新夹具 vs 验证者旧夹具 ──────────────────────
N7 = [
    ("T1", "newID 由**模板 id** 推（tplID+1）", "\tnewID := maxID + 1", "\tnewID := tplID + 1"),
    ("T2", "newID 用 maxID 不 +1（首验用的那个）", "\tnewID := maxID + 1", "\tnewID := maxID"),
]

def main():
    global WT
    WT = os.path.join(os.path.dirname(REPO.rstrip("/")), "wt-m2b-r008-replay")
    if os.path.exists(WT):
        sh(f"git worktree remove --force {WT}", cwd=REPO)
    r = sh(f"git worktree add --detach {WT} {BASE}", cwd=REPO)
    if r.returncode != 0:
        print("建 worktree 失败：", r.stderr); sys.exit(1)
    print(f"worktree @ {BASE[:12]}  →  {WT}\n")

    orig = {f: open(os.path.join(WT, f), encoding='utf-8').read() for f in (PU, CL, PT)}
    SHA  = {f: hashlib.sha256(s.encode()).hexdigest() for f, s in orig.items()}
    MAIN_SHA = {f: sha(os.path.join(REPO, f)) for f in (PU, CL, PT)}

    print("=" * 78, "\n第一组：fix_items[1][3] 的守卫（只用交付测试集）\n")
    for name, tgt, desc, fn, only in MUTS:
        try:
            mut = fn(orig[tgt])
        except AssertionError as e:
            print(f"{name:4s} ❌ 锚点失配：{e}"); continue
        if mut == orig[tgt]:
            print(f"{name:4s} ❌ 与原文相同，跳过"); continue
        open(os.path.join(WT, tgt), 'w', encoding='utf-8').write(mut)
        if sh(f"GOTOOLCHAIN=local gofmt -e {tgt} > /dev/null").returncode != 0:
            print(f"{name:4s} ❌ 语法闸命中，早退"); sh(f"git checkout -- {tgt}"); continue
        cmd = "GOTOOLCHAIN=local go test ./internal/hestia/sheets/ -count=1 -timeout 300s"
        if only: cmd += f" -run '{only}'"
        t = sh(cmd); comb = t.stdout + t.stderr; f = fails(t.stdout)
        if any(k in comb for k in ("build failed", "declared and not used", "cannot ")):
            print(f"{name:4s} ❌ 有效性闸：编译失败，判无效"); sh(f"git checkout -- {tgt}"); continue
        if t.returncode != 0 and not f:
            print(f"{name:4s} ❌ 有效性闸：exit={t.returncode} 零条 FAIL，判无效"); sh(f"git checkout -- {tgt}"); continue
        print(f"{name:4s} {desc:42s} {'KILLED' if f else '🔴 SURVIVED'}  {f[:3]}")
        sh(f"git checkout -- {tgt}")
        assert sha(os.path.join(WT, tgt)) == SHA[tgt], f"{name} 还原失败"

    print("\n" + "=" * 78)
    print("第二组：N7 对照实验 —— final-report 那张表的出处\n")
    src_fix = os.path.join(HERE, OLD_FIXTURE)
    if not os.path.exists(src_fix):
        print(f"  ⚠️ 找不到旧夹具 {OLD_FIXTURE}，跳过对照组")
    else:
        dst = os.path.join(WT, "internal/hestia/sheets/zz_oldfixture_test.go")
        shutil.copyfile(src_fix, dst)
        # 🔴 对照组必须先在**未变异**的树上为绿，否则它的红不承载任何信息。
        #    旧夹具断言「恰四步」，而 TASK-006 返工给 CreateYearTab 加了第 5 步 ⇒ 本来就红。
        s = open(dst, encoding='utf-8').read()
        s = s.replace('require.Len(t, reqs, 4, "恰四步，不多不少")',
                      'require.Len(t, reqs, 5, "TASK-006 返工后是五步（加了清空新表录入区）")')
        s = s.replace('require.Len(t, batches, 1, "四步必须在**一次** batchUpdate 里（API 原子）")',
                      'require.Len(t, batches, 1, "各步必须在**一次** batchUpdate 里（API 原子）")')
        open(dst, 'w', encoding='utf-8').write(s)
        pre = sh("GOTOOLCHAIN=local go test ./internal/hestia/sheets/ -count=1 "
                 "-run 'TestW_CreateYearTabFourStepsStructurally'")
        ok = not fails(pre.stdout) and pre.returncode == 0
        print(f"  前置：对照组在未变异树上{'为绿 ✅（可用作对照）' if ok else '为红 ❌（不可用，先修好再比）'}")
        if ok:
            print(f"  {'变异':32s} {'dev 新夹具':12s} {'验证者旧夹具'}")
            for name, desc, old, new in N7:
                open(os.path.join(WT, CL), 'w', encoding='utf-8').write(sub(orig[CL], old, new))
                a = sh("GOTOOLCHAIN=local go test ./internal/hestia/sheets/ -count=1 "
                       "-run 'TestCreateYearTabDerivesNewIDFromMaxNotTemplate'")
                b = sh("GOTOOLCHAIN=local go test ./internal/hestia/sheets/ -count=1 "
                       "-run 'TestW_CreateYearTabFourStepsStructurally'")
                print(f"  {name} {desc:29s} {'KILLED' if fails(a.stdout) else '🔴 不红':12s} "
                      f"{'KILLED' if fails(b.stdout) else '🔴 不红 ← 夹具盲区'}")
                sh(f"git checkout -- {CL}")
                assert sha(os.path.join(WT, CL)) == SHA[CL]
        os.remove(dst)

    print("\n" + "=" * 78)
    ok_wt   = all(sha(os.path.join(WT, f)) == SHA[f] for f in orig)
    ok_main = all(sha(os.path.join(REPO, f)) == MAIN_SHA[f] for f in MAIN_SHA)
    print(f"收尾：worktree 还原 {ok_wt} | 主工作区未变 {ok_main}")
    sh(f"git worktree remove --force {WT}", cwd=REPO)
    sh("git worktree prune", cwd=REPO)
    print("worktree 已拆")

if __name__ == "__main__":
    main()
