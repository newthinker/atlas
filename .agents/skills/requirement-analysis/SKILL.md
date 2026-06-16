---
name: requirement-analysis
description: 解析 Markdown 需求文档，生成结构化任务列表（含 done_criteria、依赖图、wave）。Leader 在需求分析阶段使用。
---

# 需求分析 Skill

## 输入
- Markdown 格式的需求说明文档（默认 `requirements.md`）。

## 分析步骤

### 1. 需求理解
- 提取核心功能列表。
- 识别非功能性需求（性能、安全、可用性）。
- 标注模糊或缺失的需求点。

### 2. 模块识别
- 将功能划分为独立模块。
- 识别模块间的接口和依赖。

### 3. 任务拆分与完成标准定义
每个任务输出为 JSON（写入 `.arcforge/tasks/TASK-xxx.json`）：

```json
{
  "id": "TASK-001",
  "title": "实现用户注册接口",
  "description": "提供 POST /api/v1/register 接口，接受邮箱和密码，完成注册并返回 JWT",
  "module": "user-service",
  "complexity": "medium",
  "estimated_files": ["pkg/user/handler.go", "pkg/user/service.go", "pkg/user/repository.go"],
  "done_criteria": {
    "functional": [
      "POST /api/v1/register 接受 {email, password} 并返回 201 + JWT token",
      "注册成功后用户数据持久化到数据库",
      "返回的 JWT token 包含 user_id 和 email claims，有效期 24 小时"
    ],
    "boundary": [
      "邮箱格式不合法时返回 400 + 具体错误信息",
      "密码少于 8 位或不含数字+字母时返回 400",
      "邮箱为空或密码为空时返回 400",
      "邮箱已注册时返回 409 Conflict"
    ],
    "error_handling": [
      "数据库连接失败时返回 500 + 通用错误信息（不暴露内部细节）",
      "JWT 签名失败时返回 500 并记录日志"
    ],
    "non_functional": [
      {"desc": "密码使用 bcrypt 存储，cost >= 10", "verify_by": "test"},
      {"desc": "接口响应时间 < 200ms（不含数据库延迟）", "verify_by": "benchmark"}
    ]
  },
  "dependencies": [],
  "wave": 1,
  "context_from": [],
  "status": "pending",
  "assigned_to": null,
  "discovery": ".arcforge/discoveries/TASK-001.json",
  "packages": ["./pkg/user"],
  "assignment_epoch": 0,
  "rework_count": 0,
  "questions": [],
  "verifier": null
}
```

**新字段说明（并发协调内核）：**
- `packages`：任务声明的 Go package 范围（TaskCompleted hook 的门禁输入）。
  Leader 拆分时从 `estimated_files` 的目录推导初始值；Dev 首次写代码前如发现
  实际涉及 package 与声明不符，**先更新此字段再动手**（防护性写入）。
  同一时刻所有在途任务的 packages 必须两两不相交且非空（validator 强制）。
- `assignment_epoch`：Leader 每次（重）派 +1；Dev 写文件前在任务锁临界区内校验
  （认领协议见 AGENTS.md）。
- `rework_count`：每次从 `rejected`/`review_fix` 重派回 Dev 时 +1，
  超过 `max_rework` 转 `blocked_human`。
- `questions`：`blocked_clarification` 状态下的澄清请求
  `{"q", "asked_at", "answer"}`，Leader 周期扫描答复。
- `verifier`：Leader 派验时写入的 Test Agent 实例名（TeammateIdle 按实例过滤用）。

**任务图字段说明（借鉴 CCW 任务图三元组）：**
- `dependencies`：本任务依赖的 task id 列表（DAG，不能成环）。
- `wave`：并行批次号。同一 `wave` 的任务可并行；约束 `本任务.wave > 所有依赖.wave`。
- `context_from`：开工前需加载的上游任务 id 列表——读它们的 `discovery` 文件获取上游决策/产物。
- `discovery`：本任务完成时写出的结构化发现文件路径。

**verify_by 标注**（每条标准可为纯字符串——视同 `verify_by: "test"`——或带标注的对象）：

| verify_by | 含义 | 核对方 |
|---|---|---|
| `test` | 可转化为单测断言（默认） | Dev 写测试，Test Agent 矩阵核对 |
| `benchmark` | 需 `go test -bench` 或专项压测 | Dev 写 benchmark，Test Agent 看输出数字，不计入覆盖矩阵 |
| `review` | 只能靠人/Agent 审查判断 | QA 第一轮清单核对，矩阵标 N/A |
| `manual` | 需人工验证（如 UI 体验） | 列入验收阶段清单 |

Leader 拆分时若某任务 `review`/`manual` 条目占比过高，提示该任务可能不适合全自动流程。
不要逼 Dev 为不可单测的标准写形式化断言（fantasy assertion）。

**完成标准编写原则：**
- 每条标准必须是**可测试的**，能直接转化为一个断言。
- 使用具体的输入/输出描述，避免模糊表述如"正确处理错误"。
- functional 覆盖核心 Happy Path；boundary 覆盖输入边界/空值/极值；
  error_handling 明确期望的错误码和行为；non_functional 明确可量化指标。

**Realistic Scope 约束**：每任务 ≤ 1 个 package、`done_criteria` 总条数 ≤ 8、预计改动文件 ≤ 5。
超出则继续拆分。

### 4. 依赖图与 wave
生成任务依赖关系，标注哪些可并行、哪些必须串行，分配 `wave`。
**拆分完成后运行 Go validator 校验任务图**：

```bash
go run ./validator/cmd/arcforge-validate .arcforge/tasks
```

### 5. 团队规模建议
根据独立任务链/同 wave 任务数量建议 Dev Agent 数量（不超过 `team.max_dev_agents`）。
