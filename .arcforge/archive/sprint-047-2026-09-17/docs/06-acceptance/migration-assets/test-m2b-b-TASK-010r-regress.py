#!/usr/bin/env python3
# 回归变异 — 首验的 12 个变异在返工后是否仍被 **dev 自己的测试** 杀掉
# （验证者夹具已移出，本轮只用交付测试集）
import subprocess, hashlib, os
WT="/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-r010"
MAIN="/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
IG="internal/hestia/ingest.go"
orig=open(os.path.join(WT,IG),encoding='utf-8').read()
SHA=hashlib.sha256(orig.encode()).hexdigest()
MAIN_SHA=hashlib.sha256(open(os.path.join(MAIN,IG),encoding='utf-8').read().encode()).hexdigest()

def sub(s,old,new,n=1):
    assert s.count(old)==n, f"锚点 {s.count(old)} 次: {old[:50]!r}"
    return s.replace(old,new)

PROJ='\t\t\t\tfmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\\n", perr)\n'
BUILD='\t\t\t\tfmt.Fprintf(d.Out, "sheets: 组装失败（不影响入库）: %v\\n", berr)\n'
ELSEIF='\t\t\t} else if perr := recoverPanic(func() error { return d.ProjectSheets(ctx, rows) }); perr != nil {\n'
NIL='\t\tif d.ProjectSheets != nil {'
BLOCK_START='\t\t// 投影在契约之后（M2b 的 TASK-010）：契约有下游在等，投影没有。'
EVAL='\t\ttemp = Evaluate(obs, d.Cfg.Signals)\n\t}\n'

def move_outside(s):
    i=s.index(BLOCK_START); j=s.index('\t\ttemp = Evaluate(obs, d.Cfg.Signals)', i)
    block=s[i:j]; s2=s[:i]+s[j:]
    return sub(s2, EVAL, EVAL+block)

MUTS=[
 ("M1","C8 投影失败改成 return fail", lambda s: sub(s,PROJ,PROJ+'\t\t\t\treturn fail("sheets", perr)\n')),
 ("M2","C8 组装失败改成 return fail", lambda s: sub(s,BUILD,BUILD+'\t\t\t\treturn fail("sheets", berr)\n')),
 ("M3","文案改一字：投影失败→投影错误", lambda s: sub(s,"投影失败（不影响入库）","投影错误（不影响入库）")),
 ("M4","文案全角括号→半角", lambda s: sub(s,"投影失败（不影响入库）","投影失败(不影响入库)")),
 ("M5","文案冒号后空格删掉", lambda s: sub(s,"投影失败（不影响入库）: %v","投影失败（不影响入库）:%v")),
 ("M6","去掉 nil 判断（改 if true）", lambda s: sub(s,NIL,"\t\tif true {")),
 ("M8","else if → if（组装失败后仍投影）",
  lambda s: sub(s,ELSEIF,'\t\t\t}\n\t\t\tif perr := recoverPanic(func() error { return d.ProjectSheets(ctx, rows) }); perr != nil {\n')),
 ("M9","%v 参数换成固定串", lambda s: sub(s,PROJ,'\t\t\t\tfmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\\n", "see logs")\n')),
 ("M10","投影块挪出 New/Revision 块", move_outside),
 ("M12","投影失败时发 [P1]",
  lambda s: sub(s,PROJ,PROJ+'\t\t\t\tif d.Notify != nil {\n\t\t\t\t\t_ = d.Notify.SendText("[P1] sheets 投影失败")\n\t\t\t\t}\n')),
]
def fails(o): return sorted({l.split()[2] for l in o.splitlines() if l.startswith("--- FAIL:")})
def run(c): return subprocess.run(c,cwd=WT,shell=True,capture_output=True,text=True)

print("回归变异：首验 12 个中，M7=R9、M11=R8 已在返工专项里跑过（均 KILLED），此处补其余 10 个")
print("⚠️ 本轮**已移出验证者夹具**，只用交付测试集 —— 验的是 dev 自己的守卫\n")
for name,desc,fn in MUTS:
    try: mut=fn(orig)
    except AssertionError as e:
        print(f"{name:4s} ❌ 锚点失配：{e}"); continue
    open(os.path.join(WT,IG),'w',encoding='utf-8').write(mut)
    g=run(f"GOTOOLCHAIN=local gofmt -e {IG} > /dev/null")
    if g.returncode!=0:
        print(f"{name:4s} ❌ 语法闸命中，早退"); run(f"git checkout -- {IG}"); continue
    t=run("GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -timeout 300s")
    comb=t.stdout+t.stderr; f=fails(t.stdout)
    if any(k in comb for k in ("build failed","[setup failed]","declared and not used","cannot ")):
        print(f"{name:4s} ❌ 有效性闸：编译失败，判无效"); run(f"git checkout -- {IG}"); continue
    if t.returncode!=0 and not f:
        print(f"{name:4s} ❌ 有效性闸：exit={t.returncode} 零条 FAIL，判无效"); run(f"git checkout -- {IG}"); continue
    print(f"{name:4s} {desc:34s} {'KILLED' if f else '🔴 SURVIVED'}  ({len(f)} 条) {f[:3]}")
    run(f"git checkout -- {IG}")
    assert hashlib.sha256(open(os.path.join(WT,IG),encoding='utf-8').read().encode()).hexdigest()==SHA, f"{name} 还原失败"
print("\n还原校验: wt=OK  main=", hashlib.sha256(open(os.path.join(MAIN,IG),encoding='utf-8').read().encode()).hexdigest()==MAIN_SHA)
