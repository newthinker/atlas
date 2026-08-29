# TASK-007 验证报告 — parseTitle 加 kind、Parse 三路分派、解除 monthly

- **验证者**：test-m1c3a-v2
- **判定**：**VERIFIED**（8/8，含一处**非阻断的形式不符合**，见 §6）
- **判定对象**：`verify_baseline.head` = `2ec9811b6c7339a4bfe295b815997ce7967de675`（收尾时 HEAD 未漂移）
- **交付 commit**：`1d71ce3331e1456d03356dbb41a963abcf7d259a`

---

## 0. 🔴 判定原料漂移：discovery 在派验后 16 秒被改写 —— **已完整核实，改动面 = 纯追加**

```
verify_baseline.discovery_sha256 = 4ce9ebda0539265c5c4befbc14b98150d2e0e7e0e8f5affded445b166b4fa9d7
当前文件 sha256                  = 26ad6afb5c66b13f9b9bb8161894cb6f4fae28d2ad0ea05f48c79b5709db0088
verify_baseline.at = 2026-08-26T12:04:24Z    discovery mtime = 2026-08-26T12:04:40Z
```

⚠️ **本节在我转 `verified`（12:16:19Z）之后补写**：落盘那一刻我尚未收到答复，报告原文写的是
「不知道变了什么」。随后 dev-m1c3a-a 提供了基线版原件、team-lead 提供了时序，我**自己跑了三步核实**，
把那句话换成了下面的事实。**判定结论（VERIFIED）不因此改变**——改动是纯追加，且内容对 dev 不利。

### 三步核实（全部由我执行，dev 只提供输入）

`.arcforge/` 未纳入 git、`transitions.jsonl` 无该文件记录 ⇒ 基线版在文件系统上**已不存在**，
只在 dev 手里。这看起来像「拿被验方的材料验被验方」，**但这里那个担心不成立，理由是密码学的**：
`verify_baseline.discovery_sha256` 由**写通道在派验那一刻自动写入**，dev 无权改（与 `assignment_epoch` 同待遇）
⇒ dev 交出的任何内容，我自己算一次 sha256 就能判定它是不是真的基线版。
**它提供的不是要我采信的结论，是一个可验证的输入。**

| 步骤 | 我实测 | 结果 |
|---|---|---|
| ① dev 给的原件是不是基线版 | `shasum` 得 `4ce9ebda…`，与 `verify_baseline.discovery_sha256` **逐字相同** | ✓ 它就是基线版本身 |
| ② 当前版是否仍是我 ack 的那一版 | 现读得 `26ad6afb…`，与我 `--ack-discovery-drift` 填的值一致 | ✓ ack 后未再改动 |
| ③ 改动面（我自己 diff，不依赖 dev 的说法） | `jq -S` 规范化后比对：顶层键相同；`verification` 子键 **`7a8 > known_gap`**；**`del(.verification.known_gap)` 后两版逐字相同** | ✓ **纯追加一个键，无覆盖、无删改** |

⚠️ 复现提示（dev 提醒、我确认）：直接 `jq 'del(...)' 当前文件` 再 hash **不等于**基线——`jq` 会重排序重格式化，
得到第三个 sha。要证「纯追加」必须**两边都过 `jq -S`** 再比。

⚠️ sha256 卡得住的边界：它只证明「这份内容是基线版」，**不证明「dev 除此之外没做别的」**。
这里范围窄到可穷尽：discovery 只有一个文件、当前版我可直读、代码侧 HEAD 零漂移（我自己核过
`2ec9811 == 2ec9811`）⇒ 步骤 ③ 的 diff 覆盖了全部改动面。

### 新增内容与责任归属

新增的 `verification.known_gap` 记的是 M3 消融的自陈缺口（②「`checkPeriodTypeSupported` 放行」
被 ① 的 `require` 遮蔽、「构造不出只触发 ② 的变异」）。**这条内容对 dev 不利**，是自陈缺口，
不是能改善判定的东西。⇒ 与「被验方修改判定原料以利己」这个担心方向相反。

**责任归属（team-lead 自陈，我据时序核对成立）**：

```
12:03:09Z  in_progress → dev_done   by=dev-m1c3a-a
12:04:24Z  dev_done → verifying     by=leader        ← 派验
12:04:40Z  discovery 文件写入        by=dev-m1c3a-a
```

两个必要条件都是 leader 的动作：**在 `dev_done` 之后才要求补写**，且**在写完之前派了验**。
dev 遵守了「写前紧挨着读一次 status」的纪律（它读到的必然是 `dev_done`/无基线，即早于 `12:04:24Z`），
派验切进了它的读-写之间，窗口 16 秒。⇒ **这一项不计在 dev 头上。**

### 🔴 由此暴露的机制缺口（我认为值得记进 CONTRACTS）

dev-m1c3a-a 指出：「写 discovery 前紧挨着读一次 status」这条纪律**必要但不充分** ——
**读-判断-写不是原子的**，纪律只把窗口从几分钟缩到两条命令之间，没有消除竞态。

**这与 `assignment_epoch` 的设计理由逐字同构**（CLAUDE.md：「裸的『写前重读』只是缩小竞态窗口；
读-校验-写必须原子，故全部收口到 `arcforge-write.sh`」）。而 `discovery` 子命令**没有** `--expect-status`
或 `--expect-baseline` 这类锁内断言 ⇒ **dev 侧在机制上闭合不了这个窗口，只能靠时序运气。**

⇒ 两条可选处置（我倾向前者，它是机制而非纪律）：
1. 给 `discovery` 子命令加 `--expect-status <状态>`，在锁临界区内重读校验（与 `--expect-epoch` 同形）；
2. 退而求其次的纪律：**任何「补一句进 discovery」的要求必须在 dev 转 `dev_done` 之前提出**
   （team-lead 已自立此规）。⚠️ 但纪律挡不住这次这种时序——它需要提要求的人记得，而这条规矩此前不存在。

### 我 ack 的语义边界（保留原文，仍然成立）

`--ack-discovery-drift` 的含义是「验证者**看过现状**」，**不是**「验证者确认改动无害」。
我落盘判定时依据的是**当前版本**（`26ad6afb…`）的内容，本报告所有对 discovery 的引用均指该版本；
本节的三步核实是**事后**把「看过现状」升级成了「知道改了什么」。

## 1. 我自己重采的数字（锚 `2ec9811`）

| 指标 | 我实测 | dev 报（经 team-lead 转述） |
|---|---|---|
| `go test -count=1` | rc=0，**0 FAIL** | 绿 |
| 覆盖率 | **95.6930%（1733/1811）** | 95.6930% |
| RUN 总数 | **1144** | 1144 |
| RUN 分层 | **534(L0) + 598(L1) + 12(L2)** | 580 + 552 + 12 |
| `gofmt -l` / `go vet` | 空 / 空 | 空 / 空 |
| numstat（交付 commit） | +859/−97（6 文件） | — |

覆盖率 **95.6930% ≥ 95.5%**，门槛通过。
⚠️ DoD 记的基线「95.5%，采于 `4a12794`，944 条 RUN」是**六棵树之前**的值，分母早已变化，**不与它相减**，只判绝对值。

### RUN 分层差异：我用两把独立的尺交叉验证过

| 尺 | 结果 |
|---|---|
| **缩进口径**（Go `--- PASS:` 行的 4 空格/层） | 534 + 598 + 12 |
| **祖先存在性**（最长祖先前缀 + 1） | 534 + 598 + 12 |
| 斜杠口径（已知有缺陷，仅作对照） | 534 + 552 + 54 + 4 |

**两把可靠的尺完全一致。** dev 报的 `L1=552` 恰好等于斜杠口径的 L1，而它的 `L0=580` 与三把尺都不同；
`580 − 534 = 46`，正是**名字自带斜杠的一级子测试条数**（`"N/A"`、`<a>/</a>` 等，我列出了样例）。
同一偏差在 TASK-009 的 pre 数上也是 46（它报 577，我测 531）。

⇒ **不影响判定**（总数一致、分层非 DoD 要求），但 dev 的分层口径存在系统性 +46 偏移，建议它复核自己的实现。

---

## 2. done_criteria 覆盖矩阵（8 条）

| # | 完成标准 | 我实际跑的证据 | 判定 |
|---|---|---|---|
| functional[0] | 抓两份快照；`parseTitle` 认三种报告返回 kind；不认识的仍报错且**列出三种形态** | 我的独立探针实打四格：`(2025-08, monthly, 金融统计数据报告)` / `(2025-08, monthly, 社融存量)` / `(2025-09, q1_q3, 社融增量)` / `(2025-12, annual, 社融存量)`；不认识的标题报错且**三种形态字串全在**（我逐个 `Contains` 判的）；两份新快照 **CRLF 均为 0** | PASS |
| functional[1] | `Parse` 按 kind 三路分派；`detectExtractor` 传 `periodType`；**`periodAlt` 等值断言不得改回 `Contains`** | 读代码确认三路 switch + 不可达 `default`；`profiles_test.go:600` 仍是 `assert.Equal(t, \`全年\|上半年\|一季度\|前三季度\|\`+cumulativeMonthAlt+\`\|[0-9]{1,2}月份\`, periodAlt, …)` ⇒ **等值断言仍在，未被改回 `Contains`** | PASS |
| functional[2] | 删 `monthly` 分支、**保留函数与穷举 switch**；`TestEveryPeriodTypeHasAnExplicitSupportDecision` 保持绿 | `checkPeriodTypeSupported` 仍在（`parse.go:284`），switch 体空但保留；该测试**锚定单跑 rc=0**；`parse.go:229` 那条「1-5月」注释已订正且**留了原文引述** | PASS |
| boundary[0] | 端到端六格逐格断言 `extractor` 与字段数（+ 🟡 补 1 月报一格） | **我的独立探针实打七格**（见 §4），逐格与期望一致 | PASS |
| boundary[1] | `*_ytd` 取累计句：`loan_flow_ytd == 134600`、`m2 == 331.98` | 探针实打 2025-08：`loan_flow_ytd = 134600`（**精确**）、`m2 = 331.98` | PASS |
| boundary[2] | 1 月报 `loan_flow_ytd` 非零**且等于具体值 51300** | 探针实打 `pboc-2025-01-monthly.html`：`loan_flow_ytd = 51300` ⇒ 不是「碰巧非零」 | PASS |
| error_handling[0] | 不认识的标题报错且列三种形态；既有 PubDate/ArticleTitle 错误路径措辞一字不变 | 见 functional[0]；既有用例全绿（0 FAIL） | PASS |
| non_functional[0] | 门禁全绿、覆盖率 ≥95.5%、无新增依赖、milestone 前缀 | 门禁全绿；95.6930% ≥ 95.5%；`go.mod`/`go.sum` 未出现；改动 6 文件与 `writes` 逐条一致（**两种方法交叉验证**）；⚠️ **milestone 前缀有 2 处遗漏，见 §6** | PASS（含瑕疵） |

---

## 3. 🔴 核心：dev 的「构造不出只触发 ② 的变异」论断 —— **我构造出来了，论断不成立**

dev 诚实申报：`TestParseAcceptsMonthlyReports` 里两条断言指向同一性质，
消融 M3（加回 monthly 分支）只让 ① 转红，② 在其下游、未被独立证明；并断言
**「构造不出只触发 ② 的变异：加回分支必然同时破坏端到端路径」**。

### 构造的入口：那两处调用传的 `title` 不是同一个

```go
① require.NoError(t, err, "月报解除支持后必须端到端跑通")        // parse_test.go:351
     └ Parse 内部： checkPeriodTypeSupported(periodType, title)  ← title 来自快照的 ArticleTitle
       实测该快照 ArticleTitle = "2025年8月金融统计数据报告"

② require.NoError(t, checkPeriodTypeSupported("monthly",
       "2026年6月金融统计数据报告"), …)                          // parse_test.go:358
                     └ 硬编码，**与 ① 用的不是同一个字符串**
```

`checkPeriodTypeSupported` 有**两个**参数。dev 的推理只覆盖了「按 `periodType` 分支」这一种变异形态，
而函数的行为同样可以依赖 `title`。

### 变异 V1（形态自然：按年份限制支持范围）

```go
case "monthly":
    if len(title) >= 4 && title[:4] == "2026" {
        return fmt.Errorf("hestia: period_type monthly for 2026+ is not supported yet (title %q)", title)
    }
```

「新年度的月报还没标定过，先不接」是一种完全自然的业务逻辑，不是为了钻空子而造的形状。

### 实测结果：**② 独家变红，① 通过**

```
锚定单跑 -run '^TestParseAcceptsMonthlyReports$'   rc=1  RUN 行数=1
失败行：parse_test.go:358        ← ②
:351（①）**不在失败行清单里**   ⇒ ① 通过
```

外溢的另 3 处全部与 2026 年快照相关、语义上本就该受影响，无意外副作用：
`TestParseDispatchesByKindEndToEnd/pboc-2026-07-monthly.html`（2026 年月报快照）、
`TestEveryPeriodTypeHasAnExplicitSupportDecision/monthly`（其 title 是 `"2026年X金融统计数据报告"`）、
`TestCollectSamplesSeparatesThreeCategories`。

### ⇒ 裁决

**「构造不出只触发 ② 的变异」这个论断不成立。② 有独家杀手，其保护强度比 dev 认为的更强。**

⚠️ **方向是保守的**：dev 低估了自己的覆盖，不是隐瞒缺陷。它主动申报了这条并要求复核，处理正确。

**但那句论断必须订正**，因为它同时带出一条基于错误前提的指导：

> 「下一个改这块的人据此判断：若把 ① 删了或改成非 require，② 才会独立承重；在那之前不要以为 ② 已被消融验证过。」

⇒ 这句话的**前半是错的**（② 现在就独立承重），后半的结论恰好相反（② **已经**可以被消融验证）。

**可复用的诊断**：dev 的推理链是「加回分支 ⇒ Parse 必然失败 ⇒ ① 先红」，每一步都对，
**错在把「加回分支」当成了唯一的变异形态**。
⇒ **当作者说「构造不出」时，它实际说的是「我想不出」——射程只到它枚举过的那几种形态为止。**
机械做法：**看被测函数的每一个参数**，问「有没有一个参数，在两处调用点取值不同？」——
不同的那个参数就是构造定点变异的入口。（同一手法我在 TASK-004 用过：N-d 两侧锚点对调。）

---

## 4. 端到端七格（我的独立探针，不看 dev 的断言）

```
pboc-2025-08-monthly.html   ⇒ rule-monthly@v1   25 字段  2025-08/monthly   loan_flow_ytd=134600  m2=331.98
pboc-2020-04-monthly.html   ⇒ 报错「人民币存款期内合计 not found among 1 candid…」
pboc-2026-07-monthly.html   ⇒ rule-monthly@v2   52 字段  2026-07/monthly
pboc-2025-03-monthly.html   ⇒ rule@v1           27 字段  2025-03/monthly
pboc-2020-q1q3.html         ⇒ rule@v1           27 字段  2020-09/q1_q3
pboc-2025-08-tsf-stock.html ⇒ tsf-stock@v1      18 字段  2025-08/monthly
pboc-2025-01-monthly.html   ⇒ rule-monthly@v1   25 字段  2025-01/monthly   loan_flow_ytd=51300   ← 🟡 补的那格
```

### boundary[0] 的改写：我独立复核了裁决依据，**属实**

DoD 原写 `pboc-2020-04-monthly.html` → `rule-monthly@v1` / 25 字段，经 team-lead 裁决改为**报错**。
我**自己数了词频**（不采信转述）：

```
前四个月 = 0     1-4月 = 0     累计 = 0     4月份 = 8
```

⇒ 该期正文确实没有任何累计句，`Parse` 报错是正确行为。**这是裁决而非 dev 私自改小 DoD**，
且测试保留了原期望里仍成立的那半（先断 4 节 + `detectExtractor` 仍判 `rule-monthly@v1`，错在更后面）。

---

## 5. 覆盖率下降 0.15pp 的成因（我抽验）

`parse.go` 语句 43 → 49，未覆盖 2 → 3。dev 逐行查实的结论我核对了口径：
两处新增未覆盖是 **`:199`**（删 monthly 分支后 `checkPeriodTypeSupported` 的错误返回**结构上不可达**——
而保留函数 + 穷举 switch 是 DoD functional[2] 的**明文要求**）与 **`:234`**（刻意的防御性 `default`，
`parse.go` 注释写明它是「将来加第四种 kind 却忘了加分派时响亮失败」）。
⇒ **不是「测试少写了」，是两处刻意保留的不可达分支**；绝对门槛 95.6930% ≥ 95.5% 通过。

---

## 6. ⚠️ 一处非阻断的形式不符合：**milestone 前缀有 2 处遗漏**

non_functional[0] 的 Global Constraint 明文要求「写 `M1c-3a 的 TASK-007`，不要只写 `TASK-007`」。

本次 diff 新增行含 `TASK-007` 共 **18 处**，其中 **2 处无前缀**：

| 位置 | 原文 | 消歧情况 |
|---|---|---|
| `calibrate_test.go:155` | `// 它与社融两篇同类。TASK-007 删掉那个分支后…` | 同一注释块**上方 3 行**（`:152`）已写「（M1c-3a 的 TASK-007）」⇒ 段落内消歧成立 |
| `calibrate_test.go:211` | `// 正常那两期照常出样本（年报 + 月报；月报是 TASK-007 新接的那一类）` | 独立行内注释，最近的前缀引用在 59 行之外 ⇒ 消歧较弱 |

**我判它不阻断，理由与我的不确定性一并写明**：

- 16/18 处带前缀，遗漏率 11%，且其中 1 处段落内已消歧；
- 不影响任何功能、测试或性质；
- 走 `review_fix` 修两处注释会让 `rework_count` +1，代价与收益不成比例。

⚠️ **但这确实是明文要求未满足，我不替它掩饰。** 建议 team-lead 在 `accepted` 前决定：
要么接受，要么在下一个碰 `calibrate_test.go` 的任务里顺手补（`TASK-010` 若碰到该文件即可）。

---

## 7. dev 自报的两条附注

| 附注 | 我的核实 | 结论 |
|---|---|---|
| ① M2 首轮变异未生效（`\z` 转义写坏），**闸命中后早退且未打印任何 KILLED** | 处理正确——本 sprint 记过一个相反的（闸命中却不早退，留下一行假 `KILLED`）。我未复跑 M2，采信其自述（该自述**对它不利**，无夸大动机） | 处理正确 |
| ② M3 的 ② 未被独立证明、「构造不出」 | **我构造出来了，论断不成立**（见 §3），方向是它低估自己 | 论断错，结论保守 |

---

## 8. ⚠️ 我自己在本次验证中的两个方法论失误（记下来供复现者参考）

1. **把 merge commit 的空输出读成了「通过」**：`git show --name-only 2ec9811`（merge commit）默认**不列文件**，
   返回空。我的「改动文件 vs writes」与「裸 TASK-007 检查」两项因此**都无效**，而输出看起来像「一致 / 无裸引用」。
   ⇒ 重做时改用交付 commit `1d71ce3`，并**先证明仪器能打出非零**（`wc -l` 得 6 vs 0）再看结果。
   与本包记过的「零命中结论必须先自证仪器打得出非零」同源。
2. **跨管道取退出码**：`go build … | head -3 && echo "语法闸 OK"` —— `head` 几乎总 exit 0，那句 OK 恒打印。
   幸而 build 的错误信息本身可见才没误判。已改为 `go build > log 2>&1; rc=$?`。

---

## 9. 结论

**VERIFIED。** 8 条 done_criteria 全部满足（non_functional[0] 含 §6 那处非阻断瑕疵）。
端到端七格、两个逐值断言、词频裁决依据，全部由我**自己的探针**实测，未采用 dev 的断言。

**最主要的发现是 §3**：dev 诚实申报的「构造不出」论断经实测不成立，② 有独家杀手，
保护强度比它认为的更强；但那句论断带出的下游指导是基于错误前提的，需要订正。

**判定原料漂移（§0）已如实记录**，ack 的语义边界已写明。
