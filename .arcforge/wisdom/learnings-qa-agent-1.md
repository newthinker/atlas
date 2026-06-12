# QA Agent learnings — qa-agent-1 (sprint-003 qlib-eval pipeline)

> 仅追加写本文件（单写者）。Leader 在阶段边界聚合进 _digest。

## 复审方法论（Reality Checker 兑现）
- **不接受"测试绿即闭合"**：对每个我报告的崩溃 WARNING，亲自构造同场景反例跑真实入口
  （empty signals 跑 main()、benchmark 抛异常跑 collect_outcomes），眼见 exit code / 无 traceback
  才判闭合。先证旧代码确实崩（NaT.strftime ValueError 实测），再证新代码不崩——前后对照才算数。
- **跨语言契约不靠看代码，靠拉通**：Go encoding/csv 写出 → Python csv.reader 读回，实测
  `{"k":1}` 转义 / %.2f / 日期逐字段一致；并确认 Go json.Marshal map 键有序（确定性 golden 的前提）。

## 本 Sprint 高价值发现模式（可复用）
1. **降级路径的不对称健壮性**：逐 symbol 有 try/except 降级，但同函数顶部的 benchmark() 裸调用没有
   → 单点崩溃。审"部分失败"时，对每个外部调用逐一问"它失败时整跑会怎样"。
2. **空集 / 退化输入打穿格式化层**：空 DataFrame 的 .min() = NaT，NaT.strftime 抛错。
   凡"对聚合结果再 strftime/格式化"处，必查零样本路径。
3. **格式化精度是否进入数学**：CSV price 用 %.2f，但收益用的是 qlib 价格而非 CSV price——
   先确认"被舍入的字段是否参与计算"，避免误报。confidence %.2f 才真正影响桶边界（弱）。
4. **负索引静默取末行**：pandas searchsorted-1 = -1 时 iloc[-1] 静默取末行 → 事件研究最易错处；
   实现已显式 <0 防御并有守门测试，值得作为正面范例沉淀。

## 协作机制经验
- **锁内前置校验救了一次双写**：我按角色要写 review_fix 时，Leader 已并发把 TASK-007 推进到
   assigned；锁临界区内 `status==verified` 前置校验直接 abort，未覆盖 Leader 的状态。
   教训：QA 写任务文件前的状态校验不是形式，是真防竞态的关键。
- **owner 边界**：verified→accepted 是 Leader 的转换，QA 不持有；review 完成后我无 task-state 待办，
   正确动作是产出报告 + 通知 + 沉淀 wisdom，而非硬找任务写。

## 残留（留下一 Sprint，已与 Leader 确认非阻塞）
- S4 exit_date 超基准末日 _last_le 取末行无 gap 标记（终点侧缺与起点对称的越界标记）。
- S5 confidence %.2f 桶边界舍入伪影。
- S6 backtest.New 在 symbol×strategy 循环内构造（性能，无害）。
