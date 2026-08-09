# H11 — 跨包契约：`bitemporal.CurrentQuery` 的输出格式是 hestia 的开库前置条件

**来源**：test-agent-20 在 TASK-004 返工验证中实测发现（探针实证，非推断）。
**性质**：不是缺陷，是**一层没有被任何一侧写下来的依赖**。

## 事实

`hestia.verifyCurrentView`（TASK-004 的 C1 修复）从 `sqlite_master` 读实际部署的视图定义，与 `currentViewDDL(spec)` 派生的期望值**全等比对**，不符即 `NewStore` 失败。

而 `currentViewDDL` 的主体来自 `internal/macro/bitemporal.CurrentQuery`——**本 Sprint 范围之外的包**。

**探针实证**：对视图定义做**纯格式改动（只加一个空格，SQL 语义完全相同）**，既有库 `NewStore` **直接失败**。

⇒ **`bitemporal` 包的一次纯排版重构，会让全部既有 hestia 库打不开。**

## 为什么不判为缺陷

按 dev-agent-42 在 T4 的设计判准，这正是要的**「版本不一致」信号**：

> 视图对不上说明这个库出自另一个代码版本，而列检查对「多出来的列」是放行的——那种库的表也可能与本版不同。**静默修好视图，等于把唯一还会响的警报关掉。**

错误信息也写明了 drop the view 即可恢复（视图不存数据，重建无损）。

## 问题在于改动方无从得知

`bitemporal` 包的维护者**没有任何线索**知道自己的输出格式已成为下游的开库前置条件。M1a 交付时这层依赖还不存在——它是 M1b-1 的 C1 修复引入的。

## 落点要求（二选一，或都做）

1. 在 `bitemporal.CurrentQuery` 的注释里加一行指回 `hestia.verifyCurrentView`，写明「本函数的**输出格式**（非仅语义）是 hestia 的开库前置条件，纯排版改动会让既有 hestia 库失败」
2. 在 M1b 后续子迭代的交接清单里登记这条跨包契约

**不做的后果**：下一次 `bitemporal` 的格式调整（哪怕只是 gofmt 风格或一次 SQL 换行重排）会在**部署环境**才暴露，且症状是「所有既有库突然打不开」——排查方向会指向 hestia 而非 bitemporal。

## 关联

这与 M1a 已有的另一条跨包契约同形：`bitemporal.Lookup` 的包注释明写「revision 的形态由建表方与写入方保证」，M1b-1 的 `Meta.validate` 接下了它（见 handoff H1 的 G1）。

**区别是那条被写下来了，这条没有。**
