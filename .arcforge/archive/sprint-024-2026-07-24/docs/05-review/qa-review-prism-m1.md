# QA Code Review — Prism M1 (第 2 轮终审 · 修正 verdict)

- 审查者: qa-agent-2(本体)+ 对抗子视角 Skeptic / Architect / Minimalist
- 快照锚点: HEAD 53dc55a(工作树无产品代码未提交改动)
- 复核事实(硬证据): 全仓 `go build ./...` OK; `go test ./...` 55 包全绿、零 FAIL; `go vet ./...` 干净; 10/10 任务 status=verified。

## 最终 Verdict: PASS

第 1 轮 REJECT 的唯一硬阻塞(refreshEngine 缺 EPS≤0 熔断)及所有 CRITICAL/正确性 MAJOR 均已修复并有测试覆盖; 唯一遗留 MAJOR(detail 401)已达成共识处置(文档化 + M2 横切 ticket)。满足「无 CRITICAL 且 MAJOR 有共识处置即 PASS」。

---

## 第 1 轮发现的闭环核验(逐条以 git/代码/测试实况锚定)

| 原发现 | 级别 | 修复 | 复核证据 |
|---|---|---|---|
| serve 启动崩溃(磁盘模板未跟踪) | CRITICAL | c065e5e | internal/api/templates/prism_*.html 已跟踪提交; 配套「移走即 FAIL」回归测试; serve 文件系统路径可加载 |
| 增量刷新时区倒挂(西经宿主) | MAJOR | 6973e44 | refresh.go 改 parse-first + `start.Format>=now.Format` 守卫; 消除 startDate>endDate 倒挂 |
| refreshEngine 缺 EPS≤0 熔断 | MAJOR(硬阻塞) | 6973e44 | refresh.go:146-148 `if e,ok:=currentEPS(eps);ok&&e<=0 { return ErrNonPositiveEPS }`; 亏损标的不入库、进 Failed; 专项测试 TestRefreshEngineNonPositiveCurrentEPS(8正+最新亏损)绿 |
| prism-daily plist 配置路径不一致 | MAJOR | 45d4020 | plist 已对齐 configs/config.yaml, 与 serve plist/文档一致 |
| 500 回显 err.Error() | MINOR | c065e5e/53dc55a | web 侧"internal error"; api 侧改用 response.Error helper |
| PctlVal 死字段 | MINOR | c065e5e | 已删 |
| TestRefreshBadLatestDate 瞬时红(修复中间态) | — | 6973e44 | parse-first 修正, 连跑绿 |

## 遗留项(共识处置, 不阻塞 M1 交付)

- **[MAJOR→M2 横切 ticket] /prism/detail 在设 ATLAS_API_KEY 时图表 401 空白**: /api/prism/* 在 authMiddleware 下, detail 页浏览器 fetch 无 X-API-Key。裁定为**继承性缺陷**(与既有 /symbols/ symbol_detail.html 同源, 非 Prism 回归)。处置: deployment.md:299-300 已文档化「已知限制」, 另开 M2 ticket 与既有页统一整改(Prism 只读 API 改 same-origin 免鉴权)。QA 认同该共识处置。
- **[MINOR→M2] status 分级 api/web 双实现**: 当前口径一致, 抽共享分类器收敛记 M2 技术债。
- **[MINOR/INFO] ps_ttm 只写不读 / ReconstructPESeries 空序列静默成功 / NaN 进度条 width:—% 外观**: 均记 M2 follow-up 或注释意图, 不阻塞。

## Skeptic/Minimalist 已验证健全项(记录, 非缺陷)
NaN↔NULL 往返对称; 幂等(ON CONFLICT DO UPDATE + 同日零请求跳过); Board 关联子查询每标的恰一行; Series 参数化查询正确; RollingPercentile 窗口边界正确; 窄接口/upsertMeta/alignPE 抽取均为合理抽象。

## 结论
Prism M1 全部 10 任务 verified, 全量测试绿、vet 干净、build OK。CRITICAL 与正确性 MAJOR 全部闭环, 遗留 MAJOR 有共识处置。**判定 PASS, 进入最终验收。** 遗留技术债已登记 M2, 建议 final-report 一并记录。
