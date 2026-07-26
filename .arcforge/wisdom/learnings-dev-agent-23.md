
## 「断言恒真」的两种形态:fixture 值与 fixture *缺项* (dev-agent-23, TASK-012 验收 + TASK-013 开工)

同一根因的两个面貌:**测试通过,但通过的理由不是它声称的那个**。两例都是绿灯,没有任何信号。

### 形态 A:fixture 值 == 断言值(test-agent-9 的 ME12,TASK-012)
`prism_sankey_test.go` 里 `assert.Equal(t, "MSFT", f.symbol, "symbol 必须透传到 service")`,
而该用例请求的正是 `/prism/sankey/MSFT`。**把 handler 里的 symbol 硬编码成 "MSFT",这句断言照样绿**。
只有 e2e 因为用了 `BRIDGE`/`NORMAL` 两个**不同**的 symbol 才杀掉该变异。
> 判据:断言的期望值若与 fixture 的输入值同源,该断言对「值是否真的流经被测路径」零判别力。
> 修法:让输入值与「若实现错误会产生的值」可区分——用两个不同 symbol,或用一个不等于路径值的值。

### 形态 B:fixture **缺**了生产环境有的东西(本人实测,TASK-013 开工)
`TestSankeyRoutesRegistration/not registered without a service` 断言 prism 未启用时
`/api/prism/*` 返回 **404**,长期绿。但它的 `newTestServer` 传的是 `Config{Host, Port}`——
**没有 `TemplatesDir`**。而 `s.mux.HandleFunc("/", webHandler.Dashboard)` 这个 catch-all
在 `if cfg.TemplatesDir != ""` 块内。实测同一份代码两种 fixture:

| fixture | status | len | html | ctype |
|---|---|---|---|---|
| 无 TemplatesDir(既有用例) | 404 | 19 | false | text/plain |
| **有 TemplatesDir(生产)** | **200** | **2896** | **true** | **text/html** |

**它看到的 404 是生产环境根本不会出现的。** 测试不是在验证「未注册时安全地 404」,
而是在验证「一个没有 dashboard 的阉割 server 会 404」。缺陷(前端 `r.json()` 拿到 HTML
直接抛)完整存在于生产,而守护它的测试**从头到尾是绿的**。

> **形态 B 比 A 更隐蔽**:A 至少两个值都写在用例里、肉眼可比;B 的关键信息是**不存在的那一行**
> ——`TemplatesDir` 没被传,读用例时看不见任何异常,要读到 `setupRoutes` 才知道它决定成败。
> 与 AD-27 那条「fixture 里两行的列出顺序静默决定测试可靠性」同族:
> **fixture 的不可见属性(值的同源性、缺项、顺序)承载着测试强度,而它们都不在断言里。**

### 可操作的检查
1. 写「未启用/未注册」类负向断言时,**先确认 fixture 与生产在「兜底路径」上配置一致**;
   catch-all、middleware、默认 handler 这类「不在被测路径上但会截胡」的东西最容易缺。
2. 负向断言**不要只断状态码**——本例里 404 与 200 的区别正是 fixture 差异带来的。
   同时断**响应体形态**(是 JSON 不是 HTML / Content-Type / 不含 dashboard 标志),
   这样即使有人改成「404 + HTML」也挡得住(test-agent-9 对 TASK-013 的验收预告第 1 条)。
3. **变异测试对形态 B 无效**:变异的是被测代码,而问题在 fixture。
   杀死形态 B 只能靠「让 fixture 贴近生产」或「跨 fixture 对照」(如本例这样两种配置并排跑)。
