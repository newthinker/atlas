# 需求 ↔ DoD 双向追溯矩阵

> 源需求：`docs/plans/2026-06-13-atlas-us-signal-eval-implementation.md`
> 生成：Leader · 2026-06-13 · 调度模式 dag · 6 Tasks

## 正向：需求 → DoD（无孤儿需求）

| 需求点（计划） | 落点 | 覆盖 Task | DoD 锚点 |
|---|---|---|---|
| F1 US ticker/指数 → qlib instrument（Go） | `toQlibInstrument` | TASK-001 | func①②③ + bound④⑤ |
| F1' 锚定契约（AAPL123/AAPL.B reject） | `usTickerRe ^[A-Z]{1,5}$` | TASK-001 | bound④ |
| F2 benchmarkForMarket us→^GSPC | Go | TASK-002 | func① |
| F2 inMarket us 识别 | Go | TASK-002 | func② |
| F2 --market us 白名单 + 文案 | Go | TASK-002 | func③ + err |
| F2' RejectsUnknownMarket 移除 us（既有测试迁移） | Go test | TASK-002 | bound⑤ |
| F3 symbols.py 对称镜像 US（re.fullmatch） | Python | TASK-003 | func①②③ |
| F3' 跨语言契约对称验证 | Go+Py | TASK-003 | non-func（双侧全绿） |
| F4 QlibPriceSource region 参数化 | `prices.py` | TASK-004 | func①② |
| F4 evaluate.py --region 透传 | `evaluate.py` | TASK-004 | func③ |
| F4' region 默认 cn 向后兼容 | Python | TASK-004 | bound④ |
| F4'' 构造不触发 qlib | Python | TASK-004 | bound⑤ |
| F5 signal-eval-us target | Makefile | TASK-005 | func① |
| F5 qlib-data-us target | Makefile | TASK-005 | func② |
| F5 .PHONY 登记 | Makefile | TASK-005 | func③ |
| F5' 走 venv python | Makefile | TASK-005 | err |
| F6 config US watchlist | config.yaml | TASK-006 | func①③ |
| F6' export-signals 美股非空 CSV | smoke | TASK-006 | func② |
| NF 全量测试零回归 | 全包 | 各 Task non-func + TASK-006 | non-func |
| NF pytest qlib-free | Python | TASK-003/004/006 | non-func |
| R1 region=us 端到端验收 + 降级 | Makefile 运行 | TASK-006 | err + non-func③（manual） |
| 收尾 code-simplifier | 全文件 | TASK-006 | non-func②（review） |

**孤儿需求检查：** 计划「验收对照」6 条全部映射到 DoD（见下），无遗漏。
**凭空 DoD 检查：** 每条 DoD 均可回溯至计划某 Step/验收项，无无源 DoD。

## 反向：计划「验收对照」→ DoD

| 计划验收对照条目 | 覆盖 |
|---|---|
| 两侧 AAPL→AAPL/^GSPC→GSPC 一致；AAPL123/AAPL.B 两侧 reject | TASK-001 func + TASK-003 func/non-func |
| 既有契约 AAPL/^GSPC 由 reject 迁 accept，无 CI 红 | TASK-001 non-func（RED→GREEN）|
| benchmarkForMarket(us)=^GSPC、inMarket us、--market us；cn/hk 不回归；RejectsUnknownMarket 移除 us | TASK-002 全部 DoD |
| QlibPriceSource region 默认 cn 可传 us | TASK-004 func + bound |
| make signal-eval-us 真实 atlas_us 非空报告（异常回退 cn） | TASK-006 err + non-func③ |
| Go+Python 全量通过；pytest qlib-free | 各 Task non-func + TASK-006 |

## verify_by 分布（自动化适配性）

| Task | test | benchmark | review | manual | 评估 |
|---|---|---|---|---|---|
| TASK-001 | 6 | 0 | 0 | 0 | 全自动 ✓ |
| TASK-002 | 7 | 0 | 0 | 0 | 全自动 ✓ |
| TASK-003 | 7 | 0 | 0 | 0 | 全自动 ✓ |
| TASK-004 | 7 | 0 | 0 | 0 | 全自动 ✓ |
| TASK-005 | 6 | 0 | 0 | 0 | 全自动 ✓ |
| TASK-006 | 5 | 0 | 1 | 2 | 含端到端 manual（需真实 qlib 数据/网络），列入验收阶段清单 |

**提示**：TASK-006 的 2 条 manual（端到端 region=us 报告、降级回退）依赖真实数据与网络，
不强制 Dev 写形式化断言，由验收阶段人工/Leader 执行并记录。其余 38 条全部可单测。

---

## 独立 reviewer 反审比对（只读需求、未看 DoD）

reviewer 独立产出验收清单后与本 DoD 比对，结论：核心功能/边界/契约对称/零回归/端到端
**全部已被 DoD 覆盖**。9 条挑刺处置如下：

| # | reviewer 发现 | 处置 | 理由 |
|---|---|---|---|
| 1 | region=us 可被「直接回退 cn」逃避验收 | **采纳** → TASK-006 强化：须先记录 us 真实结果，凭失败证据才许回退 | 防 region 参数化沦为未验证死代码 |
| 2 | 美股 OHLCV 数据可得性未单独验收 | 部分采纳：归入 TASK-006 端到端 manual（E1 非空） | 依赖 yahoo/网络，本质 manual |
| 3 | SIGNAL_SYMBOLS_US ↔ config 双写无机器守门 | **不强制**（记录） | HK 先例同为注释级约束，忠实镜像；加守门属 scope 扩张，留作后续 backlog |
| 4 | ^GSPC 自我比较语义未定义 | 记录 → QA/验收目视 | 相对自身超额≈0，benign；HK ^HSI 同情形已交付 |
| 5 | build_data `^GSPC` vs `GSPC` mismatch | **已化解（核查证伪）** | build_data.py:183 对 expected-symbols 先经 to_qlib_instrument 映射；HK 用 ^HSI 跑通同理；依赖链 T005/006>T003 已保证 |
| 6 | 缺 6 字符 off-by-one reject 样本 | **采纳** → TASK-001/003 boundary 补 ABCDEF(6) | 低成本硬化锚定契约 |
| 7 | 文案改动 advisory 无测试 | 记录 → QA 目视 | 计划已标 advisory；TASK-002 已覆盖 flag help |
| 8 | go vet 未作独立门 | 已在 TASK-006 收尾（计划 Step 5） | go vet ./... 纳入最终验收 |
| 9 | reject 降级摘要未验收 | 不在本 sprint scope | watchlist 无类别股；留 backlog |

**最高杠杆处置**：#1 已强化 DoD、#5 已核查证伪、#3 明确按「忠实镜像 HK」不扩张并记录为 backlog。
