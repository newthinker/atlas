# Sprint 032 进度 — M1a `internal/macro/bitemporal`

**阶段**：Step 5 开发中。**wave 1 已交付，两个任务在验证。**

## 全局

```
TASK-001  verifying   test-agent-18   224c960  spec.go + spec_test.go        (100% 覆盖)
TASK-002  verifying   test-agent-19   96641ec  classify.go + classify_test.go (100% 覆盖)
TASK-003  pending     ← 001           dev-agent-39 只读预备中
TASK-004  pending     ← 001,003
TASK-005  pending     ← 001,002,003   预定 dev-agent-40（它做的 State）
TASK-006  pending     ← 全部
```

分支 `feat/macro-bitemporal`，2 个提交。

## 团队

`dev-agent-39` / `dev-agent-40` / `test-agent-18` / `test-agent-19`

**spawn 前置踩到一个历史遗留**：`dev-agent-1`/`test-agent-1` 等在过往 Sprint 已登记 token，
写通道的 R1 收口要求「重置已登记实例须携带其当前 token」，而那些 token 从未落盘 ⇒ 改用
未占用的实例名。**失败是静默的**（`set-token` 非零退出但无输出），靠 `&& echo` 没打印才发现。

## 已发现的 DoD 缺陷（1 处，Leader 的）

**TASK-002 `boundary[1]` 的变异判据打的是不存在的靶。**

独立 reviewer 在 Step 3 就指出了这条（「指定的 `Classify` 里根本没有 `<`」），
**而我改的是 `boundary[0]`，坏的那条在 `boundary[1]`，两条现在都在文件里**。
更糟的是我在给 test-agent-19 的派验单里也说「boundary[0] 我改过」——**指错了位置**。

**dev-agent-40 没有停下来问，而是先把两个字面分支都实测了**：

- `<` → `<=`：`grep -n '<' classify.go` 无输出 ⇒ **靶不存在**
- `>` → `>=`（「或反之」分支）：**实测 0 条转红** ⇒ **等价变异**

第二条把独立 reviewer 当初给的**论证**升级成了**验证**。

处置：TASK-002 已 `verifying`（DoD 写不进），更正走 inbox 送达 test-agent-19，
要求按 dev 的 M4a/M4b 判并独立复现。**DoD 文本待该任务落定后修。**

## 已承接的移交（1 处，dev-agent-39 → TASK-004）

> 本任务只证明了 `NewSpec` **拒收**非法标识符，**没证明所有拼进 SQL 的标识符都取自 Spec**。

已加进 TASK-004 的 `error_handling`，要求逐个核对 `query.go` 里进入 SQL 的每个标识符来源，
并特别点了 **`correlate` 的 alias 参数是调用方传入的字符串、不过 identRE**。

**包的「注入面为零」声明有两半，001 守住了一半，004 守另一半，缺一半声明就不成立。**

## 待办

- [ ] wave 1 验证结果（001 / 002）
- [ ] 001 verified → 派 TASK-003 给 dev-agent-39（**fixture 数据须有「共享恰好一列」的键**，
      否则 TASK-004 的 T1 变异打不中）
- [ ] wave 3：004‖005 同包并行——**派发 prompt 须带文件级隔离命令**
      （`go test spec.go spec_test.go` 式，dev-agent-39 在 wave 1 撞过包级编译被对方未落地文件挡住）
- [ ] Step 6 QA / Step 7 交付归档

## 写通道踩坑记录（2 条，进 final-report 待同步清单）

1. `set-token` 对**已登记实例**静默失败（需携带旧 token，而旧 token 从未落盘）
2. 内容一律走 **stdin**，**没有 `--content` 参数**；误传会让脚本阻塞在读 stdin 直到超时（dev-agent-40 踩到）
