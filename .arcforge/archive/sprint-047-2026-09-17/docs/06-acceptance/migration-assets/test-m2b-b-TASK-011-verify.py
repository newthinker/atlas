#!/usr/bin/env python3
"""
TASK-011（docs-only）验证复算 —— 验证者 test-m2b-b 事后重建。

⚠️ 为什么后补：011 的 DoD 六条全是 `verify_by: review`，没有变异表，结论靠一张
   **22 项自证复算表**；而那些复算当时全是内联 python / shell，脚本随命令消失。
   报告的「复现命令」一节只覆盖其中约三分之一（numstat / 样例配置装载 / 选行 / 全仓 /
   两个 git show），**解析类的那批（守卫集合差、判据表计数、注释逐字比对）一条都没落盘**。
   ⇒ 这是「清单里的每一项都对」与「清单本身完整」取相同值的又一例：
     缺失项在清单里没有坐标，只能从「结论要重放需要什么」倒着查才看得见。

用法（在主仓库执行；自建自拆 worktree，主工作区不碰）：
    python3 test-m2b-b-TASK-011-verify.py

仓库外依赖（找不到即 SKIP 并说明，不算失败）：
  · 需求原文 plans/2026-09-16-hestia-m2b-sheets.md（三条 🔴 注释的逐字比对）
  · runtime 库 / 主仓库库（选行 77 vs 76 的两库对照）
"""
import subprocess, os, re, sys, json

BASE   = "d42435241b3c7a24c10d2b016db17c364b325631"   # 011 的 verify_baseline.head
DELIV  = "d3d1f5794e95f8dcacba9eec73125d59cdc2beaf"   # 交付 commit
PREV   = "477664a5651449490ddc602c090501bfd2ab9ead"   # 前一个 commit
SPRINT_START = "1c7af81"                              # TASK-001 的父提交
T001   = "5a2c1a2"                                    # TASK-001 本身（AST 守卫改递归）
REQ_DOC = "/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-16-hestia-m2b-sheets.md"
RUNTIME_DB = "/Users/zuowei/workspace/runtime/atlas/data/hestia.db"

HERE = os.path.abspath(os.path.dirname(__file__))
REPO = (subprocess.run(["git","rev-parse","--show-toplevel"], cwd=HERE, capture_output=True, text=True).stdout.strip()
        or subprocess.run(["git","rev-parse","--show-toplevel"], capture_output=True, text=True).stdout.strip())
WT = None
PASS = FAIL = SKIP = 0

def sh(c, cwd=None):
    return subprocess.run(c, cwd=cwd or WT, shell=True, capture_output=True, text=True)

def check(name, got, want, note=""):
    global PASS, FAIL
    ok = got == want
    PASS, FAIL = PASS + ok, FAIL + (not ok)
    print(f"  {'✅' if ok else '❌'} {name:46s} 实测 {got!r}" + (f"  期望 {want!r}" if not ok else "") + (f"   {note}" if note else ""))

def skip(name, why):
    global SKIP
    SKIP += 1
    print(f"  ⊘  {name:46s} SKIP —— {why}")

def want_lens(ref):
    """解析某 commit 的 store_test.go，返回两条守卫 want 的元素集合。"""
    src = sh(f"git show {ref}:internal/hestia/store_test.go").stdout
    out = {}
    for fn in ("TestStoreExposesNoWriteMethods", "TestPackageExposesNoWriteFunctions"):
        i = src.find(f"func {fn}(")
        if i < 0: out[fn] = None; continue
        m = re.search(r"want\s*(?::=|=)\s*\[\]string\{", src[i:])
        start = i + m.end(); depth = 1; j = start
        while depth:
            if src[j] == "{": depth += 1
            elif src[j] == "}": depth -= 1
            j += 1
        out[fn] = re.findall(r'"([^"]*)"', src[start:j-1])
    return out

def main():
    global WT
    WT = os.path.join(os.path.dirname(REPO.rstrip("/")), "wt-m2b-v011-replay")
    if os.path.exists(WT): sh(f"git worktree remove --force {WT}", cwd=REPO)
    if sh(f"git worktree add --detach {WT} {BASE}", cwd=REPO).returncode != 0:
        print("建 worktree 失败"); sys.exit(1)
    print(f"worktree @ {BASE[:12]}  →  {WT}\n" + "="*78)

    print("\n[A] 范围与纯追加")
    ns = [l.split("\t") for l in sh(f"git diff --numstat {PREV}..{DELIV}").stdout.strip().splitlines()]
    check("writes 三文件、删除列全 0", sorted((a, f.split('/')[-1]) for a, d, f in ns if d == "0"),
          sorted([("21","config.example.yaml"),("145","TASK-011-vault-content.md"),("174","CONTRACTS.md")]))
    check("改动文件数", len(ns), 3)
    hunks = [l for l in sh(f"git diff --unified=0 {PREV}..{DELIV} -- internal/hestia/CONTRACTS.md").stdout.splitlines() if l.startswith("@@")]
    check("CONTRACTS 只有一个 hunk（末尾追加）", len(hunks), 1)
    check("hunk 头即 -4039,0 +4040,174", hunks[0].split("@@")[1].strip() if hunks else "", "-4039,0 +4040,174")
    before = len(sh(f"git show {PREV}:internal/hestia/CONTRACTS.md").stdout.splitlines())
    after  = len(open(os.path.join(WT, "internal/hestia/CONTRACTS.md"), encoding="utf-8").read().splitlines())
    check("行数自洽 before+174 == after", before + 174, after, f"({before}+174)")

    print("\n[B] 导出面增量与历史断言（集合差，非计数）")
    base, head = want_lens(SPRINT_START), want_lens(BASE)
    for fn, short, exp_b, exp_h in (("TestPackageExposesNoWriteFunctions","AST",38,51),
                                    ("TestStoreExposesNoWriteMethods","reflect",14,15)):
        b, h = set(base[fn]), set(head[fn])
        check(f"{short} 守卫 起点→现在", (len(b), len(h)), (exp_b, exp_h))
        check(f"{short} 移除项数（须 0）", len(b - h), 0)
        if short == "AST":
            added = sorted(h - b)
            check("AST 新增 13 项", len(added), 13)
            check("其中 sheets.* 11 项", len([x for x in added if x.startswith("sheets.")]), 11)
    for ref, label in ((SPRINT_START,"001 的父"), (T001,"001 本身")):
        w = want_lens(ref)
        check(f"历史断言：{label} AST 长度恒 38",
              len(w["TestPackageExposesNoWriteFunctions"]), 38,
              "⇐ 改递归那次一个数没动" if ref == T001 else "")

    print("\n[C] CONTRACTS 的自证（判据表 / 测试名 / 行号引用）")
    c = open(os.path.join(WT, "internal/hestia/CONTRACTS.md"), encoding="utf-8").read()
    seg = c[c.index("## Sprint M2b-2"):]
    check("判据表「待人回填」处数", seg.count("待人回填"), 4)
    check("判据表「✅ 2026-09-17 dev-m2b-c」处数", seg.count("✅ 2026-09-17 dev-m2b-c"), 3)
    check("判据行数 7，且 4+3 自洽", len(re.findall(r"^\| [一二三四五六七] \|", seg, re.M)), 7)
    check("TestW_ 全仓命中（须 0）",
          int(sh("grep -rn 'TestW_' --include='*.go' . | wc -l").stdout.strip()), 0,
          "⇐ 那是验证者 worktree 里的临时测试")
    for nm, path, line in (("TestPushDryRunSendsNoWriteRequest","internal/hestia/sheets/push_test.go",117),
                           ("TestPushDryRunReportsMissingTabsWithCreateFlag","internal/hestia/sheets/push_test.go",167),
                           ("TestSheetsPushDefaultsToDryRun","cmd/atlas/hestia_sheets_test.go",240)):
        got = sh(f"grep -n '^func {nm}(' {path} | cut -d: -f1").stdout.strip()
        check(f"⑤ {nm[:38]} 行号", got, str(line))
    for path, line, needle in (("internal/hestia/store_test.go",391,"func TestStoreExposesNoWriteMethods"),
                               ("internal/hestia/store_test.go",412,"func TestPackageExposesNoWriteFunctions"),
                               ("internal/hestia/exported_funcs_test.go",38,"exportedFuncs"),
                               ("internal/hestia/sheets/client.go",21,"entryLastCol"),
                               ("internal/hestia/config.go",85,"hestia_sheets"),
                               ("scripts/ops/deploy.sh",100,"configs/config.yaml")):
        lines = open(os.path.join(WT, path), encoding="utf-8").read().splitlines()
        check(f"行号引用 {os.path.basename(path)}:{line}", needle in lines[line-1], True)
    sv = open(os.path.join(WT, "internal/hestia/sheets/diff.go"), encoding="utf-8").read().splitlines()
    check("sameValue 函数体恰在 diff.go:100-108",
          (sv[99].startswith("func sameValue"), sv[107].strip() == "}"), (True, True))
    ig = open(os.path.join(WT, "internal/hestia/ingest.go"), encoding="utf-8").read().splitlines()
    check("C8 投影块恰在 ingest.go:482-489",
          (ig[481].strip().startswith("if d.ProjectSheets != nil"), ig[488].strip() == "}"), (True, True))

    print("\n[D] 未决项三条陈述")
    check("hestia_sheets 只被 config.go:85 读",
          "hestia_sheets" in open(os.path.join(WT,"internal/hestia/config.go"),encoding="utf-8").read().splitlines()[84], True)
    check("internal/config 无 HestiaSheets 字段（grep 须 0）",
          int(sh("grep -rn 'HestiaSheets' internal/config/ | wc -l").stdout.strip()), 0)
    check("configs/hestia.yaml 已被 git 跟踪",
          sh("git ls-files configs/hestia.yaml").stdout.strip(), "configs/hestia.yaml")
    check("rsync 排除表里 hestia.yaml 命中（须 0）",
          int(sh("grep -c 'hestia.yaml' scripts/ops/deploy.sh").stdout.strip() or 0), 0)
    check("而 /configs/config.yaml 在 deploy.sh:100",
          "--exclude='/configs/config.yaml'" in open(os.path.join(WT,"scripts/ops/deploy.sh"),encoding="utf-8").read().splitlines()[99], True)

    print("\n[E] 交付内容")
    check("docs/hestia-m2b/ 在 sprint 起点不存在（新建）",
          sh(f"git ls-tree {SPRINT_START} -- docs/hestia-m2b/ | wc -l").stdout.strip(), "0")
    check("commit subject 以 docs(TASK-011): 开头",
          sh(f"git show -s --format=%s {DELIV}").stdout.startswith("docs(TASK-011): "), True)
    v = open(os.path.join(WT,"docs/hestia-m2b/TASK-011-vault-content.md"),encoding="utf-8").read()
    check("vault 两段标题齐全",
          ("## → Projects/Hestia/README.md" in v, "## → M2b-Sheets-接入手册.md" in v), (True, True))
    # 🔴 按**行首**数：全文 count 会把「粘贴后自查」清单里**引用**这个字符串的那一行也算进来
    #    （它以 "3. 手册 §七…" 开头）。正文那三处都在行首。
    check("vault 含 D1/D2/D3 三段「已实现」",
          len([l for l in v.splitlines() if l.startswith("**已实现**（2026-09-17）")]), 3)
    ex = open(os.path.join(WT,"configs/config.example.yaml"),encoding="utf-8").read()
    check("C10 两个绝对路径都在 example.yaml",
          ("<repo>/configs/config.yaml" in ex,
           "/Users/zuowei/workspace/runtime/atlas/configs/config.yaml" in ex), (True, True))

    print("\n[F] 三条 🔴 注释逐字取自需求原文")
    if not os.path.exists(REQ_DOC):
        skip("需求原文逐行比对", f"需求文档不在本机：{REQ_DOC}")
    else:
        req = open(REQ_DOC, encoding="utf-8").read()
        rseg = req[req.index("## TASK-011"):][:3000]
        # 🔴 只取 yaml 块内的注释行：`#` 开头的 markdown 标题（## TASK-011 / ### …）
        #    不是 yaml 注释，混进来会造出「交付里缺失」的假失败。
        req_lines = [l.rstrip() for l in rseg.splitlines()
                     if l.startswith("# ") or l.startswith("#\t") or l.rstrip() == "#"]
        dseg = ex[ex.index("# Sheets 投影（M2b）"):][:2000]
        dlines = [l.rstrip() for l in dseg.splitlines() if l.startswith("#")]
        check("需求注释行在交付里逐字缺失数（须 0）", len([l for l in req_lines if l not in dlines]), 0,
              f"（需求 {len(req_lines)} 行）")

    print("\n[G] 选行 77→61 与两库对照")
    keys = json.load(open(os.path.join(WT,"internal/hestia/testdata/period-keys-2026-09-16.json"), encoding="utf-8"))
    check("testdata 顶层条数", len(keys), 77)
    t = sh("GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -run 'TestSelectRowsCollapsesSeventySevenToSixtyOne'")
    check("选行测试通过", t.returncode, 0)
    if not (os.path.exists(RUNTIME_DB) and sh("which sqlite3", cwd=REPO).returncode == 0):
        skip("两库行数对照 77 vs 76", "runtime 库或 sqlite3 不可用")
    else:
        # 🔴 主仓库那份库在 REPO 下不在 WT 下：data/ 被 .gitignore（第 64 行），
        #    而 worktree 是按 commit checkout 的，未跟踪文件不会出现在里面。
        for db, exp in ((RUNTIME_DB, "77"), (os.path.join(REPO, "data/hestia.db"), "76")):
            n = sh(f'sqlite3 "{db}" "select count(*) from hestia_observations;"').stdout.strip()
            check(f"{'runtime' if 'runtime' in db else '主仓库'} 库行数", n, exp)

    print("\n[H] 全仓与样例配置")
    full = sh("GOTOOLCHAIN=local go test ./... -count=1")
    check("全仓退出码", full.returncode, 0)
    check("ok 包数", len([l for l in full.stdout.splitlines() if l.startswith("ok")]), 65)
    check("FAIL 行数", len([l for l in full.stdout.splitlines() if l.startswith("FAIL")]), 0)
    # 🔴 装载 config.example.yaml 的测试叫 TestExampleConfigDeclaresHestiaRules，
    #    名字里没有 "Health" —— `-run Health` 跑不到它（派验消息给的那条命令是假绿）。
    cfg = sh("GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -run 'TestExampleConfigDeclaresHestiaRules' -v")
    check("样例配置真能装载（用正确的 -run）", "--- PASS: TestExampleConfigDeclaresHestiaRules" in cfg.stdout, True)
    wrong = sh("GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -run 'Health' -v")
    check("对照：-run Health 跑不到那条（证明它是假绿）",
          "TestExampleConfigDeclaresHestiaRules" in wrong.stdout, False)

    print("\n" + "="*78)
    print(f"复算结果：PASS {PASS}  FAIL {FAIL}  SKIP {SKIP}")
    sh(f"git worktree remove --force {WT}", cwd=REPO); sh("git worktree prune", cwd=REPO)
    print("worktree 已拆")
    sys.exit(1 if FAIL else 0)

if __name__ == "__main__":
    main()
