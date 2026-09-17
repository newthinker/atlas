#!/usr/bin/env python3
# TASK-009 返工复验变异 harness · 验证者 test-m2b-b
import subprocess, hashlib, difflib, os
WT="/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-r009"
MAIN="/Users/zuowei/workspace/go/src/github.com/newthinker/atlas"
SH="cmd/atlas/hestia_sheets.go"
PL="deploy/launchd/com.newthinker.atlas.hestia-ingest.plist"
PKG="./cmd/atlas/"
def rd(f): return open(os.path.join(WT,f),encoding='utf-8').read()
def sha(p): return hashlib.sha256(open(p,'rb').read()).hexdigest()
def run(c): return subprocess.run(c,cwd=WT,shell=True,capture_output=True,text=True)
ORIG={SH:rd(SH), PL:rd(PL)}
SHA={f:hashlib.sha256(s.encode()).hexdigest() for f,s in ORIG.items()}
def sub(s,old,new,n=1):
    assert s.count(old)==n, f"锚点 {s.count(old)} 次: {old[:60]!r}"
    return s.replace(old,new)

ONCE_BLOCK='''		once.Do(func() {
			// 建客户端也给超时：JWT 交换要出网，代理没起时它会一直挂着，
			// 而 ingest 是 launchd 唤起的，挂住就是这一轮永远不结束。
			cctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
			defer cancel()
			client, build = newSheetsClient(cctx, cfg.HestiaSheets.CredentialsFile, cfg.HestiaSheets.SpreadsheetID)
		})
'''
MUTS=[
 ("S1", PL, "plist 删掉 http_proxy 键",
  lambda s: sub(s,'    <key>http_proxy</key>\n    <string>http://127.0.0.1:7897</string>\n','')),
 ("S2", PL, "plist 的 http_proxy 值改成别的端口",
  lambda s: sub(s,'<key>http_proxy</key>\n    <string>http://127.0.0.1:7897</string>',
                  '<key>http_proxy</key>\n    <string>http://127.0.0.1:9999</string>')),
 ("S3", PL, "plist 删掉 no_proxy 键",
  lambda s: sub(s,'    <key>no_proxy</key>\n    <string>localhost,127.0.0.1</string>\n','')),
 ("S4", SH, "去掉 sync.Once（每次调用都重建 client）",
  lambda s: sub(s,ONCE_BLOCK,'''		{
			cctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
			defer cancel()
			client, build = newSheetsClient(cctx, cfg.HestiaSheets.CredentialsFile, cfg.HestiaSheets.SpreadsheetID)
		}
''')),
 ("S5", SH, "改成提前建（非懒建）",
  lambda s: sub(s,'''	var (
		client *sheets.Client
		build  error
		once   sync.Once
	)
	return func(ctx context.Context, rows []sheets.Row) (err error) {''',
 '''	eagerCtx, eagerCancel := context.WithTimeout(context.Background(), sheetsCallTimeout)
	defer eagerCancel()
	client, build := newSheetsClient(eagerCtx, cfg.HestiaSheets.CredentialsFile, cfg.HestiaSheets.SpreadsheetID)
	var once sync.Once
	_ = once
	return func(ctx context.Context, rows []sheets.Row) (err error) {''')),
 ("S6", SH, "投影侧建 client 去掉 WithTimeout",
  lambda s: sub(s,'''			cctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
			defer cancel()
			client, build = newSheetsClient(cctx,''','''			client, build = newSheetsClient(ctx,''')),
 ("S7", SH, "投影侧 Push 去掉 WithTimeout",
  lambda s: sub(s,'''		pctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
		defer cancel()
		_, perr := sheetsPush(pctx,''','''		_, perr := sheetsPush(ctx,''')),
 ("S8", SH, "CLI 侧 Push 去掉 WithTimeout",
  lambda s: sub(s,'''	pctx, pcancel := context.WithTimeout(ctx, sheetsCallTimeout)
	defer pcancel()
	res, err := sheetsPush(pctx,''','''	res, err := sheetsPush(ctx,''')),
 ("S9", SH, "CLI 侧建 client 去掉 WithTimeout",
  lambda s: sub(s,'''	cctx, ccancel := context.WithTimeout(ctx, sheetsCallTimeout)
	defer ccancel()
	c, err := newSheetsClient(cctx,''','''	c, err := newSheetsClient(ctx,''')),
 ("S10", SH, "明细行分隔符两空格→一空格（只有整行 Equal 杀得掉）",
  lambda s: sub(s,'"%s  %d月  %s %s  %s\\n"','"%s %d月 %s %s %s\\n"')),
 ("S11", SH, "changeVerdict 文案「将写」→「待写」（只有整行 Equal 杀得掉）",
  lambda s: sub(s,'return fmt.Sprintf("表中 %s  将写 %v", cellDisplay(ch.Current), ch.Want)',
                  'return fmt.Sprintf("表中 %s  待写 %v", cellDisplay(ch.Current), ch.Want)')),
]
def fails(o): return sorted({l.split()[2] for l in o.splitlines() if l.startswith("--- FAIL:")})
for name,tgt,desc,fn in MUTS:
    print("="*76); print(f"{name}  [{os.path.basename(tgt)}]  {desc}")
    try: mut=fn(ORIG[tgt])
    except AssertionError as e: print(f"  ❌ 锚点失配：{e}"); continue
    if mut==ORIG[tgt]: print("  ❌ 与原文相同"); continue
    open(os.path.join(WT,tgt),'w',encoding='utf-8').write(mut)
    d=list(difflib.unified_diff(ORIG[tgt].splitlines(True),mut.splitlines(True),'orig',name,n=1))
    print("  --- diff ---")
    for l in d[2:][:14]: print("   "+l.rstrip())
    if tgt==SH:
        g=run(f"GOTOOLCHAIN=local gofmt -e {tgt} > /dev/null")
        if g.returncode!=0:
            print("  ❌ 语法闸命中，早退"); run(f"git checkout -- {tgt}"); continue
    else:
        g=run(f"plutil -lint {tgt}")
        if g.returncode!=0:
            print(f"  ❌ plist 语法闸命中，早退: {g.stdout[:200]}"); run(f"git checkout -- {tgt}"); continue
    t=run(f"GOTOOLCHAIN=local go test {PKG} -count=1 -timeout 300s")
    comb=t.stdout+t.stderr; f=fails(t.stdout)
    if any(k in comb for k in ("build failed","[setup failed]","declared and not used","cannot ")):
        print("  ❌ 有效性闸：编译失败，判无效"); print("   "+comb[:300]); run(f"git checkout -- {tgt}"); continue
    if t.returncode!=0 and not f:
        print(f"  ❌ 有效性闸：exit={t.returncode} 零条 FAIL，判无效"); print("   "+comb[:300]); run(f"git checkout -- {tgt}"); continue
    print(f"  结果: {'KILLED' if f else '🔴 SURVIVED'}   FAIL 条数={len(f)}")
    for x in f: print("    - "+x)
    run(f"git checkout -- {tgt}")
    assert sha(os.path.join(WT,tgt))==SHA[tgt], f"{name} 还原失败"
print("="*76)
print("收尾：wt", all(sha(os.path.join(WT,k))==v for k,v in SHA.items()),
      "| main", all(sha(os.path.join(MAIN,k))==v for k,v in SHA.items()))
