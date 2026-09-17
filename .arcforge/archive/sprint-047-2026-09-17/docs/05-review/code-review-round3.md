# Sprint M2b · 第 3 轮 QA 复审（返工验证 + 三件独立判定）

- 审查者：qa-m2b
- 对象：master `6ed3984cf89733aa457e4405060675d3394e46bf`（上轮基线 `d4243524`，中间 22 个 commit）
- 本轮改动：19 文件 / +1484 / −67
- 基线自跑：`GOTOOLCHAIN=local go test ./... -count=1` ⇒ exit 0、**65 ok / 0 FAIL**

## 结论：**PASS（有条件）**

代码层面可接受。5 条 CRITICAL 中 4 条已修且修得好，第 5 条按 Leader 申报部分修。
**条件是 final-report §8 的三步人执行清单必须在部署前完成**——否则部署那一刻 6 个 launchd 任务全停。
这是人执行项不是代码缺陷，但它是本 sprint 唯一真正的阻塞。

---

## 一、5 条 CRITICAL 的复核（逐条自验，不采信表格）

| # | 修了吗 | 我的求证 |
|---|---|---|
| C1 | ✅ | `client.go:29` 定义 `valueRenderRaw = "UNFORMATTED_VALUE"`，`:126` 与 `:282` 两个读取点都加了 `.ValueRenderOption(valueRenderRaw)`。`toFloat` 补了 `case string: parseSheetNumber(x)` |
| C2 | ✅ | `CreateYearTab` 加第 ⑤ 步清空录入区。**范围核对过**：`SheetId: newID`（不是 `tplID`）、`StartRowIndex: entryFirstRow-1`、`EndRowIndex: entryLastRow` ⇒ 只清新表的行 4–15，不碰表头与 AJ–BB 公式 |
| C3 | ✅ 代码 / ❌ 生产 | 源树 plist 补了 `http_proxy`/`https_proxy`/`no_proxy`；守卫改名并**反转判据**为 `TestHestiaPlistSetsProxyKeysForSheets`。**生产那份我实测仍只有 PATH**（见下） |
| C4 | ✅ | 两层：`internal/config/config.go:Load` 用 `v.InConfig("hestia_sheets")` 探测并报错指路；`internal/hestia/config.go:validate` 拦「配了凭据没给表 id」 |
| C5 | 部分 | 超时 ✅（`sheetsCallTimeout = 2 * time.Minute`，4 处 `context.WithTimeout`）、client 复用 ✅。**退避与重试未做**：`grep -rniE "retry\|backoff\|429"` 于非测试文件 ⇒ **0 命中**。Leader 如实申报，属实 |

### C1 的实现质量高于我 round1 的要求

`parseSheetNumber` 覆盖了我 round1 对照表里的全部六种格式：千分位（去 `,`）、百分号（**只去符号不除以 100**，理由写明了本表同比列存的就是不带符号的百分数）、会计负数（`(933)` ⇒ `-933`）。
这正是我 round1 那张表里 `sameValue=false` 的那几行。

### C3 的守卫反转是本轮最好的一处

新守卫 `TestPBOCFetcherDoesNotProxyPBOC`（`fetch_test.go:32`）是**行为测试**：设 `HTTP_PROXY`/`HTTPS_PROXY`
后取出 transport，`Proxy == nil` 直接放行，非 nil 则对一个真实 PBOC URL 求值并断言返回 nil。
比原来的「plist 不得含 proxy 键」强一个量级——它测的是被保护的那条性质本身，
而不是一个恰好与它同向的代理指标。这是我 round2 建议的「把守卫收窄成它真正要断言的那件事」。

### 🔴 C3 的生产故障此刻仍然存在（我实测）

| 项 | 实测结果 |
|---|---|
| `~/Library/LaunchAgents/…hestia-ingest.plist` 的环境变量键 | `['PATH']` —— **只有 PATH** |
| 与源树那份 `diff -q` | **不一致** |

⇒ final-report §8 第 1 条的声称属实。**在同步并 `launchctl` 重载之前，launchd 下的 Sheets 投影恒失败且静默。**

---

## 二、Leader 交办的三件独立判定

### ① C4 引出的阻塞项：我把它拆成两层，结论不同

**层 A（Leader 问的那一层）：错误文案指向的 `configs/hestia.yaml` 放不住凭据。**

**我的判定：不算 010 的缺陷，同意两位验证者。** 理由三条：

1. 落位决策被 `fix_items[4]` **明确排除在任务外**，这是 Leader 自己写的边界。
2. dev 要做的是「让错误可见」，它做到了，而且做得准确——**文案指向 `configs/hestia.yaml` 是事实正确的**，
   那确实是 `hestia_sheets` 唯一的生效位置。
3. 「那个位置怎么保住凭据」是部署策略问题，不是配置校验代码的职责。
   拆分时留下的缝不等于执行者的缺陷。

Leader 在 final-report `:731` 自己写了「这道缝是我拆出来的」。我核对了 `fix_items[3]` 与 `[4]` 的分工描述，
这个自评准确。**拆分缺陷与执行缺陷是两件事，不该记在 010 头上。**

**层 B（我认为更该被记下的那一层）：这个修复的爆炸半径是全局的。**

`config.Load` 全仓非测试调用点 3 个（`serve.go:69`、`broker.go:79`、`export_ohlcv.go:286`），
而第三个在 `loadConfigOrDefaults()` 内，后者有 **8 个调用点**覆盖 7 个子命令。
⇒ 一个 Sheets 专属配置键的误放，会让 6 个与 Sheets 无关的 launchd 任务全部装载失败。

**这一点有我的责任。** 我在 round2 的原话是「让配置装载在检测到主配置有 `hestia_sheets` 顶层键时
报错并指明正确位置」——**我没有指明在哪一层报错**。dev 选了主配置装载器，字面完全符合我的建议。

**我不要求本轮改它**，因为「让它响」本身是对的，而 final-report §8 已经把顺序约束写成了有方向的三步。
但把更轻的替代设计记下来，供将来取用：**在 `config.Load` 里打 stderr 警告而不返回 error，
同时在 hestia 的命令路径上硬失败**。这样 Sheets 的配置错误只让 Sheets 相关命令失败，
爆炸半径与故障来源相称。

### ② P3（`push.go:203` 的 `DroppedCells` 无守卫）：判 PASS，我同意，但要补一行注释

**我做了隔离 worktree 的 A/B 变异实测**（按 CLAUDE.md 变异纪律，主工作区一个字节未碰）：

| 变异 | 位置 | 所在路径 | 结果 |
|---|---|---|---|
| M-A | `:203` `res.DroppedCells += dropped` ⇒ `_ = dropped` | 第 6 步（新建表后二次 diff） | **存活**：exit 0、`--- FAIL` **0 条** |
| M-B | `:183` 同样变异（对照组） | 第 3/4 步（已有表） | **KILLED**：exit 1、1 条 FAIL（`TestPushReportsDroppedCellsWhenMonthOutOfRange`） |

- 对照组（未变异的 worktree）先跑过一遍：exit 0、`ok`。
- 收尾指纹校验：主工作区 `push.go` 的 sha256 变异前后均为 `422b7ab2049ce8aa…`，`git status` 无源码改动。

⇒ 验证者的描述属实，我独立复证。**成因**：唯一那条测试用 `Options{}`（Apply=false），
第 5 步就 return 了，**根本到不了第 6 步**。`:203` 的覆盖块计数为 1 是别的建表用例带来的，
而那些用例的行月份全合法 ⇒ `dropped` 恒为 0 ⇒ 累加与否无差别。

**这是「覆盖是绿的、守卫是空的」最干净的一个实例**，Leader 在 final-report §6.8 的归纳成立。

**判 PASS 我同意，三条理由我逐条核了：**

1. 代码本身对 —— 读过，两处累加逻辑一致。
2. 验收线只要求一条测试而那条存在且有效 —— M-B 证明它真在守。
3. 生产路径够不到 —— **我核实了**：`periodYearMonth`（`sheets_project.go`）校验
   `month < 1 || month > 12` 并返回 error，经 `BuildSheetRows` 组装的 rows 月份恒合法 ⇒ `dropped` 恒为 0。

**但第 3 条理由有一个没写下来的依赖：** 它成立是因为 `periodYearMonth` 的月份校验，
而那是**本轮 TASK-010 第 2 轮才加的**。也就是说 P3 的可接受性依赖另一个任务的修复。
将来若有人放松 `periodYearMonth`，P3 会同时从「够不到」变成真缺口，**而没有任何东西会提示这个联动**。

⇒ **建议（SUGGESTION，不阻断）**：在 `push.go:203` 附近加一行注释记这个依赖，
例如「本行无独立守卫；`dropped` 在生产路径恒为 0 依赖 `periodYearMonth` 的 1–12 校验」。
一行注释，把一个隐式跨任务依赖变成可搜索的。

### ③ 跨任务盲区：需要机制补丁，但应当比「人工回验」轻一个量级

**先修正一个事实**：Leader 说「结论是绿的，靠的是有人想起来去查」。
我核了本轮 006 对 `cmd/atlas/` 的改动 numstat：

```
8f386a4  cmd/atlas/hestia_sheets.go        9   0
8f386a4  cmd/atlas/hestia_sheets_test.go  13   0
```

**删除列全 0，是纯追加。** ⇒ 这次的绿**不完全是人工补位**：纯追加这个结构事实本身就保证了
009 的测试一条都没少、都还在跑。验证者回去逐条验 009 的 `done_criteria` 是加分项，
但它不是这次绿的唯一支撑。

**我的判定：需要机制补丁，判据就是 numstat 的删除列。**

建议的规则：**越界申报时，若 diff 对他人 `writes` 文件的 numstat 删除列非 0，
要求显式列出删了什么、并由原任务的验证者复核；删除列为 0 时免除。**

四条理由：

1. **可机器判定** —— `git show --numstat <sha> -- <他人writes>`，一条命令。
2. **精确对准真实风险** —— 危险的是删掉已 verified 任务的测试，不是追加。
   本 sprint 真发生过一次：008 删了 007 的占位测试 `TestPushCreateSheetsHookRunsBeforeWrite`。
3. **纯追加占多数，免除它能避免机制退化成形式主义** —— 本轮这次就该被免除。
4. **比「每次跨任务改动都人工回验上游 DoD」成本低一个量级**，而覆盖的正是后者真正要防的那部分。

**为什么 `verify_baseline` 兜不住**：它保护的是「被验任务自己的交付物在验证窗口内不漂移」，
射程是本任务。**他人已 verified 的交付物被本任务改动**不在它的定义域里。这是两个不同的不变量。

---

## 三、final-report 的对抗性检验（Leader 明确要求不要只读它）

我挑了可机器求值的声称逐条复算。**结果是它经得起检验**，逐条列出我验了什么：

| final-report 的声称 | 我的独立求证 | 结论 |
|---|---|---|
| 两份 `config.yaml` 都有 `hestia_sheets`（源树 `:332`、运行时 `:357`） | 直读两个文件 | ✅ 逐字命中 |
| 现跑的 `bin/atlas` 不含新配置结构，所以「现在没炸」 | `strings bin/atlas \| grep -c` ⇒ `hestia_sheets` **0**、`HestiaSheets` **0**、`UNFORMATTED_VALUE` **0**、`DroppedCells` **0**；mtime Sep 15 22:25 | ✅ 成立 |
| 生产 plist 仍只有 PATH | `plistlib` 读出环境变量键 `['PATH']`；与源树 `diff -q` 不一致 | ✅ 属实 |
| `config.Load` 非测试调用点 3 个 | grep 求证 | ✅ 3 个 |
| `loadConfigOrDefaults` 8 个调用点 | grep 求证（9 行，含 1 行定义） | ✅ 8 个 |
| 停摆范围 6 个 launchd 任务 | 我独立解析全部 11 份 plist 的 `ProgramArguments` | ✅ 完全一致 |
| 真实返工轮次是 7 不是 6（§1 自己标注的订正） | 我用 git log 数 fix commit：006 三轮、008 一轮、009 一轮、010 两轮 | ✅ 合计 **7** |
| 移植资产 14 个 + README | `ls migration-assets` ⇒ **15** 条目 | ✅ 一致 |

**额外的实测确证（不是读来的）**：我用当前 runtime 的真实配置跑了一次命令，
退出码单独取、不跨管道：

```
/tmp/qa-atlas prism refresh -c <runtime>/configs/config.yaml   ⇒ exit=1
Error: loading config: … 里出现了 hestia_sheets …
```

⇒ §8 描述的停摆不是推演，**用今天的配置文件加今天构建的二进制就能复现**。
现跑的旧二进制之所以没事，只因它构建于 C4 修复之前。

**我的结论**：final-report 在我抽查的这些点上没有一处不实。它自己标注的几处订正
（核错二进制、路径归因、返工少计）都是真的订正，且订正后的说法比订正前更准。
Leader 要我对抗性检验它，检验结果是它可靠。

**唯一一条我建议补的**：§8 的三步顺序约束里，第 2 步「确保 `configs/hestia.yaml` 不会被下次 rsync 冲掉」
现在是空的，而它是**前置于第 3 步**的。建议把它降级为「本 sprint 内先把 `/configs/hestia.yaml`
加进 `deploy.sh` 的排除表」作为临时措施——这一条不需要在三条出路里做选择，
且它与三条出路中的任何一条都不冲突。没有它，第 1 步挪过去的配置会在第 3 步之后被冲掉。

---

## 四、结转缺口的判定：四条全部可接受

| 缺口 | 我的判定 | 理由 |
|---|---|---|
| R7（010）：「这个 error 会让整批停下」无人守 | **可接受，建议记入下 sprint** | 实质风险是「错误被降级成警告而无人发现」。夹具已在 `migration-assets/`，补测成本低。不阻断上线 |
| R6（010） | **可接受** | 同族但更轻 |
| S9（009）：CLI 侧建 client 的 deadline 无守卫 | **可接受** | 超时值本身已落地（`sheetsCallTimeout`），缺的是守卫。deadline 被误删的后果是回到本轮之前的状态，不是新故障 |
| P3（006） | **可接受，附一行注释建议** | 见第二节② |
| `DroppedCells` 的「期望 − 实得」边界未进注释 | **可接受** | dev 因 006 已进 `verifying` 改不了，它主动不改是对的（判定对象漂移）。该边界已写进 final-report §5.16，载体足够 |

---

## 五、给 Leader 的判定汇总

1. **代码 PASS。** 4 条 CRITICAL 修得好，C5 部分修且如实申报。本轮新增的测试质量高
   （行为守卫替代结构守卫、反空洞断言、纯追加不动他人测试）。
2. **唯一阻塞是人执行项**：final-report §8 的三步。我实测确证部署即 6 任务停摆，
   且建议把第 2 步落成「先把 `/configs/hestia.yaml` 加进 rsync 排除表」这个不需要拍板的临时措施。
3. **三件独立判定**：① 层 A 不算 010 缺陷（同意验证者），层 B 的爆炸半径有我的责任，记下更轻的替代设计；
   ② P3 判 PASS 同意，补一行依赖注释；③ 跨任务盲区建议用 numstat 删除列做轻量机制。
4. **final-report 经得起对抗性检验**，我抽查的 8 条声称无一不实。
5. 结转四条缺口全部可接受，不阻断。

**这是 `max_iterations` 的第 3 轮，我不建议再开第 4 轮。** 剩余项要么是人执行、
要么是下 sprint 的补测，没有一条需要再动本 sprint 的代码。
