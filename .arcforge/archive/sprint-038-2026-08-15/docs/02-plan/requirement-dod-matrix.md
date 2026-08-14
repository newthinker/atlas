# M1c-1 需求 ↔ DoD 双向追溯矩阵

用途：机器化暴露**孤儿需求**（需求无 DoD 覆盖）与**凭空 DoD**（DoD 不对应任何需求）。
生成于 2026-08-14，对应 **8** 个任务（TASK-001..008），共 **60 条 DoD**。

> ⚠️ 本矩阵在独立 reviewer 反审后**已修订一轮**：新增 TASK-008，并改写 TASK-001/002/006/007 的 DoD。§6 记录了完整差集。

## 1. 上游 spec §9 的 13 条 DoD → 本 sprint 落点

| spec §9 条目 | 落到 | 备注 |
|---|---|---|
| `functional[0]` 三种标题都识别 | **TASK-001** `functional[0]` | 样本从 p145 改为 **p146**（ADR-M1c1-03：p145 实测缺《金融统计数据报告》） |
| `functional[1]` id 7 位 / 19 位都提取得到 | **TASK-001** `functional[1]` | 7 位复用既有 `pboc-index-p18.html`（id `5868082`） |
| `functional[2]` 日期从 `<span class="hui12">` 提取 | **TASK-001** `functional[2]` | 实测结构与 spec §2.3 逐字一致 |
| `functional[3]` 停止条件按日期 | **TASK-002** `boundary[0]` | 归到 boundary 更合适（它本质是判停边界）；加了「断言 Fetcher 调用次数」的具体要求 |
| `functional[4]` manifest 逐篇追加写入，含 sha256 | **TASK-003** `functional[1]` + `functional[2]` | 拆成两条：追加时机 / sha256 正确性 |
| `functional[5]` 断点续抓，不重发请求 | **TASK-003** `functional[3]`（判据）+ **TASK-006** `functional[0]`（不重发） | 拆成「怎么判已抓」与「判了之后真的不发」两层 |
| `boundary[0]` 某页 0 条 ⇒ 报错 | **TASK-001** `boundary[0]` | — |
| `boundary[1]` 翻满 MaxPages ⇒ 报错 | **TASK-002** `boundary[1]` | — |
| `boundary[2]` `--from` 比最新还新 ⇒ 第一页停不报错 | **TASK-002** `boundary[2]` | — |
| `error_handling[0]` 单篇失败 → 记 `failed` 并继续，返回非零 | **TASK-006** `error_handling[0]` | — |
| `error_handling[1]` 落盘失败 ⇒ 立刻中止 | **TASK-006** `error_handling[1]` | 与上一条**处置相反是刻意的**，DoD 里显式标注了 |
| `non_functional[0]` 两次请求之间调用 sleep | **TASK-006** `non_functional[0]` | 加了「断言传入的时长」要求 |
| `non_functional[1]` manifest 每篇之后就落盘 | **TASK-003** `functional[1]` | 与 `functional[4]` 合并为一条（同一件事的两种说法），DoD 里要求「中途直接读文件」的断言方式 |

**13 条全部有落点，无孤儿。**

## 2. Global Constraints → 本 sprint 落点

| 约束 | 落到 |
|---|---|
| `article_id` 不设位数下界 | TASK-001 `functional[1]`（含 Sprint 037 教训原文） |
| 某页匹配 0 条 ⇒ 报错 | TASK-001 `boundary[0]` |
| 停止条件按日期，翻满 MaxPages ⇒ 报错 | TASK-002 `boundary[0]` / `boundary[1]` |
| manifest 逐篇追加落盘 | TASK-003 `functional[1]` |
| 产物存 runtime 不进 git；只有测试样本进 git | TASK-007（`--out` 仓库外绝对路径，ADR-M1c1-04）；样本仅 2 份进 git（TASK-001 / TASK-004 各 1） |
| 包 `internal/hestia`，不新建子包 | 全部任务的 `packages` / `writes` 声明（ADR-M1c1-05） |
| 无新增依赖 | 全部任务（复用 `Fetcher` / `parsePaging` / `pageURL` / `resolveURL` / `tagRE`） |
| 每个 task 结束 `gofmt -l` / `go vet` / `go test` 干净 | 每个任务的 `non_functional` 末条 |
| 本迭代**不做**解析 / 期次映射 / 校验 / 入库 / LLM / 标定 | **刻意无 DoD**（非目标不该有验收标准），写在 design-spec §1 与各任务 description |

### ⚠️ 两条刻意不进 DoD 的过程约束（不是漏掉）

| 约束 | 为什么不进 DoD | 落到哪 |
|---|---|---|
| 注释里引用任务编号**带 milestone 前缀**（写 `M1c-1 的 TASK-003`） | 它是**编码风格**，不是可验收的行为；进 DoD 会挤占 8 条上限里的一格，而它的违反成本极低（review 一眼看出） | design-spec §11 + spawn prompt |
| 测试文件的 `import` **按需增补**，别把后面 Step 才用到的包提前写进去 | 同上；且 `gofmt`/`vet` 会挡住未使用的 import | design-spec §11 + spawn prompt |

**这两条被显式登记为「已知的孤儿义务」**，以免下次审查时把它们当成漏项——它们是权衡后的取舍，不是遗忘。

## 3. spec §10 交付验收 → 落点

| 要求 | 落到 |
|---|---|
| 真跑一次，回答「多少期多少篇」 | TASK-007 `non_functional[1]`（`verify_by: manual`） |
| 真跑一次，回答「`rule@v1`→`v2` 切换点」 | 同上 |
| 真跑一次，回答「哪些期缺篇」 | 同上；**并由交叉校验的差集补强**（TASK-005） |
| 抓完立刻单独备份 | **人类会话外执行**（ADR-M1c4：产物落 `~/hestia-backfill-2026-08-14/`，`cp -r` 即可）。⚠️ 不进 DoD——agent 做不了「异地备份」这件事，硬写进 DoD 只会得到一条假绿 |
| 契约里 H8「同期两篇」按 §2.1 更正为**三篇** | TASK-007 `non_functional[0]`（`verify_by: review`） |

## 4. 本 sprint 新增需求（不在上游 spec）→ 出处

这些**不是凭空 DoD**，每条都有出处：

| 新增 DoD | 出处 |
|---|---|
| TASK-004 全部（搜索侧 7 条） | **ADR-M1c1-01**（人类 2026-08-14 裁决）：站内搜索做交叉校验第二来源 |
| TASK-005 全部（交叉校验 7 条） | 同上 |
| TASK-004 `error_handling[0]` + TASK-005 `error_handling[0]`（搜索侧 fail-open + `search_skipped_reason`） | **ADR-M1c1-02**：搜索侧失效不阻断主路径，但必须有声跳过 |
| TASK-001 `functional[3]`（省市分行报告被拒） | **实测发现**：搜索一次命中 30+ 家省市分行的同名报告（「2020年11月厦门市金融统计数据报告」等），既有 `reportTitleRE` 靠「期次段紧跟报告种类」挡住它们；新正则必须保留该结构 |
| TASK-004 `functional[2]`（栏目前缀筛） | **实测发现**：同一篇报告在两个栏目各有一份，调查统计司那份 id 是 32 位 hex ⇒ 不筛则两侧 id 无法比对 |
| TASK-004 `boundary[1]`（条数骤增 ⇒ 报错） | **实测**：`advtime=5` 是前端已注释、后端仍接受的**未公开参数**；它失效时返回全站量级（549141 条）而「看起来完全正常，只是多得多」 |
| TASK-005 `boundary[1]`（搜索空集 vs 搜索失效**分开判**） | 推论自 ADR-M1c1-02：合成一种处置会让「关键词写错导致 0 条」被当成「服务挂了」而静默放过 |
| TASK-001 `boundary[1]`（有列表项但 0 条报告 ⇒ 不报错） | 上游 spec §4.2 明确区分了这两种情形（「这页有列表项但其中没有报告是正常的」），但没给它单独的 DoD 条目 ⇒ 本 sprint 补上，并注明**它与 `boundary[0]` 互补、不是重复** |
| TASK-002 `functional[1]`（跨页去重） | 既有 `Discover` 已有此机制（`seen` map，注释解释了「翻页时新文章上架会把边界那条挤到下一页」），spec 未明写 ⇒ 补上 |
| TASK-002 `error_handling[1]`（`parsePaging` 失败不退化成只扫第 1 页） | `discover.go` 的 `parsePaging` 注释：「月报发布 3 周后必然掉出第 1 页」 |
| TASK-003 `boundary[1]`（manifest 非法 JSON ⇒ 报错不静默当空） | 推论自 spec §4.2「不静默退化」：静默当空会让断点续抓退化成全量重抓 400 次请求且不报错 |
| TASK-003 `error_handling[0]`（原子写，失败不破坏既有 manifest） | CLAUDE.md 的原子写纪律（写临时文件再 rename） |
| TASK-007 `boundary[0]`（`--out` 必填无默认） | ADR-M1c1-04：不设默认值使「误落进仓库」需显式打出来才会发生 |
| TASK-007 `boundary[1]`（`--from` 格式校验） | 命令行常规；`parsePeriod` 对「13月」「0月」的语义层拒绝是同一思路 |
| TASK-007 `functional[1]`（flag 经 cobra 真实解析验，不是查 flag 存在） | **Sprint 037 的 TASK-008** 补过同形状的守卫（`f4d6017`） |

**无凭空 DoD。**

## 5. 机器检查结论

| 检查项 | 结果 |
|---|---|
| 孤儿需求（需求无 DoD） | **0 条实质孤儿**；2 条过程约束刻意不进 DoD，已在 §2 显式登记 |
| 凭空 DoD（DoD 无需求出处） | **0 条** |
| Realistic Scope（每任务 ≤1 package / ≤8 条 DoD / ≤5 文件） | **8/8 通过**（DoD 最多 8 条 = TASK-001/002/003/004/008；writes 最多 4 = TASK-007） |
| Go validator（DAG 无环 / wave 序 / scope 非空且互斥 / 单 owner / context_from 闭合 / epoch 不变量） | **exit 0，8 个任务，0 告警**（反审修订后重跑） |
| 独立 reviewer 反审（只读需求，不看本矩阵与 DoD） | **已完成**，15 条被采纳、1 条部分采纳，见 §6 |

## 6. 独立 reviewer 反审结论

reviewer 只读两份需求文档 + 现有实现代码，**未读 `.arcforge/` 下任何文件**（未看过本矩阵与任何 DoD）。
实测语料：**52 个 index 页请求 / 748 条列表项 / 58 条目标报告 / 2 篇文章页**。

### 6.1 独立证实了 Leader 的两处订正

| 项 | Leader 的实测 | reviewer 独立实测 | 一致 |
|---|---|---|---|
| spec §9「p145 三种齐全」 | 假，只有 2 篇 | 假，`grep -c 金融统计数据报告` = **0**；第三篇在 **p144** | ✅（reviewer 多查明了第三篇的去向） |
| 7 位 id 复用既有 p18 | 已采纳 | 独立指出「按 spec 字面实现，`functional[1]` 无样本可依」 | ✅ |

**「同期三篇被页边界劈开」是常态而非异常**——reviewer 实测 41 对相邻页中 **19 对（46%）**
是同日边界。这解释了 p145 为什么只有 2 篇：不是站点异常，是页边界。

### 6.2 reviewer 发现、Leader 核验后**采纳**的缺陷（本矩阵 §4 之外的新增）

| # | 缺陷 | 落到 | 为什么原 DoD 挡不住 |
|---|---|---|---|
| R1 | **缺正向完整性判据**——52 条 DoD 全是否定式/局部判据，无一条断言「应有 N 篇，实得 N 篇」 | **新增 TASK-008** | 结构性：四条设计叠起来构成一条完整静默漏通道（详见附录 §D） |
| R2 | **判停页 `break` 在扫描前会丢掉该页 ≥`from` 的条目** | TASK-002 `boundary[0]` | 实测 p151 跨 2019-12-25..2020-01-09，且该页恰好 0 条目标报告 ⇒ **这个 bug 写进去，真实语料照样绿** |
| R3 | **相邻页日期连续性** ⚠️ **理由已两度订正，见下** | TASK-002 `boundary[3]` | **原写「抓页码漂移导致跳页」——那是错的**：test-agent-27 用探针直接观察到，真跳页（条目下架⇒后续前移）时守卫**完全沉默**（`TASK-002-verification.md:101`）。必然如此：前移 ⇒ `p(N+1).newest` 变老 ⇒ 判据更满足 ⇒ 永不触发。**它实际抓的是「新文上架⇒内容下移⇒日期倒挂」，且需插入 ≥2 条**。跳页归 **TASK-008** 的完整性对账。判据本身成立（三批实测 0 反例），保留理由是「页序自洽当场可判」+「真跑中就地生效」。详见附录 §G |
| R4 | **越界页是 HTTP 404 而非 200+0 条**（p409/p500 实测 146 字节） | TASK-001 `boundary[0]` | 真实站点**给不出** 0 条这个输入 ⇒ 若只用真实语料，该 DoD 以「没遇到过」的形式**恒绿** |
| R5 | **13 类干扰项**（`小额贷款公司统计数据报告`、`地区社会融资规模增量统计表`、**`第三季度`**…） | TASK-001 `functional[3]` | 原 DoD 只要求「三种都识别」⇒ 一条 `.*统计数据报告` 就能满分通过 |
| R6 | **标题取 `title=` 属性这条在真实语料上无守卫**（748 条里 124 条链接文本被截断，而目标标题恰好都没被截） | TASK-001 `functional[0]` | 改成取链接文本，**全部真实样本测试照样绿** |
| R7 | **`sha256` 与「跳过已抓」互相抵消** | 设计附录 **A2** | 被 §6 声明为「唯一能发现改版的途径」的机制，被同文档 §5.2 关掉 |
| R8 | **`--from` 语义歧义**（发布日期 vs 期次） | 设计附录 **A1** | 实测 2019 年度三篇发布于 2020-01-16 ⇒ 不定义则期望值必然对不上 |
| R9 | **index 快照按页码命名不稳定** | 设计附录 **A3** | §4.3 自己说页码会漂，§3 却定 `index/11040-<N>.html` |
| R10 | **`sleep` 断言应是计数等式**（`次数 == 请求数 − 1`） | TASK-006 `non_functional[0]` | 原措辞「两次请求之间调用了 sleep」**一次 sleep 就满足**：1 条 sleep + 365 条请求照样绿 |
| R11 | **页内 id 位数会混合**（p15 = 1×19 位 + 14×7 位） | TASK-001 `functional[1]` + TASK-007 CONTRACTS | **推翻 `discover.go` 现有注释**「页内不混合」 |
| R12 | **macOS `sort -u` 把任意两个汉字判为相等**，对本语料静默少 25%，少掉的恰是「存量/增量」 | TASK-007 / TASK-008 `non_functional` | **一个用来防静默漏的对账机制，会被同一类静默漏毁掉** |
| R13 | **spec §7 的坑只记了三分之一**（年度期次三者都无期次段；累计期次里只有存量退化成月） | TASK-007 CONTRACTS + TASK-008 `functional[3]` | §7 自称「留给 M1c-2 的坑，现在只记录」，记漏了等于没记 |
| R14 | **末页可少于 15 条**（p408 实测 13 条） | TASK-001 `boundary[2]` | 硬编码 15 会在 `--from 2005-01` 上炸 |
| R15 | **index 抓取失败 vs 文章抓取失败未区分** | TASK-002 `error_handling[0]` | 少一页 index = 静默少 15 条候选，与「整页 0 条」等价危害却走另一条代码路径 |

### 6.3 Leader **不完全采纳**的一条（附理由）

reviewer 建议「硬期望值 `--expect-articles 214` / `--expect-periods 78`，**不符即非零退出**」。

**处置：默认作告警，`--expect-*` 显式传入时才硬失败**（TASK-008 `functional[2]`）。

**理由**：214/78 是**推算**（reviewer 自己标注为推算而非实测），且与 R8 的 `--from` 语义直接耦合
——按设计附录 A1 定的发布日期口径，2019 年度三篇会被收进来，期望值应是 **79 期 / 217 篇**。
让一个未经实测、且随语义定义而变的数**有权阻断交付**，会在推算偏差时卡死整个 sprint。

**但 reviewer 的核心关切全部采纳**：`functional[0]`/`[1]` 的**逐期对账表**用的是**结构规则**
（v1 期次三篇 / v2 期次一篇，由 CONTRACTS 的 H8 支持），**不依赖任何外部推算** ⇒ 它是硬判据。
正向完整性判据这件事本身没有被打折。

### 6.4 reviewer 自己修正过的两处判断（记此以免被当成结论引用）

1. 一度认为「某期开始只有一篇」不成立（因 p20 三篇齐全），抓 p18/p19 后确认切换点存在（**2025-09**）。
2. 一度把「p145 有 21 处 `class="hui12"` 而只有 15 条列表项」当成发现；拆开是 15×span + 5×td + 1×table，
   精确匹配 `<span class="hui12">` 恰好 15 处 ⇒ 数据没问题，是 grep 不精确。
   剩下的真实结论只是那条更窄的话：**`class="hui12"` 不是日期单元格的唯一标识**（已落 TASK-001 `functional[2]`）。

### 6.5 差集小结

| | 条数 |
|---|---|
| Leader 独立发现、reviewer 未提 | 1（`istitle="true"` 计数守卫挡「部分匹配失效」，TASK-001 `boundary[2]`） |
| reviewer 独立发现、Leader 未提 | **15**（R1–R15） |
| 两者独立发现同一问题 | 2（p145 不齐全、7 位 id 需复用 p18） |

⇒ **这次反审的产出远大于成本**：R1（缺正向完整性判据）直接催生了 TASK-008，
而它正是「三个月后发现 manifest 少了几十期」这一失效模式的唯一结构性解药。

## 7. 任务图

```
wave1:  TASK-001 (index 单页扫描)   TASK-003 (manifest)   TASK-004 (搜索侧)
           │                            │                     │
wave2:  TASK-002 (翻页与判停) ←─────────┼─────────────────────┤
           │                            │                     │
wave3:  TASK-005 (交叉校验) ←───────────┼─────────────────────┘
           │                            │
wave4:  TASK-006 (限速抓取与续抓) ←─────┤
        TASK-008 (完整性对账)    ←──────┘        ← 与 006 并行，不同文件
           │                            │
wave5:  TASK-007 (cobra + CONTRACTS + 真跑验收) ←┘  依赖 006 与 008
```

`scheduling=dag` ⇒ 就绪条件是「`dependencies` 全部 `verified`」，不等整个 wave。
wave1 三个任务 scope 互斥（`backfill_scan.go` / `backfill_manifest.go` / `backfill_search.go`
三份互不重叠，两份新样本各归一个任务，`testdata/README.md` 由 TASK-007 统一登记）⇒ 可真并行。

**TASK-001 与 TASK-002 写同一个文件** `backfill_scan.go` ⇒ 靠 wave 序串行（1 → 2），不并行。
