# TASK-018 验证补充：互补性表格的对抗复核（找到一处真缺口）

- 验证者：test-agent-17
- 触发：Leader 在派验单里提的挑战 ——「**如果你能造出一个变异让两条同时绿，那就是真缺口**」
- 时点：**在我已出具 `verified` 之后**。主报告 `.arcforge/docs/04-test/TASK-018-verification.md`
  的 11 条矩阵不变；本篇是对 `boundary[1]` 的一次更强复核，结论**与该矩阵冲突**。
- 结论：**能造出来。`boundary[1]` 的后半句（「errors.Is 行为一字不变」）在 yahoo 与
  lixinger 两包无守护。**

## 一、我先复核了 Leader 那张表，两格都成立

| 缺陷强度 | `PassesThrough`（值断言） | `NonPolicyErrorsUnaffected`（文本断言） |
|---|---|---|
| Y6：包一层但 `%v` **保留**原文 | **红** `errmap_test.go:241` | **绿** |
| N-A：包一层且**丢掉**原文 | 红 | **红** |

两条测试各自有内容、互补性成立，**都必须留**。这部分 Dev 的论证属实。

## 二、但表格覆盖的不是缺陷的全部形态

两条测试的判据分别是「**是不是同一个 error 值**」与「**文本里还有没有原判别词**」。
两者都不看**错误链**。而 DoD `boundary[1]` 的原话是：

> 上游 HTTP 错误、解析错误等原本就存在的错误，映射前后 **`errors.Is` 行为一字不变**
> （否定断言，契约陷阱 8——**直接断言这些错误仍可被原有方式识别**）

`errors.Is` 行为这半句，**没有任何测试在守**。

### 对抗变异（我构造的）

保持 `mapPolicyErr` **完全不动**，只在**调用点**用 `%v` 多包一层：

    - return nil, mapPolicyErr(err)
    + return nil, fmt.Errorf("yahoo: %v", mapPolicyErr(err))

为什么两条都躲得过：
- `PassesThrough` 是**直测 `mapPolicyErr` 的单元测试**，而 `mapPolicyErr` 一个字没改 ⇒ 绿。
- `NonPolicyErrorsUnaffected` 是端到端**文本包含**断言，`%v` 把原文原样带过去 ⇒ 绿。
- 但 `%v` **切断了非 policy 错误的链** —— 这正是 DoD 明令禁止的那件事。

### 实测（yahoo，改 3 处调用点）

**整个 yahoo 包无一转红**（不只是那两条，是全部）。

2×2 同环境对照（探针：截断 JSON ⇒ `json.Decoder.Decode` 返回 `io.ErrUnexpectedEOF`，
断言 `errors.Is(err, io.ErrUnexpectedEOF)`）：

| | 既有全套 | 探针 |
|---|---|---|
| 无变异 | 绿 | **绿** |
| 对抗变异 | **绿（漏检）** | **红**：`既有错误的 errors.Is 链被切断了: yahoo: decoding response: unexpected EOF` |

右列上下不同 ⇒ 变异真实改变行为，**不是等价变异**；左列上下相同 ⇒ 既有套件测不到。

## 三、逐包边界：两包有缺口、一包等价、一包已守住

| 包 | 非 policy 错误有无可判定链 | `NonPolicyErrorsUnaffected` 的判据 | 对抗变异结果 | 判定 |
|---|---|---|---|---|
| **yahoo** | 有（8 处 `%w`） | 文本 | **整包全绿** | **缺口** |
| **lixinger** | 有（12 处 `%w`） | 文本 | **整包全绿** | **缺口** |
| twelvedata | **无**（唯一的 `%w` 在注释里；`wrapErr` 按设计对一切错误断链） | 文本 | 等价变异 | 无风险面 |
| tushare | 有（8 处 `%w`） | **`errors.Is`**（`wantIs`/`wantNot`） | `client_test.go:114` / `:200` / `:214` **三条转红** | 已守住 |

lixinger 的 2×2（探针改用 `errors.As` 取回 `*json.SyntaxError`）：格A 绿、格B 红，
变异真实有效；整包在该变异下 `ok`。

**这条边界本身就是结论**：`tushare` 之所以自动挡住，是因为它**有哨兵**、
于是它的 `NonPolicyErrorsUnaffected` 天然写成了 `errors.Is` 判据。这与
Leader 已入契约的「正/否定断言可守护性不对称」是同一现象的又一面——
**有哨兵的包免费获得类型级守护，无哨兵的包必须显式对某个上游可判定错误写
`errors.Is`/`errors.As` 断言，否则「链保留」这个属性没有任何东西钉住。**

## 四、我的处置建议

**实现是对的**（交付并没有在调用点多包一层），这是**测试缺口**，与我判 TASK-016
`rejected` 的那条同类。但 TASK-018 我已经出具 `verified`，`verified → review_fix`
是 Leader 专属边，请你裁定是否返工。

修复很小：给 yahoo 与 lixinger 的 `TestNonPolicyErrorsUnaffected` 各加一条
链判定断言即可（不必新增测试）：

- yahoo：截断 JSON ⇒ `errors.Is(err, io.ErrUnexpectedEOF)` 必须为真
  （注意 yahoo 用 `json.Decoder.Decode`，返回的是 `io.ErrUnexpectedEOF`）
- lixinger：截断 JSON ⇒ `errors.As(err, &*json.SyntaxError)` 必须成立
  （lixinger 用 `json.Unmarshal`，返回 `*json.SyntaxError`，**不是** `io.ErrUnexpectedEOF`）

## 五、我自己在本轮踩的一个坑，记在这里

lixinger 的探针**首版无效**：我照搬 yahoo 的 `errors.Is(err, io.ErrUnexpectedEOF)`，
结果**两格都红** —— 断言在基线就不成立。根因是两包解码方式不同
（`json.Decoder.Decode` → `io.ErrUnexpectedEOF`；`json.Unmarshal` → `*json.SyntaxError`）。

若不先看格A、只看到格B 转红，就会把「我的探针写错了」误报成「缺口已证实」。
**探针也是断言，也必须先在基线上验证它为真**——这与「变异有效性要单独证明」是同一条纪律
的另一半。改用 `errors.As` 后 2×2 才成立。

（顺带：这与 crypto 那条空断言是同一个母题——**照搬一个形态时，被照搬那一侧的前提未必在这边成立**。）

## 六、原始产物指针

- 主报告：`.arcforge/docs/04-test/TASK-018-verification.md`
- git ref：`4c92611ec5e4c375f66b15f3ea3e14e5ed2afedf`
- 复现（锚钉全 sha）：

      git worktree add --detach ../wt-018adv 4c92611ec5e4c375f66b15f3ea3e14e5ed2afedf
      # 对抗变异：把 yahoo.go / eps.go 的 3 处
      #   return nil, mapPolicyErr(err)
      # 换成
      #   return nil, fmt.Errorf("yahoo: %v", mapPolicyErr(err))
      GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1   # 预期:全绿(漏检)
