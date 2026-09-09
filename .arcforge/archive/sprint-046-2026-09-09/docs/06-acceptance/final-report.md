# Sprint M3 最终验收报告 —— hestia `warp-hestia` 解读 skill

- **需求文档**：`~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-06-hestia-m3-warp-hestia.md`
- **周期**：2026-09-06 ～ 2026-09-09
- **结果**：**8/8 accepted**，QA 两轮均 REJECT，人类裁决只修三条已复现缺陷，返工后三条全部 VERIFIED
- **团队**：dev-m3-a / dev-m3-b / dev-m3-c、test-m3-a/b/c/d、qa-m3（Leader = 本 session）

---

## 1. 三仓库交付锚（全 sha，不使用会过期的符号引用）

| 仓库 | 锚 | 状态 |
| --- | --- | --- |
| atlas | 归档时的 `master`（见 `changelog.md` 尾部实采值） | 已合入，27 条 `TASK-00x` commit |
| nanoclaw | `2a6d39388e4648eb3be0f15b42ee28b52428c5d8`（分支 `feat/warp-hestia`） | **PR #5 只开不合**（6 commits） |
| loom | `6d6c38f96901dc60f516ef2154283a213782ad5e`（分支 `feat/spool-source-param`） | **PR #13 未合并**，`main` 仍在父提交 `0466116c…` |

⚠️ **两个 PR 都未合并是刻意的**，不是遗漏：合并共享分支是人的决定，属需求文档 TASK-006 Step 0 的人执行前置（见 §4）。

## 2. 任务清单

| ID | 标题（节选） | dev | verifier | rework | 交付形态 |
| --- | --- | --- | --- | --- | --- |
| TASK-001 | atlas history 侧车 + ingest 先侧车后契约 + AST 守卫 | dev-m3-a | test-m3-a | 1 | atlas 代码 |
| TASK-002 | atlas `contract emit` 同产侧车 + CONTRACTS §A/§D 骨架 | dev-m3-a | test-m3-b | 0 | atlas 代码 |
| TASK-003 | loom `spool.archive` 加 `source` 白名单参数 | dev-m3-b | test-m3-b | 0 | **跨仓库**（代码在 loom，证据落 atlas） |
| TASK-004 | nanoclaw worktree + 队列挂载 + 触发段 | dev-m3-c | test-m3-a | 0 | **跨仓库** |
| TASK-005 | nanoclaw `warp-hestia` skill 文档（SKILL.md + 三份 references） | dev-m3-c | test-m3-d | 1 | **跨仓库** |
| TASK-006 | nanoclaw 夹具 + `prepare.py` + `test_prepare.py` | dev-m3-c | test-m3-d | 1 | **跨仓库** |
| TASK-007 | nanoclaw `verify.py` + `test_verify.py` + 开 PR + worktree 收尾 | dev-m3-c | test-m3-d | 1 | **跨仓库** |
| TASK-008 | CONTRACTS §A–§D 全四节 + 全 sprint 自证数字对账 | dev-m3-a | test-m3-d | 0 | atlas 文档（**记录员代理模式**） |

**五个任务是跨仓库的**（003–007）：真实代码在 loom / nanoclaw，atlas 侧只有交付记录文档。这是本 sprint 最主要的结构特征，也是 §6 多条机制发现的共同来源。

## 3. QA Review：两轮均 REJECT

| 轮次 | 结论 | finding |
| --- | --- | --- |
| 第一轮（常规） | **REJECT** | 1 CRITICAL / 3 HIGH / 2 MEDIUM / 2 LOW / 2 INFO |
| 第二轮（跨视角对抗） | **REJECT** | 新增 1 HIGH / 4 MEDIUM / 3 LOW / 3 INFO（不重复第一轮） |

第二轮的核心判断不是「又找到几个 bug」，而是**这道闸守的位置**：封条保护机器区的表格，却不保护承载结论的部分（frontmatter、叙述段）。

### 人类裁决：只修三条已复现的

| 修复 | 任务 | `reason_class` | 缺陷 |
| --- | --- | --- | --- |
| F1 | TASK-005 | `task_defect` | `SKILL.md` Step 3 用 `${EXISTING:+…}` 判「变量非空」而非「文件存在」，而 `$EXISTING` 恒非空 ⇒ **create 场景每期首次生成都失败** |
| F2 | TASK-006 | `dod_defect` | 契约与侧车配对无校验，侧车顶层 `for` 字段**被读 0 次** ⇒ 错配的一对喂进去，prepare 与 verify **双双 exit 0** |
| F3 | TASK-007 | `dod_defect` | 封条作用域到 seal 行之前为止 ⇒ seal 与 end 之间的文本**既不被分段 check 覆盖也不被封条覆盖**，可在机器区内部插伪造表而 verify 放行 |

**F2/F3 判 `dod_defect` 而非 `task_defect`**：DoD 逐字规定了封条作用域，dev 精确实现了它，洞在规格里。两条各累计 1 次，未触发「同任务 `dod_defect` 第 2 次转 `blocked_human`」。

### 返工验证：每条都做了消融，不只验「修后能跑」

| | 修后独立实测 | 消融（拆回缺陷态） |
| --- | --- | --- |
| F1 | 三个独立 shell 实现 × 两场景全绿，create **3247** / update **3263** 可区分 | 换回 `${EXISTING:+…}` → create 全 `exit 2` / 0 字节 |
| F2 | 正配 6/6；错配 `exit 2` / 0 字节并打印两边的值 | `assert_pair` 恒真 → 错配复活 `exit 0` / **3161 字节**，仅一条测试变红 |
| F3 | 攻击 `exit 1`，报 seal 76 行 / end 84 行 / 夹 7 行 | 条件恒假 → 攻击 `exit 0` 复现，4 条断言变红 |

**三个消融复现的数字（0 字节 / 3161 / exit 0）与修前实测逐字吻合** —— 这是「修复真的闭合了缺陷」的直接证据，强于「N 个变异全 KILLED」这类计数（后者只说明有断言变红）。

回归：`Ran 95 tests / OK / EXIT=0`，两把独立的尺同值（静态 `grep -c` 63+32、动态 `Ran 63`/`Ran 32`）。两份 golden 三路证实未变（sha256 现读 / git 变更文件数 0 / 用当前 `prepare.py` 重新生成 `IDENTICAL`）。

---

## 4. 结转项（人执行，不属任何任务）

以下四条**均未实证**，是需求文档 TASK-006 Step 0 的人执行前置：

1. **loom PR #13 合并** —— `main` 是共享分支，合并属人的决定
2. **nanoclaw PR #5 合并** —— 同上
3. **`configs/config.local.yaml` 未改、selvage 未重启**
4. **`Wiki/Macro/PBOC/` 是否被 Spool 自动创建：未实证** —— 已核实该目录当前不存在；`archive.go` 里有无条件 `os.MkdirAll` 是**代码阅读不是运行结果**

### QA finding 中未修、结转的部分

| finding | 等级 | 为何不修 |
| --- | --- | --- |
| H1 八问框架有 5 问所需字段不在笔记里 | HIGH | 规格层取舍，需人拍板改需求而非改实现 |
| H2 CONTRACTS §D 第六条低估未受保护面 | HIGH | 同上；§D 第六条已按「实测行为 / 为何不判红 / 待决问题」三段式登记 |
| A2 批注区「首次出现 + 子串」匹配会致永久卡死 | MEDIUM | 未复现（人类裁决只修已复现的） |
| A3 叙述段是真正的篡改面，无任何核对 | MEDIUM | 规格层，与 H2 同源 |
| B1/C1 侧车与契约大量零消费字段（侧车约 2/3 字节无读者） | MEDIUM | 不影响正确性 |
| B2 同一事实在三仓库各存一份副本，无机制让它们一起变红 | MEDIUM | **本 sprint 最值得后续处理的一条**，见 §6 |
| M1/M2/L1/L2/A4/B3/C2/C3 | MEDIUM–LOW | 详见两份 review 原文 |

**`SKILL.md:64` 注释的覆盖面措辞**（写「sh / bash 3.2 / zsh」，而 `sh` 就是 bash、且未提 dash）：验证者判 finding【低】非缺陷，依据是「注释断言的性质经实测为真且在未列出的 dash 上同样为真，方向是低报适用范围而非高报」。Leader 裁决不单开返工轮，此处留痕。

---

## 5. 验证严格度声明（必须留痕，不得淡化）

本 sprint 有三处验证强度低于原定标准：

1. **TASK-007 砍掉「独立复算变异」** —— 人类决定，理由**不是**那活重（Leader 实测其工作量为 84 tests / 2.313s），而是**减少暴露在环境挂起风险里的工具调用次数**。
2. **QA 第二轮三个 lens 的结论未回收** —— 独立 context 的交叉验证这一层缺失，第二轮报告的每条 finding 均为 qa-m3 本体自己实测。
3. **返工轮的 10 个变异未逐个复算** —— 验证者改做三个针对性消融（见 §3），Leader 判定后者更对准这三条修复，但「未逐个复算」这一事实保留在验证报告中。

---

## 6. 机制发现

**12 条待决机制变更已记入 `PENDING-MECHANISMS.md`**（40 行，本 sprint 新增/扩展 6 条），落点全部是 Arcforge 上游仓库。本节只列本 sprint 特有的，细节不复述：

- **跨仓库任务的 `verify_baseline` 有结构性盲区**（#9）：基线只锚 atlas 的 `head` + `discovery_sha256`，**不锚 loom / nanoclaw 的 commit**。TASK-005 实测 nanoclaw 在派验后 66 秒多了一个 commit，它被发现**只因为 dev 同时也改了 atlas 侧文档**才触发 `--ack-drift`；只改目标仓库则基线完全一致、零告警。
- **跨仓库写通道 fail-open 且静默**（#7）：在另一个也装了 arcforge 的仓库里调写通道，相对路径解析到那边、写进那边的 `.arcforge/`，退出码 0、零告警。
- **门禁的覆盖率尺系统性偏高**（#10）：`go tool cover -func` 只聚合具名函数声明，包级 `var` 里的匿名函数字面量被静默丢弃（cobra 惯用法正是这个形状）。实测偏差 76.8% vs 真值 76.6804%，**方向单向偏高**。
- **Leader 的工作区编辑会阻塞每一个 dev 转 `dev_done`**（#8）：门禁的「他人在途 scope」只从任务 JSON 取，Leader 的未提交编辑不属于任何任务、减不掉。
- **B2 三仓库副本无联动**（QA 第二轮）：同一事实在 atlas / loom / nanoclaw 各存一份，没有任何机制让它们一起变红。本报告的 §1 锚点表本身就是这个问题的又一个实例。
- **行号引用是会过期的锚**：`CONTRACTS.md:3568` 用行号指向验证报告的内容，验证者靠主动想到「写通道是全量覆盖」才改用追加式落盘，否则那个引用会静默失效。

---

## 7. 过程教训

### 7.1 「实例不可见」的判据不足以判定它已死 —— 本 sprint 最贵的一条

四个实例先后出现完全相同的形态：`ListAgents` 恒为 `running`、建完隔离 worktree 后再无任何落盘、不回问询。前三个（test-m3-b/a/c，87 / 94 / 62 分钟）确实已死，被 TaskStop 换人，累计扔掉三份上下文。

**第四个（dev-m3-c，静止 155 分钟）没有死** —— Leader 据同一组判据向人类报了「第四次同形卡死且跨越角色」，而它 155 分钟后自行提交了完整交付并正常收尾。

⇒ **那三条判据只能证明「不可见」，不能证明「已死」**；两种成因在文件层完全同形。唯一实测有效的区分是**要求过程性落盘**（每完成一个大步骤写一行 checkpoint）：做到的实例全程可见（test-m3-d 两轮皆是），没做到的两种成因仍不可分。判据应改为「**无过程性落盘**」这个可要求、可核查的条件，而不是「无产物」这个两义的观察。

dev-m3-c 自己给出了机制化处方：**把 checkpoint 落盘绑到 `git commit` 之后** —— commit 是必然发生的动作，挂在它后面就不需要额外的意志。

### 7.2 仪器对两种不同成因给出同一读数（本 sprint 撞到四次）

| 仪器 | 两种成因 | 同一读数 |
| --- | --- | --- |
| `sh` 这个名字 | dash / bash 的 POSIX 模式 | 同一个调用形式 |
| `unrecognized arguments` | 参数被拆开 / 参数被粘成一个 | 同一条错误消息 |
| `sort -u`（`LANG=en_US.UTF-8`） | 真重复 / collation 折叠不同的行 | 同一个去重结果（方向是**假阴**） |
| 复用输出文件名 | 本次跑出的值 / 上次的残留值 | 同一个看起来合理的数字 |

第四条的实撞形态：验证者补跑 `ARGS=()` 对照时复用了输出文件名，dash 在解析阶段就语法错退出、根本没写出文件，`wc -c` 于是读到上一次 bash 留下的字节数。

**污染没有产生错误，只因为残留值恰好等于真值**（上次跑的是同一场景，而 `prepare.py` 输出确定性）。换个顺序就不是了 —— 若上次跑的是 create，残留会是 3247，对照表就会出现「dash create 3247 / dash update 3247」，读者据此得到「dash 上两场景不可区分」这个**假结论**，而它没有空值、没有异常、没有报错，两个数都是「合理且在别处出现过」的值。**唯一能识破它的是「这个数是这次跑出来的吗」，而那正是复用文件名抹掉的信息。**

⇒ **处方：别信名字，去问它的版本；别信数字，问它是这次跑出来的吗。**「我试过了」必须能答上「在哪个实现上试的」。

### 7.2.1 一次核查，以及它证伪了 Leader 自己的推断

验证者自曝上述污染后，紧跟着写「该值未在任何结论中被引用」。**Leader 判断这句自评偏轻**（表格里明明有 dash 的字节数，而「两场景可区分」这个结论正引用它），并据此建了临时 worktree 独立重跑三 shell 六格取真值。

**核查结果：验证者是对的，Leader 错了。** 受污染的是第二轮 `ARGS=()` 对照表，而那张表在报告里**只有 exit 与错误消息两列、没有字节数列**；六格表用的是六个各自独立的文件名（`rf2-{bash,dash,zsh}-{create,update}.md`，至今在盘上，字节数与结论一致）。Leader 把「自曝复用文件名」这一事实套到了另一张表上。

⇒ **这条留在报告里，落点不是「谁判轻了」，而是那个句式本身**：

> 「我犯了个错，但它不影响结论」—— **前半句自带诚实的外观，会让读者直接跳过后半句**，而后半句才决定要不要继续追。

它的危险**不需要一个真实的失败案例来支撑**。这次核查发现后半句是真的，恰恰说明成本很低（一次 worktree + 六次调用）而收益确定：**无论后半句真假，核过之后它才有证据地位。** 而 Leader 独立重跑得到的 3263 与验证者那个独立文件互证，这个结论比任何一方单独给出的都强。

**另一处同族**：Leader 的更正通报里说「dash 在 macOS 需 `brew install`」，实为 Darwin 25 系统自带于 `/bin/dash`，被验证者当场订正 —— 把「我印象里没有」当成了「它不存在」。而验证者自己写「三个 shell」时，也是把 `sh` 这个名字当成了一个独立实现，没去问它的版本。**同一形状，两个方向，同一轮内各犯一次。**

### 7.3 `ARGS=()` 那个错修法为何能被写进 `fix_items`

Leader 在 `fix_items` 里给的 F1 修法是 `ARGS=(); [ -f "$EXISTING" ] && ARGS=(--existing "$EXISTING")`。它**坏在两个彼此独立的原因上**：

- **dash**：第 6 行 `Syntax error: "(" unexpected`（数组赋值语法非 POSIX，**解析阶段**）
- **bash 3.2 + `set -u`**：第 8 行 `ARGS[@]: unbound variable`（空数组展开，**执行阶段**），而 create 场景下它恰恰是空的
- **zsh**：`exit 0` —— **唯一两条都躲过的**

⇒ dev-m3-c 的总结准确：**「这解释了它为何能被写进 `fix_items`：在写它的人手上那个 shell 上，它是绿的。」**

这条还暴露了一个流程缺口：Leader 发现 `fix_items` 有错时**已无权修改**（`review_fix` 的 owner 随 dev 认领从 leader 转走），只能走消息这条弱载体，**两次都没拿到回执**。最终是 dev 独立实测撞见同一处才闭合的，不是消息闭合的。

### 7.4 巡检脚本自身的盲区

Leader 的六项巡检中，「④ 等 merge 的分支」用 glob `task/TASK-*`，而本轮返工分支叫 **`task/M3-fix`** —— 不匹配，于是从 dev 提交起每一轮都报「无」。**这条恰恰是设计文档里写的「流水线上唯一无活性保障的环节」，而给它兜底的巡检项本身漏掉了它。** 靠 dev 主动发消息才发现。修法是把 glob 放宽为 `task/*`。

### 7.5 Leader 自身的错误（择要）

- **两次把本地时间当 UTC**（`stat -t '%H:%M:%SZ'` 的 `Z` 是字面量），其中一次把 117 分钟前的时间戳当成「刚刚」传给了 verifier，它据此做了错误的活性推断。
- **`grep -cE 'pycache\|\.pyc'` 报 0** —— ERE 里 `\|` 是字面竖线不是交替；同一份输出里的章节标题就是反证。
- **转述他人数字未核原文**：把 `57+27` 抄成 `57+29`。原值 `57+27=84` 与 `55+29=84` 总数守恒 ⇒ 求和自洽校验**恒过**，而我抄错的版本破坏了守恒，**把「校验挡不住」的例子说成了「校验能挡住」的例子**。
- **用错的 jq 键名读 `questions`**（字段名是 `q` 不是 `question`），打出 `null` 后一度认为「数据是空的」——**仪器错被读成数据错**。
- **两处被验证者当场订正**：`dash` 需 `brew install`（实为系统自带）、以及把污染套到了错误的那张表上（见 §7.2.1）。两处都是**把「我印象里/我推断的」当成了「事实如此」**。
- **答在了对方明确指出不可靠的载体上**：TASK-002 的 dev 在提问里写「若 merge 遇到障碍，把障碍写进本条 `answer` 即可 —— 写进文件比发消息可靠」，而我只在 inbox 回了一句，`answer` 字段空到 sprint 结束才补（且因 `verified` 态 leader 无写权，只能等转 `accepted` 后）。

---

## 待同步 hooks 清单（人类执行）

**无需同步。**

本 sprint **未改动任何运行时资产**：`.claude/hooks/`、`.claude/scripts/`、`.claude/settings*.json` 全部零变更（本仓库是 Arcforge 的消费项目，不含 `project-template/`，无模板↔运行时同步面）。

本 sprint 发现的 **12 条待决机制变更全部落在 Arcforge 上游仓库**（`newthinker/ArcForge`），已逐条记入本仓库根的 `PENDING-MECHANISMS.md`（40 行，含落点列）。**这些不是本仓库可同步的项**，需在上游走 TDD + 人类确认后发布，再由消费项目升级。

⚠️ 其中 #11 是关于 write-guard 的 DENY 文案（拦的是命令文本、不是写入目标），**明确建议不改判定逻辑**，只改文案或对写通道调用形态补豁免 —— 该启发式的文件头注释已声明「宁漏不误拦」，按写入目标判定需要解析命令语义，正是它刻意不做的事。

---

**报告作者**：Leader（atlas-7f session）｜**落盘时机**：8 条任务转 `accepted` 之前
