# 需求分析 — Hestia M1b-4a discover + fetch + config（Sprint 036）

**需求文档**：`~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-12-hestia-discover.md`（2143 行）
**目标包**：`internal/hestia`（本仓库 atlas）
**基线**：`f5a17d5`（Sprint 035 归档提交），`internal/hestia` 覆盖率 **92.1%**

## 1. 交付内容

让 hestia 能从央行发布页**发现**未入库的报告期次，并从 YAML 装载阈值配置。

| 任务 | 交付 |
|---|---|
| T1 | `Fetcher` 接口 + 绕代理的 PBOC client（`fetch.go`） |
| T2 | index 页快照 + 分页模板解析（`parsePaging` / `pageURL`） |
| T3 | 标题正则、期次映射、条目提取（`parsePeriod` / `scanPage` / `Candidate`） |
| T4 | `Store.HasPeriod` + `PeriodChecker` 接口 |
| T5 | `Discover` 主循环（翻页直到找到未入库的期次） |
| T6 | 口径豁免的键补 `PeriodTypes`（**唯一破坏性改动**） |
| T7 | `LoadConfig`（viper + mapstructure，照 crisis 先例） |

**边界**：`Discover` **不抓文章页** —— 只产出候选清单，取正文是 M1b-4b 的事。
这条边界让 discover 能用快照完整测试、不碰网络。

## 2. 前提核验（Leader 实测，非照抄计划声明）

| 前提 | 实测 |
|---|---|
| `viper` 已在 go.mod | ✅ `v1.21.0` |
| `validPeriodTypes`（types.go:40） | ✅ `map[string]bool{monthly,h1,annual}` |
| `checkEnum` | ✅ 存在（**types.go:116，计划写的 110 已漂**） |
| 测试 helper `newTestStore`(:47) / `passing`(:498) / `failing`(:973) / `saveMonthly`(:1546) | ✅ 全部存在 |
| `knownCheckIDs`（validate.go:119）、`checkCompleteness`（thresholds.go:15） | ✅ 存在 |
| `internal/crisis/config.go` 先例 | ✅ 存在，viper + 「每个数字都在 YAML 里」 |
| **T6 改动清单** | ✅ **逐字一致**：`CaliberExemption{` 构造点 **10 处**（两测试文件各 5）、`exemptionFor` 调用点 **4 处**（生产代码仅 `validate.go:93`） |

### 时效项已验（T2 Step 1 的前提，计划标了「先做」）

index 页每天在变，报告条目会随新闻累积下沉。Leader 当日实测：

```
p1  HTTP 200  38147 bytes   报告条目：无        ← 正是要测的常态
p2  HTTP 200  38692 bytes   报告条目：2026年上半年金融统计数据报告
分页控件：jumpTo(this,'408','1','/goutongjiaoliu/113456/113469/11040-%1.html')  两页都有
干扰项：国新办举行新闻发布会 介绍2026年上半年货币政策执行和金融统计数据情况   ← 就在 p2
```

⇒ **T2 用 p1+p2 的方案成立，不需要改用 p3**。且报告与干扰项同页，是 `functional[1]` 的理想素材。
**但快照必须由 T2 的 dev 当场抓取**（Leader 这次只验证可达性，不代抓 —— 抓取是 T2 的交付物）。

## 3. 需求性质：与 Sprint 035 同形

这同样是一份**已完成的实施计划**（每个 Step 给出完整代码、预期 RED 信息、提交信息），
且自审声称「无遗漏」。Sprint 035 的经验直接适用：

- 独立 reviewer **实跑计划**查出了 4 条必卡 dev、4 条必假绿的问题
- 「守卫在场 ≠ 守卫有效」出现 10 次，**十次全靠构造反例发现，无一靠读代码**

⇒ 本 Sprint 沿用同一套：开工前 spawn 独立 reviewer 逐字落盘实跑；四条派单硬要求全部保留。

## 4. 计划自身值得称道的一点

计划有一节「一个贯穿全计划的判据」，列出**「静默失效」在本迭代出现四次，每次都用「响亮失败」replace 掉**：

| 位置 | 静默形态 | 处置 |
|---|---|---|
| 正则漏 annual | 每年 1 月无声跳过 | 期次段改可选 |
| 分页解析失败 | 退化成只扫第 1 页 | 报错 |
| 豁免 `PeriodTypes` 为空 | 默认命中全部三条序列 | 报错 |
| 阈值漏写 | 变成 0，每期都进 pending | 预填默认值 + `> 0` 检查 |

这与 Sprint 035 反复撞到的形态同族。**DoD 要把这四条各自钉住**，
且判据是「**改坏它有东西会红吗**」而非「代码里写了 return err」。

## 5. 识别出的风险（处置见 architecture-decisions.md）

1. **AD-036-1 计划声称的「T1–T4、T6 可并行」在文件层面不成立** —— T2/T3/T4/T5 **全部触碰 `discover.go`**
2. **AD-036-2 四个任务都会打红导出面守卫** —— 本 Sprint 新增 3 个包级导出函数 + 1 个导出方法
3. **AD-036-3 T6 是破坏性改动，且两类改动的暴露方式不同** —— 签名变更是编译错误，加必填字段不是
4. **AD-036-4 快照有时效性** —— 必须由 T2 的 dev 当场抓，且 README 要记抓取日期
