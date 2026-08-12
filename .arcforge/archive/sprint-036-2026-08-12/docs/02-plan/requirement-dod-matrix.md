# 需求 <-> DoD 双向追溯矩阵 — Sprint 036

**需求真相源**：上游 spec `2026-08-12-hestia-discover-design.md` 第 7.3 节的 DoD 表（**17 条**）。
**不以计划的自审表为准** —— 那是计划对自己的追溯，本矩阵的价值正在于独立复核它。

---

## 方向一：spec 的每条 DoD 是否都有任务覆盖（查孤儿需求）

| spec DoD | 内容 | 覆盖 | 状态 |
|---|---|---|---|
| `functional[0]` | 三种期次都能解析 | TASK-003 f0 | 覆盖（我写了**四**种，多钉 `2026年12月`） |
| `functional[1]` | 干扰项 `国新办…情况` 不匹配 | TASK-003 e0 | 覆盖（Leader 已实测干扰项与报告**同在 p2**） |
| `functional[2]` | 分页模板 + 总页数一并取到 | TASK-002 f1 | 覆盖 |
| `functional[3]` | 第 1 页无报告时继续翻第 2 页 | TASK-005 f0 | 覆盖 |
| `functional[4]` | 碰到已入库即停，**且不再请求后续页**（断言 `calls`） | TASK-005 f1 | 覆盖 |
| `functional[5]` | **空库时翻满 `MaxPages` 后停** | TASK-005 b0 | **见发现 1** |
| `functional[6]` | `LoadConfig` 读 YAML，装载后立即 `validate()` | TASK-007 f1 + e0 | 覆盖 |
| `functional[7]` | `HasPeriod` 三态，**pending 不算已入库** | TASK-004 f0 + b1 | 覆盖 |
| `boundary[0]` | 总页数 < `MaxPages` 时不越界请求 | TASK-005 b0 | 覆盖 |
| `boundary[1]` | 分页边界重复只产出一个候选 | TASK-005 b1 | 覆盖 |
| `boundary[2]` | `PeriodTypes` 为空 / 含非法值 → 报错 | TASK-006 b0 + e0 | 覆盖 |
| `boundary[3]` | 豁免只对列出的 period_type 生效 | TASK-006 f0 | 覆盖 |
| `error_handling[0]` | 分页解析不到 → 报错，不退化成只扫第 1 页 | TASK-002 e0 | 覆盖 |
| `error_handling[1]` | HTTP 非 200 → 报错，带状态码与 URL | TASK-001 e0 | 覆盖 |
| `error_handling[2]` | 查库失败 → 返回 error，不当成「未入库」继续翻 | TASK-005 e0 | 覆盖 |
| `non_functional[0]` | **设了 `HTTP_PROXY`，PBOC client 仍直连** | TASK-001 f0 | 覆盖（含「零鉴别力写法」的反面警告） |
| `non_functional[1]` | `Discover` 全程不抓文章页 | TASK-005 n0 | 覆盖 |

**孤儿需求：0 条。**

## 方向二：任务 DoD 是否都可回溯（查凭空 DoD）

55 条任务 DoD 全部可回溯，三类来源：

| 来源 | 说明 |
|---|---|
| spec 第 7.3 节 DoD 表 | 上表 17 条对应关系 |
| 计划正文的设计约束 | 如 10MB 响应上限（T1 Step 6）、`13月` 语义校验（T3 Step 1）、README 记抓取日期（T2 Step 3）、改动顺序先签名后字段（T6） |
| **Arcforge 流程要求**（每任务 1-2 条） | RED 真实性、`gofmt`/`vet`/整包绿、覆盖率不低于 92.1%、**导出面守卫登记** |

第三类不在 spec 内但来自 `arcforge.config.json`（`tdd.require_failing_test_first`、`coverage.dev_minimum`）
与本仓库既有守卫，**不是凭空发明的验收标准**。

**凭空 DoD：0 条。**

---

## 发现 1：计划的 `TestDiscoverFindsReportOnSecondPage` **推演必定失败**

**Leader 独立推演（尚未实跑，标注为推断）**，逻辑链如下：

1. `twoPageFetcher` 的 `pages` 只准备了**两个** key：`testIndexURL` 与 `u2`（计划 T5 Step 1）
2. 该用例传 `DiscoverCfg{MaxPages: 3}`
3. `Discover` 的 `limit = min(MaxPages, totalPages) = min(3, 408) = 3`（totalPages 从真实快照解析得 408，Leader 已实测）
4. 循环 `for page := 1; page <= limit; page++` ⇒ **会走到 page=3**
5. 关键：**找到新候选后不 return** —— 实现只在 `has == true`（已入库）时 `return out, nil`；
   空库时 `fakePeriodChecker.have` 是空 map ⇒ 恒 false ⇒ **继续翻页**
6. page=3 请求 `u3`，fake 没准备 ⇒ 返回 `fake: 没有为 %s 准备页面` ⇒ `Discover` 返回 error
7. ⇒ 用例的 `require.NoError(t, err)` **红**，且 `assert.Equal([]string{testIndexURL, u2}, f.calls)` 也对不上

**这与 spec `functional[5]`「空库时翻满 `MaxPages` 后停」是一致的** —— 实现没错，
**错的是测试**：它一边要求「翻满 MaxPages=3」，一边只准备 2 页、又断言「应当只请求这两页」。

**可能的修法**（留给 dev 判断，不由 Leader 指定）：用 `MaxPages: 2`；或给 fake 补一个空的第 3 页。
**前者更贴近断言意图**（「只请求这两页」），后者更贴近 spec `functional[5]`（「翻满后停」）。

⚠️ **本条是推断，不是实测。** 已 spawn 的独立 reviewer 正在逐字落盘实跑，
其结论回来后**以实跑为准**；若它未发现此条而我的推演成立，说明两条路径都需要复核。

⇒ 已写进 TASK-005 的 `functional[0]`，要求 dev **实际跑一遍再决定修法**，并把实际失败输出记入 discovery。

---

## 发现 2：`functional[5]` 与 `functional[4]` 的断言对象必须分开

spec 把两条分开写是有道理的，但它们**共用同一个 fake**，容易被合并成一个用例：

- `functional[4]`（碰到已入库即停）：**必须断言 `calls` 变短**
- `functional[5]`（空库翻满 MaxPages）：**必须断言 `calls` 达到 MaxPages**

**只看返回值两者不可分** —— 空库翻满与提前停都可能返回同样的候选清单。
这与 Sprint 035 反复出现的「守卫在场 ≠ 守卫有效」同族：断言必须**观测到那个区别本身**。

---

## 待独立 reviewer 复核

已 spawn 一个**只读上游计划与 spec、禁读 `.arcforge/`** 的独立 reviewer，
要求它**逐字落盘实跑**（含实抓 index 页、实测 `articleLinkRE` 对真实 HTML 的匹配、
实测四个新导出物各打红哪条守卫）。其结论回来后并入本文件。
