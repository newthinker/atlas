# Final Report — 银行股关键指标月度监控（sprint 2026-09-27）

- 分支：`feature/bank-indicator-monitor`，起点 `b2f85462aa32f7766a07dba5938ea9c8d4440830` → 终点 `7234cd3f2e883a7f8d68d99fe749064ce6ff74dc`
- 需求：`docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md` + `docs/superpowers/plans/2026-09-27-bank-indicator-monitor.md`；人类裁决 R14–R17（见 `docs/01-design/requirements-analysis.md`）
- 本报告所有数字于 `7234cd3` 上、最后一次改动之后统一重采（Leader，16:3x）

## 1. 交付物
`atlas bank report [--bank-config] [--dry-run]`：拉取东方财富（经 aktools）三项监管指标（不良率 / 拨备覆盖率 / CET1），生成预警 + 同期统计 + 排名报告，按 ≤4000 rune 分段推 Telegram；launchd 每月 1 日 09:00。

| 类别 | 文件 |
|---|---|
| 新包 `internal/bank` | types / config / source / analyze / collect / summary / render（+ 各 `_test.go`、`helpers_test.go`、`source_integration_test.go`、`testdata/` 2 个） |
| 命令 | `cmd/atlas/bank.go`、`bank_test.go` |
| 配置 / 部署 | `configs/bank-monitor.yaml`、`deploy/launchd/com.newthinker.atlas.bank-monthly.plist` |

与 master 相比 23 个文件**全部新增**、删除 0 行；未修改任何既有函数。

## 2. 任务与验证
| 任务 | 内容 | 返工 | 验证 |
|---|---|---|---|
| TASK-001 | 类型与配置加载 | 0 | verified，变异 24/25 KILLED（1 等价） |
| TASK-002 | EMSource 数据源 | 0 | verified，26/27（1 等价） |
| TASK-003 | 按指标回退的环比同比与预警（D2） | 1（「不改入参」守卫依赖执行顺序） | verified，29/29 |
| TASK-004 | A+H 去重与同期统计（D15 众数统计期） | 0 | verified，33/34（1 未执行）；观察项 Q27 转入 TASK-005 DoD |
| TASK-005 | 渲染与分段（golden、Q27 守卫、F1/F2） | 1（Split 吞块边界空行——DoD 措辞改判 + 并入 F1/F2） | verified，40/41（1 等价） |
| TASK-006 | `atlas bank report` 命令（D1/D14） | 2（D1 全成功+无 sender 无守卫；QA r1 FIX-1/FIX-2） | verified，26/27（1 等价） |
| TASK-007 | 集成冒烟 + plist + 真实 dry-run | 0 | verified；dry-run 在 sandbox-exec（外连全拒）内执行，Telegram 推送 0 |

返工均为 `task_defect`、均为**测试未守住应守的行为**（实现本身无一返工）。无任务触发 `max_rework`。

## 3. 覆盖率与测试（`7234cd3` 重采，两把尺）
| 包 | `go tool cover -func` | profile 逐块求和 | 门槛 |
|---|---|---|---|
| internal/bank | 99.7% | 303/304 = 99.67% | 80 |
| cmd/atlas | 79.4% | 1312/1655 = 79.27% | coverage_floor 78（基线 b2f8546：78.6% / 78.44%） |

`go build ./...` 通过；`go vet` 净；internal/bank 117 PASS；cmd/atlas 0 FAIL（含 gate_wiring）。`-func` 比逐块求和高，方向与 PENDING-MECHANISMS #10 一致。

## 4. Code Review
- 第 1 轮 **REJECT**（`docs/05-review/qa-review-round1.md`）：Skeptic / Architect / Minimalist 三 lens + codex 跨模型（本次 `codex exec` 可用）。FIX-1（WARNING）launchd 生产路径「非 dry-run + sender 成功 + 部分失败 ⇒ exit 2」无守卫（Leader 隔离复现变异存活）；FIX-2（四方一致）全失败摘要缺 H 股别名附注。
- 第 2 轮 **PASS**（`qa-review-round2.md`）：三个验收变异均转红、无回归。
- 未排期：第 1 轮 S2–S12 共 LOW 12 条（仅建议，不阻断）。

## 5. 与 spec / 计划的差异（全部有据）
- 人类裁决（2026-09-27）：**D1** 非 dry-run 无 sender ⇒ 打印 + exit 1；**D2** 按指标回退最近非空值并标「截至」，环比/同比对该指标自己的上一非空期；**D15** 统计期取披露家数最多的期（并列取晚），晚于统计期的主体单列「领先」；**D16** 预警期次 ≠ 统计期时标注期次。
- 计划缺陷由 Leader / 独立 reviewer 实测发现并改入 DoD：计划注释称 `1.02-0.92=0.10000000000000009`，Go 与 Python 实测均为 `0.099999999999999978`（该边界用例测不到浮点假阳），改用 `0.57→0.67` / `256.04→236.04` / `16.01→15.51`；spec §7 两处孤儿需求（`--dry-run` 不调用 Sender、render golden）补进 DoD；`day()` 测试辅助的跨任务耦合拆除。
- 已声明的格式偏差：集成标签 `integration`（非 `live`）；头部分列「领先 / 未更新 / 失败」；空预警渲染为 `⚠️ 预警 (0)` 换行 `无`；排名标题「按X由优到劣」；三项指标统一 2 位小数。
- 接口差异：`buildBankSender` 返回 `(bank.Sender, error)`（D1 要求 stderr 写原因；与 `buildHestiaSender` 同构）。

## 6. GitNexus detect-changes —— ⚠ Risk: HIGH（如实上报，未自行豁免）
详见 `docs/06-acceptance/detect-changes.md`。compare vs master：11 个受影响流程，经 cypher 闭合截断后**全部是本特性新建的 `RunBankReport →` 流程**，未触及既有流程；diff 全部为新增。工具报 24 files 与 git 的 23 差 1，来源未查明。另：开发全程索引停在 `072f82d`（早于本特性），dev 提交前跑的 `detect-changes --scope staged` 均看不到 bank 代码、结论无效——已于 QA 阶段重建索引后补做。

## 7. 机制问题（本 sprint 实测，供 PENDING-MECHANISMS / 上游）
1. **idle hook 对子代理循环下达越权指令 ×8**（PENDING #3 同族）：code-simplifier ×5（T001、T002、T003 返工、T006 QA 返工、T007）、QA lens ×3（Skeptic / Minimalist / Architect）；其中 6 次由 Leader TaskStop（T002、T006 QA 返工、T007、三个 lens）；被停方在父实例侧显示为「interrupted by user」，**致 dev 三次误以为人类中断而停工待命**（T002 转了 blocked_clarification；T007、T006 QA 返工停下询问），QA 第 1 轮报告也写成了「被用户中断」（第 2 轮已订正）。子代理均按规定拒绝执行。⇒ `teammate-idle.sh` 需识别子代理身份；这是运行时资产，**须人类决定**。
2. **lead 会话自身挂起 ×2**（PENDING #4 新形态：发生在 Leader 侧）：TASK-005 merge 延迟约 1h50m、TASK-006 返工 merge 延迟约 70m（merge commit 时间戳为证）。`in_progress` 刻意无 stale 阈值 ⇒ 只能靠 dev 催办发现；**我一度把它错误归因为 dev 侧消息未达/互等**，靠时间戳才更正。
3. **write-matrix 与 CLAUDE.md 状态表不一致**：`rejected`、`verified` 的实际写者是 `test-*`，CLAUDE.md 状态表写 owner 为 Leader ⇒ Leader 改 DoD 须先迁 `assigned`；`orphan-obligation` 对 verified 任务无法消音（TASK-005 那条为误报：义务已在 DoD boundary 末条且 Q27 变异已 KILLED）。
4. **写通道不校验 `done_criteria` 形状**：Leader 曾把整份任务 JSON 写进 `done_criteria`，rc=0；落盘后直读才发现并从源重建（任务尚未派出，无下游影响）。
5. code-simplifier 回报「无改动/已完成」而实际改动 ×2（T004、T006），均被 dev 的前后 sha256 比对拦下（事后补发的详细报告与指纹一致 ⇒ 措辞错误而非隐瞒）。
6. Leader 自身差错（如实记录）：给出的自检处方 `-shuffle=on -count=N` 只用一个 seed（dev 纠正为多 seed）；转述验证者误指的出处未先核实（dev 纠正）。
7. **范围外既有问题（未修）**：`cmd/atlas` 整包 `-shuffle` 下 `TestBackfillLoadRequiresDBFlag` 顺序依赖（基线 `6d83a0d` 同样失败；疑 `hestia_test.go:1139` 传 `--db` 后未复位 cobra flag）——任何用 `-shuffle` 的门禁都会随机假红。

## 9. 验收前漂移核对（Leader，`verified → accepted` 无机制守卫，PENDING #14，故手工补）
逐任务比对 `verify_baseline.head..7234cd3` 在其声明范围（writes，缺省 packages）内的变更：6 个任务为 0；**TASK-004 有 1 个：`internal/bank/summary_test.go` +21/−0**——即 TASK-005 追加的 Q27「Ahead 不进同期统计」守卫用例（TASK-005 的验证者已对它跑 Q27 变异并 KILLED）。纯追加、TASK-004 原有断言与实现文件均未变 ⇒ TASK-004 的验证结论仍成立，**作为已知漂移接受**。
⚠ 仪器事故如实记录：第一版核对在 zsh 下因未加引号的 `$paths` 不分词、第二版因 macOS bash 3.2 无 `mapfile`，两次都**静默给出错误结果**（前者恒 0 = 假绿，后者不限范围）；靠「TASK-004 那格应为 1」的已知答案反推才发现，第三版打印了「声明路径个数」作为自证。

## 8. 待人类决定 / 执行
**上线（计划 Task 7 Step 6，AD-9，均为人类动作）**
1. 部署二进制到 `/Users/zuowei/workspace/runtime/atlas/bin/atlas`。
2. **同步 `configs/bank-monitor.yaml` 到 `/Users/zuowei/workspace/runtime/atlas/configs/`——当前不存在，不同步则首次触发 exit 1**（test-bk-a 核实；runtime 主配置 telegram 四键齐全）。
3. 真实推送一次确认 Telegram 收到；`cp` plist 到 `~/Library/LaunchAgents/` 并 `launchctl load`。

**待决（不阻断）**
- QA 第 1 轮 LOW 12 条是否排期。
- spec §6：「全部主体无数据但无报错」现为 exit 0（spec 明文）；若希望 launchd 能察觉，需改 spec。
- 第 7 节第 1 条的 hook 修改。

### 待同步 hooks 清单（人类执行）
| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 sprint 未改动任何 `project-template/hooks/` 或 `project-template/scripts/`（本仓库为消费项目，无 `project-template/`）；机制建议见第 7 节，落点为上游 ArcForge 仓库 | — |

未改动 `templates/CLAUDE.md.template`，无运行时 `CLAUDE.md` 文档漂移需同步。
