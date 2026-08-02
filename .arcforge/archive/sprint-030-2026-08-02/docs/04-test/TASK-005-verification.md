# TASK-005 验证报告(复验轮 / review_fix 轮次 1)— 编排层降级链扩展

- **验证者**: test-agent-14 (Reality Checker 模式)
- **验证时间**: 2026-08-02T08:22Z
- **判定**: **PASS → status=verified**(已 jq 直读核实)
- **epoch**: 1(`--expect-epoch 1` 通过)/ **rework_count**: 1
- **本轮范围**: C2 / M1 / M2 / M3b / 限频文案延伸 + 全量回归。M4 仅知悉(按裁决交 TASK-006)。首轮 8 条 DoD 已于 2026-08-02T05:12Z 全部 PASS。
- **验证环境**: **git worktree**(`../wt-verify-T001`,detached @ fe7e1f6 + 覆盖未提交改动),验后已 `git worktree remove`,**主仓零派生目录**——本轮起不再在仓内建变异副本。

## 1. 本轮 fix 覆盖矩阵

| 项 | 要求 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| **C2**<br>CRITICAL | `refreshTushareValuation` 加「至少一点三值非全 NaN」门槛(**按 dev 实现判,非 QA 字面的 PE-only**),全 NaN 视同无数据、不推水位 | 实现 `hasAnyValuation` 确为**三值或判**,注释写明为何不照抄 akshare 的 PE-only(daily_basic 对亏损标的正是 pe_ttm 缺失而 pb/ps_ttm 有值)。两项突变**双向夹逼**:<br>① **C2-a 去掉守卫 → `TestRefreshTushareValuationAllNaNIsNotSuccess` 转红**(全 NaN 确实被挡住;该用例断言 `store.upserts` 为空 ⇒ **水位不推进**、且无 `fallback ok`);<br>② **C2-b 改成 PE-only(即 QA 字面版)→ `TestRefreshTushareValuationKeepsRowsWhenOnlyPEIsNaN` 转红** ⇒ **边界用例真有判别力**,证明守卫没把亏损标的的真实估值一起挡掉(该用例断言 PE 为 NaN 但 PB=6.2 确实落库) | **PASS** |
| **M1**<br>MAJOR | `refreshTusharePrices` 的 `upsertPrices` 写失败上抛,签名简化为 `error` | 实现改为 `store.UpsertPrices` 失败即 `fmt.Errorf("price upsert: %w", err)` 上抛,注释说明为何本跳与 refreshEngine/refreshEdgar 不同(那两处价格是估值链路附属产物,本跳交付物**只有**价格)。**突变(改回吞掉)→ `TestRefreshTusharePricesUpsertFailureIsError` 转红**。该用例把 QA 原实证的四个自相矛盾现象逐个反转:Refreshed **0**(原 1)、Failed **含 0700.HK 与 disk full**(原空)、无 `fallback ok`(原两条自相矛盾 Degraded) | **PASS** |
| **M2**<br>MAJOR | 三处零行守卫补锁定用例 | **美股 TD 零行**:突变 M2-b(去掉守卫)→ `TestRefreshUSPriceTwelvedataEmptyIsNotSuccess` **转红** ✅<br>**A 股估值零行**:单独去掉 `len(pts)==0` 守卫后**突变存活**,但我验证后确认这是 **等价突变**而非测试缺口——`hasAnyValuation` 对空切片返回 `false`,C2 的守卫已经覆盖了零行,行为完全不变,故**任何测试都杀不掉它**。为证明用例本身有判别力,我**同时废掉两道守卫** → `TestRefreshTushareValuationEmptyIsNotSuccess` **转红** ✅<br>**第三处**即 C2(全 NaN),见上 | **PASS** |
| **M3b**<br>MAJOR | 按 Leader 裁决方案 2:原用例改名 + 注释声明非生产证据;新用例锁生产缺口 | **语义诚实性已确认**:改名后的 `TestRefreshHKPriceOnlyHopWiring` 注释明确写「⚠ 本用例**不构成生产可用性证据**」,并解释 fake 命中只因按配置形态建键、真实上游不会命中,同时指向缺口用例、说明保留价值(分派逻辑/仅价格语义/水位保护三件事仍需回归)——**不再宣称生产可用**。<br>新增 `TestRefreshHKProductionSymbolHitsKnownGap` 的 fake **按上游契约以 5 位 `00700.HK` 建键**(注释:「不迁就被测代码」),断言 Refreshed=0、Failed 含该标的、零行不落库、无 `fallback ok`、且**确以 4 位形态发起调用**。<br>**我另做探针突变确认它真锁住了缺口**:把港股 4→5 位归一补上(即修复缺口)→ **该用例转红** ⇒ 缺口一旦被修必须显式改测试,不会静默漂移 | **PASS** |
| **延伸**<br>限频文案 | `ErrRateLimited` 消费点 Degraded 改「限频,本次跳过,下次自动重试」,`NotContains`「配置性问题」 | 实现为 `switch` 双分支(限频在前、无权限在后),注释说明两者运维动作相反。`TestRefreshTushareRateLimitedIsTemporary` 断言含「限频」「下次自动重试」,**且 `NotContains`「配置性问题」与「权限不足」**,并断言标的仍进 Failed、Refreshed=0。**突变(限频分支失效、回落单分支)→ 该用例转红** ⇒ 分叉存在且断言有效 | **PASS** |
| **M4**<br>MINOR | 仅知悉,无需改代码 | 已确认本轮未改路由层(`app.go` 未在改动面内);按 Leader 裁决交 TASK-006 文档降级 + 立后续任务 | **N/A** |
| **回归** | 全仓绿;覆盖率 | 五包全 ok:**prism 94.0% / tushare 94.2%**(与 dev 自报**逐项一致**)/ cmd 74.3% / collector 98.2% / baostock 95.7%。**全仓 `go test ./...` 61 包 ok、0 FAIL**。`go vet` exit=0 | **PASS** |

## 2. 突变汇总

**真实突变 6 项,全部杀死**;另 1 项经证明为等价突变。

| 突变 | 注入内容 | 转红用例 | 结果 |
|---|---|---|---|
| C2-a | 去掉全 NaN 守卫 | AllNaNIsNotSuccess | 杀死 |
| C2-b | 判据改 PE-only(QA 字面版) | KeepsRowsWhenOnlyPEIsNaN | 杀死 ⭐ |
| M1 | 价格写失败重新吞掉 | TusharePricesUpsertFailureIsError | 杀死 |
| M2-b | 美股 TD 零行守卫失效 | USPriceTwelvedataEmptyIsNotSuccess | 杀死 |
| 限频 | 限频分叉失效 | TushareRateLimitedIsTemporary | 杀死 |
| M3b 探针 | 补上港股 4→5 位归一(修复缺口) | HKProductionSymbolHitsKnownGap | 杀死 ⭐ |
| M2-a | 单独去掉 A 股估值 `len==0` 守卫 | — | **等价突变**(见 §1 M2 行) |

## 3. 静态与卫生

- `go vet`(prism + tushare)exit=0。
- `gofmt -l` 唯一告警 `internal/prism/sankey/template_test.go`:已核实**改动前遗留**——本任务未改该文件(`git status` 该目录为空),且 `git show HEAD` 版本同样不满足 gofmt。**本任务改动的 4 个文件 gofmt 全部干净**。
- 密钥哨兵:4 个改动文件(**已逐个核实存在**)对 runtime config 的 3 个长 key **0 命中**;扫描管道有效性已用「必然命中的串」自检。

## 4. 观察项

**无新增缺陷。** 另记两条状态更新:

1. **我上轮报告的 M8 存活突变已被封堵**:上轮「A 股估值跳零行守卫无用例锁定」这一缺口,本轮由 C2 守卫 + `TestRefreshTushareValuationEmptyIsNotSuccess` 共同覆盖(见 §1 M2 行的双守卫验证)。
2. **港股生产缺口仍在,但现在是「被测试锁住的已知缺口」而非「被假测试掩盖的未知缺口」**——这是 M3b 的实质改进。归一修复见 D4 后续任务;TASK-006 侧文档标注。

## 5. 判定

**C2 / M1 / M2 / M3b / 限频文案延伸 5 条 fix 全部 PASS,M4 按裁决 N/A,首轮 8 条 DoD 回归全部保持。**
证据为 worktree 内亲跑 + 6 项突变(含 2 项专为验证「守卫没误伤真实数据」与「缺口确被锁住」而设计的反向突变)+ 1 项等价突变的显式证明 + 文件直读,非采信 dev 报告文字。
`status=verified` 已落盘并经 `jq` 直读核实(`verifying→verified by test-agent-14 @ 2026-08-02T08:22:56Z`,epoch=1 / rework_count=1)。
