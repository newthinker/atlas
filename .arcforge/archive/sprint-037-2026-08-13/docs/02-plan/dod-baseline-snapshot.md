# DoD 基线快照（`63ac5b6` 时点，wave1 派发前）

> **为什么存在**：`.arcforge/tasks/` **不被 git 跟踪**（实测 `git ls-files` 返回 0），任务文件是**覆盖写**，
> 而 `transitions.jsonl` 的 update 审计**只记 keys 与 added/removed 摘要、不记字段值全文**
> ⇒ **任务文件的任何历史内容都不可再生**（test-agent-26 查实）。

> 本 Sprint 授权了「dev 作为 owner 自己改 DoD」（解决 Sprint 036 的 G33），代价是**被验方能改判定依据**（见 H1）。
> verifier 抢在 dev-52 改写前存下快照；**Leader 将其从 session 级 scratchpad 迁到此处**，使它随归档进 git
> —— 否则这条守卫的可核实性依赖于「scratchpad 恰好没丢」，而那是偶然。

**口径**：`jq -S -c '.done_criteria' <file> | shasum -a 256`（与 verifier 预读时一致；
用文件字节 sha 会得到另一组数，两者都对但不可互比）。

| 任务 | 基线 sha（改动前） | 当前 sha | 判定 |
|---|---|---|---|
| TASK-001 | `ef0d8b3e8fd77da6…` | `d49b7b9be8a82e2e…` | **已变** ✅ 符合预期（Leader 授权的「钉 0 不钉总数」澄清）→ 仍须读 diff 核范围 |
| TASK-002 | `df79d4a151501d15…` | `df79d4a151501d15…` | **未变** ✅ 符合预期 |
| TASK-003 | `3f65bb6f5840d146…` | `3f65bb6f5840d146…` | **未变** ✅ 符合预期 |

**Leader 于 `67249ffb` 时点的比对结果**：TASK-001 已变（符合预期），TASK-002 / TASK-003 未变。
⇒ 这条守卫**在本 Sprint 真实生效了一次**，不是摆设。

---

## 快照全文（`done_criteria`，改动前）

```json
{
  "TASK-001": {
    "boundary": [
      "**回归守卫（对季度分支零鉴别力，别算成季报覆盖）**：`2026年二季度金融机构贷款投向统计报告`（p1 真实条目）必须仍被拒。reviewer 实测两种季度改法它都被拒，拒它的是「期次段**紧跟**金融统计数据」那个机制，与季度分支正交。\n\n既有 `13月`/`0月` 语义校验必须仍绿；新增季度分支后若正则能匹配上非法季度值（如「五季度」），语义层必须拒。"
    ],
    "error_handling": [
      "🔴 **两个 map 的一致性守卫必须是单向的**（reviewer B3 实测推翻原 DoD）：\n\n`types.go:62-77` 写明 **`monthly` 刻意不在 `periodEndMonth` 表内**（「任意月份都合法」），`types.go:150` 的校验正是靠 `if want, ok := periodEndMonth[...]; ok && ...`「查不到就跳过」实现。⇒ 原 DoD 的「反之亦然」**对现状即为假**，照做只有两条路：给 monthly 编一个期末月（**会让除该月外每一期月报都被 `Meta.validate` 拒**），或悄悄改成单向让 DoD 变空话。\n\n**改成两条可满足且真能防漏改的**：① `periodEndMonth` 的每个键必须在 `validPeriodTypes` 里；② **除 `monthly` 外**，`validPeriodTypes` 的每个键都要有期末月 —— 并在注释写明 monthly 是唯一豁免及其理由。\n\n---\n\n🔴 **同时：两处纯字符串的取值列表必须由 `validPeriodTypes` 派生**（`types.go:144` 的 `(want monthly|h1|annual)` 与 `thresholds.go:126` 的同一串）—— 加了季度类型后它们会**静默过期**。`types.go:116` 的 `checkEnum` 注释专门警告过这种抄一遍的写法，而这两处正是它自己没用 `checkEnum` 的地方。"
    ],
    "functional": [
      "**先抓真实样本再动代码**。reviewer 已 curl 核实（可直接用）：\n\n| | 一季度 | 前三季度 |\n|---|---|---|\n| 标题 | `2026年一季度金融统计数据报告` | `2025年前三季度金融统计数据报告` |\n| URL | `…/113469/2026041311133582598/index.html` | `…/113469/**5868082**/index.html` |\n| article_id | 19 位 | **7 位** |\n| PubDate | 2026-04-13 | 2025-10-15 |\n| 所在页 | p7 | p18 |\n\n两份 index 快照 + 两份正文快照落 `testdata/`，记下抓取 URL 与日期。**不得用合成标题代替真实样本**。",
      "🔴 **`articleLinkRE` 的 `\\d{14,}` 必须放宽**（reviewer B1，实测）——这是第一处静默失败点，**而原 DoD 曾要求断言 `^\\d{14,}$`，那是把缺陷钉成契约**。\n\n实测：p18 的前三季度报 article_id 是 **7 位**，`articleLinkRE` **整条链接根本不进入 `scanPage` 循环体**；逐页统计 id 位数，**分界在 p14/p15 之间**（p1–p14 全 19 位，p15–p18 全 7 位）⇒ **第 15 页起 `scanPage` 一条候选都产不出，且完全静默**（`Discover` 照常翻满 `MaxPages` 返回空）。\n\n改成 `\\d+` 或 `\\d{6,}`，并**断言 `scanPage` 能从真实 p18 快照提取到 `5868082` 那条**（钉字面量，不钉位数下界）。\n\n⚠️ **放宽必须配否定式边界**：reviewer 在仓库 p1 快照实测 `\\d{14,}`→15 条、`\\d{6,}`→57 条，多出的 42 条全是栏目导航页（`/rmyh/105145/index.html` 这类），链接文本过不了 `parsePeriod` ⇒ 放宽本身安全。**但这 42 条产出 0 候选要有一条断言钉住**，否则下次有人再放宽就没有网了。",
      "`parsePeriod` 对两种真实季报标题正确，且既有四种映射（h1/annual/monthly×2）**必须仍绿**。期次值与 periodType 由你定，但**必须与 `periodEndMonth` 一致，并写进 discovery 供 TASK-004 消费**。"
    ],
    "non_functional": [
      "**消融自证**：删掉季度分支，确认新增季报用例转红且**红的是你新加的那条**（贴出失败输出的具体那一行）。⚠️ 判据是「哪条断言红」不是「测试红不红」。\n\n**同时对 `articleLinkRE` 做一次**：把它改回 `\\d{14,}`，确认 p18 那条断言转红 —— reviewer 指出 `require.NotEmpty` 那条前置锚点（G9）在这里**第一次真正兑现**，因为 `scanPage` 会返回 nil。",
      "`types.go:38` 的注释「月均折算除数 1/6/12」在加了季度类型后会变成假的（该折算在本包**没有实现**，注释只是声明）——**顺手改**。",
      "`types.go:38` 的注释「月均折算除数 1/6/12」加了季度类型后会变成假的（该折算在本包**没有实现**，注释只是声明）——**顺手改**。\n\n覆盖率不低于 93.2%；`gofmt`/`vet` 空、整包 `-count=1` 与 `-race` 全绿。**单跑取证一律 `-run '^Top$/^Sub$'` 锚定式**，且只读 `--- PASS:` 那一行不读退出码。"
    ]
  },
  "TASK-002": {
    "boundary": [
      "⚠️ **计划 109 行明确警告**：加必填字段会让既有测试红，**但不是编译错误**（Go 允许 YAML 缺键）。`config_test.go` 里有 **5 处** `writeConfig` 构造的 YAML 全部要补 `storage` 段。**先加校验、跑测试、按红的逐个补**。\n\n⚠️ 同时注意 Sprint 036 的 T6 陷阱仍在：含 `caliber_exemptions` 的 YAML 必须写 `period_types`，否则 validate 抢先返回、报错理由指向 PeriodTypes 而不是你要测的东西。"
    ],
    "error_handling": [
      "`db_path` 写成相对路径时**不在 `validate` 里解析成绝对路径**（约束 C8：按进程 cwd 解析，解析发生在 cmd 层）。写一条测试钉住「装载后拿到的就是原样的相对路径」。"
    ],
    "functional": [
      "按计划 Task 1（`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md` 103–242 行）交付 `StorageCfg{DBPath string}` 与 `Config.Storage` 字段，YAML 键 `storage.db_path` 能被 `LoadConfig` 装载并断言。",
      "**`db_path` 缺失或空必须被 `validate()` 拒**，错误信息含键名。"
    ],
    "non_functional": [
      "既有五处 `writeConfig` 补完 `storage` 后，**Sprint 036 的全部 config 测试必须仍绿**（尤其「YAML 没写的阈值保持 DefaultThresholds 不退化为零值」那条）。",
      "`gofmt`/`vet` 空、整包 `-count=1` 全绿、覆盖率不低于 93.2%。"
    ]
  },
  "TASK-003": {
    "boundary": [
      "空 `articleID` 的行为必须被钉住（返回 false 还是报错由你定，但要有测试且在 discovery 里说明理由）。"
    ],
    "error_handling": [
      "查库失败必须**用 `%w` 包住底层错误**并带上下文。\n\n⚠️ Sprint 036 F8 的教训：`assert.ErrorIs` **证不了「包住」**——实现写成 `return nil, err`（完全不包）时它同样为真。要用 `require.NotNil(t, errors.Unwrap(err))` 把差集测掉。**别写 `require.NotErrorIs(t, errors.Unwrap(err), err)`**，那条在 unwrapped 为 nil 时平凡为真（跨 Sprint 存活了一整轮才被修）。"
    ],
    "functional": [
      "按计划 Task 2（`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md` 243–408 行）交付 `func (s *Store) HasArticle(ctx, articleID string) (bool, error)`，**同时查 `TableObservations` 与 `TablePending`**，任一命中即 true。真库测试（`newTestStore`）。",
      "三条各一测试：只在 observations 里 → true；只在 pending 里 → true；两张表都没有 → false。"
    ],
    "non_functional": [
      "**导出面守卫会打红**：`store_test.go` 有两条精确集合相等断言（AST 版 12 项、reflect 版 5 项），新增导出方法必须**登记**进去。\n\n⚠️ **登记不是放宽**：`assert.Equal` 一字不能动，只在切片里加一项。**正向自证**：把你新加的那一项从期望列表里删掉，确认转红（这证明守卫在按精确集合相等工作）。**不要跑「换成 assert.Subset」那种变异**——它必然存活，证明不了本次登记。",
      "`gofmt`/`vet` 空、整包全绿、`-race` 绿、覆盖率不低于 93.2%。"
    ]
  }
}
```
