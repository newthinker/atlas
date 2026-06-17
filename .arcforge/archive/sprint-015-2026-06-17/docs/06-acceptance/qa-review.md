# QA Code Review — digest PE% 列

> feature/digest-pe-percentile-column, master..HEAD（7889c8b TASK-001 + cebea8a TASK-002）

## verdict: PASS（无 CRITICAL、无 WARNING）

## 门禁基线
- go build ./... 通过；go vet 零输出
- go test app/notifier(all)/router -race -count=1 全 ok
- PE% 专项 5 测试逐个 PASS

## 重点核实（设计落地）
1. 末列规则：PE% 为新末列不补尾空格；PRICE 变中间列 padRight+补空格，正确。
2. nil/wrong-type metadata comma-ok 断言安全留空，不 panic（有专测）。
3. B6：去掉 name=="" 提前返回，name/PE 各自不覆盖；name 空+PE 有只盖 PE（presence-check 断言）。
4. B4：>=0 门控 + 渲染无短路 → 0.0 显示 "0.0%"（两层各有专测）。
5. N1：全代码 grep 确认无处读 pe_percentile_display；对照回归断言 with/without 键 routed/err/Send 一致 → 门控零影响。
6. 数据源自洽：buildFundamental 默认 -1、ETF nil、成功才写 >=0，与门控/types 注释三处一致。

## INFO（非问题）
- present-but-wrong-type 静默留空（展示层无需日志，值内存直传 float64 错类型不可达）
- make(map,2) 容量匹配最多两键

## Round2 跨视角（纯 Claude）
correctness/concurrency(-race 绿,顺序写本地 slice)/security(内部 float64 经 fmt,无注入面)/maintainability(注释充分,_display 命名清晰) — 均无 high-severity 发现，视角间无分歧。
