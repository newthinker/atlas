# 需求 ↔ DoD 双向追溯矩阵(M3.5a / Sprint-030)

> 需求来源: 计划(plans/2026-08-02-prism-m3.5a-datasources.md)Task 1-6 + Global Constraints + spec §2/§5/§6。
> 记法: TASK-NNN 的 f=functional / b=boundary / e=error_handling / nf=non_functional,序号从 1 起。

| # | 需求条目 | 来源 | DoD 覆盖 |
|---|---------|------|---------|
| R1 | tushare 客户端 4 能力方法(daily_basic/index_daily/hk_daily/daily) | plan Task1 | T001 f1,f2 |
| R2 | 40203 → 永久性 ErrNoPermission,不重试、文案注明配置性 | plan Task1/spec §2 | T001 f3 + T005 e1 |
| R3 | tushare 200ms 节流兜底 | spec §3 | T001 nf1 |
| R4 | index_dailybasic 权限 live 探针,结论定指数链形态 | plan Task1 S5/spec §2⚠ | T001 nf2 + T005 f4 |
| R5 | twelvedata time_series 客户端(字符串 close/升序) | plan Task2 | T002 f1,f2 |
| R6 | TD 8s 节流;缺 key 该跳跳过 | spec §3 | T002 nf1 + T005 f5 |
| R7 | TD 复权口径 live 校验(与 yahoo 对齐) | plan Task2 S5/spec §3⚠ | T002 nf2 |
| R8 | baostock 桥:stdlib server/仅 127.0.0.1:8181/adjustflag=3 | plan Task3/spec §3 | T003 f3 |
| R9 | baostock Go 客户端 + symbol 转换(SH/SZ) | plan Task3 | T003 f1,f2 |
| R10 | baostock launchd plist(KeepAlive,无密钥) + 装载脚本 | plan Task3/6 | T006 f1 |
| R11 | PrismConfig.BaostockBaseURL 默认 127.0.0.1:8181 | plan Task4 | T004 f1,b1 |
| R12 | example yaml 空 key + 密钥卫生注释 | plan Task4 | T004 nf1 |
| R13 | 密钥哨兵(commit 前 grep;终检 git log -S) | Global/spec §5 | T004 nf2 + T006 nf2(+各任务 description 约束) |
| R14 | A 股公司估值 akshare→tushare 降级跳 + Degraded | plan Task5/spec §2 | T005 f1 |
| R15 | 美股价格 yahoo→TD 降级跳(EPS 链不变)+ Degraded | plan Task5/spec §2 | T005 f2 |
| R16 | 港股 akshare→hk_daily 仅价格跳(估值 NaN,文案注明) | plan Task5/spec §2 | T005 f3 |
| R17 | A 股指数估值链尾 tushare 跳(形态按探针) | plan Task5/spec §2 | T005 f4 |
| R18 | baostock 注册 collector registry(Prism 不消费) | plan Task5 S5 | T005 f5 |
| R19 | 既有链路零行为变更;ts/td nil 行为不变 | Global | T005 b1,nf1 |
| R20 | 断源演练:断 akshare→tushare 恢复+Degraded(spec §6) | plan Task6 S2 | T006 nf1 |
| R21 | 部署文档段 + §10 风险表五条 + §9 状态 | plan Task6 | T006 f2,f3 |
| R22 | 正常 refresh 27 ok, 0 failed | plan Task6 S3/spec §6 | T006 nf2 |

## 反向检查(凭空 DoD)

- T001 b1,b2,e1 / T002 b1,e1 / T003 b1,e1 — 对应计划 Task1/2/3 明列的测试断言清单(空响应/倒序/业务错误面),非凭空。
- T003 nf1,nf2 — 对应计划 Task3 S1(setup.sh 幂等/版本冻结)与 S3(本机 live 验证)。
- 其余 DoD 均在上表正向映射中。

## 结论

- 孤儿需求: 无(R1-R22 全部有 DoD 覆盖)。
- 凭空 DoD: 无(反向检查全部溯源到计划条目)。
- review/manual 占比: T006 为 docs-only(5/5 review|manual,符合无代码任务声明);其余任务 manual 条目均为计划明示的 live 校验步骤,不逼 Dev 写 fantasy assertion。

## Reviewer 反审记录(2026-08-02)

独立 reviewer(只读需求)判定 NEEDS-FIX 2 条,已修正:R23 A 股行情二跳 tushare daily
登记 registry(→T005 f5);「备源未配置」提示矛盾定案不实现(→T005 b1 + ADR#9)。
建议项采纳:T002 补 close 不可解析边界(b2)与密钥哨兵(nf3)。修正后 validator 复验绿。
