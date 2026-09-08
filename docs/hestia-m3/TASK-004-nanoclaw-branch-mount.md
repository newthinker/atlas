# TASK-004（Arcforge）/ TASK-003（需求文档）交付记录：nanoclaw 分支、挂载、触发段

> 承载形态见 `.arcforge/docs/01-design/design-spec.md` §3：本文件**不是摘要，是证据载体**。
> 验证者不以本文件为准——按 ③节给出的全 sha 锚点去 nanoclaw 实跑。
>
> **编号映射**：Arcforge TASK-004 = 需求文档 `2026-09-06-hestia-m3-warp-hestia.md` 的「## TASK-003」节。
> atlas commit subject 用 Arcforge 编号（`docs(TASK-004):`），代码/文档注释里的里程碑编号用需求编号（`M3 的 TASK-003`）。
>
> **采样纪律**：本文件所有 sha256 / sha / 计数均在**最后一次改动之后统一重采**，与 ③节锚点同一时刻。

---

## ① 改动清单

### 概览

| 文件 | git 状态 | 改动 |
| --- | --- | --- |
| `/Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/container.json` | **未跟踪**（`.gitignore:15` `groups/*`） | `additionalMounts` 追加 hestia-queue 一项（保留 vault） |
| `/Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/CLAUDE.local.md` | **未跟踪**（`.gitignore:15` + `.gitignore:19` `**/CLAUDE.local.md`，双重命中） | 末尾追加「## 金融数据解读（Hestia）」段 |
| `~/.config/nanoclaw/mount-allowlist.json` | 不在仓库内 | **未改**（硬约束：agent 绝不修改，片段见 ④节 A） |
| `/Users/zuowei/workspace/ai/nanoclaw/data/v2.db` | **未跟踪**（`.gitignore` `data/`） | **未改**（见 ④节 B：这才是真正生效的地方，SQL 留给人执行） |

两个被改文件都不被 git 跟踪 ⇒ **无 `git show --numstat` 可用**，按 design-spec §3 的替代口径给 `sha256sum` 前后值 + 完整 diff。

### 未跟踪状态的证据（原样粘贴）

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw ls-files groups/cli-with-warp
$ echo "EXIT=$?"
EXIT=0
```
（无输出 ⇒ 该目录下 0 个被跟踪文件；**命中条数 = 0**）

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw check-ignore -v \
    groups/cli-with-warp/container.json groups/cli-with-warp/CLAUDE.local.md
.gitignore:15:groups/*	groups/cli-with-warp/container.json
.gitignore:15:groups/*	groups/cli-with-warp/CLAUDE.local.md
```

```
$ grep -n 'groups/\*\|CLAUDE.local.md' /Users/zuowei/workspace/ai/nanoclaw/.gitignore
15:groups/*
18:# per-group memory (CLAUDE.local.md) must never be committed.
19:**/CLAUDE.local.md
```

### 1.1 `container.json`

**备份路径**（error_handling 要求）：`/Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/container.json.bak-2026-09-08`

| | sha256 |
| --- | --- |
| 改动前（= 备份文件，逐字节相同） | `8cd094770a23ce8b60a9c7d291ccd46a63aaae79e1903db8ba83745bccb21e97` |
| 改动后 | `23f602a810854fc6152cdf7e9864196362fe92e348561c199d35fdc749c22add` |

**`additionalMounts` 改动前全文**：

```json
  "additionalMounts": [
    {
      "hostPath": "/Users/zuowei/Obsidian/ClawdVault",
      "containerPath": "vault",
      "readonly": true
    }
  ],
```

**`additionalMounts` 改动后全文**（vault 项原样保留，新项追加在后）：

```json
  "additionalMounts": [
    {
      "hostPath": "/Users/zuowei/Obsidian/ClawdVault",
      "containerPath": "vault",
      "readonly": true
    },
    {
      "hostPath": "/Users/zuowei/workspace/runtime/atlas/queue/hestia",
      "containerPath": "hestia-queue",
      "readonly": false
    }
  ],
```

**diff（原样粘贴）**：

```diff
$ diff -u container.json.bak-2026-09-08 container.json
--- container.json.bak-2026-09-08	2026-09-08 11:20:17
+++ container.json	2026-09-08 11:20:29
@@ -10,6 +10,11 @@
       "hostPath": "/Users/zuowei/Obsidian/ClawdVault",
       "containerPath": "vault",
       "readonly": true
+    },
+    {
+      "hostPath": "/Users/zuowei/workspace/runtime/atlas/queue/hestia",
+      "containerPath": "hestia-queue",
+      "readonly": false
     }
   ],
   "skills": "all",
```

改动手法是**行级 splice**（锚点唯一性 `assert count == 1`），不是 JSON round-trip 整文件重写 ⇒ 其余字段（`mcpServers` / `packages` / `imageTag` / `skills` / `groupName` / `assistantName` / `agentGroupId`）字节不变，diff 里只有 +5 行、0 删除，可核。

### 1.2 `CLAUDE.local.md`

**备份路径**（非 DoD 要求，同为未跟踪文件、git 无法回滚，故一并备份）：
`/Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/CLAUDE.local.md.bak-2026-09-08`

| | sha256 |
| --- | --- |
| 改动前（= 备份文件） | `2ef704db62e776337e558a8797b3058d27fef990cb96f4d4591be66a4293f39e` |
| 改动后 | `3ec592e68932d34a54d91b78686fa2cc8f51cbf415a6e6d39258071b182444a3` |

**diff（原样粘贴）**：

```diff
$ diff -u CLAUDE.local.md.bak-2026-09-08 CLAUDE.local.md
--- CLAUDE.local.md.bak-2026-09-08	2026-09-08 11:20:58
+++ CLAUDE.local.md	2026-09-08 11:20:58
@@ -15,3 +15,10 @@
 遇到模糊的"帮我查/调研/对比/评估/梳理"类请求,**加载并遵循 `warp-research` skill**(读其 `SKILL.md` 主流程)。
 单次一句话能答的简单事实查询不触发调研流程,直接答即可。
 调研产物按该 skill 的 `archive-format` 经 `selvage_call("spool.archive", …)` 归档到 `Wiki/Loom-Research/`。
+
+## 金融数据解读（Hestia）
+遇到「处理 hestia 队列」「生成金融数据解读」「解读这期央行数据」类请求，**加载并遵循 `warp-hestia` skill**（读其 `SKILL.md` 主流程，按六步走，不跳步）。
+- 契约队列在 `/workspace/extra/hestia-queue/`（**可写**，这是本组唯一的可写挂载）：只动 `pending/ processing/ done/ failed/` 里的文件，不建别的目录。
+- 写回只经 `selvage_call("spool.archive", …)`，`params` 多传一项 `source: "hestia"`，路径在 `Wiki/Macro/PBOC/` 下。
+- **一次只处理一份**；处理完回复「已写入 <路径>，队列还剩 N 份」，等用户再说一句。
+- 笔记的 frontmatter 与数据表由 `scripts/prepare.py` 生成，**你只写 `<!-- narrative -->` 段落**；写回前必须跑 `scripts/verify.py`。
```

- **位置**：新段落追加在文件末尾，而「## 调研」段（原 14-17 行）是原文件的最后一节 ⇒ 满足 DoD「在『## 调研』段**之后**追加」。diff hunk 头 `@@ -15,3 +15,10 @@` 可核（原文件 17 行，新增 7 行含一个空行分隔）。
- **内容**：逐字照抄需求文档「Step 3」代码块（1 条引导句 + 4 条 bullet = DoD 说的「五条」）。原文用全角标点，与本组既有段落的半角风格不同——**保持需求原文原样**，不做标点归一化。

---

## ② 实测输出

以下全部采于**最后一次改动之后**，与 ③节锚点同一时刻。

### 2.1 worktree 建成（functional①）

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw fetch fork
$ echo "EXIT=$?"
EXIT=0

$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse fork/main
d791101e1defcfd3d3d3fcf7ae32a85ec420547b

$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree add -b feat/warp-hestia \
      /Users/zuowei/workspace/ai/wt-warp-hestia fork/main
Preparing worktree (new branch 'feat/warp-hestia')
branch 'feat/warp-hestia' set up to track 'fork/main'.
HEAD is now at d791101 docs(skills): 微博段加 mcporter get_trendings + NO_PROXY 直连姿势
EXIT=0
```

**先例命中**（DoD 明写「必须在」）：

```
$ ls -l /Users/zuowei/workspace/ai/wt-warp-hestia/container/skills/warp-research/SKILL.md
-rw-r--r--@ 1 zuowei  staff  6164 Sep  8 11:16 /Users/zuowei/workspace/ai/wt-warp-hestia/container/skills/warp-research/SKILL.md
EXIT=0
```

**worktree 列表**：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree list
/Users/zuowei/workspace/ai/nanoclaw        aefea6c [feat/vendor-agent-reach-skill]
/Users/zuowei/workspace/ai/wt-warp-hestia  d791101 [feat/warp-hestia]
```

### 2.2 主 checkout 未被触碰（functional① 的红字约束）

开工前与全部改动之后，主 checkout 的分支与工作区状态**逐字相同**：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse --abbrev-ref HEAD
feat/vendor-agent-reach-skill
$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse HEAD
aefea6ce5439cfcf15b3dc54ea9bd507c5846c95
$ git -C /Users/zuowei/workspace/ai/nanoclaw status --short
 M CLAUDE.md
 M container/agent-runner/src/mcp-tools/index.ts
 D groups/global/CLAUDE.md
 D groups/main/CLAUDE.md
?? .claude/skills/gitnexus/
```

即 2 改 2 删 1 未跟踪，与开工前一致（未 checkout、未 stash、未提交）。
`groups/cli-with-warp/` 的两个改动**不出现在 `git status`**，因为它们被 `.gitignore` 忽略——这正是它们「不进 PR」的机制根据。

### 2.3 JSON 合法性（error_handling）

```
$ python3 -c 'import json;json.load(open("/Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/container.json"));print("JSON OK")'
JSON OK
EXIT=0
```

解析成功 ⇒ 未触发「回滚备份 + blocked_clarification」分支。备份文件保留在原地未删。

### 2.4 sha256 汇总（一次命令，同一时刻）

```
$ shasum -a 256 container.json container.json.bak-2026-09-08 CLAUDE.local.md CLAUDE.local.md.bak-2026-09-08
23f602a810854fc6152cdf7e9864196362fe92e348561c199d35fdc749c22add  container.json
8cd094770a23ce8b60a9c7d291ccd46a63aaae79e1903db8ba83745bccb21e97  container.json.bak-2026-09-08
3ec592e68932d34a54d91b78686fa2cc8f51cbf415a6e6d39258071b182444a3  CLAUDE.local.md
2ef704db62e776337e558a8797b3058d27fef990cb96f4d4591be66a4293f39e  CLAUDE.local.md.bak-2026-09-08
```

### 2.5 skill 分发方式的实测证据（boundary ④，详见 ④节 C）

```
$ grep -n '^COPY\|^ADD' /Users/zuowei/workspace/ai/wt-warp-hestia/container/Dockerfile
85:COPY agent-runner/package.json agent-runner/bun.lock ./
113:COPY cli-tools.json install-cli-tools.sh /tmp/
152:COPY entrypoint.sh /app/entrypoint.sh
```
（**COPY/ADD 命中 3 条，无一条涉及 `skills`**）

```
$ sed -n '9,11p' /Users/zuowei/workspace/ai/wt-warp-hestia/container/Dockerfile
# Source is never baked in — /app/src is provided by a shared read-only
# bind mount at runtime (see src/container-runner.ts). Source-only changes
# never require an image rebuild.
```

```
$ sed -n '346,349p' /Users/zuowei/workspace/ai/wt-warp-hestia/src/container-runner.ts
  // Shared skills — read-only, symlinks in .claude-shared/skills/ point here.
  const skillsSrc = path.join(projectRoot, 'container', 'skills');
  if (fs.existsSync(skillsSrc)) {
    mounts.push({ hostPath: skillsSrc, containerPath: '/app/skills', readonly: true });
  }
```

```
$ grep -n 'projectRoot' /Users/zuowei/workspace/ai/wt-warp-hestia/src/container-runner.ts
273:  const projectRoot = process.cwd();
343:  const agentRunnerSrc = path.join(projectRoot, 'container', 'agent-runner', 'src');
347:  const skillsSrc = path.join(projectRoot, 'container', 'skills');
```

**先例给出的现成反证**（不是推理）——`warp-research` 在 `fork/main` 有、在主 checkout 的工作树里没有，于是 Warp 组的容器至今看不到它：

```
$ ls /Users/zuowei/workspace/ai/nanoclaw/container/skills/
agent-browser
agent-reach
frontend-engineer
onecli-gateway
self-customize
slack-formatting
vercel-cli
welcome
whatsapp-formatting

$ ls /Users/zuowei/workspace/ai/nanoclaw/container/skills/ | wc -l
       9

$ ls -l /Users/zuowei/workspace/ai/nanoclaw/container/skills/warp-research/
ls: /Users/zuowei/workspace/ai/nanoclaw/container/skills/warp-research/: No such file or directory
EXIT=1

$ ls /Users/zuowei/workspace/ai/nanoclaw/data/v2-sessions/ag-1782440732275-pqkuxs/.claude-shared/skills/
agent-browser
agent-reach
frontend-engineer
onecli-gateway
self-customize
slack-formatting
vercel-cli
welcome
whatsapp-formatting

$ ls /Users/zuowei/workspace/ai/nanoclaw/data/v2-sessions/ag-1782440732275-pqkuxs/.claude-shared/skills/ | wc -l
       9
$ ls /Users/zuowei/workspace/ai/nanoclaw/data/v2-sessions/ag-1782440732275-pqkuxs/.claude-shared/skills/ | grep -c warp
0
```

两处都是 **9 项、`grep -c warp` 命中 0**：主 checkout 的 `container/skills/` 里没有 `warp-research`
（它只在 `fork/main` 上），Warp 组 `.claude-shared/skills/` 里的 9 个条目全是指向 `/app/skills/<name>`
的符号链接（`ls -la` 可见 `agent-browser -> /app/skills/agent-browser` 等），同样没有 `warp-research`。

### 2.6 队列根与 allowlist 现状（boundary ②）

```
$ ls -d /Users/zuowei/workspace/runtime/atlas/queue/hestia
ls: /Users/zuowei/workspace/runtime/atlas/queue/hestia: No such file or directory
EXIT=1

$ ls -d /Users/zuowei/workspace/runtime/atlas/queue
ls: /Users/zuowei/workspace/runtime/atlas/queue: No such file or directory

$ ls -d /Users/zuowei/workspace/runtime/atlas
/Users/zuowei/workspace/runtime/atlas
```
⇒ 队列根**及其上一级 `queue/` 都不存在**，只有 `runtime/atlas/` 在。

```
$ cat ~/.config/nanoclaw/mount-allowlist.json
{
  "allowedRoots": [
    {
      "path": "/Users/zuowei/Obsidian/ClawdVault",
      "allowReadWrite": false,
      "description": "Loom Plan 1: Obsidian vault (RO for Warp)"
    }
  ],
  "blockedPatterns": [],
  "nonMainReadOnly": true
}
```
**未修改**（本节仅为记录现状，供 ④节 A 的粘贴片段定位）。

### 2.7 DB 现状（④节 B 的依据）

```
$ sqlite3 /Users/zuowei/workspace/ai/nanoclaw/data/v2.db \
    "SELECT agent_group_id, additional_mounts, skills, image_tag FROM container_configs;"
ag-1782389975230-deeugq|[]|"all"|
ag-1782440732275-pqkuxs|[{"hostPath":"/Users/zuowei/Obsidian/ClawdVault","containerPath":"vault","readonly":true}]|"all"|nanoclaw-agent:agentreach-h3
```
（**只读查询，未做任何写入**；`ag-1782440732275-pqkuxs` = `container.json` 里的 `agentGroupId`，即 Warp 组）

---

## ③ 锚点

| 项 | 值 |
| --- | --- |
| nanoclaw worktree **绝对路径** | `/Users/zuowei/workspace/ai/wt-warp-hestia` |
| nanoclaw worktree 分支 | `feat/warp-hestia`（tracking `fork/main`） |
| nanoclaw worktree HEAD **全 sha** | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
| nanoclaw 主 checkout 路径 | `/Users/zuowei/workspace/ai/nanoclaw` |
| nanoclaw 主 checkout 分支 / HEAD 全 sha | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95` |
| nanoclaw PR 链接 | **无**（本任务零提交，见 ④节 D） |
| atlas 基线 HEAD 全 sha（本文件写于其上） | `fe95ac707ea46f9939bb6554261830a95f1b93e1` |
| atlas worktree / 分支 | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-004-m3` / `task/TASK-004-m3` |

**worktree 状态**（建成后未提交任何东西，交给下游时是干净的 `fork/main`）：

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia status --short
$ echo "EXIT=$?"
EXIT=0
```
（无输出 ⇒ 工作区干净）

> 🔴 **下游 TASK-005 / 006 / 007 直接复用上面这个 worktree，不要新建**；TASK-007 负责 `git -C /Users/zuowei/workspace/ai/nanoclaw worktree remove /Users/zuowei/workspace/ai/wt-warp-hestia`。
> 同样的路径与分支已写进 `.arcforge/discoveries/TASK-004.json` 的 `interfaces_exposed`。

---

## ④ 未做与理由

### A. `~/.config/nanoclaw/mount-allowlist.json` —— **未改**（硬约束）

需求全局约束 + CLAUDE.md 运行时资产纪律：agent 绝不修改该文件。以下片段供**人**粘贴到
`allowedRoots` 数组（保留既有 ClawdVault 项，追加在后）：

```json
    {
      "path": "/Users/zuowei/workspace/runtime/atlas/queue/hestia",
      "allowReadWrite": true,
      "description": "Hestia M3: contract queue (RW for Warp)"
    }
```

粘贴后 `allowedRoots` 应为 2 项。三条附带事实：

1. **队列根当前不存在**（②节 2.6，连上一级 `queue/` 都没有）。冒烟前须由 Atlas 侧 `contract emit`
   造出（`EnsureQueueDirs` 随之建 `pending/ processing/ done/ failed/`）。
2. 🔴 **mount-security 对不存在 hostPath 的行为——运行时未验证，但代码给出了明确答案，且与需求文档的猜测不同。**
   需求文档说「要在冒烟第一步看日志（`Mount forced to read-only` / `not under any allowed root`）」，
   而按代码读，**两条都不会出现**，实际走的是第三条路径：

   | 环节 | 代码位置（`/Users/zuowei/workspace/ai/wt-warp-hestia`，@ `d791101…`） | 行为 |
   | --- | --- | --- |
   | `getRealPath()` | `src/modules/mount-security/index.ts:137-143` | `fs.realpathSync` 抛异常 → 返回 `null` |
   | `validateMount()` | 同文件 `:256-260` | `realPath === null` ⇒ `{allowed:false, reason:'Host path does not exist: "<path>" (expanded: "<path>")'}`，**在 blockedPatterns / allowedRoots 检查之前就返回** |
   | `validateAdditionalMounts()` | 同文件 `:346` | 走 `log.warn('Additional mount REJECTED', …)`，该挂载**被整条丢弃**，容器照常启动 |
   | `findAllowedRoot()` | 同文件 `:171-179` | 另有一条：**allowedRoot 本身不存在也会被 `continue` 跳过** ⇒ 即使 allowlist 粘好了，队列根不存在时那条 root 等于不生效 |

   ⇒ **结论：队列根必须在容器 spawn 之前就存在**，否则挂载静默消失（只在日志里留一行 warn），
   容器内 `/workspace/extra/hestia-queue/` 根本不出现。冒烟第一步该找的日志关键字是
   **`Additional mount REJECTED`**（不是需求文档写的那两条）。这一条**仅为代码阅读，运行时仍未验证**，
   冒烟时以实际日志为准。
3. 🔴 **粘贴 allowlist 之后必须重启 nanoclaw 进程**（需求文档只提了重启 selvage）：
   `loadMountAllowlist()`（`src/modules/mount-security/index.ts:62-67`）把 allowlist
   **缓存在进程内存里，进程生命周期内不重载**（源码注释原文：`Result is cached in memory for the lifetime of the process.`）。
   ⇒ TASK-006 Step 0 的人执行前置应为**两个重启**：selvage（`launchctl kickstart -k gui/$(id -u)/com.loom.selvage`）
   **与** nanoclaw 主进程。

   附带一条无害观察：现有 allowlist 里的 `"nonMainReadOnly": true` 是**死键**——
   `grep -rn 'nonMainReadOnly' src/ --include='*.ts'` 在 `d791101…` 上**命中 0 条**，
   `MountAllowlist` 接口（`:21-24`）只有 `allowedRoots` / `blockedPatterns`。它是旧版残留，不影响本任务。

**selvage 重启与 allowlist 粘贴均属需求 TASK-006 Step 0 的「人执行」前置，本 sprint 结转，不在本任务范围内。**

### B. 🔴 `container.json` 不是真相源 —— 手改**零效果**，真正生效的地方是 `data/v2.db`（**未改，SQL 留给人**）

这是本任务查出的、与 DoD functional② 直接冲突的机制事实。**DoD 要求的 container.json 改动已按字面完成
（①节 1.1），但它在运行时不参与任何决策。** 证据链：

| 环节 | 代码位置（`/Users/zuowei/workspace/ai/nanoclaw`，主 checkout @ `aefea6c…`；worktree `d791101…` 同） | 事实 |
| --- | --- | --- |
| 模块头注释 | `src/container-config.ts:4` | `Source of truth is the container_configs table in the central DB.` |
| `materializeContainerJson()` | `src/container-config.ts:74-89` | 从 DB 行构造 config，然后 **`fs.writeFileSync` 无条件覆盖** `groups/<folder>/container.json`，并把 config **返回**给调用方 |
| 调用点 | `src/container-runner.ts:129` | `const containerConfig = materializeContainerJson(agentGroup.id);` —— 每次 spawn 都调 |
| 消费点 | `src/container-runner.ts:353-356` | `buildMounts` 用的是**上面那个返回值（DB）**，从不读文件 |
| 回读路径 | `src/backfill-container-configs.ts:29-35` | `backfillContainerConfigs()` 头两行即 `if (getContainerConfig(group.id)) continue;` —— **只对没有 config 行的组**从 container.json 回读；Warp 组已有行（②节 2.7），永不回读 |

⇒ 手改 container.json 有两重失效：**① 在被覆盖之前就已不参与决策**（消费的是 DB 返回值）；
**② 下一次 spawn 会把它覆写回 DB 的内容**，改动消失。

**CLI 也没有出口**：`src/cli/resources/groups.ts` 的 `config` 子命令只支持标量字段
（`--provider/--model/--effort/--image-tag/--assistant-name/--max-messages-per-prompt/--cli-scope`，`:229-241`）
与 `mcp add/remove`、`packages add/remove`；**没有任何 additional_mounts 子命令**。

**⇒ 唯一有效路径是直接改 DB。以下 SQL 未执行，与 A 节的 allowlist 片段同待遇（人执行，TASK-006 Step 0 前置）**：

```sql
-- 先停 nanoclaw 进程再执行，避免与运行中的写并发
UPDATE container_configs
SET additional_mounts = '[{"hostPath":"/Users/zuowei/Obsidian/ClawdVault","containerPath":"vault","readonly":true},{"hostPath":"/Users/zuowei/workspace/runtime/atlas/queue/hestia","containerPath":"hestia-queue","readonly":false}]',
    updated_at = '2026-09-08T00:00:00.000Z'
WHERE agent_group_id = 'ag-1782440732275-pqkuxs';
```

执行前建议 `cp data/v2.db data/v2.db.bak-<日期>`；执行后核实：

```bash
sqlite3 /Users/zuowei/workspace/ai/nanoclaw/data/v2.db \
  "SELECT additional_mounts FROM container_configs WHERE agent_group_id='ag-1782440732275-pqkuxs';"
```

**为什么我没做**：`data/v2.db` 是本机运行时数据库，不在本任务的 `writes` 声明内，
Leader 下发的硬约束也只覆盖 `container.json`（「改 `container.json` 前先备份」）。
改一个正在被进程持有的 sqlite 库属于本任务范围外的不可逆动作，故按 allowlist 的同一原则留给人。
**已于交付前经 inbox 报给 Leader，等其裁决是否要我补做。**

> ⚠️ 给验证者的提示：本节不是「没做完」，是**做了 DoD 要求的事之后发现 DoD 要求的事不足以生效**。
> functional② 的字面要求（保留 vault + 追加一项 + JSON 合法 + 前后 sha256 与 diff）已 100% 完成，见 ①节 1.1 与 ②节 2.3。

### C. skill 如何被容器看到（boundary ④ 的 reviewer G7 问题）—— **已查清：不需要重建镜像**

**结论（三句）**：

1. **skill 不随镜像打包**，是**运行时只读 bind mount**：`src/container-runner.ts:346-349` 把
   `<projectRoot>/container/skills` 挂到容器的 `/app/skills`（`readonly: true`）。
   Dockerfile 的 COPY/ADD 一共 3 条（`:85` / `:113` / `:152`），**没有一条涉及 skills**；
   Dockerfile 头注释 `:9-11` 原文即「Source is never baked in … Source-only changes never require an image rebuild.」
   ⇒ **TASK-005/006 新增 `container/skills/warp-hestia/` 不需要重建镜像**，spec §9 风险表里
   「重建镜像（若 skills 随镜像打包）」这个尾巴可以**划掉**。
2. **每组的可见 skill 由符号链接决定，`"skills": "all"` 会自动纳入新 skill**：
   `syncSkillSymlinks()`（`:371-408`）在 `.claude-shared/skills/` 下按选择集建
   `<name> -> /app/skills/<name>` 的符号链接（宿主上是悬空的，容器内有效）；
   `selectedSkillNames()`（`:414-426`）在 `skills === 'all'` 时**从 `container/skills/` 目录重算**。
   Warp 组的 `container.json` / DB 行都是 `"skills": "all"`（②节 2.7）⇒ **不需要改 skills 字段**。
3. 🔴 **但 `projectRoot = process.cwd()`（`src/container-runner.ts:273`），即 nanoclaw 主进程的工作目录 = 主 checkout
   `/Users/zuowei/workspace/ai/nanoclaw`，不是 worktree。**
   ⇒ **TASK-005/006 写在 `/Users/zuowei/workspace/ai/wt-warp-hestia/container/skills/warp-hestia/` 的文件，容器看不见。**
   冒烟之前必须让**主 checkout 的工作树**里存在该目录（合并 `feat/warp-hestia`，或人在主 checkout 上切分支/cherry-pick）。

   **这不是推理，先例已经把它演示了一遍**（②节 2.5）：`warp-research` 在 `fork/main` 上存在，
   而主 checkout 停在 `feat/vendor-agent-reach-skill`（不含它）⇒ 主 checkout 的 `container/skills/` 只有 9 项、
   Warp 组的 `.claude-shared/skills/` 也只有 9 个符号链接、**没有 warp-research**。
   换句话说：**M3 的先例 skill 目前在容器里同样是不可见的**，这不是本任务引入的问题，但它会同样卡住 TASK-005/006 的产物。

   附带：符号链接是在 spawn 时 sync 的，所以文件到位后**起一个新容器**即可，无需重建镜像；但由于
   `process.cwd()` 是进程启动时的目录，若主 checkout 的分支切换发生在 nanoclaw 运行期间，
   目录内容变化会被下次 spawn 读到（`fs.readdirSync` 是实时的），**不需要重启 nanoclaw 进程**——
   与 A 节 3 的 allowlist 缓存不同，那个才需要重启。

### D. nanoclaw 侧**零提交、无 PR** —— 这是预期结果，不是漏做

本任务的两个改动文件（`groups/cli-with-warp/container.json`、`groups/cli-with-warp/CLAUDE.local.md`）
**都被 `.gitignore` 忽略**（①节给了 `ls-files` 空输出 + `check-ignore -v` 双证），
需求文档 Step 5 的 `git add … # 若被跟踪` 这个条件**为假**。

⇒ **本任务在 nanoclaw git 上无提交，PR 内容为空，③节的 PR 链接为「无」。**
产出是三样：**① 一个 worktree（`/Users/zuowei/workspace/ai/wt-warp-hestia` @ `d791101e1defcfd3d3d3fcf7ae32a85ec420547b`，分支 `feat/warp-hestia`）
② 两个本机未跟踪文件的改动（①节的 sha256 + diff 即其全部证据）③ 本文件。**
`feat/warp-hestia` 分支此刻与 `fork/main` 同 sha，是留给 TASK-005/006 提交用的空分支。

> 给验证者：**不要按「PR 链接缺失」判 rejected**（DoD non_functional 第 1 条明写此为预期）。
> 核实路径是照 ③节锚点去 nanoclaw 跑 ②节的命令，比对 sha256。

### E. 需求文档 Step 1 的 `git checkout main && git pull fork main` —— **未执行**（有意偏离）

需求原文 Step 1 是在主 checkout 上切 `main` 再拉取。人类 2026-09-08 裁决（AD-M3-3 / design-spec §5）
改为 **worktree 隔离**：主 checkout 停在 `feat/vendor-agent-reach-skill` 且工作区脏（2 改 2 删），
切分支会动到别人的在途改动。故实际做的是 `git fetch fork` + `git worktree add -b feat/warp-hestia … fork/main`，
效果等价（拿到 `fork/main` 的最新提交 `d791101…`）而不碰主工作区（②节 2.2 给了前后一致的证据）。
需求 Step 1 里的 `git status --short 必须干净；不干净先 stash` 因此**不适用**，未 stash。
