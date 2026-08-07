# TASK-021 派发前量定：eastmoney / baostock / crypto 的链保留判据

- 测量者：test-agent-17（应 Leader 请求，**只给判定与取证方法，不给补丁**）
- 锚：`f6a78afeba34b152922da818c58f3c1f04f167a1`（量定时主工作区无在途改动）
- 结论：**那个「或」必须消解为二选一——判据由 body 形态唯一决定，不能两条都写。**

## 一、直接回答三个问题

### Q1/Q2：eastmoney 与 baostock 的 body 形态与错误类型（实测，非推断）

三包（含已验的 yahoo）在**受守护路径**上都用 `json.NewDecoder(...).Decode`，行为一致：

| body 形态 | 产生的错误 | `errors.As(*json.SyntaxError)` | `errors.Is(io.ErrUnexpectedEOF)` |
|---|---|---|---|
| `not-json` 式（非法字符） | `invalid character 'o' in literal null` | **true** | false |
| `{"data":` 式（截断） | `unexpected EOF` | false | **true** |

**两者互斥。** 逐包实测输出：

- eastmoney（`FetchHistory` → `fetchStockHistory` → `eastmoney.go:510 decoding response: %w`）
  - `not-json` → `decoding response: invalid character 'o' in literal null` ⇒ As=true / Is=false
  - `{"data":` → `decoding response: unexpected EOF` ⇒ As=false / Is=true
- baostock（`FetchHistory` → `client.go:68 baostock: daily %s: decode: %w`）
  - `not json` → `... decode: invalid character 'o' in literal null` ⇒ As=true / Is=false
  - `[{"date":` → `... decode: unexpected EOF` ⇒ As=false / Is=true

⇒ **`functional[3]` 里那个「`errors.As(*json.SyntaxError)` 或 `io.ErrUnexpectedEOF`」两条都对，但对应不同 body。**
既然 `functional[0]` 已把 yahoo/lixinger 统一到 `errors.As(*json.SyntaxError)`，
**eastmoney/baostock 也用 `errors.As` + `not-json` 式 body**，五包里四包写法一致。

### Q3：crypto 的注入点**已存在且已被使用**，不必新加

crypto 的受守护路径上**没有解码**（`policy.Fetch` 的 fn 是 `fetchHistoryFromProviders`），
链来自 `crypto.go:203 all providers failed for %s: %w`（包住 provider 返回的 `lastErr`）。

- 既有夹具 `countingProvider` 有 `err` 字段，**已在 `gate_test.go:447`（`TestErrorIsNotCached`）被使用**。
- 实测：`countingProvider{err: 自定义哨兵}` ⇒ `FetchHistory` 返回
  `all providers failed for BTCUSDT: meas: upstream sentinel`，**`errors.Is(err, 哨兵) = true`**。

⇒ 注入点可复用，**无需新增夹具**。唯一障碍是 `TestErrorIsNotCached` 用的是**内联**
`errors.New("upstream down")`，要写 `errors.Is` 就得有具名哨兵 ⇒ 要么把它提成包级 var
（**这算改既有测试**），要么另起一小格。这一步由 Leader 定，我不给补丁。

## 二、「能否挂在既有那格上」——五包答案不同，这是本次量定最要紧的发现

`functional[0]` 现文本说「各自挂在既有的『畸形 JSON』那一格上」。**这句只对 yahoo/lixinger 成立。**

| 包 | 既有可挂的格 | 实情 |
|---|---|---|
| yahoo | **有** | `TestNonPolicyErrorsUnaffected` 的『畸形 JSON』格，body=`not-json` ⇒ 直接追加 `errors.As` |
| lixinger | **有** | 同上，body=`not-json` |
| **eastmoney** | **没有** | 既有「非policy错误原样透传」格用的是 `emptyKlineBody`（空 klines）⇒ 错误是 `no history for symbol: ...`，**纯 `fmt.Errorf`、根本没有链**（实测 As=false 且 Is=false）。**挂不上去，须新增一格（body 用 `not-json`）** |
| **baostock** | **没有** | `TestPolicyErrorsDoNotLeak` 只有「配额耗尽」「超时」两格，**没有非 policy 那一格**。⚠ `client_test.go:TestFetchDailyBadJSON` 虽然用了 `not json`，但它直测 `New(url).FetchDaily(...)`、**绕过 Gate 与 mapPolicyError**，不在受守护路径上，**不能拿来充数** |
| crypto | 注入点有、格要斟酌 | 见 Q3 |

**⇒ 建议把 `functional[0]` 的「挂在既有那格上」改成逐包指明**，否则 dev 会在 eastmoney/baostock
上找不到那一格，或者更糟——**误用 baostock 那个绕过 Gate 的既有测试**，得到一条永远绿的断言
（它根本不经过映射层，对抗变异动不了它）。

## 三、变异判据（三包各自的 2×2 已跑完）

对抗变异统一形态：**在调用点用 `%v` 多包一层，不动映射函数本身**。

| 包 | 变异写法 | 候选断言（格A 无变异） | 格B 变异后 | 格C 既有全套 |
|---|---|---|---|---|
| eastmoney | `fmt.Errorf("eastmoney: %v", mapPolicyError(symbol, err))` | **PASS** | **FAIL** `链被切断: eastmoney: decoding response: invalid character ...` | **ok（全绿，无人抓到）** |
| baostock | `fmt.Errorf("baostock: %v", mapPolicyError(symbol, err))` | **PASS** | **FAIL** `链被切断: baostock: baostock: daily ...: decode: invalid character ...` | **ok（全绿）** |
| crypto | 非 policy 分支 `return nil, err` → `return nil, fmt.Errorf("crypto: %v", err)` | **PASS** | **FAIL** `链被切断: crypto: all providers failed for BTCUSDT: meas: upstream sentinel` | **ok（全绿）** |

格C 全绿**独立复现了 test-agent-16 的横向排查结论**：三包确实全缺这一守护。

## 四、方法说明

- 三条候选断言**先在无变异基线上确认为真（格A）**，再注入变异（格B）——
  这正是 021 `boundary[0]` 那条纪律，本次量定自己也照做了。
- 全部作业在隔离 worktree（锚 `f6a78af`）内完成；每包还原后 `git status --porcelain` 为 0，
  三包收尾复跑全绿；主工作区零污染。
- ⚠ 我在拆 worktree 时**站在它里面**执行了 `git worktree remove`，导致后续命令 `Unable to read
  current working directory`。已从主仓库复核清理结果。这是我第二次踩同一个坑，记在此处。

## 五、原始产物指针

- 任务文件：`.arcforge/tasks/TASK-021.json`
- 上游：`.arcforge/docs/04-test/TASK-018-verification-addendum.md`（缺口的发现与对抗变异形态）
- git ref：`f6a78afeba34b152922da818c58f3c1f04f167a1`
- 复现（锚钉全 sha）：

      git worktree add --detach ../wt-021meas f6a78afeba34b152922da818c58f3c1f04f167a1
      # 各包放一个 meas 探针(body 用 not-json 式)，先跑格A 确认 PASS，
      # 再在调用点用 %v 多包一层，确认转红且既有全套仍绿。
