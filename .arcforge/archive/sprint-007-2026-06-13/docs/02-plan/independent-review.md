# 独立 reviewer 反审结论（只读需求，未看 Leader DoD）

## 与 Leader DoD 比对
独立 reviewer 的验收维度与 Leader DoD **高度吻合**，确认无重大缺口。其额外补强点处置：

| reviewer 提出 | 处置 |
|---|---|
| 测试生产默认退避 = 1/2/4/8/16s（防静默置空）| **采纳** → 并入 TASK-001 functional |
| 请求带 User-Agent 头 | **采纳** → 并入 TASK-001 functional |
| HK/US 指数(^GSPC/^HSI)也走 .mcw 分派 | **采纳** → 并入 TASK-002 functional |
| 基金 profile 失败→nil 分支 | 已覆盖（TASK-005 error_handling）|
| 现任经理选取/最大回撤取最小值 | 已覆盖（TASK-005 functional[1] 断言侯昊/-0.3369）|
| 500+合法 body 仍报错 | 已覆盖（TASK-002 _HTTPError）|
| 空 data 行情不报错 vs Quote 报错 | 已覆盖（分别在 TASK-003）|
| ROE=0 + body 不含非法指标 | 已覆盖（TASK-004）|
| 单 bar / PrevClose=0 不 panic/除零 | 代码守卫(len>1 / prev!=0)，列为实现细节，不单列 DoD |
| serve retry 缺省安全回退、core 零改动、fallback 契约不变、_probe 清理 | 已覆盖（TASK-006/004/007）|

## 结论
DoD 充分、可测试、边界齐全。3 处补强已并入对应任务（仍满足 Realistic Scope ≤8 条/任务）。
