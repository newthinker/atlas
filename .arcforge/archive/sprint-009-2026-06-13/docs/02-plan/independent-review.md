# 独立 reviewer 反审结论（只读需求，未看 Leader DoD）

## 与 Leader DoD 比对
高度吻合。补强处置：

| reviewer 提出 | 处置 |
|---|---|
| **最高风险**：benchmark() 须断言 D.features 收到 to_qlib_instrument 转换后 instrument（防残留硬编码 ["SH000300"]，仅测字段不够）| **采纳** → TASK-001 functional[2] 改为注入 fake qlib 断言 D.features 入参（双向 ^HSI→HSI / 默认→SH000300）|
| report.py 渲染层基准透传（非仅 _meta dict）| 已覆盖（TASK-004 集成 grep 报告「基准 ^HSI」验渲染层）|
| 港股「非空」实质判定（非仅退出 0/文件存在）| 已覆盖（TASK-004 要求丢弃数 << 总数 + mean_ret/win_rate 表，对照基线 8129/8129）|
| A股零回归仅靠单测（本环境 eastmoney 屏蔽）| 强化后单测钉住「默认→请求 SH000300」，零回归有实证 |
| Makefile Tab/make -n 双行参数 | 已覆盖（TASK-003 functional）|
| 基准失败优雅降级（含不支持符号 ValueError）| 已覆盖（TASK-002 error_handling，既有 benchmark_error）|

## 结论
DoD 充分。1 处最高风险补强（benchmark() D.features 入参断言）已并入 TASK-001。
