# TASK-006 返工验证报告 —— `Parse` 的 PubDate 校验（QA WARNING-2）

- **验证者**: test-agent-22 / `assignment_epoch` = 1 / `rework_count` = **1**
- **验证对象**: `internal/hestia/parse.go` + `parse_test.go`，返工提交 `29adc9e65ba8c30008f20d59c088b51d43fdd0b8`
- **`verify_baseline.head`**: `29adc9e…`（当前 HEAD 已前移至 `defdc5e`，T3 返工合入）
- **判定**: **PASS → verified**

> 只覆盖本轮返工。首轮见 `TASK-006-verification.md`，判定不变。

---

## 一、转前清单（四条，全过）

| # | 检查 | 结果 |
|---|---|---|
| 1 | `has("discovery")` | **true** |
| 2 | discovery sha256 | `da6e61e4…97a7d`，与基线**逐字相同** |
| 3 | 改动范围 vs `writes` | `29adc9e` = **2 文件**（`parse.go` +30/−2、`parse_test.go` +89/−0），均在 `writes` 内，**无越界** |
| 4 | 转后 validator | 见第六节 |

**漂移核查**：HEAD 已从基线 `29adc9e` 前移至 `defdc5e`（T3 返工：`28408bd` 板块序号连续性 + `3058044` 删除 `section.has`），
两个提交只动 `sections.go` / `sections_test.go`：

```
git diff --stat 29adc9e..HEAD -- internal/hestia/parse.go internal/hestia/parse_test.go
  → （空）
```

⇒ **声明范围内无变更，判定对象未漂移。**

---

## 二、基线（自证）

| 项 | 我的实测 @ `29adc9e` | dev-45 自报 | 一致 |
|---|---|---|---|
| `go test ./internal/hestia/ -v -count=1` | **372 PASS / 0 FAIL / exit 0** | 372 / 0 | ✓ |
| coverage | **89.6%** | 89.6% | ✓ |
| `go vet` / `gofmt -l` | exit 0 / **0** | 绿 / 干净 | ✓ |

> `fix_items` 第四条写「全包保持 **358** PASS」——那是**下达返工时**的数字。
> 实际交付态是 372，因为 T5 的返工（+7）先行合并、T6 自身再 +7。
> **不是偏差**，是 fix_items 写于合并之前。我按实际交付态判。

---

## 三、修复本身：`if !ok` → 四态 `switch`

```go
switch {
case !ok:                                  // meta 不存在（选择器写错 / 页面结构变了）
case pubDate == "":                         // 存在但为空（站点没填）
case !publishedAtRE.MatchString(pubDate):   // 形态不合 YYYY-MM-DD
}
```

形态校验**复用 `types.go` 的 `publishedAtRE`**，与 `Store.Save` 同一条正则——各写一份迟早分叉，
而分叉的表现正是「`Parse` 放行、`Save` 拒绝」那条缝本身。

### 3.1 四种输入独立复现（不采信 QA 摘要）

在真实 2025 样本上**只改一个标签**：

| 输入 | `err` | 信息含 `PubDate` | `len(Values)` |
|---|---|---|---|
| 缺标签 | ✔ | ✔ | **0** |
| `content=""` | ✔ | ✔ | **0** |
| `2026-1-15`（少补零） | ✔ | ✔ | **0** |
| `2026-01-15 09:30:00`（带时分秒） | ✔ | ✔ | **0** |
| **（对照）原样** | ✘ | — | **54** |

**对照组同样重要**：它证明守卫没有过度触发。四种失败输入**均不产出任何 Values**。

### 3.2 A/B：修复前确实静默放行

把实现退回「只判 `!ok`」：

| 输入 | 修复态 | **退回后** |
|---|---|---|
| 缺标签 | 报错 / 0 Values | 报错 / 0 Values |
| `content=""` | 报错 / 0 Values | **`err=nil` / 54 Values** |
| `2026-1-15` | 报错 / 0 Values | **`err=nil` / 54 Values** |

**QA 的发现精确复现**：错误此前被推迟到 `Store.Save`——而那时 raw HTML 已不在手上。
该退回被两条常驻测试打红（`TestParseRejectsBadPubDate`、`TestParseDistinguishesPubDateFailureModes`）。

### 3.3 「三条措辞必须不同」——实测互不相同

`fix_items` 要求两种情形给不同措辞（`metaContent` 的第二返回值正是为分辨
「站点没填」与「选择器写错」而设计）。实测三条错误信息**两两不同**，
且各自指向不同的排障方向（页面结构 / 站点没填 / 形态不合）。

---

## 四、变异

| # | 变异 | PASS（基线 372） | 判定 |
|---|---|---|---|
| **S1** | 删掉「存在但为空」分支（dev-45 报首轮 SURVIVED 的那个） | 371 / FAIL 1 | **KILLED** |
| **S2** | 去掉形态校验分支 | 368 / FAIL 4 | **KILLED** |

### S1 的失败原因，与 dev-45 的根因叙述**逐字吻合**

```
Error:    "…content \"\" does not match YYYY-MM-DD…" does not contain "empty"
Messages: 为空应指向「站点没填」，而不是复用形态不合的措辞
```

⇒ 删掉空值分支后，空串**落到形态分支照样报错**——所以「有没有 error」这个判据对该分支毫无鉴别力。
**是补上的各自关键词断言（missing / empty / YYYY-MM-DD）才让那一支承载了断言。**

**这是本 Sprint「竞争性错误路径」的第三次**（前两次：`TestExtractFieldsRejectsUnknownExtractor` 传 nil、
`require.Error` 被「找不到板块」满足）。三次形状相同：**断言被一个不是你想验的原因满足**。
dev-45 这次是**自己发现并修的**，不是被 QA 或我指出来的。

---

## 五、dev-45 的两处自述，我独立核实

**① 「最终提交态复跑全部变异」——属实且重要。** 它写明首轮变异跑在 code-simplifier **之前**，
而 simplifier 确实改过 `parse_test.go`，于是在最终提交态 `29adc9e` 上重跑了 T5+T6 四个变异体
（全 KILLED，vet 均 0，基线 372 经**断言**而非仅打印）。

这直接对着本 Sprint 已记的那条：**文件一变就要重采全部自证数字**，否则报告里的数字测的是改动前的树。
**它这次主动做了，而不是等人来问。**

**② discovery 用追加而非覆盖**：`verification.rework` 新增，首轮的
`verification.commit`（`4948f0f`）与 `tests`（52 PASS / 52 RUN）**逐字保留**。

---

## 六、validator 与收尾

- 转 `verified` 后完整输出见发给 Leader 的消息（格式 `N 条告警 / EXIT=M`）
- worktree 已拆除，无残留

---

## 七、结论

QA 的 WARNING-2 已闭合：`published_at` 这个「全包唯一逐字来自外部 HTML、不经任何模板」的字段，
现在与其余 14 处 error 分支一样遵守「认不出就报错、绝不猜」——**那条规则的唯一偏离点被补上了**。

四种输入独立复现全部拦下且措辞可分辨，对照组正常；A/B 证明修复前两种输入静默产出 54 个字段；
两个变异均 KILLED，其中 S1 的失败原因逐字印证了 dev-45 对首轮存活根因的判断。
范围无越界，判定对象未漂移，自证数字一致。

**判定：PASS → `verified`**
