# TASK-003 验证报告

- **验证者**：test-agent-12
- **裁决**：**PASS（verified）**
- **承接**：HEAD `c70afd7`（master），`assignment_epoch=1`，上游 TASK-002 = `a73110e`
- **日期**：2026-07-26

## 0. 方法声明（为什么这份报告的证据是独立的）

**未复用 dev 的 `mut29.sh`**，自写 harness `scratchpad/mut12.sh`，并加了 dev 设计里没有的一道守护：

> **`-run` 作用域自检**：施加变异后先统计该正则实际选中了几个测试，选中 0 个即 FATAL 退出。
> 理由：正则匹配不到任何用例时 `go test` 恒绿，会被误读成「变异存活」——而本任务的全部结论
> 都建立在「存活/杀死」的读数上。实测该守护在 pre 树上对尚不存在的
> `TestDominantClassUsesMaxSharesNotSum` 正确触发（exit 6），未被当成 SURVIVED。

**所有变异都施加在 scratchpad 的 `git archive` 副本上，全程不触碰用户工作区。**
每次施加前后记 sha256、`go vet` 确认可编译、`trap EXIT` 还原后逐字节比对。
收尾核实：`internal/` 始终 `git status` 干净；两份源文件的 sha256 与 git 对象、
实时工作区**三方逐字节相同**：

```
classeps.go       dcabbae04983c56b681cfd7b5d78aefa9e726aafd81aeb0190f176df4d2e360a
classeps_test.go  e6728f85b98cf8e685c562a124896cdf3f7a846a913ca4ddbe544416c1d6edfe
```

`classeps.go` 的 sha256 与 dev 在 discovery 中记录的值完全一致，构成对其 harness 的交叉印证。

## 1. Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | ① 注释在数据增长后仍正确 | review | `classeps.go:46-67`（见 §3） | **PASS** |
| functional[1] | ② 回看上限用例对取值有判别力 | test | `TestFetchClassEPSRespectsLookbackCap`；M-cap12/27/40 隔离各 **20/20 红** | **PASS** |
| functional[2] | ③ max/sum 选择有测试钉住 | test | `TestDominantClassUsesMaxSharesNotSum`；M-sum 隔离 **20/20 红** | **PASS** |
| boundary[0] | ③ fixture 与真实形态可区分 + discovery 说明 | test | 见 §4 反事实实验 | **PASS** |
| non_functional[0] | AD-27 变异纪律 | test | 见 §2，全部翻转 + 对照组 0/3 + sha256 还原 | **PASS** |
| non_functional[1] | 覆盖率 / -race / 全仓 / 生产零改动 | test | 见 §5 | **PASS** |

## 2. 变异结果（全部独立重跑，用例层面作用域）

### 2.1 改动前（`a73110e`）——本任务存在的理由，已复现

| 变异 | 作用域 | 选中用例数 | 结果 |
|---|---|---|---|
| M-cap12 (28→12) | `^TestFetchClassEPSRespectsLookbackCap$` | 1 | **0/5 红（完全存活）** |
| M-cap12 (28→12) | 整包 `.` | 113 | **5/5 红（杀死）** |
| M-sum (`max`→`sum`) | `^TestDominantClass` | 4 | **0/5 红（完全存活）** |
| M-sum (`max`→`sum`) | 整包 `.` | 113 | **0/5 红（完全存活）** |

dev 报的两个「改动前 0/5」**均复现属实**。

**额外发现（比 dev 报的更严重一档）**：M-sum 在改动前不只是隔离作用域存活，
**整包 113 个用例全跑也是 0/5 红**——它是一个**全包性空洞**，包里没有任何人杀得掉。
这与 M-cap12 的情形不同（后者整包有人兜底），说明 ③ 修补的洞比 ② 更彻底地无人看守。

### 2.2 「整包 KILLED 掩盖了什么」——已定位到具体用例

在 `a73110e` 上施加 M-cap12 后整包跑，`--- FAIL` **只有一条**：

```
--- FAIL: TestClassEPSAndSegmentCapsAreIndependent (0.00s)
```

`TestFetchClassEPSRespectsLookbackCap` **不在失败名单里**。这逐字证实了 dev 的论断：
TASK-002 报的「M12 KILLED 10/10」完全来自别处那句字面量断言，被审查的那个用例对取值零判别力。

### 2.3 改动后（`c70afd7`）

| 变异 | 作用域 | 选中用例数 | 结果 |
|---|---|---|---|
| 对照组（不施加） | `^TestFetchClassEPSRespectsLookbackCap$` | 1 | 0/3 红 |
| 对照组（不施加） | `^TestDominantClassUsesMaxSharesNotSum$` | 1 | 0/3 红 |
| M-cap12 (28→12) | `^TestFetchClassEPSRespectsLookbackCap$` | 1 | **20/20 红** |
| M-cap27 (28→27，相邻值) | 同上 | 1 | **20/20 红** |
| M-cap40 (28→40，≥nFilings) | 同上 | 1 | **20/20 红** |
| M-sum (`max`→`sum`) | `^TestDominantClassUsesMaxSharesNotSum$` | 1 | **20/20 红** |
| M-first (`return members[0]`) | 同上 | 1 | **20/20 红** |

对照组 0/3 排除「恒红」假象。dev 的表格**全部复现属实**，M-first 亦如其所述顺带被杀。

**关于「20/20 是结构性保证而非概率压制」的论证——确认成立。**
- M-cap：断言是 `ElementsMatch`（集合相等），改常量后元素个数**确定性地**不同（28 vs 12/27/40），无随机性可言。
- M-sum：`dominantClass` 遍历的是 `sort.Strings(members)` 排序后的切片、比较用严格 `>`，
  经读码确认 map 迭代序**不参与**结果（map 只用于聚合，选优在有序切片上进行）。
  故两者在用例层面 p = 1。20/20 是这一结构性事实的体现，而非靠重复次数压低偶然性。

## 3. ①：改写后的注释在 3 个月后是否仍站得住 —— 站得住

DoD 的要求是「把结构性事实与当日快照分开，并明确将来调大有效」。改写后三段式做到了：

1. **结构性事实**：`reportDate ≤ 2019-03-31` 是老式非 inline XBRL，`_htm.xml` 推导不成立 ⇒ 必 404；
   明写「真正的上限是这条时间线，不是下面那个数字」。
2. **当日快照**：标注 2026-07-26、序号 0–27 全 200、28 起连续 404，且明确这是「**在那一天**」的巧合。
3. **失效方向**：显式声明「这个巧合不会持续：将来调大是有效的」，并给出重估判据
   「按 2019-06-30 至今的季度报告份数重算，**不要沿用 28**」，附线性外推代价（每份 +1.9 MB / +1.6 s）。

**独立复算验证重估判据自洽**：按该判据数「2019-06-30 起至 2026-03-31 的季度数」
= **恰好 28**（2019 年 3 个 + 2020–2025 各 4 个共 24 个 + 2026Q1 1 个）。
判据在今天能复现出 28 这个观测值，说明它不是一句空话，而是对同一事实的**不依赖快照的表述**——
这正是原注释缺的东西。3 个月后 V 再申报一季，该判据给出 29，此时 cap=28 会截掉最旧一份，
「调大有效」成立，注释的预言方向正确。

原注释失败在把「28 = 当日全部可用」写成「调大永远无效」；改写后这个跃迁被拆掉了。**判定 PASS。**

**顺带改动核实**：`TestClassEPSAndSegmentCapsAreIndependent` 的改动经 diff 逐行确认
**只碰断言消息串**，`assert.Equal(t, 28, maxClassEPSFilings)` 的断言三元组
（函数、期望值、实际值）字节不变。理由成立——该消息原文是同一句过期断言的逐字复述，
只改注释不改它等于留了一份过期副本。

## 4. ③ boundary：fixture 自检是否真能防静默退化 —— 经反事实实验确认

DoD boundary[0] 要求 fixture 与真实形态可区分。这点 discovery 已说明且属实。
但我认为更该验的是 dev 自加的那两行 `require.Greater` 自检**是否真的 load-bearing**
（它不在 DoD 里，是 dev 主动加的防退化装置）。做了两组反事实实验：

**A) 退化 fixture（`nSmall` 12→4，使 sum 与 max 重新同答案）+ 保留自检**：

```
--- FAIL: TestDominantClassUsesMaxSharesNotSum
    Error:    "6e+08" is not greater than "9e+08"
    Messages: sum 口径下必须是 A 类胜出
```

立即失败，且错误信息直指病因。

**B) 同样退化 + 移除那两行自检**：
- 用例**静默变绿**（`ok`）；
- 且 M-sum 变异**重新存活 0/5**——判别力确实随 fixture 退化而丢失。

A 与 B 对照证明：这两行把一次**静默退化**转成了**响亮失败**，确实 load-bearing，
而非装饰性断言。这正是本任务在修的病（无判别力的测试照常变绿）被 dev 用在了自己的新测试上。**判定 PASS。**

## 5. 质量门禁

| 项 | 结果 |
|---|---|
| edgar 覆盖率（改后，5 次采样） | **94.7% × 5**，全部一致 |
| edgar 覆盖率（改前 `a73110e` 基线，5 次采样） | **94.7% × 5** ⇒ **无回退**，达门槛 |
| `go test -race ./internal/collector/edgar/` | ok 2.379s |
| 全仓 `go test ./...` | **exit=0，58 个包 ok，0 FAIL / 0 panic** |
| `go build ./...` | 通过 |
| `gofmt -l internal/collector/edgar/` | 空 |
| `go vet ./internal/collector/edgar/` | exit=0 |
| **生产行为零改动** | `git diff -U0 a73110e c70afd7 -- classeps.go` 滤掉 `+++/---`、`//` 注释行与空行后**输出为空**（grep exit 1）；22 行改动全在注释 |
| 提交纯净性 | 仅 2 个文件，无 `.arcforge/` / `.claude/` 混入 |

## 6. 观察项（不阻塞，不构成 reject 理由）

**注释里那条结构性事实被表述为普适，实际是分档的。**
改写后写的是「`reportDate ≤ 2019-03-31` 的 filing 是老式非 inline XBRL」，语气上普适。
但 SEC 的 inline XBRL 是**分档强制**：大型加速申报人自 2019-06-15 起的会计期间、
加速申报人 2020-06-15、其余 2021-06-15。`maxClassEPSFilings` 是**通用常量**
（任何 companyfacts 缺 EPS 的公司走这条路径都用它），对小型申报人真实的 404 边界要晚得多。

不作为缺陷的理由：
- 后果良性且**已被测试守护**——多出来的只是 404，而「连续 404 尾部逐份跳过、不中断、
  可用的照常产出」由 `TestFetchClassEPSSkipsPre2019NotFoundTail` 钉住，不会坏数据，只浪费请求；
- 注释已显式标注测量对象是 V(CIK 1403161)，对 V 而言该边界精确正确
  （V 财季止于 3/31、6/30、9/30、12/31，「期间止于 2019-06-15 之后」与「reportDate ≥ 2019-06-30」重合）；
- 该偏差在**跨公司**轴上，不在**时间**轴上，不会随时间推移而失效，
  因此不属于 DoD functional[0]（「数据增长后仍然正确」）的范围。

建议后续任务处理，不在本任务返工。

## 7. 结论

三项均达成，六条 done_criteria 全部 PASS，无未覆盖项。两条原本无判别力的位置
经独立复现确认「改前存活、改后杀死」，且 ③ 的洞在改前是全包性的。生产行为零改动已用
diff 独立证明。覆盖率无回退，全仓绿。

**裁决：verified。**
