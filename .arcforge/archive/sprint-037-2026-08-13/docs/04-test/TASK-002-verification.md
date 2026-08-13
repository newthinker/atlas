# TASK-002 验证报告 —— T1：Config 补 storage 段

- **验证者**：test-agent-26
- **被验交付**：dev-agent-53
- **验证基线**：`verify_baseline.head = 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf` = 承接时 HEAD ⇒ **无漂移**
- **assignment_epoch**：1
- **结论**：**VERIFIED**（6/6 条 done_criteria 全部 PASS，无未覆盖项）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| 验证对象漂移 | `verify_baseline.head` == `git rev-parse HEAD` ✅ 无漂移 |
| **DoD 未被改写** | 指纹 `df79d4a151501d15` == wave1 开工前预读基线 ✅ **未变** |
| 实际改动 vs `writes` | `config.go` + `config_test.go`，与声明**逐字一致**，无越界 ✅ |
| discovery | 文件存在（9406 B）；**任务文件的 `discovery` 字段原本缺失，由我补上**（见 §5） |

### ⚠️ 交付 sha 与合入 sha 不同（rebase），用**内容判据**确认等价

dev-53 的 discovery 记的交付提交是 **`f9136e298efdc82b61640fdaac31c05364530455`**，而实际合进 master 的是
**`bcbcdad`**（leader 因 master 被 TASK-002 之外的提交推进过，用三方 merge 合了原 sha）。实测：

```
git merge-base --is-ancestor f9136e29… HEAD  → 否（rebase 产物，不在祖先链上）
git merge-base --is-ancestor bcbcdad   HEAD  → 是
```

**纯 sha 判据在这里会给出假的「没合进去」。** 故改用内容判据，结果**逐字节吻合**：

| 文件 | master 上实测 sha256 | dev-53 报的 |
|---|---|---|
| `config.go` | `352f16ef…08e0c2` | **同** |
| `config_test.go` | `bda2fef7…ffdccb` | **同** |

`git diff --numstat 63ac5b6 bcbcdad` = `13/1` + `97/4`，与 dev 报的 `+13/-1`、`+97/-4` **完全一致**。
⇒ **rebase 换了 sha 但没换内容，dev-53 的全部自证数字对 master 上这棵树同样有效。**

---

## 1. 完成标准覆盖矩阵

| # | done_criteria | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | `StorageCfg{DBPath string}` + `Config.Storage`，YAML 键 `storage.db_path` 能被 `LoadConfig` 装载并断言 | `config.go`：`StorageCfg` 带 `mapstructure:"db_path"`、`Config.Storage` 带 `mapstructure:"storage"`；`TestLoadConfigReadsStorage` 断言 `cfg.Storage.DBPath == "data/hestia.db"` | **PASS** |
| functional[1] | `db_path` **缺失或空**必须被 `validate()` 拒，错误信息含键名 | `TestLoadConfigRequiresDBPath`（缺失）+ `TestLoadConfigRejectsEmptyDBPath`（显式空串），两条均 `assert.Contains(err, "db_path")`；实现 `case c.Storage.DBPath == "": return errors.New("storage.db_path is required")` | **PASS** |
| boundary[0] | 既有 **5 处** `writeConfig` 全部补 `storage` 段；含 `caliber_exemptions` 的 YAML 必须写 `period_types` | 5 处全覆盖（见下）；`TestLoadConfigRejects/豁免缺_period_types` 绿 | **PASS** |
| error_handling[0] | 相对 `db_path` **不得**在 `validate` 里解析成绝对路径（C8），要有测试钉住「拿到的就是原样相对路径」 | `TestLoadConfigKeepsDBPathRelative`：`assert.Equal("data/hestia.db")` + `assert.False(filepath.IsAbs(...))` + `assert.NotContains(..., filepath.Dir(p))`；实现的 `validate` 只做非空检查，无任何 `filepath.Abs` | **PASS** |
| non_functional[0] | 补完后 Sprint 036 全部 config 测试仍绿，**尤其**「YAML 没写的阈值保持 DefaultThresholds 不退化为零值」 | `TestLoadConfigKeepsDefaultsForOmittedThresholds` **绿**；全部 17 条 `TestLoadConfig*`（含 10 条子测试）全绿、0 FAIL | **PASS** |
| non_functional[1] | `gofmt`/`vet` 空、整包 `-count=1` 全绿、覆盖率 ≥ 93.2% | 见 §2 | **PASS** |

**5 处 `writeConfig` 的补法**（`config_test.go`，`:15` 是函数定义不计）：

| 原调用点 | 补法 |
|---|---|
| `:28` `TestLoadConfigKeepsDefaultsForOmittedThresholds` | 内联 `storage:\n  db_path: data/hestia.db` |
| `:58` `TestLoadConfigFull` | 同上 |
| `:106/:110` `TestLoadConfigRejects` 的 `head` | 抽出 `const storage`，`head = storage + …` |
| `:122-124` 同函数的 3 条自带 YAML 的表驱动用例 | 各自改成 `storage + "discover:\n…"` |
| `:166` `TestLoadConfigRejectsMalformedYAML` | 内联 |

dev-53 在 `const storage` 处补了一条注释，点明 `db_path` 必填且**排在 `validate()` 第一位**，
缺它会让每条用例红在 `db_path` 上而不是它想测的那一项 —— 与既有 `period_types` 那道注释是同一种抢先返回。
**这正是 DoD boundary 提醒的陷阱，它不仅避开了，还把避法写成了给后人的注释。**

---

## 2. 实跑证据

⚠️ **在隔离 worktree（`git worktree add --detach … 67249ffb…`）的干净树上采**，理由见 §4。

```
gofmt -l internal/hestia/                   → 空
GOTOOLCHAIN=local go vet ./internal/hestia/ → 空
GOTOOLCHAIN=local go build ./...            → OK
go test ./internal/hestia/ -count=1 -cover  → ok  coverage: 93.3%   (门槛 93.2% ✅)
go test ./internal/hestia/ -count=1 -race   → ok
顶层 PASS 282 / 全部 PASS 614 / FAIL 0
go tool cover -func → config.go LoadConfig 100.0%、validate 100.0%
```

全部 17 条 `TestLoadConfig*` 绿（含 `TestLoadConfigRejects` 的 10 条子测试），**0 FAIL**。

---

## 3. 消融（我独立重跑了核心那条）

DoD **没有**要求消融自证，dev-53 仍自主做了 4 条（A/B/C1/C2）并记录在 discovery。
我独立重跑了**直接支撑 functional[1] 的那条**：

| # | 变异 | dev 声称 | **我实测** | 外溢 |
|---|---|---|---|---|
| A | 删掉 `case c.Storage.DBPath == ""` 必填分支 | 2 条红：`RequiresDBPath`、`RejectsEmptyDBPath` | **完全一致**：红在 `config_test.go:216` 与 `:237` | 280+2=282 ✅ |

**B/C1/C2 我未重跑**，采信 dev-53 的记录，理由说明白：其 discovery 的
`file_sha256` 与 master 上两文件**逐字节吻合**（见 §0），即它的消融确实跑在与被验对象相同的内容上；
且 C1/C2 的结论（`filepath.Abs` / 相对配置文件目录解析各致 4 条红）可由 `TestLoadConfigKeepsDBPathRelative`
的断言直接读出，不依赖信任。**这是验证深度的如实交代，不是「全部复现」。**

另核一条 dev 的自述是否准确：它在注释里说 `KeepsDBPathRelative` 的三条断言**不是三道独立的闸**，
「真正杀掉那两种实现的是第一条 `Equal`，另两条被它蕴含」。**这个自述准确** —— `Equal("data/hestia.db")`
为真则该值必然非绝对路径、必然不含临时目录前缀，后两条是逻辑蕴含。
**它没有把三条断言夸大成三重保险，这一点值得记一句。**

---

## 4. ⚠️ 验证环境事件：主工作区在我验证期间被 wave2 在途代码污染

验完 TASK-003 后我发现主工作区 `git status` 已不干净：

```
 M internal/hestia/store.go        (+88)
 M internal/hestia/store_test.go   (+277)
?? internal/hestia/ingest_test.go
?? internal/hestia/status.go
?? internal/hestia/status_test.go
```

是 wave2 的 `RecentObservations` / `StatusRow` / status 命令在**主工作区**开发（纯新增，365 insertions / 0 deletions，
未改动任何既有代码）。

**处置**：我把 TASK-001/002 的全部判定数据在隔离 worktree（`67249ffb…` 干净树）上**重采了一遍**。
重采结果与污染前采的**逐字一致**（282 顶层 / 614 全部 / 0 FAIL / 93.3%）
⇒ **本次判定未受污染**（污染发生在我采数之后）。

但这属于运气而非机制：`go test` 会编译未跟踪的 `status_test.go`，若它当时已存在且是红的，
我会把**别人的红**记到 TASK-002 头上。已单独报 leader。

---

## 5. 我补的字段（申报，非静默修复）

承接时 `jq 'has("discovery")'` 返回 **false** —— 任务文件的 `discovery` 字段缺失（discovery **文件**是在的，9406 B）。

**实测确认这会阻断**：把任务副本改成 `verified` 后跑 validator，报
`[missing-discovery] status "verified" 必须有非空 discovery 产物，但任务未声明 discovery 字段`，**退出码 1（阻断级）**。
而该规则只在 `status == verified` 时触发 ⇒ 转之前查不出、转之后 leader 与 dev 都写不了。

⇒ 我在转 `verified` **之前**经写通道补上：
`task TASK-002 update --field discovery=.arcforge/discoveries/TASK-002.json`，落盘后 jq 直读核实。

**TASK-001 同样缺失，一并补上；TASK-003 由 dev-54 自己写了。** 两人漏一人不漏 ⇒ 见给 leader 的消息。

---

## 6. DoD 之外的观察（不据此判定）

**O1（轻微）** dev-53 顺带把 `config.go` 末行注释的 `T6` 改成 `M1b-4a 的 TASK-006`（其 discovery 的 decisions 里
已申报，称出自计划 Step 5）。DoD 未要求这条，但改动落在声明的 `writes` 内、是纯注释、且消歧义
（跨 Sprint 复用任务号时 `T6` 会指向两个不同任务）。**不构成越界，登记备查。**

**O2** `TestLoadConfigRejects` 的表驱动里，三条自带 YAML 的用例改成了 `storage + "discover:\n…"` 字符串拼接，
而另外几条用 `head + …`。两种写法并存是因为前者要**故意省略** `head` 里的某段来触发校验。
读起来需要停一下，但注释已说明。**不是问题，只是下次加用例的人要注意别混用。**

---

## 7. 复现命令（锚已钉全 sha）

```bash
git worktree add --detach ../wt-verify-t2 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf
cd ../wt-verify-t2
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
shasum -a 256 internal/hestia/config.go internal/hestia/config_test.go   # 352f16ef… / bda2fef7…
```
