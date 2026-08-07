# TASK-016 三轮验证报告（crypto 聚合度空断言修复）

- 验证者：test-agent-17
- 交付：`8623e14c7fd665294e3bb98feb3e7922b8e47a85`（`gate_test.go` +14/-1，生产代码零改动）
- 承接时 `assignment_epoch`：**4**；`rework_count` = 2
- 裁决：**verified**

## 只验上轮 FAIL 的 `functional[4](a)`，其余十二条上轮已逐条变异 PASS

修复正是我上轮给的方向：`gate_test.go` 的偏移量由 `+50ms / +900ms` 改为 **`+3s / +15s`**，
**实现零改动**，删除行只有一行（就是偏移量那行），既有断言体与场景未动。

## 双向判据实测：两个方向各自有守护，且互不重叠

| 变异 | DoD 要求 | 实测转红位置 | 子测试① 聚合度 | 子测试② 不得放粗 |
|---|---|---|---|---|
| D1 去掉 `Truncate(time.Minute)` | (a) 须转红 | `gate_test.go:250`（取 3 次 want 1） | **FAIL** | PASS |
| D2 `Truncate(time.Hour)` | (b) 须转红 | `gate_test.go:265`（取 1 次 want 2） | PASS | **FAIL** |

**两个方向的红互不重叠**——D1 只打中①、D2 只打中②，说明两条断言各守一个方向、
都有分辨率，不是「一条红盖住另一条」。这正是上轮缺的那一格。

对照上轮同一命令下的结果：D1 曾让**整包 60 个测试无一转红**。同一变异现在转红，
构成完整的前后对照。

## 抖动检查

`base := time.Now().Truncate(time.Minute).Add(30*time.Second)`，最远偏移 `+15s`
⇒ `30 + 15 = 45s < 60s`，**不跨分钟边界**。`-count=10` 连跑 10 次全绿。

## 回归

| 项 | 值 |
|---|---|
| `=== RUN` / PASS / SKIP / FAIL | 60 / 60 / 0 / 0 |
| 覆盖率 | 75.0%（`coverage_floor` 70，与上轮持平） |
| `-race` | 绿 |
| 本 commit 触碰文件 | 仅 `internal/collector/crypto/gate_test.go` |
| 全仓回归 | **62 包 ok / 0 FAIL** |

**抽验上轮已 PASS 的守护未回潮**（本 commit 只改一行加注释，结构上不应受影响，
仍实测确认）：C11（分句②，长度校验移出被缓存的 `fn`）→ `gate_test.go:436` 红；
D5（去掉 `slices.Clone`）→ `gate_test.go:400` 红。每次注入后均还原，
`crypto.go` md5 回到 `295247761d1f9d8b471a68cbcd078954`，工作区改动文件数 0。

## 观察项

Dev 在注释与 commit message 里把上轮的实测归给了 **test-agent-16**，实际是
**test-agent-17（我）**。不影响任何判定，但注释里那句「test-agent-16 实测：D1 下整包
60 个测试无一转红」的署名建议订正，以免日后追溯时找错人。

它自己写下的根因值得留档，我核对属实：

> 首轮我用「退回 `UnixNano()`」这个**我自己选的**变异验过它会转红，就以为守护成立。
> 但 DoD 规定的变异是「去掉 `Truncate(time.Minute)`」（留 `.Unix()`），两者不是一回事——
> 前者把粒度放到纳秒（毫秒偏移可见），后者停在秒（毫秒偏移不可见）。
> **拿自己的变异验证，不等于满足了 DoD 指定的变异判据。**

## 原始产物指针

- 任务文件：`.arcforge/tasks/TASK-016.json`
- 上轮报告：`.arcforge/docs/04-test/TASK-016-verification-round2.md`（含完整 13 条矩阵）
- git ref：`8623e14c7fd665294e3bb98feb3e7922b8e47a85`
- 复现命令（锚钉全 sha）：

      git worktree add --detach ../wt-v016r3 8623e14c7fd665294e3bb98feb3e7922b8e47a85
      # D1：删掉 crypto.go 里两处 Truncate(time.Minute)，子测试① 须转红
      # D2：改成 Truncate(time.Hour)，子测试② 须转红
      GOTOOLCHAIN=local go test ./internal/collector/crypto/ -count=1 -run TestCacheKeyAggregatesNearbyTimes -v
