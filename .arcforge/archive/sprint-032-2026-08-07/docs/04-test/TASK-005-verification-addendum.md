# TASK-005 验证报告 · 附录 —— 对 dev 交付报告五条声称的独立复核

- **验证者**：test-agent-18
- **触发**：Leader 在 TASK-005 判定落盘（verified）后转来 dev-agent-40 的交付报告，请求复核其中五条。
- **对判定的影响**：**无。原判定 PASS（verified）不变**——五条复核**全部证实** dev 的声称，
  另得一条措辞层面的精化（第 1.2 节）与一条比 dev 自己给的理由更强的正当性证据（第 3 节）。
- **基线**：`feat/macro-bitemporal` @ `f2205ac1b462436ed322e11a5ed4cd71a25ab4e1`，
  隔离 worktree `../wt-verify-T005b`（已拆除），包级基线 **120 PASS**。

> 标注约定同主报告：**【实测】**= 本次亲自运行命令并观察输出；**【推断/记录采信】**= 未能亲自复现的历史事件。

---

## 一、`error_handling[1]`：`errors.Unwrap` 前提断言（Leader ①）

### 1.1 前提漂移模拟：那条断言**真的会红**【实测】

Leader 问的是「(a) 那个 `Unwrap` 前提断言是否真的能挡住前提变化」。
这不能靠读代码回答——必须让前提**真的不成立**，看断言是否报警。

**手法**：把 `badCol` 的表名换成一张不存在的表，这样驱动返回的原始错误变成
`no such table: <表名>`——**驱动信息里就含了表名**，正是「前提不再成立」的形态。
（两处硬编码字符串同步替换，避免测的是别的东西。）

```
--- FAIL: TestLookupWrapsSQLError/列不存在——表名只可能来自包装
    Error:    "SQL logic error: no such table: hestia_obs_gone (1)" should not contain "hestia_obs_gone"
    Messages: 前提：驱动的原始信息不含表名——前提不成立则下一条断言什么也证明不了
```

⇒ **前提断言是真守护，不是装饰。** 若将来 sqlite 驱动改了措辞、开始在
「no such column」里带上表名，这条会立刻转红，迫使维护者重新设计该子测试，
而不是让它像原「表不存在」子测试那样**静默退化成空转**。

### 1.2 这条断言到底买到了什么——一处措辞精化【实测】

我另做了一个隔离实验（P2）：**删掉整个 `Unwrap` 前提块**，再施加 E1-a（从包装里删掉表名）。

**结果：`列不存在` 子测试仍然 KILLED。**

⇒ 精化：**杀掉 MT4/E1-a 的是后一条 `assert.Contains(err.Error(), 表名)`，前一条不参与。**
`Unwrap` 块买到的是**另一样东西**——「后一条断言在未来仍然有意义」这个前提的看门狗（1.1 已实证）。

dev 的原话「两条合起来才构成守护」在**逻辑证明**的意义上是对的：要证成
「表名只可能来自包装」这个命题，确实需要「驱动不给」+「最终有」两条。
我这里区分的是**证明**与**变异守护**两个层面——不是纠正，是把它拆细：

| | 杀 E1-a（包装删表名） | 挡前提漂移（驱动开始带表名） |
|---|---|---|
| `require.NotContains(inner, 表名)` | 不参与 | **唯一守护** |
| `assert.Contains(err, 表名)` | **唯一守护** | 不参与 |

两条各守一件事，缺任一条都会留下一个无人看守的失效模式。**设计成立。**

### 1.3 「原『表不存在』子测试仍然绿」的对照——**确认**【实测】

这条我在主报告已实测（E1-a 的红名单），此处重述结论：
E1-a 下红的只有 `TestLookupWrapsSQLError` 与其 `列不存在——表名只可能来自包装` 子测试，
**`表不存在` 子测试保持绿**。⇒ dev 的诊断准确：需求文档样例的那条**从来就没守护过这条判据**，
因为 sqlite 的原始错误 `no such table: no_such_table` 本身就带表名，表名是驱动给的。

---

## 二、`functional[0]`「WHERE 用上业务键每一列」（Leader ②）

**漏列变异**（WHERE 只用业务键首列）全套跑完：**PASS=117（基线 120），红 3 条**【实测】

```
TestLookupUsesEveryKeyColumn
TestLookupUsesEveryKeyColumn/crisis
TestLookupUsesEveryKeyColumn/hestia
```

**`TestLookupHitAndMiss` 保持绿** —— 它里面就有需求文档样例的那段「另一个业务键不受影响」，
用的是 `("2026-05","monthly")`，**两列都与库中数据不同**，所以 WHERE 只用首列也照样查不到。

⇒ dev 的判断实测成立：**若照抄样例，这条 criteria 无人守护**。
补的 `TestLookupUsesEveryKeyColumn` 复用 TASK-003 探针里共享首列的 `probe[0]/probe[1]`
（`probe[0]` 的 revision 更大，漏列时会被算进 `MAX`），是这条的**唯一**守护。

**与我在 TASK-003 发现的 hestia 空断言确为同一族**，但失效机理不同，值得区分：

| | TASK-003 的 `TestFailureMessagesNameTheShape`（hestia） | 本条的样例段 |
|---|---|---|
| 失效原因 | 断言被**命名巧合**满足（`"hestia"` 是表名 `"hestia_observations"` 的子串） | 断言被**测试数据的过度差异**满足（两列都不同，少用一列也查不到） |
| 共同点 | 断言在场、整体也 KILLED，**但那一条本身对目标变异零守护** | 同左 |

两者都符合 Leader 转进 TASK-006 的那条判据——
**「如果被测代码完全不做 X，这条断言还会通过吗」**：两处的答案都是「会」。

---

## 三、注入防护的两个设计判断（Leader ③）

### 3.1 反向证明是**承重的**，不是装饰【实测】

Leader 的问题：只验「注入不命中」的话，一个把所有查询都改成返回空的实现也满足。

我直接造了这个实现（命中时也返回 `Exists:false`），跑注入测试：

```
红：TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}/含单引号的合法取值仍能被正确命中
绿：TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}/注入载荷不得命中
```

⇒ **「一律查不到」的实现确实能让「注入载荷不得命中」全绿**，
**唯一抓住它的就是 `O'Brien` 那条反向断言**——三套形状全部由它转红。
Leader 的判断完全正确，dev 这条反向设计是承重构件。

### 3.2 跑 `allShapes` 的加强**成立**，且我拿到了比 dev 自己给的理由更强的证据【实测】

dev 给的理由是「单列键没有尾随 ` AND ...` 可被 `--` 注释掉」。这个说法方向对，
但它**没有给出「存在一个只有单列键才抓得住的变异」**——而那才是加强的正当性所在。

我造了一个：**只把【最后一列】的取值拼进 SQL**（其余列仍走占位符）。
测试载荷放在**首列**，于是：

- **hestia / crisis（两列键）**：首列（载荷所在）仍走占位符，末列是良性的 `"h1"` 被无害拼接 ⇒ **打不中**
- **single-key（单列键）**：首列即末列 ⇒ 载荷被拼进 SQL ⇒ **打中**

结果 **PASS=116（基线 120），红 4 条，全部在 `single-key` 分支**：

```
TestLookupRejectsInjectionInKeyValues
TestLookupRejectsInjectionInKeyValues/single-key
TestLookupRejectsInjectionInKeyValues/single-key/注入载荷不得命中
TestLookupRejectsInjectionInKeyValues/single-key/含单引号的合法取值仍能被正确命中
```

⇒ **存在一个真实的注入形态变异，只有 `singleKeyShape` 抓得住，`bothShapes` 完全打不中。**
若这条测试按 C8 裁定只跑 `bothShapes`，这个变异会**静默存活**。
**加强成立，且有独立价值**——不是保险起见，是覆盖了一个 `bothShapes` 结构上够不到的区域。

（说明：我的证据机理与 dev 陈述的机理不完全相同——它说的是「载荷与 WHERE 其余部分的相互作用」，
我打的是「载荷所在列与被拼接列的位置关系」。两者都指向同一结论：**单列键提供了独立覆盖**。
结论一致，证据是我这一侧独立取得的。）

---

## 四、并发导致的假「0 红」场景（Leader ④）

**这一条我只能【记录采信】，不能【实测】**——它是一次**已经过去的瞬时状态**
（dev-agent-39 当时正在同包写 `query_test.go`，包内一度不可编译），
现在 `query.go` / `query_test.go` 已稳定落地，那个窗口无法重建。

我能实测的是它的**同型机制**：我自己的 harness 内建同一条第三自证（`--- PASS` 计数 vs 基线），
本轮全部运行的 PASS 计数均落在 116/117/120 等**可解释的值**上，无一次出现 `PASS=0`。

值得单独记下这个场景**为什么特别险**（我认同 dev 的判断）：
MT4 当时**确实也存活**（真因是测试不足）。所以那一刻有**两个独立原因**同时能产生「0 红」：

1. 测试根本没跑（编译失败）——**假**结论
2. 测试跑了但打不中（守护不足）——**真**结论

**只看 `--- FAIL` 计数，这两者产出完全相同的观测。** 第三条自证（PASS 计数）是唯一能
把它们分开的信号：情形 1 下 `PASS+FAIL==0`，情形 2 下 `PASS==基线`。
⇒ 这条纪律不是冗余校验，它**分辨的是两个会得出相反行动的结论**（前者「重跑」，后者「补测试」）。

---

## 五、封装重查与 `spec.go` 完整性（Leader ⑤）

主报告第 3.3 节已实测，此处重述关键证据：

```
$ for c in 224c960 96641ec d928dd8 89bc09c f2205ac; do git rev-parse $c:.../spec.go; done
  → 五个 commit 全部为 7aee7f42be28649c3fd9fa7a971f070d65240fd7
$ git log --oneline 224c960..f2205ac -- internal/macro/bitemporal/spec.go  → 无输出
$ md5 -q internal/macro/bitemporal/spec.go  → 59934cd2238daeeedb3ab9c8494cc437
$ grep -nE '^func \(s Spec\)' *.go → 仍只有 zero() / checkKey() / correlate()
```

**`spec.go` 的 git blob hash 在五个 commit 中完全相同，从未被任何 commit 触碰。**
`Spec{` 全包 11 处，我独立 grep 复现 dev 的分类：spec.go 内 6 处零值错误返回 + 1 处
`NewSpec` 自身的带字段构造；`lookup_test.go` / `query_test.go` ×2 / `spec_test.go` 共 4 处
**全为零值 `Spec{}`**，且正是用来测零值守卫本身。**无第二处带字段构造点，004/005 均未加 getter。**

⇒ dev 的结论「TASK-001 的守卫在包写完之后仍然成立」**成立**。
这也是我在 TASK-001 报告第六节留下、TASK-003 验收时复查过一轮的那条复查项的**最终关闭**。

---

## 六、结论与仍然开放的一项

**五条复核全部证实 dev 的声称**，TASK-005 判定维持 **PASS（verified）**。

新增的实测证据（本附录独有）：
1. `Unwrap` 前提断言在前提真的漂移时**会红**（1.1）；
2. 该断言与后一条断言**各守一个不同的失效模式**，缺一不可（1.2）；
3. 「一律查不到」的实现**只被 `O'Brien` 反向断言抓住**（3.1）；
4. **存在只有 `singleKeyShape` 抓得住的注入形态变异**，`allShapes` 加强有独立价值（3.2）；
5. 漏列变异**只红** `TestLookupUsesEveryKeyColumn`，样例段确实抓不住（第二节）。

**仍然开放、需要 Leader 处置的一项**（主报告 F1，本次复核未改变其状态）：

> 注入载荷缺**双引号系**。`fmt.Sprintf("%s = %q", ...)` 形态的拼接可存活于全套 120 条，
> 而我已实证载荷 `x" OR 1=1 --` 在该形态下**能实际命中**（返回全表 MAX）。
> 根源在 DoD 的示例载荷本身是单引号系。建议加一条双引号载荷（1 行，加后当前即绿），
> 并把「载荷须同时覆盖单双引号系」补进 TASK-006 终验。

**这一项与 Leader 转进 TASK-006 的那条判据是同一族**：
问「如果被测代码完全不做参数化，这条断言还会通过吗」——
对单引号载荷答案是「不会」（守住了），**对双引号载荷答案是「会」**（没守住）。
⇒ 判据本身是对的，只是要**逐载荷形态**地问，而不是逐断言地问。

## 收尾
复核 worktree `../wt-verify-T005b` 已从主仓库以绝对路径 `git worktree remove` 拆除，无残留；
`lookup.go` / `lookup_test.go` / `spec.go` 三者 md5 还原后与基线一致，主工作区无污染。
