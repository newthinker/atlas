# TASK-008 验证报告 —— Parse 输出值的常驻守护

- **验证者**: test-agent-22 / 承接时 `assignment_epoch` = 1
- **验证对象**: `internal/hestia/parse_test.go` @ `9e773118ba6f9ca95a3e113054f9164e26b40bca`
- **实施者**: dev-agent-45
- **判定**: **PASS → verified**

> 本任务补的正是我在准备 T7 判据时核出的缺口：`Parse` 的 Values **只有键数断言、无逐值比对**。

---

## 一、基线与转前清单

| 项 | 实测 |
|---|---|
| `go test ./internal/hestia/ -v -count=1` | **358 PASS / 0 FAIL / exit 0** |
| `go test -cover` | **89.5%** |
| `go vet` | exit 0 |
| 改动范围 | **1 文件 +28/−2**，与声明 `writes`（`parse_test.go`）**逐条相同，无越界** |

> PASS 数与 T6 相同（358）是预期的：T8 只强化既有断言，**未新增测试函数**。

**转前清单四条全过**：`has("discovery")`=true；discovery sha256 `cce7d1ad…de1ea` 与基线**逐字相同**；
改动范围与 `writes` 一致；转后 validator 见第五节。`verify_baseline.head` == 当前 HEAD == `9e77311` ⇒ **未漂移**。

---

## 二、五条 DoD

| # | 完成标准 | 判定 | 依据 |
|---|---|---|---|
| functional[0] | 走 `Parse` 入口调 `assertMatchesGolden`，复用既有 helper 不另写 | **PASS** | `parse_test.go:207` 调用；`extract_test.go:311` 的 helper 未被复制 |
| functional[1] | 两条断言并存，不得以「重复」为由删掉 T5 那条 | **PASS**（**但 DoD 的理由不成立，见 G1**） | U2 对照实验 |
| boundary[0] | `require.NotEmpty` 非空转自证 | **PASS**（**但注释的机制不成立，见 G2**） | 空 Values 实测 |
| error_handling[0] | 差异必须能定位到具体字段名 | **PASS** | 扰动实跑，见 2.2 |
| non_functional[0] | 不引入新的包级名字 | **PASS** | `git diff` 无 `^+(func\|var\|const\|type) ` |

### 2.1 关键对照实验：证明「两条断言不可互替」

DoD 只写了约束（「必须并存」），没写怎么证明它成立。我做了双向变异：

| 变异 | T5 的 `TestExtractFieldsOnV2Sample` | T8 的 `TestParseRealSamples` |
|---|---|---|
| **U1b** 上游 `stripHTML` 输出中 `4.81`→`4.82`（值变、键数不变） | **FAIL（也接住了）** | **FAIL** |
| **U2** `detectExtractor` 恒不返回 v2 | **PASS（看不见）** | **FAIL（接住了）** |

⇒ **非重复性真实存在，DoD 的结论成立**——U2 证明 T8 能抓住 T5 结构上抓不到的东西。
但**机制不是 DoD 说的那个**，见 G1。

### 2.2 `error_handling[0]`：扰动实跑而非看代码

在隔离 worktree 上把 `golden2025[FieldM2]` 由 `340.29` 改成 `340.30`：

```
--- FAIL: TestParseRealSamples/pboc-2025-12-annual.html
    Error:    Max difference between 340.3 and 340.29 allowed is 1e-06, but difference was 0.009999999999990905
    Messages: 字段 m2
```

**字段名与两个数值都在**。跑完还原，`golden_test.go` sha256
`247b68a9ace70e3be4cbf2bc4c0d09c4952496a427a1b612cb9b6c3cefa27f4e` **前后逐字节相同**。

---

## 三、G1（中）—— DoD `functional[1]` 的理由不成立，**而源头是我**

DoD 写：

> T5 的 `assertMatchesGolden` 喂的是 `extractFields` 的**固定输入**（它自己构造的 sections），
> 本条喂的是 `Parse` 从原始 HTML 走完四层的结果 ⇒ 本条能抓住 T5 那条抓不到的：**上游任一层的变化导致值错**。

**实测不成立。** T5 的测试原文是：

```go
func TestExtractFieldsOnV2Sample(t *testing.T) {
	secs := splitSections(stripHTML(readSample(t, "pboc-2025-12-annual.html")))
	got, err := extractFields(secs, extractorV2)
```

它调的正是 `splitSections(stripHTML(...))`——**同一条上游管线**。所以上游变化 T5 **同样看得见**（U1b 实测两条都红）。

**真正的差异只有一处**：T5 **硬编码** `extractorV2`/`extractorV1`，而 `Parse` 从 `detectExtractor` 取。
外加 T8 顺带覆盖了 `metaContent` / `parseTitle` / monthly 守卫这一段。U2 正是打在这个差异上。

### 传播链——这条错误经过三个人的手

1. **我**在向 Leader 报缺口的消息里写：「若将来 `stripHTML` 之类上游发生微变……**T5 的测试喂的是它自己构造的输入不受影响**」
2. **Leader** 把它写进 TASK-008 的 `functional[1]`
3. **dev-45** 抄进 `parse_test.go` 的注释（「T5 → 喂 `extractFields`，输入是它自己构造的固定 sections」）

**同一个未经实测的机制断言，被复制了三次，没有人在中途验证它。** 这正是本 Sprint 反复出现的形态
（F1 的正则机制、交替顺序），区别只在于**这次的源头是我**——而我恰恰是那个一路在说
「带机制断言的句子要跑一次最小复现」的人。

**结论不受影响**：两条断言确实不可互替（U2 为证），只是理由要换成正确的那个。
**建议**：修正 `parse_test.go` 的注释（注释非判定依据，可直接改写）；DoD 的更正按
「保留原文 + 追加」处理（判定依据有审计负担）。

---

## 四、G2（低）—— `require.NotEmpty` 结构上不可达

`parse_test.go` 的断言顺序：

```
198:  require.Len(t, obs.Values, tc.values)        // 54 / 27
202:  require.NotEmpty(t, obs.Values, "抽出 0 个字段，本比对毫无意义")
207:  assertMatchesGolden(t, obs.Values, tc.golden)
```

`require.Len` 是 `require`（失败即 `FailNow`），空 map 必然先在它这里失败。**实测**（让 `Parse` 返回空 `Values`）：

```
parse_test.go:198  Error: "map[]" should have 54 item(s), but has 0
```

**第 202 行永远执行不到。** 而它的注释写着「让『一个字段都没抽到』**以这条的措辞**失败，
而不是刷出 54 条『字段 X 没被抽到』」——**这个机制描述不成立**，空的情形以 `require.Len` 的措辞失败。

**不是缺陷**：DoD 明说 `require.Len`「可保留也可由本断言取代」，若将来删掉 `Len`，这条就成为唯一守卫。
所以它是对未来删改的防御，只是注释把它的当前作用说错了。**建议改注释**。

---

## 五、G3（信息）—— dev-45 的 A/B 证明的不是非重复性

它的 commit message 记了一次 A/B：注入「余额归一 `toWanYi→toYi`」，改动前 `TestParseRealSamples` PASS、改动后 FAIL。

**那证明的是「T8 的断言优于旧的 Len-only」**——这是本任务的要点，成立。
但它**不能证明「T8 与 T5 不重复」**：`toWanYi→toYi` 这个变异 **T5 也会红**
（正是我验 T5 时的变异 **P12**，当时打红了两份 golden 比对）。

⇒ 要证明非重复性，必须用**只在 `Parse` 路径上可见**的变异，也就是 U2 那一类。记录备考。

---

## 六、validator

- 在途：**1 条告警 / EXIT=0**（`TASK-008` 的 `scope-writes-outside-packages`）
- 转 `verified` 之后：见发给 Leader 的消息

---

## 七、结论

五条 DoD 全部达成：`assertMatchesGolden` 经 `Parse` 入口接上、helper 复用未另写、
非空转自证已加、差异可定位到字段名（扰动实跑为证）、零新增包级标识符。改动最小（+28/−2），
范围无越界，判定对象未漂移。

DoD 要求的「两条断言必须并存」我用双向变异**证明了它确实成立**——而不是当作约定接受。
过程中查出 DoD 给的**理由**不成立，且**源头是我自己写给 Leader 的一句未经实测的话**，
经三次复制进入代码注释。结论不变，理由已更正。

**判定：PASS → `verified`**
