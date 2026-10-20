# TASK-006 验证报告（AI 下单建议）

- 验证者：test-pw-b；`assignment_epoch`=1
- **被测树**：`feature/pricewatch` @ `04bed52d45612d45386e794efdb60e47536eedd2`（= `verify_baseline.head`；出裁决前主仓库 HEAD 未变），独立 worktree `../wt-verify-TASK-006-b`（detached），主工作区未改动
- 文件 sha256：`advisor.go` `a0072d4680f8a912e55a4e8b074126f94861995fee60676cf8255815f974f96f`、`advisor_test.go` `10f5d9926bc0ad5ae20555d1f49d858bb20a9139f04f570bbbabc875d3da658f`（= dev 提交 `2be75a9e2325ab487624b8f012f7cc5bcd1e9dc9`）
- discovery sha256 `6f9d5b5a9930b84e3b2b438db8e2ce7b578757919f770ca7eed07049cd86026a` = 基线，无漂移
- 范围：`2be75a9 feat(TASK-006)` 只改 `writes` 声明的两个文件
- 工具链：go1.24.4，`GOTOOLCHAIN=local`

## 结论：VERIFIED

## 1. 门禁重跑

| 命令 | 结果 |
|---|---|
| `go test -race -count=1 -coverprofile ./internal/pricewatch/` | `ok`，包覆盖率 **94.0%** |
| `go vet ./internal/pricewatch/` | 无输出，rc=0 |
| `gofmt -l advisor.go advisor_test.go` | 无输出 |
| advisor.go 函数覆盖 | 7 个函数全部 100% |
| `go test -race -count=20 -run 'TestAdvise\|TestSides'` | `ok`（超时类用例无 flaky） |

## 2. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 守卫证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | 解析 plan 给出的 JSON 得到 1 条且各字段正确；JSONMode / MaxTokens 1500 / Temperature 0.3；消息含现价与每条规则 id | `TestAdvise_ParsesJSON` | A21（JSONMode=false）、A22（1000）、A23（0.5）、A14（删现价）、A15（规则行去 id）KILLED。68120 不会出现在 K 线（取值 40001–50060）里，所以现价断言不会被 K 线数据顺带满足 | PASS |
| functional[1] | 提示词含「双币赢」、每个方向、6 个键名；K 线行数 = len(M1)+len(M5)，每行恰好 5 个字段 | `TestAdvise_Prompt`、`TestAdvise_PromptOnlyRequestedSides` | A01（改「双币赢」）、A43（删整行）、A09–A13（6 个键名逐个改名）、A16/A17（缺 M1/M5）、A18（4 字段）、A19（千分位破坏字段）、A20（高低互换）、A08（三处方向全删）、A03（只跌时出现「高卖」）KILLED。**方向这一项的守卫是冗余的，见 §4-3** | PASS |
| functional[2] | Sides 三种组合 | `TestSides`（另有「同向多条」） | A39（顺序反）、A40（涨跌判反）、A41b（恒含低买）KILLED | PASS |
| boundary[0] | ```` ```json ```` 围栏 / 前置说明可解析；FinishReason=max_tokens 返回 error | `TestAdvise_LenientExtract`（5 例）、`TestAdvise_Invalid` | A26（起点用 LastIndex）、A27（终点用 Index）、A28（不提取直接解析整段）、A24（不判 max_tokens）、A25（不判 length）KILLED | PASS |
| error_handling[0] | 非 JSON / advice 空 / verdict 非法 / side 不在 Sides / LLM error 都返回 error | `TestAdvise_Invalid`（13 例） | A29（允许空）、A30（不校验 side）、A31（不校验 verdict）、A38（吞掉 LLM error）KILLED，每个都落在对应的子用例上 | PASS |
| error_handling[1] | Timeout=50ms 时 1 秒内返回 error；LLM==nil 返回 error 不 panic | `TestAdvise_Timeout`、`TestAdvise_TimeoutProviderIgnoresCtx`、`TestAdvise_NilLLM` | A36（不加 Timeout）：TestAdvise_Timeout 一直阻塞到 `-timeout` panic；A35（去 nil 检查）：nil 解引用 panic；A37（直接调用、不用 select）：被 TimeoutProviderIgnoresCtx 抓到 | PASS |
| non_functional[0] | race/vet/gofmt/包覆盖率 ≥80% | §1 | — | PASS |

## 3. 独立变异（44 个，`go test -overlay`，隔离 worktree，`-timeout 60s`）

- 每个变异先过 `go vet -overlay`。A41 首版编译失败（`drop` 未使用），改写为 A41b 后 KILLED。源文件 sha256 跑前跑后一致；worktree `git status --porcelain` 为 0 行。首轮计数自洽：36+7+1=44。
- **最终 37 KILLED（其中 2 个是 panic/超时形态：A35、A36）/ 7 SURVIVED**。按「有具名 FAIL」的严格口径算是 35 个。
- dev 自报：严格口径 21/25，含 panic/超时 24/25，唯一存活是删方向行。我的集合是它的超集，存活集合多了 A04–A07、A42、A44，分析见下。
- QA-4：各处 Contains 断言都做了「删/改文案」变异（A01、A43、A09–A13、A14、A15），全部被抓到。

### 存活变异

| 变异 | 判断 |
|---|---|
| A02 删「请分析的方向」行 | DoD 层面等价（见 §4-3） |
| A04 两个方向时 JSON 示例的 side 写死「低买」 | DoD 层面等价：「高卖」仍出现在「side 只能取」里。语义上会弱化对模型的提示（见 §4-3） |
| A05 「side 只能取」里删掉方向 | DoD 层面等价 |
| A06 删方向行，同时删「只能取」里的方向 | DoD 层面等价：只剩 JSON 示例这一处 |
| A07 删方向行，同时 JSON 示例写死低买 | DoD 层面等价：只剩「只能取」这一处 |
| A42 两条 advice 时只返回第一条 | **非 DoD**：DoD 只要求解析出 1 条的场景。`TestAdvise_Prompt` 的 fake 返回 2 条，但没有断言条数（见 §4-4） |
| A44 `end < start` 改 `end <= start` | 等价变异：`{` 和 `}` 是不同字符，不可能在同一位置，start≥0 时 end==start 不可达 |

## 4. 派验点名的三处评估，以及其他发现（都不构成 DoD 缺陷）

1. **QA-6：term 超过 20 个字视为 AI 失败**
   - 边界用例齐全：21 字报错，恰好 20 字接受；A33（`>` 改 `>=`）、A34（按字节计）被 `TestAdvise_TermAtLimit` 抓到，A32 被「term 超长」抓到。
   - 这项选择**补上了我在 TASK-005 报告里提的残余路径**（Term 无界，5000 字时消息 5250 码元）。现在 side 和 verdict 是枚举，term ≤20 rune（≤40 UTF-16 码元），超长的只剩 reasoning，而 reasoning 由 FormatAlert 截断。AD-24 的上限在 TASK-005 与 TASK-006 组合之后成立。
   - 代价：一个外观字段超长，就会把整段 AI 结论降级成「AI 分析不可用」。提示词里已经写了「term 不超过 20 个字」，概率很低。更温和的做法是只清空 Term（FormatAlert 对空 Term 会省略「期限」项）、保留 verdict 和 reasoning。这属于取舍，不是缺陷。合理。
2. **Chat 放在 goroutine + select 里**
   - 我读了三家 provider 的实现：claude 是 `p.client.Messages.New(ctx, …)`，openai 是 `CreateChatCompletion(ctx, …)`，ollama 是 `http.NewRequestWithContext(ctx, …)`（并且 client 自带 5 分钟 Timeout）。三家都会把 ctx 传给 SDK 或 HTTP 层，所以 ctx 取消后，goroutine 会在 HTTP 层中断后很快返回。
   - `ch` 带 1 个缓冲，迟到的结果写进去不会阻塞，goroutine 一定能退出。
   - 泄漏规模：每轮最多 1 次 AI 调用，所以同时最多残留 1 个 goroutine。最坏情况下 provider 不理会 ctx，它会存活到 provider 自己返回，ollama 的上限是 5 分钟。也就是有界、单个、不会累积（除非连续多轮都遇到完全不理会 ctx 的 provider，现有三家都不是）。
   - 有没有更好的写法：现有 provider 都理会 ctx，直接调用其实就够了。goroutine 是防御 fake 或第三方 provider 的，成本可以忽略。没有更优的写法，维持现状合理。
3. **「删『请分析的方向』行，DoD 层面等价」，我独立判断后同意**，但要补充：方向在提示词里有**三处冗余载体**，分别是方向行、JSON 示例的 side、「side 只能取」。只有三处全删（A08）才会被抓到。单删或删任意两处（A02、A04–A07）都存活，而这些情况下「提示词含每个方向」这一 DoD 性质确实仍然成立，所以是等价变异。这正是 QA-4 的那个形态：断言被其他路径顺带满足。只不过这里每条路径本身都合法地满足了 DoD，所以不算缺陷。如果以后有人以为别处还有、把其中两处改掉，测试不会提醒。A04（两个方向时 JSON 示例只写「低买」）在语义上会诱导模型只回一个方向，是最可能出问题的一处。
4. **A42：两个方向时返回条数没有断言**。如果实现只返回第一条，测试全绿，TASK-007 会在都有涨跌时丢掉「高卖」建议。DoD 没有要求，而 discovery 也写明「不保证每个方向都有一条」。建议 TASK-007 或后续补一个「2 条输入 → 2 条输出」的断言。
5. 宽松提取「第一个 `{` 到最后一个 `}`」：如果 JSON 后面的说明文字里也带 `}`（比如「注：{x}」），就会解析失败并降级为 AI 不可用。这是 AD-23 规则本身的已知局限，结果是安全的降级，不会出错。

## 5. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-006-b 04bed52d45612d45386e794efdb60e47536eedd2
cd ../wt-verify-TASK-006-b
GOTOOLCHAIN=local go test -race -count=1 -coverprofile=c.out ./internal/pricewatch/ && go tool cover -func=c.out | grep advisor.go
GOTOOLCHAIN=local go vet ./internal/pricewatch/; gofmt -l internal/pricewatch/advisor.go internal/pricewatch/advisor_test.go
GOTOOLCHAIN=local go test -race -count=20 -run 'TestAdvise|TestSides' ./internal/pricewatch/
python3 <scratchpad>/test-pw-b-T006-mut/run.py "$PWD" <输出目录>    # 44 个变异；A41 用修正版 A41b
```
