# M1.5 QA 裁决记录(Leader 聚合,降级路径)

> qa-agent-3 三个对抗子视角(Minimalist/Skeptic/Architect)发现清单完整交付,但本体会话
> 卡壳于子代理身份(与 M1 qa-agent-2 同型,两次直接指令未达),无法自行落盘 05-review
> verdict(该路径 qa-* 专属,leader 被矩阵正确 DENY——权限最小化按设计工作)。
> 按外部依赖降级原则,Leader 聚合裁决记录于此,处置后视同 QA 轮完成。

## 聚合裁决(无 CRITICAL;三视角重点攻击面均排除: 分位错位/重叠双计/增量锚点/依赖方向/端口一致性)

| 发现(视角) | 级别 | 裁决 |
|---|---|---|
| fnum 对 JSON string/null 静默 NaN,无可观测性(Skeptic) | MAJOR | **review_fix→TASK-002**: fnum 兼容字符串数值(strconv 解析)+ 拉取结果全 NaN PE 守卫(→error 进 Failed 告警,静默漂移变可观测);顺修 config Source 注释纳入 akshare(Architect nit) |
| requirements 不可复现 + lock 升级流程自我瓦解(Architect×2) | Medium | **review_fix→TASK-007**: setup.sh 改为 lock 优先安装(有 lock 用 lock,无则 requirements+freeze),升级走显式 --upgrade 旗标刷新 lock;deployment.md 升级段同步 |
| plist 绝对路径与 setup 相对路径耦合,scripts 投递未声明(Architect) | Medium | 并入 TASK-007 fix: deployment.md 声明 scripts/akshare 随 deploy.sh rsync 投递、setup.sh 须在 runtime 侧执行 |
| 混口径分位排位层未在文档点明(Architect+Skeptic) | Medium→doc | 并入 TASK-007 fix: 口径说明扩句(兜底日分位为混源序列上的排位) |
| 兜底 provenance 库内不可追溯(Architect) | Medium→ticket | M2 ticket(行级 provenance 属 spec「明确不做」,升级需重新设计) |
| fdate 单坏行硬失败(Skeptic) | MINOR→记录 | fail-fast 对 schema 漂移是可辩护语义,保留;记录 |
| 符号映射无边界校验/S1 map冗余/S2 投机键/S3 映射复用(Skeptic+Minimalist) | MINOR/SUGGESTION | 记录不阻塞;S2 投机键裁定保留(live 校验防御);S1 与 simplifier 结论相左,维持现状 |

## Verdict(Leader 代裁): REJECT→review_fix 1 轮(2 任务小修),修复复验后视同 PASS

## 流程教训(已记 wisdom): QA spawn 的子代理 dispatch 指令须显式包含「聚合落盘是你的 mandate 一部分」,或改为 Leader 直接派三个独立 reviewer 自行聚合——连续两个 Sprint QA 本体卡壳,M2 前框架侧应修 TeammateIdle 对只读 reviewer 的误触发
