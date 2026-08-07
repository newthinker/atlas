# TASK-004 验证报告 —— FileStore 跨进程配额账本

- 验证者: test-agent-17
- 被验对象: `1ca7a7de184c8bee2b9e51a20e7e912c9ed399b2`（`quota_file.go` 131 行 + `quota_file_test.go` 408 行）
- 验证环境: 独立 worktree `../wt-v004`（detached @ 1ca7a7d）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）

## 结论：**NEEDS WORK（rejected）**

**Dev 自报的每一项我都独立复现了，全部属实**：F1 的复合句分句证据（分句 2 红 / 分句 1 **仍绿**）、
`f.mu` 与 flock 各自独立充分、fd 泄漏的间接检测确实不设防、`/dev/fd` 在 go test 进程内确实读不到。
62 测试 **0 SKIP**、`-race` 绿、17 包回归绿、scope 干净。

判 rejected 的原因是一处**核心约束无守护**：

> **error_handling[0] 的 fail-open 只覆盖了「JSON 损坏」一种形态，I/O 层失败的 fail-open
> 语义三处全部无守护** —— `lock()` 失败、`write()` 失败、`read()` 的 I/O 错误，把
> `return true, err` 改成 `return false, err` 后**没有任何测试转红**。

而这些恰恰是真实环境里**比 JSON 损坏更常见**的故障（磁盘满、权限变更、只读挂载）。
一旦其中任一被改成 `false`，配额账本的 I/O 故障就会**阻断整个 collector 的降级链**（违反约束 C7）。

**且 Dev「这些分支无法可靠构造」的说明经实测不成立** —— 两种构造我都跑通了（§五）。

---

## 一、Dev 自报各项的独立复现（全部属实）

### 1.1 F1：复合句必须分句取证的硬证据

注入 F1（读到坏 JSON 只报错、从不重建）：

| 定向到 | 结果 |
|---|---|
| 分句 2 `TestFileStoreSelfHealsAfterCorruption` | **红(断言)** `quota_file_test.go:160: 账本不是合法 JSON: invalid character 't'...` |
| 分句 1 `TestFileStoreFailsOpenOnCorruptLedger` | **绿 —— 仍然通过** |

**只写分句 1 的话，「配额永久静默失效」完全不可见**：fail-open 一直成立、告警一直发出、
而配额再也不生效。这条硬证据把「复合句必须分句取证」从推断变成了实测。

### 1.2 分句 2 的判据确实独立 —— 分水岭在最后一条断言

Leader 点名要复核「判据是否真的独立」。我构造了两个针对性变异：

| 变异 | 结果 | 说明 |
|---|---|---|
| **F1b** 重建账本但计数恒归零（「不再报错」成立） | **红** `:162 损坏后应重建账本并计一次: Count = 0, want 1` | 第二环（计数从新窗口起算）有效 |
| **F1c** 前两环全过，只把 `!ok` 时的返回改成 `true`（Limit 判定失效） | **红** `:173 自愈后配额应正常生效: (true, <nil>), want (false, nil)` | **只有最后那条断言能抓** |

**F1c 证实了 Dev 的判断**：账本重建为合法 JSON ✅、Count==1 ✅、第 2 次不报错 ✅ —— 前三条断言
全部通过，只有「第 3 次到达 Limit 应被拒」这一条转红。**这就是「功能恢复」与「只是不再报错」
的分水岭**，判据独立性成立。

### 1.3 `f.mu` 与 flock 各自独立充分

| 变异 | 结果 |
|---|---|
| F5 去掉 `f.mu`（留 flock 排他） | **绿** |
| F6 flock 改共享锁（留 `f.mu`） | **绿** |
| F7 两者同时失效 | **红(断言)** `:264 并发下放行 50 次, want 10` |

与 TASK-003 那两道短路**同构**：并发测试守护的是「不超发」这个**性质**，不是某一个**机制**。
Dev 已写进测试注释，避免后人删掉 flock 看到全绿以为它冗余 —— 这个处理是对的。

### 1.4 两个「守卫失效」形态实测复现

我写了独立探针验证（探针已删除，不在提交内）：

| 形态 | 实测 |
|---|---|
| **fd 泄漏的间接检测无效** | 注入 F4（不 `Close` fd）后：`nextFD` 法**红**（`fd 号 5 → 55`）；而间接版（跑 300 次看 `too many open files`）**仍 PASS** ✅ Dev 属实 |
| **`/dev/fd` 目录法会静默 SKIP** | go test 进程内 `os.ReadDir("/dev/fd")` → `lstat /dev/fd/3: bad file descriptor` ✅ Dev 属实 |

`nextFD`（下一个可用 fd 号，POSIX 保证 `open` 返回最小可用 fd）的选择是正确的。
**62 测试 0 SKIP 已核**——这一条不是形式主义，`/dev/fd` 那条正是「SKIP 掉的守护从未执行过」。

---

## 二、我补的取证：**真正的跨进程**（验收标准 3）

Dev 诚实标注了界定：「真正的跨进程 flock 保护在单个 go test 进程内验证不了」。
**这个界定成立，但它验得了** —— 我编译了一个独立的探针程序（`policy.NewFileStore` + `Take`），
用**真实 OS 进程**取证：

### 2.1 跨进程配额累计

```
进程A 放行=6   进程B 放行=4   合计=10   （Limit=10）
账本内容: {"t":{"window_start":"2026-08-06T00:00:00Z","count":10}}
```

两个**真正独立的进程**（不是同进程的两个实例）共享账本文件，配额正确累计。

### 2.2 flock 跨进程并发互斥（Dev 说验不了的那部分）

8 个真实进程并发、各抢 20 次（共 160 次尝试）、Limit=10，跑三轮：

| 轮次 | 各进程放行 | 合计 | 账本 count | 目录残留 |
|---|---|---|---|---|
| 1 | `[6,1,3,0,0,0,0,0]` | **10** | 10 | 无 ✅ |
| 2 | `[3,1,5,1,0,0,0,0]` | **10** | 10 | 无 ✅ |
| 3 | `[6,3,0,1,0,0,0,0]` | **10** | 10 | 无 ✅ |

**恰好 Limit，无超发**，且跨进程并发下**原子写也成立**（无临时文件残留）。

⇒ **验收标准 3 的取证现在是充分的**。Dev 的界定（DoD 措辞「在 flock 保护下」在单进程内只能
理解为「在现有互斥机制下」）**可以接受**，因为那条 DoD 的核心性质已由包内测试覆盖；
真正的跨进程语义建议按 §六 补一条外部验证脚本，但**不作为本轮 FAIL 依据**。

---

## 三、**判 rejected 的核心发现：I/O 失败路径的 fail-open 无守护**

`FileStore.Take` 有四条返回 `(true, err)` 的 fail-open 路径，DoD error_handling[0] 与约束 C7
要求它们**放行 + 告警、绝不阻断降级链**。实测把 `true` 改成 `false`：

| 变异 | 内容 | 结果 |
|---|---|---|
| **F15** | `lock()` 失败时返回 `(false, err)` | **绿 ❌ 存活** |
| **F16** | `write()` 失败时返回 `(false, err)` | **绿 ❌ 存活** |
| **F17** | `read()` 的 I/O 错误返回 `nil` map 而非空账本 | **绿 ❌ 存活** |

三条**全部无测试转红**。现有的 `TestFileStoreFailsOpenOnCorruptLedger` 只覆盖了
**JSON 解析失败**这一条路径（`read()` 的 `json.Unmarshal` 分支）。

### 为什么这条重要

- 约束 C7（fail-open 不阻断降级链）是本 Sprint 的核心约束；
- I/O 层故障（磁盘满、权限变更、只读挂载、目录被占）在真实环境里**比 JSON 损坏常见得多**；
- 一旦任一处被改成 `false`，`prism refresh` 会因为**账本写不进去**而拒绝发请求 —— 配额机制
  本是为了保护降级链，反而成了阻断它的原因；
- 覆盖率数据与此吻合：`write` **50.0%**、`lock` **63.6%**、`Take` 85.7%、`read` 88.9%，
  未覆盖行（`:48 :62 :71 :75 :78 :95 :109 :113`）**正是这些 fail-open 分支**。

---

## 四、Dev「无法可靠构造」的说明**不成立**（实测）

Dev 在 discovery 里说 I/O 失败分支（`MkdirAll`/`OpenFile`/`CreateTemp` 报错）无法可靠构造。
我写探针实测，**两种构造都稳定成功且完全可移植**：

| 构造 | 方法 | 实测结果 |
|---|---|---|
| **`MkdirAll` 失败** | 让父路径是一个**文件**而非目录 | `ok=true err=policy: quota dir: mkdir .../blocker: not a directory` ✅ |
| **`CreateTemp` 失败** | `os.Chmod(dir, 0o500)` 只读目录 | `ok=true err=policy: temp quota ledger: open ...: permission denied` ✅ |

两者都用 `t.TempDir()` 隔离、都不需要 root、都在 macOS/Linux 通用。而且**它们恰好验证了
fail-open 语义正确**（`ok=true` 且 `err != nil`）—— 正是 F15/F16 缺的那个断言。

> 说明：`os.Chmod(dir, 0o500)` 对 root 用户无效，CI 若以 root 跑需注意；可用
> `t.Skip` + 显式说明，或改用「父路径是文件」这一种（对 root 同样有效）。

---

## 五、Done Criteria 逐条覆盖矩阵

变异五道门：① 改动量非空且**改的是语义**；② `go test -c` 编译通过；③ 核到**断言行**
（正则区分 panic 堆栈）；④ **确认至少跑了 1 个测试**（新增，见 §六）；⑤ 还原后 8 个文件 md5 一致。

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 两个独立实例共享账本，配额累计（验收标准 3） | `QuotaSurvivesProcessRestart` | F14 红 `:76`；**另经真实双进程取证（§二）** | **PASS** |
| functional[1] | 窗口翻篇归零；主题账本隔离 | `ResetsOnWindowRollover` / `IsolatesTopics` | F13 红 `:98`、F8 红 `:103`、F14 红 `:76` | **PASS** |
| functional[2] | 文件不存在从空账本起算；父目录自动创建 | `MissingFileStartsEmpty` / `CreatesParentDir` | F12 红 `:191` | **PASS** |
| boundary[0] | 被拒的 Take 不写入计数 | `RejectedTakeDoesNotIncrement` | F9 绿 —— **等价变异体**（写的是原样账本，count 不增，DoD 仍满足） | **PASS** |
| boundary[1] | 并发 Take 不超发 | `ConcurrentTakesRespectLimit` | F7 红 `:264`（F5/F6 各自绿，见 §1.3）；**另经 8 进程并发取证（§二）** | **PASS** |
| error_handling[0] 分句1 | 坏账本时 fail-open，不 panic 不阻断 | `FailsOpenOnCorruptLedger` | F1 下**仍绿**（正确——分句 1 本就该绿） | **PASS** |
| error_handling[0] 分句2 | **损坏后必须自愈** | `SelfHealsAfterCorruption` | F1 红 `:160`、F1b 红 `:162`、**F1c 红 `:173`（分水岭）** | **PASS** |
| error_handling[0] **I/O 层 fail-open** | lock/write/read 失败时放行 + 告警（约束 C7） | **无** | **F15/F16/F17 三条全部存活** | **FAIL** |
| non_functional[0] | 原子写：无残留临时文件 | `LeavesNoTempFiles` | F10 绿（rename 失败分支不可达，等价变异体）；跨进程并发下亦无残留 ✅ | **PASS** |
| non_functional[1] | 走 temp+rename；fd 在返回前关闭（`verify_by: review`） | `DoesNotLeakFDs` | **review 判定**：`write()` 确实用 `os.CreateTemp` + `os.Rename` ✅；F4 红 `:354`（fd 5→55）✅ | **PASS** |
| non_functional[2] | `-race` 全包全绿 | — | 实测绿 | **PASS** |

**10 项 PASS / 1 项 FAIL。** FAIL 的那项是 error_handling[0] 中未被现有测试触及的 I/O 分支。

---

## 六、我自己造的两个无效变异（方法论，建议进纪律）

**F8 首版**：写成 `if entry.Count > 0 { entry.Count = entry.Count }` —— **空操作，语义未变**，
门①放行（字节确实变了）。这是已知的第③类。重做为「FileStore 自己判窗口」后红 `:103`。

**F9x —— 新形态**：我把定向测试名打成 `TestFileStoreRejectedTakeDoesNotWrite`（实际是
`...DoesNotIncrement`）。`go test -run '^不存在的名$'` **exit 0、跑了 0 个测试**，我的门③
（无断言行）判成了「**绿 存活**」——差点报出一个不存在的缺口。

> **第④道门：确认至少跑了 1 个测试**（数 `=== RUN` 行数，为 0 即判「结果无意义」）。
> 加上后 F9x 被正确拦下（`跑了 0 个测试（-run 未匹配）→ 结果无意义`），修正测试名后
> F9 跑了 1 个测试、判定为等价变异体。

这是「变异无效」的第四种形态，前三种是：① 表达式静默失配；② 改坏代码致编译失败；
③ 改动量非空但语义未变。**四种的共同点都是「红/绿的原因不是被测行为」。**

---

## 七、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 测试 | **62 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿 |
| 覆盖率 | **91.8%**；缺口集中在 `write` 50.0% / `lock` 63.6% —— **正是 §三 那些无守护的 fail-open 分支** |
| C3 / gofmt / vet | 仅 policy 自身 / 无输出 / exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | 仅新增 `quota_file.go` + `quota_file_test.go`，落在声明内 ✅ |
| 上游产物 | `git diff 05cf499 -- quota.go gate.go policy.go` **无输出** → TASK-001/002/003 实现未动 ✅ |
| 文件系统卫生 | 所有测试用 `t.TempDir()`；跨进程探针实验的临时目录已清理；worktree `git status` 干净 ✅ |

---

## 八、修复清单

**R1（MAJOR）—— 补 I/O 层 fail-open 的守护**（对应 §三、§四）

至少补两条测试，断言 `(ok, err)` 为 `(true, 非nil)`：

```go
func TestFileStoreFailsOpenOnDirError(t *testing.T) {
	d := t.TempDir()
	blocker := filepath.Join(d, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)          // 父路径是文件 → MkdirAll 必失败
	s := NewFileStore(filepath.Join(blocker, "sub", "q.json"))
	ok, err := s.Take("t", q, now)
	if !ok || err == nil {
		t.Errorf("目录不可建时必须 fail-open: (%v, %v), want (true, err)", ok, err)
	}
}

func TestFileStoreFailsOpenOnWriteError(t *testing.T) {
	// 预热后 chmod 0500，使 CreateTemp 失败 → write 失败路径
	...
	if !ok || err == nil { t.Errorf("账本写不进去时必须 fail-open: ...") }
}
```

两条构造均已实测可靠（§四）。补上后 F15/F16 应转红。

**R2（MINOR）—— 修正 discovery 里「无法可靠构造」的说明**，改为记录这两种构造方法，
免得下一位接手 FileStore 的人重复得出「测不了」的结论。

**建议（不作为返工项）** —— §二 的跨进程探针可以固化成一个 `//go:build manual` 的验证程序
或 Makefile 目标，作为验收标准 3 的外部证据；它验的是包内测试**结构上**验不了的东西。

---

## 九、复现命令

```bash
git worktree add --detach ../wt-v004 1ca7a7de184c8bee2b9e51a20e7e912c9ed399b2
cd ../wt-v004

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race        # 62 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/f.out
GOTOOLCHAIN=local go tool cover -func=/tmp/f.out | grep quota_file            # write 50% / lock 63.6%

# 核心发现（R1）：把这三处的 `return true, err` 改成 `return false, err`
#   quota_file.go:48  lock 失败    → 无测试转红
#   quota_file.go:62  write 失败   → 无测试转红
#   quota_file.go:95  read I/O 错  → 无测试转红（改返回 nil map 亦然）

# I/O 失败分支的可靠构造（证明「无法构造」不成立）：
#   MkdirAll 失败：父路径设为一个文件 → "not a directory"
#   CreateTemp 失败：os.Chmod(dir, 0o500) → "permission denied"

# 真正跨进程取证（§二）：编译一个调用 policy.NewFileStore(path).Take 的独立程序
#   （worktree 内需 go build -buildvcs=false），多进程并发跑，合计放行须 == Limit

# 变异五道门：① 改动量非空且改的是语义 ② go test -c 编译通过
#            ③ 核到断言行（区分 panic 堆栈）④ **确认 === RUN 数 > 0**
#            ⑤ 还原后 8 个文件 md5 与基线一致
```

worktree 已于验证结束后清理；主工作区 `internal/` 零污染。
