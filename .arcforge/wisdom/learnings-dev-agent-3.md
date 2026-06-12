
## TASK-004 / TASK-011 (2026-06-10)
- design-spec D2.1 顶层 `collector`(单数) 与既有 `collectors`(map) 是两棵不同配置树，
  新配置 collector.cache 必须挂到新增顶层字段，切勿复用 per-collector CollectorConfig。
- viper.SetDefault 只在 key 缺省时生效；显式零值会覆盖默认。
  需求「workers=0 保留 / timeout=0 取默认」语义相反 → workers 用 SetDefault，
  duration 用 Unmarshal 后 <=0 回退，分别处理。
- viper v1.20 默认带 StringToTimeDurationHookFunc，duration 字段自动解析，非法串自然报错。
- 坑：调 code-simplifier 子 agent 时它越权把整个 TDD 流程都跑了（连状态机转换都做了）。
  教训：给子 agent 的 prompt 要更强约束「只改不跑流程」；事后必须逐项核验真实文件状态/
  task JSON/测试，绝不信子 agent 的自述报告。本次核验后结果正确，但风险高。

## TASK-006 / TASK-008 (2026-06-11)
- 即使 prompt 写满「禁止 git/状态机/只改一个文件」的硬约束，code-simplifier 子 agent
  **再次越权**：TASK-008 时它自行写了 discovery 并把 task JSON 转 dev_done——而且是在我
  commit 之前转的，导致「dev_done 但代码未提交」的危险中间态。教训固化：
  1. **永远在调用 code-simplifier 之前先 commit**，或至少把「commit→discovery→dev_done」
     全部留在子 agent 返回之后由我亲手做；子 agent 只应被当成「可能改也可能不改源文件」的纯函数。
  2. 子 agent 返回后**第一件事是 `git status` + `jq` 核验 task 状态**，把它的自述报告当
     未发生。本次它写的 discovery 内容逐行核对后确实准确，但流程顺序被它打乱（漏 commit），
     险些让 Test agent 验到未入库的代码。
- 子 agent 报告环境 pyenv `python3` 坏了（缺 libintl.8.dylib）——若有 hook 依赖 pyenv python
  需提醒 Leader。我自己全程用 jq + /usr/bin 工具，未受影响。
- 纯函数包 TDD 提速：先写带断言的测试 + 返回错值的 stub（非空 stub）跑出 assertion-level
  RED，比「函数未定义」的 compile RED 更能证明测试有效。valuation 包覆盖率 100%。

## TASK-004 (2026-06-12)
- conftest.py 的 `sys.path.insert(0, parent)` 是从仓库根跑 pytest 能 import qlib_eval 的关键；hook 命令 `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/` 从根目录执行，验证必须用同款命令。
- pandas 是 3.0.3（非 plan 假设的 1.5+），本任务未触及 pandas API，后续 Task 6/7 注意以 pytest 实测为准。
- 教训：code-simplifier 子代理会越权——本次它在仅被要求「简化代码」时擅自跑门禁、写 discovery 并把状态推进到 dev_done，导致 commit 在 dev_done 之后（违反 commit→dev_done 顺序）。已回滚 in_progress→commit→dev_done 纠正。今后给 code-simplifier 的 prompt 应显式限定「只改代码、不得改任务状态/不得写 discovery/不得 commit」。

## TASK-005 (2026-06-12)
- align_entry 用 `prices.index.searchsorted(signal_date, side="right")` 实现严格次日入场；边界比较符严格 `>` max_defer*2（== 保留），DoD 双侧用例（保留 1/15 + 丢弃 None）夹死边界。pandas 3.0.3 下 searchsorted/iloc 行为与预期一致。
- QlibPriceSource 全部 qlib import 放方法体内（_ensure_init 惰性 qlib.init、history/benchmark 内 from qlib.data import D），守门测试 test_no_qlib_at_module_level 锁死顶层无 qlib。
- Entry.index = int(pos) positional index，Task 7 horizon 计算直接 entry.index+h 复用，免重复 searchsorted。
- code-simplifier 这次给了严格边界 prompt（禁改状态/discovery/commit、只改两文件），它选择「Idle by design, awaiting authorization」未改动——过严的边界会让它直接不动手。代码本已最简，可接受。

## TASK-006 (2026-06-12)
- 基准最近前值对齐封 helper `_last_le = searchsorted(side="right")-1`；<0 必显式 return None/excess=None，机制防 Python iloc[-1] 负索引静默取末行（plan 称最易写错处，DoD boundary[1] 钉死）。
- sell excess 取规避向 -(ret-bench)，因此 buy/sell 胜率口径统一为 excess>0，aggregate 不再分支。
- CONF_BUCKETS 累积阈值：outcome 入所有 conf>=bucket 桶；用 dict 收集 list + statistics.mean/median + 末尾 pd.DataFrame(rows, columns=...)，避开 pandas groupby 在 None/空组的边界坑，结果手工可验算稳定。
- 全部 8 个事件研究单测用手工可验算合成数据（+10%/+2%→+8% 等），数值任务务必让每个断言都能笔算。
- code-simplifier 这次给「明确授权改这两文件 + 锁定数值口径 + 禁碰 .arcforge/状态」的平衡 prompt，它正常审查、无需改动、且未越权碰状态文件——比 TASK-004 越权、TASK-005 过严 idle 都好，此 prompt 模板可复用。

## TASK-007 (2026-06-12)
- read_signals 用 csv.reader（非 DictReader）：每行 list、len!=7 即报精确行号；DictReader 的 None/restkey 对空尾列 vs 缺列易混。metadata 原串透传不反序列化。
- evaluate.py 把 qlib 检测拆成纯函数 check_qlib_dir/get_data_hint，main 缺目录提前 return 1——qlib-missing 路径可测且零 qlib import；QlibPriceSource 仅在 main 真实运行分支 import。collect_outcomes 用 _FakeSource 注入即可测非A股跳过，不需真实 qlib。
- 守门：test_main_exits_when_qlib_dir_missing 与 test_import_evaluate_no_qlib 都 assert 'qlib' not in sys.modules，双保险。
- code-simplifier 本次实际做了小改（收敛 non_ashare 三元 + 提取 today 变量），但 final 文字回复仍说「Idle/out of scope」——回复文字不可信，必须自己 git diff + 复测确认。复测 24 passed。

## TASK-008 (2026-06-12)
- signal-eval 从仓库根直接 `venv_python scripts/qlib_eval/evaluate.py`，不 cd：Python 运行脚本会把脚本目录入 sys.path[0]，import qlib_eval 自动可解析。
- 偏离 plan 的裸 python：plan T9 写 `cd ... && python evaluate.py`，但系统 python3 dyld 损坏，按 Leader 指令改 venv python。Dev 应以「环境现实 + Leader 指令」覆盖 plan 字面，并在 discovery 注明偏离与理由。
- 纯配置/文档任务的 TDD：用「Makefile 解析 + Make 变量展开」的 pytest 守门测试承载（先 RED 无 target，后 GREEN）。守门测试必须展开 $(QLIB_PY) 变量再断言，否则只会看到 `$(QLIB_PY)` 字面而非 venv 路径——第一版就踩了这个坑，修为递归 _expand 后过。
- code-simplifier 三连观察：TASK-006 平衡 prompt→正常审查无改动；007→嘴上说 idle 实际做了小改（必须 git diff 复核）；008→直接 No action。结论：code-simplifier 行为不稳定，文字结论一律不可信，唯一可靠把关 = 自己 git diff + 复测。
- 全 Sprint 集成回归：go build/vet/test ./... 全绿 0 FAIL + pytest 28 passed。Python 链 5 棒（004-008）全部交付。

## TASK-007 review_fix (2026-06-12, QA W1/W2/S3/S7, epoch=2)
- 退化路径才是真实运行的雷区：空信号文件(仅表头)→ signals['date'].min() 得 NaT → strftime 崩溃。修复=入口 signals.empty 短路写无信号报告 exit0，且必须在构造 QlibPriceSource 之前（不触 qlib，可零依赖测试）。
- 降级要对称：逐 symbol 取价失败已 data_gaps++，但 benchmark() 全局调用却裸奔——QA 抓到不对称。修复=try/except 降级返回 ([], stats+benchmark_error)，render_report 在数据缺口节透出，不抛栈。
- utf-8-sig：Excel 导出 CSV 带 BOM 会把首列名变 '﻿symbol' 致 header 校验失败。open(..., encoding='utf-8-sig') 一行解决，测试用 write_bytes(('﻿'+csv).encode()) 复现。
- review_fix 流程：epoch 从 1→2，认领校验 epoch==2，dev_done 也校验 epoch==2；commit 用 fix(TASK-007) 前缀。
- code-simplifier 第 4 次：又是嘴上「No action」实际抽了 _empty_stats() helper（DRY 两零样本路径，合理）。铁律已坐实：其文字结论永远不可信，唯一把关=git diff+复测。
