# TASK-010 验证报告 —— lixinger 接入 Gate（仅 TTL）

- 验证者: test-agent-16 / assignment_epoch: 1
- 被验对象: `b0bc78a`（client.go +28 · lixinger.go +10 · gate_test.go +286，共 322 增 / 2 删）
- 验证环境: 独立 worktree `../wt-v010 @ b0bc78a`（已在主仓库拆除）
- 判定线: 覆盖率**绝对下限 92.0%**（Leader 裁定）

## 结论：**PASS（verified）**

7 条 done_criteria 全部通过。**9 个变异全部按预期捕获，0 存活、0 无效**。B4 达到最强形态：
**8 个既有测试文件一个字节都没改**。

我另做了两项**变异结构上够不着**的独立核查，其中一项**填上了 Dev 自己据实声明未做的那一格**。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `request()` 经 Gate 进缓存路径，同 endpoint+payload 只发一次 HTTP（**验收标准 6**）| `TestLixingerRequestIsCached` | L4 绕过 Gate → 红 `:101`；L8 主题漏前缀 → 红 `:101` | **PASS** |
| functional[1] | 缓存 key 含 payload，不同 payload 不共享 | `TestLixingerCacheKeyIncludesPayload` | L2 键固定为常量 → 红 `:121` | **PASS** |
| functional[2] | 不同 endpoint 使用互相独立的缓存 key | `TestLixingerEndpointsAreSeparateKeys` | L3 主题固定为 `lixinger.x` → 红 `:138` | **PASS** |
| functional[3] | 持有 `gate *policy.Gate`，构造函数取 `policy.Default()`，包内可注入 | `TestLixingerConstructorSnapshotsDefaultGate` | L5 `gate: nil` → 红 `:154`/`:157` | **PASS** |
| boundary[0] | **lixinger 不被节流**（只补缓存，不新增限流/配额）| `TestLixingerNotThrottled` | **L7 内置表加 MinInterval → 红 `:181`（策略表侧）+ `:219`（请求路径侧）**；**L9 内置表加 Quota → 红 `:184`** | **PASS** |
| boundary[1] | 缓存命中返回独立字节切片，调用方修改不污染缓存 | `TestLixingerReturnsIndependentBytes` | L1 不做 `bytes.Clone` → 红 `:248` | **PASS** |
| error_handling[0] | 既有测试一条不删不改且全通过；**错误不写缓存** | 既有 52 个测试 + `TestLixingerErrorIsNotCached` | §二 文件级零改动；L6 信封校验移出 fn → 红 `:278` | **PASS** |
| non_functional[0] | `-race` 全绿 + 覆盖率 ≥ 92% | 实测 | `-race ok`（1.977s）；**92.2% ≥ 92.0%** | **PASS** |

## 二、B4 核对：最强形态，文件级零改动

TASK-010 与 TASK-008 不同——**lixinger 没有私有节流状态需要删除**，因此 Leader 裁定的豁免
①②③ 在此**根本用不上**，B4 可按字面判。实测结果是最强的那种：

```
git show --name-only b0bc78a | grep '_test.go'
→ internal/collector/lixinger/gate_test.go     （仅此一个，且是新增）
```

| 既有测试文件 | 是否被改动 |
|---|---|
| client_test.go / fund_test.go / fundamental_test.go / history_test.go | ✅ 全部未改动 |
| lixinger_test.go / series_test.go / stock_test.go / valuation_test.go | ✅ 全部未改动 |

**8 个既有测试文件一个字节没动，52 个既有测试全部通过。** 无需援引任何豁免。

## 三、变异验证结果表（9 个，9 捕获 / 0 存活 / 0 无效）

每个变异**注入前先写下预期，实际与预期无一处不符**。runner 强制四道门：
① `md5` 改动量非空 ② `go vet` 通过 ③ `=== RUN` 数 > 0 ④ 判红只认
`<file>_test.go:NN:` 断言行（`exit != 0` 但无断言行报「红但非断言 ⚠不计为捕获」）。

| ID | 变异内容 | 预期 | 实际 | 断言行 |
|---|---|---|---|---|
| L1 | `request` 不做 `bytes.Clone`（`if false {}` 保留引用）| 红 | **红** ✅ | `:248` |
| L2 | 缓存键去掉 payload（固定串）| 红 | **红** ✅ | `:121` |
| L3 | 主题去掉 endpoint（固定 `lixinger.x`）| 红 | **红** ✅ | `:138` |
| L4 | 绕过 Gate，直调 `requestHTTP` | 红 | **红** ✅ | `:101` `:121` `:138` `:224` `:255` |
| L5 | 构造函数 `gate: nil` | 红 | **红** ✅ | `:154` `:157` |
| L6 | 信封校验从 fn 内部移到 `Fetch` 外面 | 红 | **红** ✅ | `:278` |
| **L7** | **（我加）policy 内置表给 `lixinger.*` 加 `MinInterval: 300ms`** | 红 | **红** ✅ | `:181` **+ `:219`** |
| L8 | 主题漏掉 `"lixinger."` 前缀（Lookup 落空 → 直通不缓存）| 红 | **红** ✅ | `:101` `:121` `:138` `:224` `:255` |
| **L9** | **（我加）policy 内置表给 `lixinger.*` 加 `Quota{5,24h,SH}`** | 红 | **红** ✅ | `:184` |

### 3.1 L7 / L9：填上 Dev 据实声明的那一格

Dev 在 discovery 里**主动声明了一处未做**，措辞诚实：

> 「boundary[0] 分句①（内置表无 MinInterval/Quota）的变异需改 policy 包，超出 writes 范围
> 且会干扰三个并行 Dev，**未注入**；它目前只有「等值谓词」这一肉眼判据，分句②已由 L1 实证。」

这个约束对 Dev 成立（它不能动别人的 scope），但**对验证者不成立**——我在**一次性 worktree**
里改 policy 内置表不影响任何人。于是我补了这一格：

- **L7**（加 `MinInterval: 300ms`）→ 红在**两处**：`:181` 策略表侧的等值断言、
  `:219` 请求路径侧的墙钟耗时断言。**boundary[0] 的两个分句各自独立有效。**
- **L9**（加 `Quota`）→ 红在 `:184`，Quota 那条等值断言同样活着。

⇒ **boundary[0] 现在有实证，不再只有「肉眼判据」。** 这正是契约那条
「变异只能证伪我写下的断言，不能证伪我没写的那些」所说的双层结构：Dev 自证到边界处
如实止步并声明，验证者从没有该约束的位置把它补上。

### 3.2 与 Dev 自述的交叉核对

Dev 做了 6 个变异，我全部独立复现，**结论逐条一致**（L1–L6 对应它的 L4/L2/L3/L1/L5/L6，
编号不同但内容对应）。我另加 L7/L8/L9 三个。其自述的一处「预期不符」也是诚实记录：

> L2 首版预期 `NotThrottled` 绿、实际红 → 诊断为**测试间隐藏耦合**（探针靠换 payload
> 避缓存，暗中依赖 functional[1]），非代码缺陷；改用「内置策略 + TTL 归零」探针后复跑。

我核了改后的探针（gate_test.go:194-211），确认它用 `p.TTL = 0` 而非换 payload 来避开缓存，
**耦合确已解除**——这也正是陷阱 9 说的「对照组必须真的进入被测路径」。

## 四、变异够不着的两项独立核查

### 4.1 验收标准 6 的结构性证明：**所有** HTTP 路径确实汇流到 `request()`

DoD functional[0] 说「Gate 放进 `request()` 两条身份同时被覆盖」。**对 `request()` 做多少变异
都证明不了「没有第三条路径绕过它」**——那是覆盖缺口，只能靠独立视角。我直接查了：

| 检查 | 结果 |
|---|---|
| 包内所有发 HTTP 的位置 | `l.client.Do(req)` **仅出现 1 次**（client.go:102，在 `doOnce` 内）|
| `requestHTTP` 的调用点 | **仅 1 处**（client.go:47，即 `policy.Fetch` 的 fn 内部）|
| 调用 `l.request(...)` 的上层 | 8 处，覆盖 series / stock / fund×4 / valuation / fundamental |

⇒ **包内不存在任何绕过 Gate 的 HTTP 路径。** 两种身份（eastmoney 内部 fallback、
Valuation/Fundamental source）无论走哪个导出方法，最终都收敛到唯一的 `doOnce`，
而它只能经 `requestHTTP` ← `request()` ← Gate 到达。**验收标准 6 结构上成立。**

### 4.2 `TestMain` 零策略闸门的必要性 —— Dev 的理由属实，且我量化了

Dev 称既有错误路径用例「大量复用同一个 endpoint + 空 payload（同一缓存槽），带 TTL 的
默认闸门会让它们互相串味」。这是个**关于 B4 是否真的成立**的关键前提——既有测试之所以
全绿，是因为 `TestMain` 把闸门中和了。我实测验证：

> 把 `TestMain` 的零策略闸门换成**内置表**（`lixinger.*` = TTL 5m）→ **18 个既有测试转红**
> （`TestRequest_No4xxRetry` / `TestRequest_4xxSurfacesErrorMessage` /
> `TestFetchFundInfoPublic_PartialFailureDegrades` 等）。

**Dev 的理由属实。** 这个做法是否合规：`TestMain` 是**新增文件里的新增测试基建**，
不是对既有测试用例的修改，与 TASK-008 的处理一致，B4「一条不删不改且全部通过」两个分句
都真实满足。

⚠️ 但由此产生一个**应当被记下的接缝**（不构成缺陷）：

> **52 个既有测试在零策略闸门下运行，因此它们对「生产缓存配置」的覆盖为零。**
> lixinger 的全部缓存行为只由新增的 7 个 gate 测试守护。

这不是生产隐患——既有测试相撞是因为它们用**相同 endpoint+payload 却期待不同响应**，
而生产上同 key 本就该同响应。但若将来有人删掉 `TestMain`，会看到 18 个测试莫名转红，
建议保留 gate_test.go:32-34 那段注释（它已经写清了原因）。

## 五、覆盖率与稳定性

| 对象 | 覆盖率 |
|---|---|
| master `b197394` 基线 | **92.1%** |
| 交付 `b0bc78a` | **92.2%** |
| **判定线（绝对下限）** | **92.0%** |

**两种口径都过**（≥92.0% ✅；未低于基线 ✅）。

- 测试规模：**59 个测试全 PASS / 0 FAIL / 0 SKIP**（陷阱 11：与 master 基线的 `0 SKIP` 一致）
- `-race`：ok（1.977s）
- `-shuffle=on` **5 次全绿**（0.62–1.01s），`TestMain` 换默认闸门无顺序依赖
- build / vet / gofmt：全 exit 0 / 无输出

## 六、约束核查

| 项 | 结果 |
|---|---|
| scope | 改动**全部落在 `./internal/collector/lixinger/`**，与 `writes` 一致，无越界 ✅ |
| **硬纪律 2**：`internal/prism/refresh.go` 一行不改 | `git diff` **空** ✅ |
| 硬纪律：只补 TTL，不新增限流/配额 | L7/L9 双向实证 ✅ |
| 调用方回归 | `internal/prism` / `internal/collector/eastmoney` / `cmd/atlas` **三个包全 ok** ✅ |
| 还原 | 每个变异 `cp` + `md5` 校验；收尾 `git status --porcelain` 空、`git diff --stat` 空 ✅ |
| worktree | 在**主仓库**执行 `remove`，无残留 ✅ |

## 七、备录（均不作为 FAIL 依据）

1. **task JSON 的 `discovery` 字段为 `null`**，但文件已完整落盘
   （`.arcforge/discoveries/TASK-010.json`，11804 字节，7 key_findings / 6 decisions）。
   与 TASK-012 同类记录瑕疵。`verifying` 状态下写权在我，**我已代为回填**（见 §八）。
2. **Dev 据实声明的两处环境限制**，我认为处理得当：
   - 项目 CLAUDE.md 要求的 GitNexus `detect_changes()` 未执行——其工具集不含 gitnexus MCP
     工具，已用 `go build ./...` 全仓编译 + 全包 `-race` 替代。**这是工具可用性限制，
     不是纪律违反**，且它主动申报了。
   - code-simplifier 在 commit 前跑过，结论为无需修改（`md5` 未变）。
3. Dev 的三处设计判断我认为正确：① 信封校验刻意留在 fn 内部（L6 实证：移出去则
   200+`code:0` 的业务错误会被当成功 body 缓存）；② `TestLixingerReturnsIndependentBytes`
   跑 3 次而非 2 次（能捕获「未命中时复制、命中时直接返回缓存」这种实现）；
   ③ a→b→a 重放序列（同时排除「key 少参数」与「压根没缓存」两种缺陷，只发两次的写法
   对后者是假绿）。

## 八、复现命令

```bash
git worktree add --detach ../wt-v010 b0bc78a
cd ../wt-v010
GOTOOLCHAIN=local go test ./internal/collector/lixinger/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/collector/lixinger/ -count=1 -coverprofile=/tmp/c.out && \
  GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | tail -1          # 92.2%

# B4 最强形态：既有测试文件零改动
git show --name-only --format='' b0bc78a | grep '_test.go'            # 只有 gate_test.go

# 验收标准 6 的结构性证明
grep -rn --include='*.go' '\.Do(req' internal/collector/lixinger/ | grep -v _test   # 仅 1 处
grep -rn --include='*.go' 'requestHTTP(' internal/collector/lixinger/               # 仅 fn 内 1 处调用

# 我补的那一格（Dev 因 scope 约束未做）：改 policy 内置表
#   给 lixinger.* 加 MinInterval → NotThrottled 红 :181 + :219
#   给 lixinger.* 加 Quota       → NotThrottled 红 :184

# TestMain 必要性：换成内置表 → 18 个既有测试转红
cd <主仓库> && git worktree remove ../wt-v010
```

---

# 附录：Leader 追加的核查点（交付后补做）

判定**不变**（`verified`）。

## B1. 复核 Dev 的 L2「预期不符」诊断 —— **2×2 闭环完全复现其结论**

Dev 报告 L2（缓存 key 去掉 payload）时预期 `NotThrottled` 保持绿、实际转红，诊断为
**测试间隐藏耦合**：节流探针原靠「逐次换 payload」避开缓存，**暗中依赖 functional[1]**。

我在**同一 worktree、同一会话**内跑了完整 2×2（两个探针形态 × 有无 L2 变异）：

| | 无 L2（正确实现） | 注入 L2 |
|---|---|---|
| **旧探针**（逐次换 payload） | 绿 | **红 `:223`** ❌ 假信号 |
| **新探针**（TTL 归零） | 绿 | **绿** ✅ 解耦成功 |

**左下角那一格是关键**，它证明的不是「新探针更好」，而是「旧探针在 L2 下会给出误导性结论」。

而且红在 **`:223`**——那是**对照组**的断言
（`throttled < throttleProbeInterval` → 「观测手段看不见节流，本测试不构成检验」）。
即：L2 下旧探针报出的是**「本测试不构成检验」**，而不是 L2 真正破坏的那件事（缓存 key）。
**这正是陷阱 10 描述的失效形态：缺陷把自己伪装成测试构造失败。**

Dev 的总结经此复核成立，值得原样保留：

> 若不写预期表，这一处会被记成「L2 让两条测试转红，覆盖更充分」而放过。

**预期表不只防「报出不存在的缺陷」，还防「误以为覆盖更充分」。** 后者更隐蔽——
多一条红看起来永远像好事。

改后探针的解耦手段（`p.TTL = 0` 而非换 payload）也符合陷阱 9：对照组必须**真的进入**
被测路径；TTL 归零保证每次调用都走到执行链的节流那一步。

## B2. 陷阱 14（200-but-error）—— L6 独立复现确认

首轮已独立注入 **L6**（把 `parseEnvelope` 从被缓存的 fn 内部移到 `policy.Fetch` 外面），
结果：

```
失败测试=[TestLixingerErrorIsNotCached,]      断言行 gate_test.go:278
```

**只有这一条转红**，与 Dev 自报一致。确认 lixinger 的 200-but-error（HTTP 200 + `code:0`）
缓存陷阱**已被钉住**——挪出去会让一次瞬时故障被缓存成整个 TTL 期间（5 分钟）的持续故障，
且期间不再发请求。

## B3. 陷阱 13（既有用例串味）—— 首轮已量化，此处给出跨包对照

首轮实测：把 `TestMain` 的零策略闸门换成内置表（TTL 5m）→ **18 个既有测试转红**。

交付后补测 twelvedata 同一项 → **2 个转红**。

| 包 | TTL 闸门下转红的既有测试 | 耦合成因 |
|---|---|---|
| **lixinger** | **18 条** | 10+ 个错误路径用例复用 endpoint `"x"` + 空 payload = 同一缓存槽 |
| twelvedata | 2 条 | 6 次调用中 5 次是 `FetchHistory("NVDA")` + 同日日期 |

⇒ 同一类耦合，**lixinger 严重得多**，与其既有测试规模（52 个）相称。两包挡法相同
（包级 `TestMain` 装零策略闸门），**实测均有效**。

## B4. boundary[0] 分句① —— 首轮已补齐 Dev 声明的那一格

Leader 建议「在隔离 worktree 里补」，**首轮报告 §3.1 已完成**：

- **L7**（policy 内置表给 `lixinger.*` 加 `MinInterval: 300ms`）→ 红 `:181`（策略表侧等值断言）
  **+ `:219`**（请求路径侧墙钟耗时断言）
- **L9**（加 `Quota{5, 24h, Asia/Shanghai}`）→ 红 `:184`

⇒ boundary[0] 的**两个分句各自独立有实证**，不再只有「等值谓词」这一肉眼判据。
