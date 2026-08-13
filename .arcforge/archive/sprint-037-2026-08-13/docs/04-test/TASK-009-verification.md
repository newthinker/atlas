# TASK-009 验证报告 —— T7：配置实例、plist、CONTRACTS 与端到端验收

- **验证者**：test-agent-26 ｜ **交付**：dev-agent-52
- **⚠️ 判定对象**：**`bb825defffd332cf6886ef32def33d7f05c455a7`（最终交付物）**，
  **不是** `verify_baseline.head = f55570f3d2b5…` —— 漂移的成因与处置见 §0
- `assignment_epoch`：1 ｜ **结论**：**VERIFIED**（7/7 条 done_criteria 全部 PASS）

---

## 0. 🔴 验证对象漂移：dev 在 `verifying` 期间提交了一次实质改动

| | 基线（派验时记） | 我判定时 |
|---|---|---|
| HEAD | `f55570f3d2b5…` | **`bb825defffd3…`** |
| discovery sha256 | `e98d789e53ba…` | **`ae14ad0fa46d…`** |

**时间线**（`transitions.jsonl` 无新 transition，任务全程 `verifying`）：

```
07:56:39  提交 f55570f（交付）
07:59:46  Leader 派验 → verify_baseline 记 f55570f
08:01:18  dev-52 修改工作区（我承接时看到 M cmd/atlas/hestia_test.go）  ← 派验后 92 秒
其后      提交 bb825de，并改写 discovery
```

### 我为什么按 `bb825def` 判而不是按基线判

那次改动**不是注释**：它把两个解析型守卫的 `if err != nil { break }` 改成
`errors.Is(err, io.EOF)` 才 break、其余 `require.NoError`，并给 `CONTRACTS.md` 加了 16 行。
**改动全部落在 `writes` 声明内**（`hestia_test.go` + `CONTRACTS.md`），无越界。

⇒ 若我按基线判 `f55570f` 为 verified，**master 上那 11 行新代码就从未被任何人验过** ——
那正是 AD-29 要防的形态（判定对象 ≠ 最终交付物）。故我**重新在 `bb825def` 上跑了全套**，
本报告全部数据采自它。转 `verified` 时以 `--ack-drift` / `--ack-discovery-drift` 显式确认。

📌 **流程上仍有问题，与结论无关**：在 `verifying` 期间改判定对象，会让「我验的是哪一版」
依赖验证者主动发现。**这次我是靠 `git status` 里一个未提交改动才注意到的** —— 若它改完直接提交
且我没重查 HEAD，我会拿着 `f55570f` 的结论去签一个 `bb825def` 的交付。
（同一 dev 在 TASK-011 也有一次派验后改 discovery，那次它主动报告了；这次没有。）

---

## 1. 完成标准覆盖矩阵（7 条）

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | `configs/hestia.yaml`（数字带 M0 来源）+ plist 三时点 | 配置 71 行、6 处来源注释、含 `storage.db_path`；plist 实测 **15:30 / 17:30 / 21:30**，与计划 1634-1638 行的 spec 一致 | **PASS** |
| functional[1] | plist **一个代理键都不设**（按后缀判，不枚举）+ **登记进 `install-services.sh`** | `TestHestiaPlistSetsNoProxyKeys` 按 `strings.Contains(lower,"proxy")` 判；`install-services.sh:32` 已把 `hestia-ingest` 加进安装循环，`TestInstallServicesInstallsHestiaPlist` 守着 | **PASS** |
| boundary[0] | `deploy.sh` 会同步（**要求 dev 自己复核**）+ `plutil -lint` 通过 | discovery 明写「**✅ 我自己复核了 `deploy.sh:35`，不是只引用他人结论**」并给出排除列表实据；`TestHestiaPlistPassesPlutilLint` | **PASS** |
| boundary[1] | `LoadConfig("../../configs/hestia.yaml")` 必须成功 | `TestRealHestiaConfigLoads` 绿；配置含 `storage` 段（我派发前提示的那个坑没踩） | **PASS** |
| error_handling[0] | 否定式断言**必须配肯定式锚点**，并注明互补 | `require.NotEmpty(keys)` + `assert.Contains(keys,"PATH")`，**另加「阳性对照」子测试**（见 §3） | **PASS** |
| non_functional[0] | CONTRACTS 记下四件 | 一级键 A/B/C、两处接缝、修订版限定、`--force` 作用域 —— 全部在场；修订版那条用**表格**把「做不到」与「没测过」的后果差别写明 | **PASS** |
| non_functional[1] | 季报支持登记 | CONTRACTS 含季报期次类型/期末月/累计前缀 | **PASS** |
| non_functional[2] | 部署闸记为「已满足的历史约束」；`gofmt`（本任务文件）/vet/build/test/race | 见 §4 实跑；部署闸已如实记录且未删 | **PASS** |

---

## 2. 两个消融（Leader 建议，我独立复现）

| 变异 | `plutil -lint` | Go 守卫 |
|---|---|---|
| 插一个 `no_proxy` 键 | **OK** | 红在 `hestia_test.go:347` |
| **删掉整个 `StartCalendarInterval` 块** | **OK** | 红在 `hestia_test.go:397` |

⇒ **两道守卫各管一半、不可互替**：lint 管 XML 合不合法，Go 守卫管**这个 job 到底跑不跑、用什么环境跑**。

**第二个尤其要紧**：一个**没有任何排班键的 plist 是合法 plist**，装得上、`launchctl list` 看得见、
日志目录空着 —— **部署成功但永远不工作**。`plutil` 对它报 OK，只有 Go 守卫拦得住。

---

## 3. 🔴 我独立复现了那个「同时骗过两道守卫」的构造 —— 这是本次最有价值的一条

`bb825def` 的 CONTRACTS 声称：**一份 plist 里放含 `--` 的注释 + 一个真实 `https_proxy` 键，
可同时骗过 `plutil -lint` 与旧版 Go 守卫。** 我自己构造了一份验证：

| 检查 | 实测结果 |
|---|---|
| `plutil -lint` | **OK** ← 接受了 XML 规范不允许的 `--` |
| `plutil -convert json` | `EnvironmentVariables = ['PATH', 'https_proxy']` ← **代理键真的在里面** |
| Go `encoding/xml` | 报错 `invalid sequence "--" not allowed in comments` |
| **旧写法 `if err != nil { break }` 会返回** | **`[PATH]`** ← 代理键**看不见** |

⇒ **两道守卫全绿，而「plist 不得设代理键」这条约束被违反。** 构造完全成立。

⇒ dev-52 的修复（`errors.Is(err, io.EOF)` 才 break，其余 `require.NoError`）**是必要的**，
不是洁癖。它写下的通则我认为该进结转：

> **解析型守卫必须把解析错误当失败，不能当结束。** 「读到哪算哪」在正常输入上与正确实现
> **无法区分**，只在被构造的输入上分道扬镳 —— 而那正是它该起作用的时候。

📌 这也解释了 §0 那次漂移的价值：**它修的正是我在验 `f55570f` 时识别出的同一处残留风险**
（我当时的判断是「本任务 plist 当前合法故无害，登记为观察」）。**它比我更进一步：它去构造了那个输入。**

---

## 4. 实跑证据（隔离 worktree @ `bb825def`，退出码不跨管道取）

```
gofmt -l <本任务改动的文件>   → 空      （DoD 已订正为文件级口径，见下）
go vet ./...                  → exit 0
go build ./...                → exit 0
go test ./... -count=1        → exit 0，无 FAIL
go test ./cmd/atlas/ ./internal/hestia/ -race → exit 0
16 条 Hestia 相关测试全部 --- PASS
```

> **`gofmt` 用的是订正后的判据**（`gofmt -l <本任务改动的文件>`，非整目录）。
> 这条订正源自我在 TASK-008 报告里指出的结构性问题，Leader 已前移进本任务 DoD。
> `cmd/atlas/backtest_test.go` / `crisis_test.go` 仍未格式化，但**是历史遗留、不在本任务 `writes` 内**，
> 已另开 chore —— **不算到本次交付头上**。

---

## 5. DoD 之外的观察

**O1（历史遗留，不属本任务）** `com.newthinker.atlas.aktools.plist` 的注释含 `--` ⇒ **非法 XML**，
Go 与 Python 都拒绝，而 `plutil -lint` 报 OK。不在任何在途任务 `writes` 内，Leader 已登记待单开 chore。
⇒ 提醒理由与 `gofmt` 那条相同：**若验证者不核归属，会把历史包袱记到当前交付头上，而那看起来像一次尽职的 reject。**

**O2（流程）** 见 §0：`verifying` 期间改判定对象，使「我验的是哪一版」依赖验证者主动发现。
⇒ 建议把「承接后、出裁决前重查一次 HEAD 与 discovery sha」写进验证者固定动作 —— **我这次是靠
`git status` 的一个残留改动才注意到的，那属于运气**。

---

## 6. 复现（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-t009b bb825defffd332cf6886ef32def33d7f05c455a7
GOTOOLCHAIN=local go test ./... -count=1 >/tmp/t.txt 2>&1; echo $?   # 不跨管道取码
# §3 的构造：plist 内放含 -- 的注释 + https_proxy 键
#   plutil -lint → OK ；plutil -convert json → ['PATH','https_proxy']
#   Go encoding/xml → invalid sequence "--" ；旧写法 break → 只返回 [PATH]
```

---

# 复验（QA `review_fix` 第 1 轮）—— **PASS**

- **判定对象**：`b6b13a4ac3ab314ac63e45e2170a8488e0fd4204`（= `verify_baseline.head`，**无漂移**）
- `discovery` 内容无漂移（`231c712e1cfb`）｜ `rework_count=1` ｜ `epoch=1`
- **基线含本任务自己的返工提交**：`4e590673abef fix(TASK-009): 排班守卫限定作用域 + --force/cwd 文案`
  经 `git merge-base --is-ancestor` 确认在祖先链上 ✅

## 5 条 `fix_items` 逐条核（**读原文，不用 grep 计数**）

| # | 要求 | 证据 | 判定 |
|---|---|---|---|
| ① | **CRITICAL-1 排班守卫限定作用域** | `plistSchedule` 改为「先认出 `StartCalendarInterval` 键 → 下一个 `<array>` → 其中的 `<dict>`」，且 `Hour`/`Minute` 必须同出一个 dict | **PASS**（对照见下） |
| ② | 配套补 CONTRACTS「两道守卫各管一半」的表 | `CONTRACTS.md` 已补 | **PASS** |
| ③ | WARNING-3 文案半：`--force` flag help 点名「Duplicate 期次重抽取值被丢弃」+ CONTRACTS「有出路」加限定 | help 改为 `…re-extracted values are DISCARDED for periods already in the observations table…`；CONTRACTS 改为「**落在 pending 里的期次重跑有出路；已进权威表的没有**」 | **PASS** |
| ④ | WARNING-4：`ingest` 补 cwd 守卫 | `ingest.go` 打印 `db: <filepath.Abs(...)>`，与 `status` 对齐 | **PASS** |
| ⑤ | 自律条：新写的注释/CONTRACTS 不用 `file:line` | 本轮新增行中 `\.go:[0-9]+` 形式 **0 处** | **PASS** |

## 四条变异对照（**旧树 `bb825def` 与新树 `b6b13a4` 各跑一遍**）

| | 变异 | 旧树 | 新树 | 失败位置（新树） |
|---|---|---|---|---|
| **M3a** | `StartCalendarInterval` 改一字母 | **全绿**（缺陷本身） | **红** ✅ | `hestia_test.go:405` — `"[]" should have 3 item(s), but has 0` ⇒ **「找不到该键 ⇒ 0 个 interval」** |
| **M2** | `Hour`/`Minute` 拆到 6 个 dict | **全绿** | **红** ✅ | `:481`→`:403` — `"StartCalendarInterval 的第 %d 个 dict 缺字段"` ⇒ **「dict 缺字段」** |
| **M3b** | 块外塞 `Hour=99` | **红**（`Len` 4≠3） | **绿** ✅ | 块外不再被误读，无退化 |

⇒ **`fix_items` 写死的两条验收判据逐条命中**，且各自**只红一个测试**、非兄弟连坐；基线（未变异）FAIL=0。

📌 **「必须在缺陷仍在的那棵树上先看到它红」这条纪律在本次兑现**：
M3b 在旧树上就红 ⇒ 它测的方向**旧版本来就守得住** ⇒ 新树绿**只说明没退化，不说明缺陷被修**。
**若只在新树上跑，三条会混成「都绿/都红」，我无法区分「堵住了」与「本来就守得住」。**

## 实跑（隔离 worktree @ `b6b13a4`，退出码不跨管道取）

```
gofmt -l <本轮改动文件> → 空    go vet ./... → exit 0    go build ./... → exit 0
go test ./... -count=1  → exit 0，无 FAIL
go test ./cmd/atlas/ ./internal/hestia/ -race → exit 0
覆盖率：internal/hestia 93.6% · cmd/atlas 75.4%（≥ coverage_floor 75）
```

## ⚠️ DoD 之外的观察：`--force` 的 flag **绑定**仍无守卫（未计入判定）

Leader 曾在消息中告知「一条新 CRITICAL（`--force` 的 flag 绑定完全没有守卫）已折进本任务」，
**但它没有出现在 `fix_items` 里**（5 条逐条读过，命中 0；`transitions.jsonl` 显示 `fix_items`
于 `00:35:43Z` 定稿后零更新，而 dev 于 `00:38:10Z` 才认领）。

**我实测确认它仍在**：把 `BoolVar(&hestiaForce, …)` 改为 `Bool(…)` ⇒ **两个包全绿、FAIL=0**，
而 `--force` 完全失效。`TestHestiaFlags` 的三条断言全是 `Lookup(...)` —— **只查 flag 在不在，不查它绑到哪**。

⇒ **不计入本次判定**：`fix_items` 是判定依据，而它不在其中；**dev 未修不是疏漏**。
⇒ 处置待 Leader 裁定（补进 `fix_items` 再返工，或另开独立项）。

📌 形态登记：**「存在性断言」作前置锚点是对的，作验收判据是空的** —— 区别不在断言形式，
在它在句子里的位置。`TestHestiaFlags` 的三条 `Lookup` **之后什么都没有**，它就是全部。
