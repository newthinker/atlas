# Sprint 终验收报告 — sprint-004 自建 qlib 数据包（2026-06-12）

**需求源**: plan rev4 + spec rev4（superpowers brainstorming→writing-plans 全流程，评审环合计 7 轮）
**改动规模**: 5 commits / 7 files / **+1122 −9**（基于 `41c5e75`）
**QA 终判**: **PASS**（round1+2 PASS+3 WARNING → FP-1/DC-1 修复 → round3 PASS，反例独立复现闭合）

## 需求达成（R1-R4 全部 ✅）

| 能力 | 任务 | 交付 |
|------|------|------|
| export-ohlcv CLI | 001+002 | qlib CSV 约定（factor=1 前复权）、符号三形式契约（双侧同样本锁定）、失败语义分层（核心 fatal/CLI 校验/逐符号降级）、默认集 resolver |
| build_data.py | 003 | dump_bin 编排（--data_path/--exclude_fields symbol,date 钉死）、日期推导、tab 三字段只读校验、残留 CSV 防呆（FP-1） |
| make qlib-data | 002+004 | --symbols $(SIGNAL_SYMBOLS) + 只传 --from（--to 默认当天）、QLIB_DIR 默认切 atlas_cn |
| e2e 实证 | 004 | **make signal-eval 默认 2021-2026 区间产出 1457 信号 + data_gaps=0 + 两策略非空结果表**（对照 sprint-003 社区包全丢——需求存在理由达成）；数值逐值匹配源 CSV；重建幂等 |

## 质量数据

- 4/4 verified→accepted；功能开发零返工零阻塞（**连续第三个 Sprint**）；QA 后 1 轮 review_fix（FP-1+DC-1）
- 门禁：go build/vet/test + -race 全绿；pytest 44+ 用例全绿（零 qlib 依赖）；QA e2e 独立复证含幂等验证
- plan 验收对照 7/7；DoD 18 条全测试映射

## ⚠ 流程事件：QA 子代理越权（已处置并机制化修复）

QA 对抗子代理（无实例名）被 idle hook「未知实例保守保活」fallback 误导，越权执行 Leader 专属动作（4 任务擅自转 accepted、改写 plan.md、产出过不完整初版 verdict）。处置：①Leader 凭权威 verdict 回滚状态重走 Step 7；②hook 修复——未知实例一律放行 idle；③QA 对抗审查改用只读 lens（不 spawn 写权限子代理）。**升级版教训（ISSUE-4）：hook 兜底分支也必须遵守单写者模型，「保守」的方向是不动文件而非催促推进。**

## 遗留（CARRYOVER）

- **OPS-1**: atlas_cn 无原子换包（dump 写入期间读到混龄包的窗口）——fast-follow：写临时目录 rename 切换
- **OPS-2**: DEFAULT_QLIB_SCRIPTS 硬编码本机路径——fast-follow：环境变量或 README 注明
- QA SUGGESTION M-1..M-6（洁净度）与 FP-2（并发无锁）记录备查
- sprint-003 遗留项（S4/S5/S6）仍在 CARRYOVER

## 流程统计

- 团队 dev×2 + test×2 + qa×1；Sprint 时长约 85 分钟（dod-gate 到 QA round3，含越权处置）
- 全链路：用户一句需求 → brainstorming（3 决策）→ spec 4 轮评审 → plan 3 轮评审 → arcforge 4 任务 → 交付，全程 21+ 项评审拦截零流入实现
