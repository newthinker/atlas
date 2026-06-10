# 项目开发规范

## 项目信息

本项目任务看板通过软链接关联到 Obsidian：
- 看板文件: `.kanban/_kanban.md`
- 任务详情: `.kanban/tasks/TASK-XXX.md`

## 开发工作流

### 开始任务

当我说"开始 TASK-XXX"时：
1. 读取 `.kanban/_kanban.md` 确认任务
2. 将任务移动到 Development 列
3. 更新任务详情的 status 和 started_dev
4. 创建 Git 分支: `git checkout -b feature/TASK-XXX-描述`
5. 在进度记录添加日志

### 开发过程

代码提交时关联任务：
```
feat(TASK-XXX): 具体改动描述
```

### 完成开发

当我说"提测 TASK-XXX"时：
1. 将任务移动到 Testing 列
2. 更新任务详情

### 关闭任务

当我说"完成 TASK-XXX"时：
1. 将任务移动到 Done 列
2. 添加完成标记

## 命令清单

| 命令 | 说明 |
|------|------|
| `看板` | 显示当前项目看板 |
| `开始 TASK-XXX` | 开始任务，创建分支 |
| `提测 TASK-XXX` | 完成开发，进入测试 |
| `完成 TASK-XXX` | 关闭任务 |

## Git 规范

分支: `feature/TASK-XXX-简短描述`
提交: `<type>(TASK-XXX): <description>`
