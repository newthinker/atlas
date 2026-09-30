# Final Report — Tiingo 美股备用数据源（sprint 2026-09-30）

- 分支：`feature/tiingo-source`，起点 `8d1c6cc7edf573c9879b53125f486eb6d515f01f` → 终点 `b3232d6f3e2b07610bafe31e7381add623fdb442`
- 需求：`docs/superpowers/specs/2026-09-30-tiingo-source-design.md` + `docs/superpowers/plans/2026-09-30-tiingo-source.md`；人类裁决见 AD-19..21（`docs/01-design/architecture-decisions.md`）
- **本报告所有数字于 `b3232d6` 上、最后一次改动之后统一重采**（Leader，20:4x；代码/配置工作区脏文件 0）

## 1. 交付物
Tiingo 接为美股日线价格备源：prism 美股价格链 `yahoo → tiingo → twelvedata`；`atlas serve`（及 `atlas watchlist`，AD-13）把 tiingo 注册为最后一个外部兜底 collector。

| 类别 | 文件 |
|---|---|
| 新包 `internal/collector/tiingo` | `symbols.go`（白名单）、`normalize.go`（拆股折算/截取）、`client.go`（直连 + Gate + 错误映射/脱敏）、`collector.go`（collector.Collector）+ 各 `_test.go`、`gate_test.go`、`main_test.go`、`testdata/nvda_split_sample.json`、`client_integration_test.go` |
| 改动 | `internal/collector/registry.go`（GetAll 按注册序）、`internal/collector/policy/policy.go`（`tiingo.daily` 40/h）、`internal/prism/refresh.go`（`PriceHop` 有序多跳）、`cmd/atlas/prism.go`（`usPriceHops`）、`cmd/atlas/collectors.go`（注册 tiingo）、`configs/config.example.yaml` |
| 测试 | 上述各包 `_test.go`；`internal/prism/tiingo_integration_test.go`；`cmd/atlas/gate_wiring_test.go` 登记 |

相对 `8d1c6cc`：25 个代码/配置文件，+1799 / −37。

## 2. 任务与验证
| 任务 | 内容 | 返工 | 验证 |
|---|---|---|---|
| TASK-001 | Registry 按注册顺序 | 0 | verified；6/6 变异 KILLED；5 个依赖顺序的调用方前后行为逐个读码核实 |
| TASK-002 | policy `tiingo.daily` | 0 | verified；14/15（1 等价） |
| TASK-003 | 美股白名单 | 1（task_defect：「基底排除数字」无测试守卫——**根因含 Leader AD-22 细化**使原用例 `7203.T` 失去区分力） | verified；13/13 |
| TASK-009 | 拆股折算 normalize（从计划 Task 4 拆出，AD-17） | 0 | verified；20/22（2 等价）；NVDA 数值用 Fraction 独立复算 |
| TASK-004 | 直连客户端 | 1（task_defect：**token 前缀泄露**——`body[:200]` 截断先于脱敏，key 跨界时前缀进错误文本；**源自计划草稿**） | verified；35/35；泄露探针新树 2112 例 0 泄露 / 旧树 612 泄露 |
| TASK-005 | collector.Collector | 0 | verified；21/23（2 存活不在 DoD，见 §5 L） |
| TASK-006 | prism 有序多跳（P17 文案） | 0 | verified；12/12 + M9 预期存活（review 项）；新旧 fetchCloses 同探针逐字对照 |
| TASK-007 | serve 装配 + 配置示例 + gate_wiring | 0 | verified；12/13（C5 预期存活，review 项）；qlib 真实启用验证顺序 |
| TASK-008 | 集成测试（integration 标签） | 0 | verified；**真实 API 断言路径未执行（无 token，AD-9）** |

返工 2 次均为 `task_defect`，均为**测试未守住应守的行为**，无任务触及 `max_rework`。

## 3. 覆盖率与测试（`b3232d6` 重采，两把尺）
| 包 | `go tool cover -func` | profile 逐块去重求和 | 门槛 | 基线 `8d1c6cc` |
|---|---|---|---|---|
| internal/collector | 100.0% | 73/73 = 100.00% | 80 | 100.0 / 100.00 |
| internal/collector/policy | 94.4% | 255/270 = 94.44% | 80 | 94.4 / 94.42 |
| internal/collector/tiingo（新） | 95.6% | 109/114 = 95.61% | 80 | — |
| internal/prism | 94.0% | 471/501 = 94.01% | 80 | 94.0 / 93.95 |
| cmd/atlas | 79.8% | 1325/1664 = 79.63% | coverage_floor 78（AD-8） | 79.4 / 79.27 |

`go build ./...` ok；`go vet`（cmd/atlas、collector/...、prism）无输出；`go test -count=1 ./...` **rc=0，67 个包 ok、0 FAIL**（与 test-tg-b 独立计数 67 一致）；`go vet -tags integration` 两包 ok，未设 token 时两集成测试 SKIP。gofmt：`cmd/atlas/backtest_test.go`、`internal/prism/sankey/template_test.go` 未格式化——**均为基线既有**，本 sprint 未触碰。

## 4. Code Review
- 第 1 轮（`docs/05-review/qa-review-round1.md`）：代码层 **PASS**，CRITICAL 0 / WARNING 3 / LOW 14，fix_items 空。codex CLI 40 秒内无产出（rc=142）⇒ 降级为纯 Claude 三 lens（Skeptic / Architect / Minimalist，结论均落盘）。
- 第 2 轮（`qa-review-round2.md`）：**W1 闭合，最终 PASS**。W1 = AD-23 simplifier 补跑在 git 历史无痕——因零改动无提交，证据见 `06-acceptance/simplifier-pass.md`（22 文件前后 sha256 一致，QA 以 `git show` 独立重算 0 不一致）。
- W2、W3：人类于终验收时**接受**（AD-24，见 §8）。

## 5. 与 spec / 计划的差异（全部有据）
- **人类裁决（2026-09-30）**：P11 白名单收紧（AD-19，Leader 细化为 `^[A-Z]{1,5}([.-][A-C])?$`，AD-22）；P17 降级文案附前序失败跳原因（AD-20）；P16 共享配额容量不足接受并记录（AD-21）；P14 spec §3.6 日志订正不做（AD-15/21）。
- **Leader 细化 AD-22 的代价**：罕见份额类 `.D` 及以上被拒（少覆盖、不耗配额）；另路由表加密前缀（`UNI*`/`LINK*`/`ADA*` 等）使部分真实美股/ETF 被拒（AD-12，既有路由行为）。
- 计划的有意差异沿用：spec §5「直连」测试改结构断言；normalize 对 O/H/L 缺失行不输出但因子仍累乘。
- 计划缺陷由独立 reviewer 实测发现并改入 DoD（AD-18）：14/21 变异在计划测试下存活（主题常量拼错致配额静默失效、缓存从未被行使、Open 未折算、400 未归类等）。
- 实现偏离（有据、已验证）：`Collector.Init` 沿用当前 baseURL（生产行为不变，使「新 key 生效」可测）；`gate_wiring` 登记挪到 TASK-007 并如实注明**不守护** prism/serve 调用点（AD-11）。
- 未登记的低风险偏离（QA L6–L8）：Transport 用 `DefaultTransport.Clone()`；小写代码被拒；spec §3.6「调用方 4 处」原文未订正（实为 5 个依赖顺序的调用方）。
- **LOW（QA L1–L14 + Skeptic L6）未排期**，要点：脱敏区分大小写；未设 `CheckRedirect`（同主机重定向带 Authorization）；`ChangePercent` 在 PrevClose=0 的守卫与 `BRK.B` 行情 Symbol 无测试；装配壳传参（M9/C5）仅 diff 审查；Tiingo 若以 200 返回错误对象，原因会被 decode 错误吞掉（未查实 Tiingo 是否如此）。

## 6. GitNexus detect-changes —— ⚠ Risk: CRITICAL（如实上报，未自行豁免）
详见 `docs/06-acceptance/detect-changes.md`：28 files / 123 symbols / 518 processes。大量 `changed: order` 疑似与仓库既有同名 `order` 标识符按名误配（本 sprint 新增仅 `Registry.order`）；真实语义变化为 GetAll 顺序与新增配额主题，均已在开发期上报。工具 28 files 与 git 27 差 1，来源未查明。

## 7. 机制问题（本 sprint 实测，供 PENDING-MECHANISMS / 上游）
1. **子代理被 idle hook 循环（PENDING #3 同族）×≥6**：code-simplifier（TASK-003、002、009、004）与 QA Skeptic lens 被以父实例名反复催「推进 dev_done / 写 verdict」（单次 7–16 次）；**TASK-009、004 两次 simplifier 只回应 hook、未实际审查** ⇒ 用户全局「提交前跑 simplifier」在 dev 侧不可靠 ⇒ AD-23 改由 Leader 子代理兜底（有效）。`TaskStop` 对 teammate 的子代理不可用；**直接 SendMessage 令其返回有效（3/3）**。**「子代理结论落盘到约定路径」这次全部生效（3 个 lens + reviewer）**——PENDING #3 M4c 那条处方有了正例。
2. **消息延迟（PENDING #4）**：TASK-004 dev 19:27:27 提交，Leader 19:41:23 才 merge（其「请 merge」消息晚到约 14 分钟，时间戳为证）；另多次消息交错（dev 查分支早于 merge 1 分钟）。dev 侧「轮询分支 + 内容判据 + 不照 hook 转 dev_done」全员执行正确。
3. **任务 ID 跨 sprint 复用（PENDING #5）×6**：门禁 `git log --grep TASK-00x` 每任务都混入往届同名提交；**TASK-006 实例最具体**：76 个提交中含 2026-09-16/17 hestia 那批，门禁把 `internal/hestia/*`、`scripts/ops/hestia-warp-trigger*.sh` 列为「本任务改动文件」（本次只打印未拦截）。PENDING 把「不改 `--grep` 实现」列为明确不做——**此条证据供人类重新评估**。
4. **`.gitnexus/run.cjs` 不可用**：选中的 pnpm runner 报 `MODULE_NOT_FOUND`（缺 corepack pnpm 12.8.1）；且 Leader 首次重建经 `| tail` 管道报 exit 0 实为失败（**Leader 仪器差错**，以 meta.json `lastCommit` 直读发现）。全程改用 `npx -y gitnexus@latest`。impact 对 prism 符号多次失效（见 §6）。
5. **validator `archive-mutated` 告警**：`sprint-047-2026-09-17`（+237）、`sprint-049-2026-09-20`（+100/−10）相对同名 tag 已改动——**本 sprint 之前既有**，未处理。
6. **Leader 自身差错（如实记录）**：①zsh 未加引号变量不分词致首次 set-token/派发四连 DENY（未落盘，无影响）；②checkpoint 用不加引号 heredoc，反引号内的 `git merge --no-ff task/TASK-xxx` 被命令替换**实际执行**（因分支不存在而失败，核实 HEAD/MERGE_HEAD 无影响）；③TaskStop 失败前已对 dev 说「我停了」，随即更正；④AD-19 推荐正则与 DoD（HSBA.L）自相矛盾，由 dev 澄清发现；⑤AD-22 细化使 TASK-003 原用例失去区分力，致其返工。

## 8. 待人类决定 / 执行
**人类裁决（2026-09-30 终验收，AD-24）：W2 接受、记入二期；W3 接受并记录。终验收通过，9 个任务转 accepted。**
- **W2 配额乘数**：同一美股标的在 serve 一个 TTL 周期内有 3 个不同缓存键（quote 10 天窗 / FetchHistory / 分析循环）⇒ 最多扣 3 次；`policy/gate.go:171-178` 先扣配额后请求、失败不缓存 ⇒ **404 标的每周期重复扣**。AD-21 接受了「容量不足」但未含此乘数。
- **W3**：yahoo 关闭 ∧ tiingo 与 arbitrator 开启时，`serve.go:331-337` 让 tiingo 承担美股市场上下文，每次仲裁拉 SPY 耗配额。当前 runtime 配置 yahoo 开启，不触发。

**上线（计划 Task 8 Step 3/5，AD-9，均为人类动作）**
1. 带 token 跑集成测试：`ATLAS_TIINGO_TOKEN=... go test -tags integration -run 'Integration|LiveDrill' -v ./internal/collector/tiingo/ ./internal/prism/`（真实断言路径本 sprint 未执行）。
2. runtime `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml` 增加 `collectors.tiingo: {enabled: true, api_key: <token>, markets: ["US"]}`，建议 `collector.topics."tiingo.daily".ttl: 6h`（当前 runtime 配置无 tiingo 行）。
3. 部署（`bash scripts/ops/deploy.sh`，先 `rsync -n -i` 预演）；重启 serve：`launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.serve`，日志应出现 `tiingo collector registered (US price last hop)`。
4. 次日 prism 报告：yahoo 正常时不应出现 tiingo 字样；出现 `tiingo fallback ok` 即降级生效。⚠ spec §6.3「A 股/港股未产生 `tiingo.daily` 计数」**不可按标的验证**——账本只按主题计数（`policy/quota.go` ledgerEntry 无 symbol）；可观测的只有「主源健康时 count==0」。
5. 合并 `feature/tiingo-source` → master 的 PR 由人类决定。

**待决（不阻断）**：QA LOW 14 条 + Skeptic L6 是否排期；§7 机制问题的上游处置。

## 9. 验收前漂移核对（Leader；`verified → accepted` 无机制守卫，PENDING #14，故手工补）
逐任务 `git diff --numstat <verify_baseline.head>..b3232d6 -- <writes>`：**9 个任务全部 0**。仪器自证：阳性对照（TASK-003 返工前基线 `b6c6a25`..HEAD，同两路径）报 **2**，与已知答案一致；脚本用 python argv 传路径（避开上一 sprint 的 zsh 不分词/无 mapfile 两次假结果）。

### 待同步 hooks 清单（人类执行）
| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 sprint 未改动任何 `project-template/hooks/` 或 `project-template/scripts/`（本仓库为消费项目，无 `project-template/`）；机制建议见 §7，落点为上游 ArcForge 仓库 | — |

未改动 `templates/CLAUDE.md.template`，无运行时 `CLAUDE.md` 文档漂移需同步。
