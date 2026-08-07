# TASK-004 验证报告（第二轮 / 返工复核）—— FileStore I/O 层 fail-open

- 验证者: test-agent-17
- 被验对象: `6183ababf679e4474741990913a00c4b5a0f2121`（`quota_file_test.go` **+89**，实现零改动）
- 验证环境: 独立 worktree `../wt-v004r2`（detached @ 6183aba）
- assignment_epoch: **2** / rework_count: 1
- 前序: `TASK-004-verification.md`（首轮 rejected）

## 结论：**PASS（verified）**

**2×2 左列全绿已复现** —— 旧测试集对 F15/F16 确实无守护，返工必要性成立。
新增三条测试**全部真实执行、无一 SKIP**（65 测试 0 SKIP），F15/F16 在新测试集下**断言红**。
防回归 7 条全部断言红，无一削弱。覆盖率 91.8% → **94.0%**，`Take` 与 `read` 升至 **100%**。

**同时我要修正首轮的一处表述**：F17 不构成第三处缺口（§三）。首轮结论应为
「**两处**无守护（lock/write）」而非三处。这不影响返工必要性，但覆盖声明要准确。

---

## 一、2×2 闭环（行=变异，列=测试集，同一 worktree、同一会话）

| | **旧测试集**（首轮 `quota_file_test.go`，62 测试） | **新测试集**（本轮 +89，65 测试） |
|---|---|---|
| **无变异** | 绿（RUN=62, SKIP=0） | 绿（RUN=65, SKIP=0） |
| **F15** `lock()` 失败返回 `(false,err)` | **绿 ❌ 漏检** | **红(断言)** `:203 目录建不了时必须 fail-open: (false, policy: quota dir: mkdir ...)` |
| **F16** `write()` 失败返回 `(false,err)` | **绿 ❌ 漏检** | **红(断言)** `:235 账本写不进去时必须 fail-open: (false, policy: temp quota ledger: open ...)` |
| **F17** `read()` I/O 错返回 nil map | **绿** | **红(PANIC)**，RUN 65→43（见 §三） |

**左列的两个「绿」就是缺口存在的证据，也是本次返工必要性的证明。**

---

## 二、三条新测试**确实执行了，没有被 SKIP**（本轮最该核的一条）

Dev 给后两条加了「先探测权限是否真的生效」的前置检查，root 环境下会 `t.Skip`。
陷阱 11 的教训是「SKIP 掉的守护从未执行过」，所以必须确认当前环境下它们真的跑了：

```
--- PASS: TestFileStoreFailsOpenOnDirError   (0.00s)
--- PASS: TestFileStoreFailsOpenOnWriteError (0.00s)
--- PASS: TestFileStoreFailsOpenOnReadError  (0.00s)
```

**65 个测试全 PASS，`--- SKIP` 零条。** 2×2 的每一格我也都记录了 `SKIP=0`
（runner 直接数 `--- SKIP` 行数），不是只看总体绿。

> 这里的 `t.Skip` 是**有意的、条件明确的**（探针先试写/试读，确认权限位真的拦得住才继续），
> 与陷阱 11 那种「读不到 `/dev/fd` 就静默跳过」性质不同：前者是「构造前提不成立时诚实退出」，
> 后者是「守护从未执行而无人知晓」。**判据是：Skip 的条件是否被显式探测并写明。** 这条满足。

---

## 三、**修正首轮的一处表述：F17 不构成第三处缺口**

首轮我把 F15/F16/F17 并列为「I/O 层 fail-open 三处无守护」。本轮做精确形态验证后发现
**第三条不成立**：

| 变异 | 形态 | 旧测试集 | 说明 |
|---|---|---|---|
| **F17** | `read()` I/O 错返回 **nil map** | 绿 | 改的是**返回值类型**，不是 fail-open 语义 |
| **F17b** | `Take` 在 `readErr != nil` 时返回 `(false, err)` | **红(断言)** `:129 账本损坏必须 fail-open 放行（设计 §4.4）` | 这才是「read 出错不 fail-open」的准确形态 |

**F17b 在旧测试集下就是红** —— 因为 JSON 解析失败也走同一条 `readErr` 路径，
`TestFileStoreFailsOpenOnCorruptLedger` 本来就守住了「readErr 时必须 fail-open」这个语义。

所以准确的首轮结论是：**lock 与 write 两处无守护**；read 路径的 fail-open **语义**本已被覆盖，
新增的 `TestFileStoreFailsOpenOnReadError` 覆盖的是 `read()` 里 **ReadFile 失败**这条
与 JSON 解析失败不同的**代码路径**（`read` 覆盖率 88.9% → **100%** 即由此而来），仍有价值。

**这不影响返工必要性**（F15/F16 两处是实打实的缺口），但覆盖声明必须准确 —— 我首轮把
「改坏返回值类型导致的存活」当成了「fail-open 语义无守护」，这与我批评 Dev 把 panic 当断言
是同一类错误：**红/绿的原因要归因到位**。

---

## 四、F17 的 panic 型红（备录，不作为缺陷）

F17 在新测试集下转红的方式是 **panic**（nil map 赋值），`RUN` 从 65 掉到 **43** —— 22 个测试
被带倒。按 TASK-003 R1 确立的标准检查是否构成缺陷：

- TASK-003 R1 之所以是缺陷：`quota.go` 注释**明写**「缺时区不该 panic」（设计意图），
  而测试没断言它；
- 这里：`read()` 返回 `make(map...)` 而非 nil 是防御性写法，但**注释未把「不该 panic」
  声明为设计意图**，且该 DoD 是「fail-open」而非「不 panic」——与 TASK-003 的 Q16
  （删 `g.quota == nil` 短路致 panic）判定一致，**不作为缺陷**。

若将来要把「`read()` 必须返回非 nil map」立为不变量，可加一条 `recover` 型断言。备录。

---

## 五、防回归抽查（7 条，全部断言红，无一削弱）

这 89 行只新增测试、不改动既有测试与实现，但按纪律实测：

| 变异 | 覆盖的 DoD | 断言位置 |
|---|---|---|
| F1 读坏 JSON 只报错不重建 | error_handling 分句2（自愈） | `:160` |
| **F1c** `!ok` 时返回 `true` | 分句2 的**分水岭** | `:173` |
| F4 `unlock` 不 `Close` fd | non_functional[1] | `:443` |
| F7 `f.mu` 与 flock 同时失效 | boundary[1] 并发不超发 | `:353` |
| F12 不建父目录 | functional[2] | `:280` |
| F13 窗口翻篇不重置 | functional[1] | `:98` |
| F14 账本不按主题隔离 | functional[0] 跨实例累计 | `:76` |

---

## 六、Done Criteria 终态

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| functional[0] | 两实例共享账本，配额累计（验收标准 3） | F14 `:76`；首轮另有真实双进程/8 进程并发取证 | **PASS** |
| functional[1] | 窗口翻篇归零；主题隔离 | F13 `:98`、F14 `:76` | **PASS** |
| functional[2] | 文件不存在从空起算；父目录自动创建 | F12 `:280` | **PASS** |
| boundary[0] | 被拒不写入计数 | 首轮已验（F9 为等价变异体） | **PASS** |
| boundary[1] | 并发不超发 | F7 `:353` | **PASS** |
| error_handling[0] 分句1 | 坏账本 fail-open 不 panic 不阻断 | F1 下仍绿（正确） | **PASS** |
| error_handling[0] 分句2 | 损坏后必须自愈 | F1 `:160`、F1c `:173`（分水岭） | **PASS** |
| **error_handling[0] I/O 层 fail-open** | lock/write/read 失败时放行 + 告警（C7） | **F15 `:203` / F16 `:235` 断言红（首轮为绿）**；read 路径由 `:129` + 新增 ReadError 测试覆盖 | **PASS（首轮 FAIL 已修）** |
| non_functional[0] | 原子写无残留 | 首轮已验（含跨进程并发下无残留） | **PASS** |
| non_functional[1] | temp+rename；fd 及时关闭（review） | review 通过；F4 `:443` | **PASS** |
| non_functional[2] | `-race` 全绿 | 实测绿 | **PASS** |

**11 项全部 PASS。**

---

## 七、覆盖率、回归、约束、scope

| 项 | 首轮 | 本轮 |
|---|---|---|
| 覆盖率（包） | 91.8% | **94.0%** |
| `quota_file.go` `Take` | 85.7% | **100%** |
| `quota_file.go` `read` | 88.9% | **100%** |
| `quota_file.go` `lock` | 63.6% | **72.7%** |
| `quota_file.go` `write` | 50.0% | **55.6%** |
| 测试数 | 62 | **65**，0 SKIP，0 FAIL |

`lock`/`write` 的剩余缺口是 `Flock` 系统调用失败、`json.Marshal` 失败、`tmp.Write`/`Close`
失败等分支 —— 这些确实难以在不引入 fs 抽象的前提下构造，**且它们都不改变 fail-open 语义
的可观测行为**（都走同一条 `return true, err`），不作为判定依据。

其余：`-race` 绿；C3 仅 policy 自身；gofmt 无输出；vet exit 0；17 包回归绿；
scope 仅 `quota_file_test.go`（+89）；`git diff 1ca7a7d -- quota_file.go quota.go gate.go`
**无输出** → 实现零改动。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-v004r2 6183ababf679e4474741990913a00c4b5a0f2121
cd ../wt-v004r2

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race       # 65 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -v | grep -c '^--- SKIP'   # 须为 0

# 2x2 左列（缺口证据）：取首轮测试文件后注入变异
git show 1ca7a7d:internal/collector/policy/quota_file_test.go > internal/collector/policy/quota_file_test.go
#   F15: lock 失败处 return true,err → return false,err   → 旧测试集全绿（漏检）
#   F16: write 失败处同样改             → 旧测试集全绿（漏检）
# 右列：git checkout 复位后同样注入 → :203 / :235 断言红

# F17b（本轮修正依据）：Take 里 readErr != nil 时 return false,readErr
#   → **旧测试集下也是红** :129，说明 read 路径的 fail-open 语义本已被覆盖

# 变异五道门：① 改动量非空且改语义 ② go test -c 编译通过 ③ 核到断言行（区分 panic）
#            ④ === RUN 数 > 0 ⑤ 还原后 md5 一致；另每格记录 --- SKIP 数
```

worktree 已于验证结束后清理；主工作区 `internal/` 零污染。
