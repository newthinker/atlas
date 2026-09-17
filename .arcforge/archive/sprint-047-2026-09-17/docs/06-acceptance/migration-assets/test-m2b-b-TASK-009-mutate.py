#!/usr/bin/env python3
# 变异 harness — TASK-009 验证者 test-m2b-b
# 作用对象：隔离 worktree wt-m2b-v009 内的 hestia_sheets.go / hestia.go（主工作区一个字节不碰）
import subprocess, hashlib, difflib, os

WT = "/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v009"
MAIN = "/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
SH = "cmd/atlas/hestia_sheets.go"
HE = "cmd/atlas/hestia.go"
SHA = {SH: "0efac7940ef90e600c145f5a8a48908b07a1e6a26998ddb2c77b91b1a48c135f",
       HE: "3f23241da0f4987cbb7d8b26dd5b3f260e71577fc1c57c7a9a676d6bb448c054"}
PKG = "./cmd/atlas/"

def rd(f): return open(os.path.join(WT, f), encoding='utf-8').read()
def sha(p): return hashlib.sha256(open(p, 'rb').read()).hexdigest()
def run(c, cwd=WT): return subprocess.run(c, cwd=cwd, shell=True, capture_output=True, text=True)

ORIG = {SH: rd(SH), HE: rd(HE)}
for f, h in SHA.items():
    assert hashlib.sha256(ORIG[f].encode()).hexdigest() == h, f"{f} 不是交付版"

def sub(s, old, new, n=1):
    assert s.count(old) == n, f"锚点出现 {s.count(old)} 次，期望 {n}: {old[:60]!r}"
    return s.replace(old, new)

VALIDATE_CALL = '''	if err := validateSheetsPushFlags(); err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	cfg, st, err := openHestia()
	if err != nil {
		return err
	}
'''
VALIDATE_AFTER = '''	out := cmd.OutOrStdout()

	cfg, st, err := openHestia()
	if err != nil {
		return err
	}
	if err := validateSheetsPushFlags(); err != nil {
		return err
	}
'''
RECOVER = '''		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("hestia sheets: 投影 panic: %v", r)
			}
		}()
'''
RECOVER_SWALLOW = '''		defer func() {
			_ = recover()
		}()
'''
DISABLED = '''		fmt.Fprintln(out, sheetsDisabledLine)
		return nil
'''
THREE_LINES = '''	fmt.Fprintf(&b, "\\n%d 行\\n将写 %d 格\\n一致跳过 %d 格\\n库缺跳过 %d 格\\n",
		len(seen), res.WillWrite, res.Same, res.AbsentInDB)
'''
PROJ_PUSH = '''		_, perr := sheetsPush(ctx, c, rows, sheetLabels(), sheets.Options{Apply: true, CreateSheets: true})
'''
FILTER_BODY = '''		if fmt.Sprintf("%04d-%02d", r.Year, r.Month) == period {
'''
MISSING_TABS = '''	if len(res.MissingTabs) > 0 {
'''
DRYRUN_PRINT = '''	if !opts.Apply {
		fmt.Fprintln(out, sheetsDryRunLine)
	}
'''
NIL_RETURN = '''	if cfg.HestiaSheets.CredentialsFile == "" {
		return nil
	}
'''

MUTS = [
    ("P1", SH, "flag 校验挪到开库之后", lambda s: sub(s, VALIDATE_CALL, VALIDATE_AFTER)),
    ("P2", SH, "recover 吞掉 panic（不转 error）", lambda s: sub(s, RECOVER, RECOVER_SWALLOW)),
    ("P3", SH, "去掉整个 defer recover（panic 穿透）", lambda s: sub(s, RECOVER, "")),
    ("P4", SH, "凭据留空时退出非零（把「没配」报成故障）",
     lambda s: sub(s, DISABLED, '''		fmt.Fprintln(out, sheetsDisabledLine)
		return fmt.Errorf("%s", sheetsDisabledLine)
''')),
    ("P5", SH, "三类计数合并成一行（spec 的单行形态，违反 DoD）",
     lambda s: sub(s, THREE_LINES, '''	fmt.Fprintf(&b, "\\n%d 行 · 将写 %d 格 · 一致跳过 %d 格 · 库缺跳过 %d 格\\n",
		len(seen), res.WillWrite, res.Same, res.AbsentInDB)
''')),
    ("P6", SH, "闭包里 Options 改成 Apply:false",
     lambda s: sub(s, PROJ_PUSH, '''		_, perr := sheetsPush(ctx, c, rows, sheetLabels(), sheets.Options{Apply: false, CreateSheets: true})
''')),
    ("P7", SH, "闭包里 CreateSheets 改成 false",
     lambda s: sub(s, PROJ_PUSH, '''		_, perr := sheetsPush(ctx, c, rows, sheetLabels(), sheets.Options{Apply: true, CreateSheets: false})
''')),
    ("P8", SH, "闭包忽略入参 rows，传 nil 给 Push（= DoD 字面「重算」的后果）",
     lambda s: sub(s, PROJ_PUSH, '''		_, perr := sheetsPush(ctx, c, nil, sheetLabels(), sheets.Options{Apply: true, CreateSheets: true})
''')),
    ("P9", SH, "凭据留空返回非 nil 空闭包（C9 的 != nil 判据被穿透）",
     lambda s: sub(s, NIL_RETURN, '''	if cfg.HestiaSheets.CredentialsFile == "" {
		return func(context.Context, []sheets.Row) error { return nil }
	}
''')),
    ("P10", SH, "filterRowsByPeriod 改回拆 period 比较（非补零形态会被放行）",
     lambda s: sub(s, FILTER_BODY, '''		if y, m, n := 0, 0, 0; true {
			n, _ = fmt.Sscanf(period, "%d-%d", &y, &m)
			if n == 2 && r.Year == y && r.Month == m {
''').replace('''			out = append(out, r)
		}
	}
	return out
}''', '''				out = append(out, r)
			}
		}
	}
	return out
}''')),
    ("P11", SH, "缺表提示无条件打印", lambda s: sub(s, MISSING_TABS, "	if true {\n")),
    ("P12", SH, "dry-run 那句无条件打印（--apply 时也说没写）",
     lambda s: sub(s, DRYRUN_PRINT, '''	fmt.Fprintln(out, sheetsDryRunLine)
''')),
    ("P13", SH, "cellDisplay 不特判 nil（显示成 <nil>）",
     lambda s: sub(s, '''	if v == nil {
		return "(空)"
	}
''', "")),
    ("P14", HE, "建了 sheets 命令但不挂到 hestia 下（CLI 上调不出来）",
     lambda s: sub(s, '''	hestiaCmd.AddCommand(hestiaIngestCmd, hestiaStatusCmd, hestiaBackfillCmd, hestiaContractCmd,
		hestiaSheetsCmd)
''', '''	hestiaCmd.AddCommand(hestiaIngestCmd, hestiaStatusCmd, hestiaBackfillCmd, hestiaContractCmd)
''')),
    ("P15", HE, "hestiaIngestDeps 不挂 ProjectSheets（010 转移来的接线漏掉）",
     lambda s: sub(s, "		ProjectSheets: sheetsProjector(cfg),\n", "")),
    ("P16", HE, "--period-type 给默认值 monthly（「只给 --period」不再是错误）",
     lambda s: sub(s, '''sf.StringVar(&hestiaSheetsPeriodType, "period-type", "", "monthly | q1 | h1 | q1_q3 | annual")''',
                   '''sf.StringVar(&hestiaSheetsPeriodType, "period-type", "monthly", "monthly | q1 | h1 | q1_q3 | annual")''')),
]

def fails(out):
    return sorted({ln.split()[2] for ln in out.splitlines() if ln.startswith("--- FAIL:")})

for name, tgt, desc, fn in MUTS:
    print("=" * 78)
    print(f"{name}  [{os.path.basename(tgt)}]  {desc}")
    try:
        mutated = fn(ORIG[tgt])
    except AssertionError as e:
        print(f"  ❌ 锚点失配，跳过：{e}"); continue
    if mutated == ORIG[tgt]:
        print("  ❌ 变异体与原文相同，跳过"); continue
    open(os.path.join(WT, tgt), 'w', encoding='utf-8').write(mutated)
    d = list(difflib.unified_diff(ORIG[tgt].splitlines(True), mutated.splitlines(True), 'orig', name, n=1))
    print("  --- 变异 diff ---")
    for ln in d[2:]:
        print("   " + ln.rstrip())
    g = run(f"GOTOOLCHAIN=local gofmt -e {tgt} > /dev/null")
    if g.returncode != 0:
        print(f"  ❌ 语法闸命中，早退\n{g.stderr[:300]}")
        run(f"git checkout -- {tgt}"); continue
    v = run(f"GOTOOLCHAIN=local go vet {PKG}")
    t = run(f"GOTOOLCHAIN=local go test {PKG} -count=1 -timeout 300s")
    combined = t.stdout + t.stderr
    f = fails(t.stdout)
    if "build failed" in combined or "[setup failed]" in combined or "cannot " in combined or "declared and not used" in combined:
        print("  ❌ 有效性闸命中：编译失败，判定无效，早退")
        print("   " + combined[:400])
        run(f"git checkout -- {tgt}"); continue
    if t.returncode != 0 and not f:
        print(f"  ❌ 有效性闸命中：exit={t.returncode} 但零条 --- FAIL，判定无效，早退")
        print("   " + combined[:400])
        run(f"git checkout -- {tgt}"); continue
    print(f"  go vet exit={v.returncode}   结果: {'KILLED' if f else '🔴 SURVIVED'}   go test exit={t.returncode}   FAIL 条数={len(f)}")
    for x in f:
        print("    - " + x)
    run(f"git checkout -- {tgt}")
    ok_wt = sha(os.path.join(WT, tgt)) == SHA[tgt]
    ok_main = all(sha(os.path.join(MAIN, k)) == v2 for k, v2 in SHA.items())
    print(f"  还原校验: worktree={'OK' if ok_wt else '❌'}  主工作区={'OK' if ok_main else '❌ 被动过'}")

print("=" * 78)
print("收尾：worktree", all(sha(os.path.join(WT, k)) == v for k, v in SHA.items()),
      "| 主工作区", all(sha(os.path.join(MAIN, k)) == v for k, v in SHA.items()))
