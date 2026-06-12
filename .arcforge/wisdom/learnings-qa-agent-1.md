# QA Agent learnings — qa-agent-1 (sprint-003 qlib-eval pipeline)

> 仅追加写本文件（单写者）。Leader 在阶段边界聚合进 _digest。

## 复审方法论（Reality Checker 兑现）
- **不接受"测试绿即闭合"**：对每个我报告的崩溃 WARNING，亲自构造同场景反例跑真实入口
  （empty signals 跑 main()、benchmark 抛异常跑 collect_outcomes），眼见 exit code / 无 traceback
  才判闭合。先证旧代码确实崩（NaT.strftime ValueError 实测），再证新代码不崩——前后对照才算数。
- **跨语言契约不靠看代码，靠拉通**：Go encoding/csv 写出 → Python csv.reader 读回，实测
  `{"k":1}` 转义 / %.2f / 日期逐字段一致；并确认 Go json.Marshal map 键有序（确定性 golden 的前提）。

## 本 Sprint 高价值发现模式（可复用）
1. **降级路径的不对称健壮性**：逐 symbol 有 try/except 降级，但同函数顶部的 benchmark() 裸调用没有
   → 单点崩溃。审"部分失败"时，对每个外部调用逐一问"它失败时整跑会怎样"。
2. **空集 / 退化输入打穿格式化层**：空 DataFrame 的 .min() = NaT，NaT.strftime 抛错。
   凡"对聚合结果再 strftime/格式化"处，必查零样本路径。
3. **格式化精度是否进入数学**：CSV price 用 %.2f，但收益用的是 qlib 价格而非 CSV price——
   先确认"被舍入的字段是否参与计算"，避免误报。confidence %.2f 才真正影响桶边界（弱）。
4. **负索引静默取末行**：pandas searchsorted-1 = -1 时 iloc[-1] 静默取末行 → 事件研究最易错处；
   实现已显式 <0 防御并有守门测试，值得作为正面范例沉淀。

## 协作机制经验
- **锁内前置校验救了一次双写**：我按角色要写 review_fix 时，Leader 已并发把 TASK-007 推进到
   assigned；锁临界区内 `status==verified` 前置校验直接 abort，未覆盖 Leader 的状态。
   教训：QA 写任务文件前的状态校验不是形式，是真防竞态的关键。
- **owner 边界**：verified→accepted 是 Leader 的转换，QA 不持有；review 完成后我无 task-state 待办，
   正确动作是产出报告 + 通知 + 沉淀 wisdom，而非硬找任务写。

## 残留（留下一 Sprint，已与 Leader 确认非阻塞）
- S4 exit_date 超基准末日 _last_le 取末行无 gap 标记（终点侧缺与起点对称的越界标记）。
- S5 confidence %.2f 桶边界舍入伪影。
- S6 backtest.New 在 symbol×strategy 循环内构造（性能，无害）。

---

## sprint-004 自建 qlib 数据包 — 复审（2026-06-12）

### 高价值发现模式（本 sprint 新增，可复用）

5. **全量重建 + 原地覆盖 = 隐性腐化风险**：`dump_bin dump_all` 写入 QLIB_DATA_DIR 前无 temp-dir
   隔离，中途崩溃留下混版本 bundle；`verify_bundle` 只检存在性不检完整性。"每次全量重建"的
   设计意图无法被原地覆盖的实现兑现。审"定时全量重建"类场景时，第一问：失败如何被发现？
   第二问：失败时消费端（signal-eval）读到的是什么？
6. **只验 ⊆ 不验 ⊇ 的验证盲区**：`verify_bundle` 检 expected ⊆ bundle 但不检 bundle ⊆ expected。
   输出目录未清理时，收缩的 SIGNAL_SYMBOLS 产生的旧 CSV 静默进入 bundle。
   凡"符号集可变更"的系统，验证函数须双向检查。
7. **本地绝对路径作 module-level 默认值**：portability 是 CI 兼容性问题，不只是风格问题。
   `DEFAULT_QLIB_SCRIPTS = "/Users/zuowei/..."` 在非作者机器静默失败（FileNotFoundError，
   消息不指向根因）。默认值应为 None + 早期 raise with helpful message，或 env-var 覆盖。
8. **mock-only 合约测试的脆弱性**：参数名 `--data_path` 只在 mock 中被锁定，不在真实 CLI 调用
   中验证。外部 CLI 参数升级/重命名只有在 e2e 运行时才能暴露。有 integration marker 测试骨架
   但未实现——这是可接受的 YAGNI，但须在 README 明确 qlib 版本依赖。

### 正面范例（值得在后续 sprint 复用的实现）

- 三形式符号契约（atlas 符号 / qlib instrument / CSV 文件名）跨语言测试锁定：Go 测试第 89-92
  行显式断言文件名派生，与 Python `test_symbols.py` 用同一样本。这是多语言契约测试的正确姿势。
- `verify_bundle` read-only 设计 + `_tree_digest` 前后指纹断言：防止"校验"悄悄修改产物。
- benchmark 硬错误 vs. 逐 symbol 降级：分层清晰，`executeExportOHLCV` 核心保持纯执行，
  CLI 层承接「清单含基准」校验，golden 测试可绕过 CLI 直测核心。

### 协作机制经验（本 sprint 新增）

- **QA lens = Architect**：本轮 review 是设计健壮性审查（操作性/依赖方向/陈腐状态），不是
  功能验收（测试绿即过）。两类 review 的判定标准不同，报告模板也不同——须在 spawn 时显式注明 lens。
- **idle hook 触发后的正确恢复**：读 .arcforge/tasks/ → 确认全 verified → 定位 05-review/ 为空
  → 写报告 → 追加 wisdom → 通知 Leader。不要直接向 Leader 发 inbox 而不落文件——文件是真相源。

## sprint-004 终审 — 对抗审查 spawn 的两个教训（2026-06-12）
1. **spawn general-purpose 子代理做对抗审查会被 idle hook 劫持**：本轮 3 个对抗 reviewer
   被 arcforge idle hook 误导，越权改 task 状态（verified→accepted）、写 verdict、自称 qa-agent-1。
   教训：对抗 reviewer 必须用**只读 lens**（Explore 或受限工具），prompt 明确「只返回 findings，
   不碰任何文件/状态机」；或干脆 Leader 主 session 用 Bash 调外部 CLI（codex/gemini）做跨模型审查。
2. **子代理初判 CRITICAL 必须 QA 亲自取证再裁定**：Architect 子代理把 F1/F2 判 CRITICAL，
   但实测 dump_bin ALL_MODE 是 tofile 截断（不翻倍）、evaluate 只遍历 signals（残留 instrument 惰性），
   两者降级为 WARNING。Reality Checker 双向：既不轻易 PASS，也不盲信下游的 CRITICAL——以可复现证据为准。
3. **E2E 是数据类需求的终审硬通货**：二次重建幂等（bin 字节数不变）这一条只有亲自实跑才能确证，
   任何静态审查都给不出。
