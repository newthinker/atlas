# QA Verdict — percentile-step feature (Round 2 结论)

**审查者**: qa-agent-1  
**日期**: 2026-06-12  
**汇总**: Round 1（工具门禁）+ Round 2（纯 Claude 对抗）  
**任务范围**: TASK-001~007（全部 verified）

---

## 总体结论

### **VERDICT: CONDITIONAL PASS** ⚠️

代码核心逻辑**正确无误**，设计符合预期，但存在 **3 个 CRITICAL 风险**和 **3 个 WARNING 项**需立即或近期处理。

---

## 发现总结

### 按严重程度

| 级别 | 数量 | 内容 |
|------|------|------|
| **CRITICAL** | 3 | pctGates 泄漏、sideOf 防守缺失、passesDispatchGate 注释不足 |
| **WARNING** | 3 | 配置告警不足、Metadata stringly-typed、配置文件一致性 |
| **SUGGESTION** | 3 | 可观测性、边界测试、默认值文档化 |

### 按交付影响

| 影响域 | 评估 | 说明 |
|--------|------|------|
| **功能正确性** | ✅ | 通过所有 done_criteria；Router/Config/App/Strategy 分层逻辑一致 |
| **长期稳定性** | ⚠️ | pctGates 无清理例程→内存泄漏风险（CRITICAL #1） |
| **扩展安全性** | ⚠️ | sideOf 对新 action 无穷举→潜在状态污染（CRITICAL #2） |
| **代码可维护性** | ⚠️ | 代码注释不足、stringly-typed 键、隐性假设（CRITICAL #3 + WARNING #5） |
| **运维体验** | ⚠️ | 配置变更告警不足、多配置文件无优先级说明（WARNING #4、#6） |
| **测试完整性** | ✅ | 10 核心场景全覆盖；86.8% 路由层覆盖率；go test/vet/-race 全绿 |

---

## 立即行动项（CRITICAL）

### 1️⃣ pctGates 清理例程激活（高优先）
- **文件**: internal/router/router.go、internal/app/app.go
- **动作**: 
  - ① 在 app.Start() 或 main() 显式调用 `r.StartCleanupRoutine(ctx, 1*time.Hour)`
  - ② 补充单测验证清理例程生效
- **时间**: 合并前

### 2️⃣ sideOf 穷举检查（高优先）
- **文件**: internal/router/router.go
- **动作**:
  - ① 改 sideOf 为 switch/case 穷举，default panic
  - ② 补充测试穷举 core.Action 所有值
- **时间**: 合并前

### 3️⃣ passesDispatchGate 代码注释（高优先）
- **文件**: internal/router/router.go
- **动作**: 补充决策树注释（见审查报告 CRITICAL #3）
- **时间**: 合并前

---

## 近期改进项（WARNING）

### 4️⃣ 配置变更告警（中优先）
- **文件**: docs、internal/app/app_test.go
- **动作**: 补充 CHANGELOG、logger.Warn、测试注释
- **时间**: 下一版本

### 5️⃣ Metadata 常量化（中优先）
- **文件**: internal/core/signal_metadata.go（新建）、router.go、strategy/**
- **动作**: 定义 MetadataKey* 常量，替换字符串字面量
- **时间**: 下一版本

### 6️⃣ 配置文件一致性（中优先）
- **文件**: configs/{config.example.yaml, percentile-watchlist.yaml}
- **动作**: 明确优先级或分离职责；补充集成测试
- **时间**: 当前版本

---

## 后续优化（SUGGESTION）

- SUGGESTION #7: GetStats 可观测性增强（非阻断）
- SUGGESTION #8: 边界测试补充（symbol 含 `|`、纯强信号、并发）
- SUGGESTION #9: 配置默认值文档化（非阻断）

---

## 风险评级矩阵

```
可能性  ×  严重程度  →  总风险
─────────────────────────────
高      ×  高       →  CRITICAL #1（内存泄漏，长期运行累积）
高      ×  高       →  CRITICAL #2（新 action 无防守，可被引入）
中      ×  高       →  CRITICAL #3（维护风险，误配可能）
中      ×  中       →  WARNING #4（运维体验降低）
中      ×  中       →  WARNING #5（未来 bug 触发率高）
低      ×  中       →  WARNING #6（多环境部署时暴露）
```

---

## 交付决策

### 如果立即修复 CRITICAL 三项：
✅ **可以合并** → final-report → accepted

### 如果暂不修复 CRITICAL：
⛔ **不建议合并** → 需 review-fix 循环

---

## 后续 Leader 行动

1. **确认 CRITICAL 修复计划**：是立即修（本轮）还是 review-fix 循环？
2. **分配修复任务**（如有）：dev-agent 修复 → test-agent 复验 → qa-agent 再审
3. **推进终态**：verified → accepted → final-report → 归档

---

*QA 审查完成 | qa-agent-1 | 2026-06-12*
