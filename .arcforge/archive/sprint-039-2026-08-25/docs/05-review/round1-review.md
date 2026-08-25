# Sprint M1c-2 第一轮 Code Review（qa-m1c2-r2）

- 范围：`fba0feb..204a0a8`，`internal/hestia/` + `cmd/atlas/`
- 主题：**守卫在场，但不设防**
- 消融/探针一律在 `git archive 204a0a8` 出的临时副本（`$(mktemp -d)`）上跑。
  收尾核实：`git status --porcelain -- internal cmd` 空，`thresholds.go` / `calibrate_report.go`
  的 sha256 与副本还原后逐字节一致 ⇒ **主工作区一字节未动**。
- 副本基线：`go test ./internal/hestia` 绿，`-v` 932 条 PASS、0 FAIL（消融对照组）

## 本轮覆盖清单

| 对象 | 状态 |
|---|---|
| `thresholds.go` / `config.go` / `validate.go` + 测试（TASK-001） | ✅ 读 + 1 探针 |
| `calibrate.go`（TASK-002） | ✅ 读 |
| `calibrate_report.go`（TASK-003） | ✅ 读 + 2 探针 + 1 消融 |
| `cmd/atlas/hestia.go` + `hestia_test.go`（TASK-004） | ✅ 读 |
| `CONTRACTS.md` 新增 8 条 + 其自检判据 | ✅ 读 + 自检命令真跑（含阴性对照） |
| `ingest_test.go` / `store_test.go` 连带改动 | ✅ 读 |
| 零生产调用方扫描 | ✅ 分母 21，命中 0 |
| 消融：默认分档表共享 map / `computeFieldStats` 就地排序 | ✅ 各 1 次 |
| **未覆盖**：`.arcforge/docs/06-acceptance/calibrate-report.md` 的 556 行正文逐条复核 | ❌ 只做了区间零宽计数（分母 58） |
| **未覆盖**：`parse.go` / `backfill_*.go` 等未改动文件与新代码的交互面 | ❌ |
| **未覆盖**：跨视角对抗审查（第二轮） | ❌ 按 leader 指示暂缓 |

---

## 发现

### R1-01 [MEDIUM] 「失败：无」与「标题解析不出期次 N 条」同时出现在一份报告里

- **file:line**：`internal/hestia/calibrate_report.go:210`
- **证据类型**：**跑出来的**（临时副本上真跑 `renderCalibrateReport`）
- **具体场景**：`Failures=nil`、`FetchFailed=nil`、
  `Unclassified=["2024年X月金融统计数据报告（新表述）"]` ⇒ 同一份报告输出

  ```
  失败：无
  标题解析不出期次（1 条，原文照录 —— 非 0 意味着站点改了期次表述）
    2024年X月金融统计数据报告（新表述）
  ```

- **为什么是「守卫在场但不设防」**：`FetchFailed` 那一半被专门堵过
  （`TestRenderCalibrateReportCountsFetchFailuresWhenParseFailuresAreEmpty`，
  注释写「`Failures` 为空**不等于**没有失败」）。同一句话对 `Unclassified` 一字不差地成立
  —— 它既没进 `noFailureClaim` 的条件，也没有对应测试。
  `renderCalibrateReport` 的函数注释自称「只渲染 Samples 会让『失败：无』这句话变成假话」，
  这句话在 ④ 这一格上**至今仍是假话**。
- **危害**：④ 的语义是「站点改了期次表述」，是本工具最该让人看见的一类变化；
  跳读表头的读者会据此判定这趟干净，而 M1c-3 入库前要清零的清单被提前否定。
- **建议**：`noFailureClaim` 的条件加 `len(res.Unclassified) == 0`；或把这句话降格为
  逐格计数（「解析失败：0 篇 / 抓取失败：0 篇」），不再对全局下断言。
  配一条与 FetchFailed 那条同形的测试。

### R1-02 [MEDIUM] 建议区间的守卫钉在 `n` 上，而它自己命名的危害在 `span` 上 —— n=22 也能产出零宽区间

- **file:line**：`internal/hestia/calibrate_report.go:23-32`（`minSamplesForSuggestion` 的理由）、
  `:84-86`（`span := s.Max - s.Min`）；测试侧 `calibrate_report_test.go:136-137`、`:124`
- **证据类型**：**跑出来的**
- **具体场景**：某字段各期取值全等（探针用 1234.5）⇒ `span=0` ⇒ `HasSuggestion=true`、
  `Suggest=[1234.5,1234.5]`。`n=3` 与 `n=22` 两档实测宽度均为 0。渲染出来是

  ```
  m2   3 annual×3   1234.5  1234.5  1234.5  1234.5  1234.5  [1234.5,1234.5]
  ```

- **为什么是「守卫在场但不设防」**：`minSamplesForSuggestion` 的注释把危害讲得很清楚
  ——「span 太小**或为 0** 时……**过窄的建议比没有建议更危险：它看起来是个结论**」，
  并**刻意**论证判据取 `n` 而非 `span > 0`。那段论证本身是对的（`span>0` 会在 n=2 时给建议），
  但两者不是二选一：**`n≥3` 不蕴含 `span>0`**，注释点名的危害在 n 大时原样存在。
- **上界断言在此处平凡为真**：`assert.LessOrEqual(SuggestMax-SuggestMin, 3*span)`
  在 `span=0` 时是 `0 <= 0` ⇒ 那条专门用来挡「建议任何值都合法」的守卫，
  对「建议只有一个值合法」这个相反极端**同样放行**。
- **未在真跑上发生**：`.arcforge/docs/06-acceptance/calibrate-report.md` 里 58 个建议区间
  （分母 58），零宽 0 个。⇒ **潜在缺陷，不是已发生事故**。
- **危害**：读者照抄进 `MagnitudeRanges` ⇒ 该字段除恰好等于那个值以外的一切观测被
  `gateMagnitudeSanity` 拦下，而区间形态上完全正常，M1c-3 排查时只会怀疑数据。
- **建议**：`HasSuggestion` 的条件补 `span > 0`（与 n 条件**并存**，不是替换）；
  `span==0` 时打显式标记（如 `span=0`）而不是留一个看起来是结论的零宽区间。
  测试补「n≥3 且样本全等」一格。

### R1-03 [MEDIUM] 两道「刻意相反」的守卫，在**生产配置**路径上不按注释宣称的方式合成

- **file:line**：`internal/hestia/validate.go:349-357`（运行时缺档记 `skipped{no_threshold}`）
  与 `internal/hestia/config.go:63-88`（装载时缺档一律拒绝）；生产配置 `configs/hestia.yaml:68-73`
- **证据类型**：**跑出来的**（探针在副本上把 `validPeriodTypes` 加一档 `q2`，两条路径各跑一次）
- **实测输出**：

  ```
  (a) 生产配置 LoadConfig err = hestia: invalid config configs/hestia.yaml:
      thresholds.stock_continuity_max 缺 period_type: q2（……）
  (b) 完全不写这张表  LoadConfig err = <nil>; 默认表档数 = 5
      运行时 q2 ⇒ status=skipped reason="no_threshold:q2"
  ```

- **问题**：运行时那道 `skipped` 分支的**全部理由**是
  「代码里有了新 period_type、配置还没跟上 …… 判 failed 会让那一批期次全部进 pending，
  而真实情况只是还没给这种序列定上限」。但 `configs/hestia.yaml` **显式写了这张表**
  ⇒ 那个场景在生产路径上产出的是 **LoadConfig 硬失败、`atlas hestia` 整个跑不起来**，
  比它想避免的「进 pending」**更重**。宽容分支只在「配置完全不写这张表」时生效，
  而生产不走那条路。
- **这不是说哪道守卫写错了**——两道单独看都对，且各自有测试。问题是**注释宣称的合成效果
  在真实配置上取不到**，下一个加 period_type 的人会照着注释预期「优雅降级」，实际撞上硬停。
  形状与本 sprint 已自查出的「旧格式标量守卫不可达」同族（`config.go` 里那段已明确
  「写了也永远进不去，只是让人以为有守卫」）。
- **建议**（择一，不必本 sprint 做）：① 把 `config.go` / `validate.go` / `thresholds.go`
  三处注释订正为「生产配置写了表 ⇒ 加档时 LoadConfig 会硬失败；运行时宽容分支只覆盖
  不写表的配置」；② 或让 `checkStockContinuityComplete` 对**新增**档降级为 warning。
  ⚠️ 注意 `defaultStockContinuityMax()` 是硬编码 5 键字面量，加第六档时
  `TestDefaultStockContinuityIsTieredByPeriodType` 会红 —— **那道守卫是有效的**，已核。

### R1-04 [LOW] `computeFieldStats 不就地排序` 的**理由**已失效（结论仍对）

- **file:line**：`internal/hestia/calibrate_report.go:58-60`
- **证据类型**：**跑出来的**（消融：`sorted := slices.Clone(samples)` → `sorted := samples`）
- **消融结果**：**恰好红 1 条**，且是直接断言者 `TestComputeFieldStatsDoesNotMutateInput`；
  环比一节的三条测试（`TestStockContinuityRates*`）**全绿**。
- **问题**：注释写「入参……顺序是期次升序，**tsf_stock 的环比变化率正依赖那个顺序**。
  就地排序会毁掉它，而本函数自己的全部断言照样绿 —— **受害的是另一个函数**」。
  该受害者**不存在**：TASK-003 把环比改成走 `res.Records` 并在
  `calibrate_report.go:315` 自己 `slices.SortFunc` 排序。`res.Samples[f]` 的顺序
  经全仓 grep 只有一个消费点（`:164` → `computeFieldStats`，而它 clone 后排序）
  ⇒ **当前没有任何东西依赖 Samples 的顺序**。
- **危害**：守卫本身该留（不该改调用方的切片），但**判断它是否 load-bearing 的人会照注释
  去测环比，发现毫无影响，从而可能删掉 clone**。这是「结论对但理由错」的标准形态。
- **建议**：把理由订正为「`collectSamples` 的返回值是调用方的数据，纯统计函数不得改它」，
  并注明环比已改走 `Records` 自排序。

### R1-05 [INFO] 零生产调用方扫描：**检查 21 个符号，命中 0**

分母是本 sprint 新增/改动的 21 个函数（`fieldCells` `writeFieldRow` `writeFieldRowWithMix`
`periodTypeMix` `stockContinuityRates` `writeStockRateSection` `writeFailureSection`
`writeParseFailures` `samplesFromRecords` `groupUnsupported` `manifestWarnings`
`writeCollectSummary` `classifyArticles` `unsupportedPeriodType` `collectSamples` `Calibrate`
`computeFieldStats` `nearestRank` `checkStockContinuityComplete` `defaultStockContinuityMax`
`stockObsOf`），逐个 `grep -rn '\bF(' --include=*.go | grep -v _test.go` 数生产调用点。

- 全部 ≥1 个生产调用点；**没有孪生副本 / 死分支形态**。
- 唯一 prod=0 的是 `stockObsOf`（定义在 `_test.go` 里的测试辅助函数，本就该是 0）。
- ⚠️ 与已知发现「`writeFieldRow` 本次真跑零执行」**不是同一件事**：它有生产调用方
  （`writeStockRateSection:346`），只是本次语料下环比一节整节为 `—`。已知发现成立，本项不重复。

### R1-06 [INFO] 消融旁证：默认分档表若共享一份 map，会红 8 条 —— 但**红的全是受害者，不是断言者**

- 消融：`defaultStockContinuityMax()` 改为返回包级共享 map。
- 结果：8 条 FAIL，含 `TestLoadConfigReadsStorage` / `TestLoadConfigKeepsDBPathRelative` /
  `TestLoadConfigRejects/豁免缺_period_types` 等**与该性质无关**的用例
  —— 成因是 `TestStockContinuitySkipsWhenPeriodTypeHasNoThreshold` 里的
  `delete(cfg.StockContinuityMax, "monthly")` 污染了全局。
- 判定：「每次调用返回新 map」这个性质**会被发现**（不是静默），但**诊断信息严重误导**：
  看到 `TestLoadConfigReadsStorage` 红的人不会想到共享 map。
- 不作为缺陷提出（性质成立、注释也写了理由）。若愿意加固，一条直接断言
  `DefaultThresholds().StockContinuityMax` 与再调一次的结果**不是同一个 map**即可。

---

## 已确认**成立**的守卫（抽查 12 项，非穷举）

按「注释里每个『必须/否则』能否指出一条会因它变红的测试」逐条对：

| 声明 | 对应测试 | 判定 |
|---|---|---|
| `checkStockContinuityComplete` 必须查 `v` 而非 `cfg` | `TestLoadConfigRejects`「显式写了 map 但漏一档」（查 cfg 则五档恒齐 ⇒ 该格红） | ✅ |
| 只在 `IsSet` 为 true 时查（不写整张表是正常路径） | `TestLoadConfig` + 「完全不写时必须拿到齐全的默认分档表」 | ✅ 成对，缺一会漏 |
| 旧格式标量失败**在 Unmarshal 层**（那段 `len==0` 守卫不可达） | `TestLoadConfigRejectsLegacyScalarStockContinuity` 断 `parsing config` 而非 `invalid config` | ✅ 罕见地钉住了「守卫在哪一层」 |
| 分档不得退化成「五个同一个数」 | `Less(monthly, annual)` + 同一 5% 跳变在 monthly/annual 判定相反 | ✅ |
| 「恰好在阈值上」那格必须真落在阈值上 | `atBoundary` + `require.Equal(阈值, *c.Value)`（**精确**相等，非 `InDelta`） | ✅ 本 sprint 质量最高的一处：它守的正是「断言静默退化成平凡真」 |
| 运行时缺档 `skipped` 且优先于 `absent_field` | `TestStockContinuitySkipsWhenPeriodTypeHasNoThreshold` 两个子测试（含两理由同时成立那格） | ✅ 顺序被钉住 |
| 建议区间加性而非乘性，且宽度有上界（挡 `±MaxFloat64`） | `TestSuggestionIsAdditiveNotMultiplicative`；`suggestSpanFactorK=3` 由公式**推导**并写死在测试侧（不从实现取） | ✅（span=0 时平凡为真，见 R1-02） |
| `n<3` 不给建议，n=2 那格两值**必须不同** | `TestSuggestionWithheldBelowMinSamples` | ✅ 该格刻意防了 `span>0` 那种错实现 |
| `Calibrate` 收 nil Out 报错 / `collectSamples` 收 nil 合法 | `TestCalibrateRejectsNilOutWhileCollectSamplesAllowsIt` + cmd 侧 `NotEmpty(out)` | ✅ 两侧都在 |
| flag 经 cobra **真实解析**后落到变量；默认值用 `DefValue` 不读变量 | `TestHestiaBackfillCalibrateFlagsParse` / `…IsRegistered` | ✅ 上个 sprint 的教训已内化 |
| 报告按列取而非 `Contains`（数字/子串冒名） | `reportRow`：首列**精确等于**字段名 + **恰好 1 行** | ✅ 同时挡住 `tsf_stock ⊂ tsf_stock_yoy` |
| 一个消融只该红一条 | `TestRenderCalibrateReportMarksFieldsWithoutSamples` 显式**不**断 n 列 | ✅ |

### CONTRACTS.md 自检判据（真跑核实，含阴性对照）

`awk` 限定小节 + `grep -oE '\*\*[A-D][0-9]+｜'` ⇒ **8**（A1 A2 B1 B2 C1 C2 C3 D1），与文档声称一致。
阴性对照：不限定小节时是 **16** ⇒ 那句「必须限定到本节再数」不是空话；
`awk` 切出 **87 行**（非 0）⇒ 排除「命令没切到、0 命中被读成 0 条」这一族。**这是本 sprint 少见的
真正自证的判据**。

### 已自我登记、不另计为发现

`store_test.go:472` 明写「`Calibrate` 不碰库这一点目前**没有**像 `Parse` 那样的独立 AST 守卫……
**这是一个已知缺口，不是遗漏**」，`CONTRACTS.md` 的 D1 同条登记。⇒ 缺口真实存在，
但已被点名并留下指针，不重复提出。

---

## 结论（第一轮）

**PASS**（无 CRITICAL、无 HIGH）。

三条 MEDIUM（R1-01 / R1-02 / R1-03）与一条 LOW（R1-04）建议进交接清单：
R1-01 与 R1-02 各是一处两三行的实现修补 + 一条测试；R1-03 与 R1-04 是**注释订正**
（都属「注释宣称的性质取不到 / 已失效」，不改行为）。

⚠️ 本结论只覆盖上表「已查」部分。第二轮（跨视角对抗）未做。
