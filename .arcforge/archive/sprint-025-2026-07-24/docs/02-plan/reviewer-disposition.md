# M1.5 独立 reviewer 反审处置记录

> 判定 NEEDS_CHANGES(G1/G2 必改 + P1-P4 建议);全部应用,validator 复跑通过;
> 依赖图/互斥/无代码任务声明/凭空 DoD 反审全绿。

- G1(M1 同型空断言) → TASK-004 error_handling 重写: akshare 直连失败注入 + Series 读回失败注入,断言实覆盖。
- G2(覆盖率门禁漏传) → T2/3/4/5 加「变更包覆盖率≥80」(test);T4/T6 注 AD-6 处置指引。
- P1(ps 兜底键零覆盖) → T2 boundary 并入「仅 ps 无 ps_ttm 经兜底取到值」。
- P2(HK 合并措辞过强) → T3 boundary 改「以 PE 日期为主键」明确口径(无 PE 无法算分位,不保留仅 PB 行)。
- P3(Degraded 格式串未断言) → T5 functional[1] 明确两段 assert.Contains。
- P4(输出/退化断言依赖未验证既有用例) → T6 功能/边界改为对 out buffer 与 sender 空的显式断言。
- dod-gate: 用户已预授权「小幅处置直接开工」,本轮处置未动任务边界,直接组建团队。
