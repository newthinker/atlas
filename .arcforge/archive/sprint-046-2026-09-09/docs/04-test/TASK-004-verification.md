# TASK-004 验证报告（Sprint M3 · nanoclaw 分支/挂载/触发段）

- **验证者**：`test-m3-a`　**被验 owner**：`dev-m3-c`　**判定日期**：2026-09-08
- **判定**：✅ **VERIFIED**
- **assignment_epoch**：1（裁决迁移携带 `--expect-epoch 1`）
- **verify_baseline**：`head = 0c3fbdc9ac7b12fcb65d6d2695541f55820cd03c`，`discovery_sha256 = 0db59c898ee0f8bf2e30cfbedcaa87cca1492b136588f381235aa5f6df475665`

> **本报告的证据全部由验证者自己实跑产出**，不采信交付文档与 discovery 的自述。
> 交付文档只被当作「去哪儿看」的索引；每一条结论都在 nanoclaw / atlas 的真实文件与 git 对象上重新求值过。
> 本任务是**跨仓库任务**（AD-M3-1）：atlas 门禁只验那份 `.md`，nanoclaw 侧无任何机制兜底，验证者是唯一的闸。

---

## 一、Done Criteria 覆盖矩阵

DoD 五条 `verify_by: manual` + 一条 `verify_by: review`，无 `test` 条目 ⇒ **不要求测试断言覆盖**，要求的是可复现的实地证据。

| # | 完成标准（摘要） | verify_by | 我实跑的核实 | 判定 |
| --- | --- | --- | --- | --- |
| **functional[0]** | worktree 建成、先例 `warp-research/SKILL.md` 在、路径/分支写进文档③与 discovery `interfaces_exposed`、**不碰主 checkout** | manual | §2.1 + §2.2 | **PASS** |
| **functional[1]** | `container.json` 的 `additionalMounts` 保留 vault 项 + 追加队列项、JSON 合法、sha256 前后值与 diff 进文档① | manual | §2.3 | **PASS** |
| **functional[2]** | `CLAUDE.local.md` 在「## 调研」段**之后**追加「## 金融数据解读（Hestia）」，内容按需求原文五条 | manual | §2.4 | **PASS** |
| **boundary[0]** | `mount-allowlist.json` 不改；④节须写明片段原文 / 队列根不存在与 mount-security 行为未验证 / 结转声明 / **skill 如何进容器（reviewer G7）** | manual | §2.5 + §2.6 | **PASS** |
| **error_handling[0]** | 改 `container.json` 前先备份并在文档①记路径；JSON 解析失败即回滚 + `blocked_clarification` | manual | §2.7 | **PASS** |
| **non_functional[0]** | nanoclaw 侧**零提交、无 PR 是预期结果**，④节须显式写明 | review | §2.8 | **PASS** |
| **non_functional[1]** | 交付流程：worktree → 提交（subject 合门禁约定）→ 预演 merge → 等 merge 进 master 才转 `dev_done`；discovery 先落定；**锚一律全 sha**；自证数字统一重采 | manual | §2.9 | **PASS** |

**结论：7/7 PASS，无未覆盖标准。** 另有 3 条观察见 §3，**均不构成 rejected 依据**。

---

## 二、逐条证据（全部为验证者实跑输出）

### 2.1 functional[0] —— worktree 建成 + 先例在

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree list
/Users/zuowei/workspace/ai/nanoclaw        aefea6c [feat/vendor-agent-reach-skill]
/Users/zuowei/workspace/ai/wt-warp-hestia  d791101 [feat/warp-hestia]

$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia rev-parse --abbrev-ref HEAD
feat/warp-hestia
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia rev-parse HEAD
d791101e1defcfd3d3d3fcf7ae32a85ec420547b
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia rev-parse --abbrev-ref feat/warp-hestia@{upstream}
fork/main
$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse fork/main
d791101e1defcfd3d3d3fcf7ae32a85ec420547b
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia status --short
$ echo "EXIT=$?"
EXIT=0
```

- worktree **实存**于 `/Users/zuowei/workspace/ai/wt-warp-hestia`，分支 `feat/warp-hestia`，HEAD **全 sha 与 discovery `interfaces_exposed` 记的逐字一致**，且 == `fork/main`，tracking 正确，工作区干净（交给下游时无残留）。

**先例命中**：

```
$ ls -l /Users/zuowei/workspace/ai/wt-warp-hestia/container/skills/warp-research/SKILL.md
-rw-r--r--@ 1 zuowei  staff  6164 Sep  8 11:16 …/warp-research/SKILL.md
EXIT=0
```

- **未拆 worktree**（下游 TASK-005/006/007 复用，TASK-007 负责拆）。

### 2.2 functional[0] 的红字硬约束 —— 主 checkout 未被触碰

交付文档给的证据是「前后 `git status --short` 逐字相同」。这条**我无法独立复现**（我看不到「前」），故**换了一件防篡改的载体**：reflog。

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw reflog show HEAD -8 --date=iso
aefea6c HEAD@{2026-07-21 08:52:14 +0800}: commit: docs(agent-reach): Reddit 归 Tier A …
07afdc4 HEAD@{2026-07-20 09:31:57 +0800}: checkout: moving from main to feat/vendor-agent-reach-skill
…（其余 6 条均为 2026-07-20 的 rebase）

$ git -C /Users/zuowei/workspace/ai/nanoclaw stash list
（无输出）  条数=0

$ git -C /Users/zuowei/workspace/ai/nanoclaw log --all --since='2026-09-08 00:00' --format='%h %ad %s' --date=iso
（无输出 ⇒ 今日 nanoclaw 全 ref 零新提交）

$ git -C /Users/zuowei/workspace/ai/nanoclaw log fork/main..feat/warp-hestia --oneline | wc -l
0
```

⇒ **主 checkout HEAD 的 reflog 最后一条是 2026-07-21**，今日（2026-09-08）**零 checkout / 零 reset / 零 commit**；stash 栈为空；`feat/warp-hestia` 相对 `fork/main` 零提交。

现状复核（与文档 2.2 逐字相同）：

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

> **为什么换载体**：`status --short` 前后相同只证明「终态一样」，`git checkout X && git checkout -` 也能满足它；reflog 是 git 自己记的、agent 不经手，它证明的是「**过程中也没动过**」。这是比交付文档给的更强的证据，且指向同一结论。

### 2.3 functional[1] —— `container.json`

```
$ cd /Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp
$ diff -u container.json.bak-2026-09-08 container.json
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

- **+5 / −0**：vault 项与其余七个字段（`mcpServers` / `packages` / `imageTag` / `skills` / `groupName` / `assistantName` / `agentGroupId`）**字节不变**，逐条对照 `cat container.json` 全文确认。
- 新项三个键与需求文档 Step 2 代码块**逐字一致**：`hostPath` / `containerPath: "hestia-queue"` / `readonly: false`。

```
$ python3 -c 'import json;d=json.load(open("container.json"));print("JSON OK")'
JSON OK
EXIT=0

$ shasum -a 256 container.json container.json.bak-2026-09-08 CLAUDE.local.md CLAUDE.local.md.bak-2026-09-08
23f602a810854fc6152cdf7e9864196362fe92e348561c199d35fdc749c22add  container.json
8cd094770a23ce8b60a9c7d291ccd46a63aaae79e1903db8ba83745bccb21e97  container.json.bak-2026-09-08
3ec592e68932d34a54d91b78686fa2cc8f51cbf415a6e6d39258071b182444a3  CLAUDE.local.md
2ef704db62e776337e558a8797b3058d27fef990cb96f4d4591be66a4293f39e  CLAUDE.local.md.bak-2026-09-08
```

**四个 sha256 与交付文档 ①节 / ②节 2.4、discovery `files_modified` 记的值逐字相同。**

> **关于「做完了但不生效」（文档 ④节 B）**：DoD functional② 的字面要求是「改 `container.json`」，**字面要求 100% 完成**。dev 另外查明真相源是中央 DB 的 `container_configs` 表，我独立复核了这条证据链——
> `src/container-config.ts:4` 注释原文 `Source of truth is the container_configs table in the central DB.`；
> `materializeContainerJson()`（`:74-89`）`fs.writeFileSync` 无条件覆盖该文件并**返回** config；
> `src/container-runner.ts` 调用点 `const containerConfig = materializeContainerJson(agentGroup.id);`；
> `buildMounts` 消费的是**返回值**、从不读文件；
> `backfillContainerConfigs()`（`src/backfill-container-configs.ts:29/35`）头两行 `if (getContainerConfig(group.id)) continue;` ⇒ 已有行的组永不回读。
> 并且 DB 现状（**只读查询**）确认 Warp 组已有行、`additional_mounts` 仍只有 vault：
> ```
> $ sqlite3 /Users/zuowei/workspace/ai/nanoclaw/data/v2.db "SELECT agent_group_id, additional_mounts, skills, image_tag FROM container_configs;"
> ag-1782389975230-deeugq|[]|"all"|
> ag-1782440732275-pqkuxs|[{"hostPath":"/Users/zuowei/Obsidian/ClawdVault","containerPath":"vault","readonly":true}]|"all"|nanoclaw-agent:agentreach-h3
> ```
> ⇒ **「做完了但不生效」与「没做完」是两件事，这里是前者。** 不改 `data/v2.db` 的裁决合理：它不在 `writes` 声明内，且是正被进程持有的运行时库；dev 把生效 SQL 写进 ④节 B 供人执行、与 allowlist 同待遇，并已报 Leader。这是**超出 DoD 的额外发现，加分项而非扣分项**。

### 2.4 functional[2] —— `CLAUDE.local.md`

**段落顺序**（DoD 要求「在『## 调研』段**之后**」）：

```
$ grep -n '^## ' CLAUDE.local.md
5:## 工作方式
14:## 调研
19:## 金融数据解读（Hestia）
```

⇒ 新段在 19 行，「## 调研」在 14 行，**顺序正确**（且新段是文件最后一节）。

**内容 —— 与需求原文逐字节比对**（不是肉眼比对）：把需求文档 `## TASK-003` 节 Step 3 代码块与实际文件的对应段各自抽出，机械 diff：

```
$ diff -u <需求原文 Step 3 段> <CLAUDE.local.md 实际段>
$ echo "EXIT=$?"
EXIT=0
$ shasum -a 256 <两份>
40d8de7db0a632de873e650b2029c394b4a6b7927cee7019fe46e77dd3119cb2  需求原文段
40d8de7db0a632de873e650b2029c394b4a6b7927cee7019fe46e77dd3119cb2  实际段
```

**两侧 sha256 相同 ⇒ 逐字节照抄，零偏差。** 五条内容（引导句 + 4 条 bullet）由此自动满足：

| DoD 要求的第几条 | 落在 | 命中 |
| --- | --- | --- |
| 触发词与加载 `warp-hestia` skill | 行 20（「处理 hestia 队列」「生成金融数据解读」「解读这期央行数据」） | ✓ |
| 队列路径且**本组唯一可写挂载** | 行 21 | ✓ |
| 写回经 `selvage_call("spool.archive", …)` 多传 `source: "hestia"`、路径在 `Wiki/Macro/PBOC/` 下 | 行 22 | ✓ |
| **一次只处理一份**并回复剩余份数 | 行 23 | ✓ |
| frontmatter/数据表由 `prepare.py` 生成、只写 `<!-- narrative -->`、写回前必跑 `verify.py` | 行 24 | ✓ |

保留需求原文的全角标点、不与本组既有段落归一化，是正确取舍（归一化会让逐字比对产生假红）。

### 2.5 boundary[0] —— `mount-allowlist.json` 未被修改 + ④节 A 三项

```
$ cat ~/.config/nanoclaw/mount-allowlist.json
{
  "allowedRoots": [
    { "path": "/Users/zuowei/Obsidian/ClawdVault", "allowReadWrite": false,
      "description": "Loom Plan 1: Obsidian vault (RO for Warp)" }
  ],
  "blockedPatterns": [],
  "nonMainReadOnly": true
}

$ ls -l ~/.config/nanoclaw/mount-allowlist.json
-rw-r--r--@ 1 zuowei  staff  239 Jun 26 10:30 …/mount-allowlist.json
```

⇒ `allowedRoots` **仍只有 ClawdVault 一项**，且 **mtime 为 Jun 26 10:30**（远早于本任务 2026-09-08）——硬约束「agent 绝不改它」**双证成立**（内容 + mtime）。

**④节 A 的片段原文**与需求文档 Step 4 代码块机械 diff：

```
$ diff -u <需求 Step 4 片段> <文档 ④节 A 片段>
$ echo "EXIT=$?"
EXIT=0
```

**队列根不存在**：

```
$ ls -d /Users/zuowei/workspace/runtime/atlas/queue/hestia   → No such file or directory (EXIT=1)
$ ls -d /Users/zuowei/workspace/runtime/atlas                → /Users/zuowei/workspace/runtime/atlas
```

⇒ 连上一级 `queue/` 都不存在，与文档 2.6 一致。

**mount-security 行为（文档称「未验证，但代码给出明确答案，且与需求猜测不同」）—— 我逐条读到代码行核实**（锚：`/Users/zuowei/workspace/ai/wt-warp-hestia` @ `d791101e1defcfd3d3d3fcf7ae32a85ec420547b`）：

| 文档断言 | 我读到的 | 判定 |
| --- | --- | --- |
| `getRealPath()` `:137-143` `realpathSync` 抛异常 → 返 null | 属实，函数体恰在 137-143 | ✓ |
| `validateMount()` `:256-260` `realPath === null` 即返 `Host path does not exist: …`，**在 blockedPatterns / allowedRoots 之前** | 属实，`:256` 为 `if (realPath === null) {`，早于任何 root 检查 | ✓ |
| `validateAdditionalMounts()` `:346` 走 `log.warn('Additional mount REJECTED', …)`，挂载整条丢弃 | 属实，`:345` 为 `} else {`、`:346` 为 `log.warn('Additional mount REJECTED', {` | ✓ |
| `findAllowedRoot()` `:171-179` 对不存在的 allowedRoot `continue` | 属实，注释原文 `// Allowed root doesn't exist, skip it` | ✓ |
| 需求猜的 `Mount forced to read-only` / `not under any allowed root` **都不会出现** | 成立——路径不存在时在两者之前就 return 了 | ✓ |
| `loadMountAllowlist()` `:62-67` 缓存在进程内存、注释原文 `Result is cached in memory for the lifetime of the process.` ⇒ 粘贴后**须重启 nanoclaw 进程**（需求只提了重启 selvage） | 属实。这是**需求文档漏掉的一步**，dev 补上 | ✓（额外发现） |
| `nonMainReadOnly` 是死键 | `grep -rn 'nonMainReadOnly' src/ --include='*.ts'` → **0 条**；`MountAllowlist` 接口（`:21-24`）只有 `allowedRoots` / `blockedPatterns` | ✓ |
| 容器侧前缀 `/workspace/extra/` 由 `validateAdditionalMounts` 拼出 | 属实（`:334` `containerPath: \`/workspace/extra/${result.resolvedContainerPath}\``） | ✓ |

**结转声明**：④节 A 末句明写「selvage 重启与 allowlist 粘贴均属需求 TASK-006 Step 0 的『人执行』前置，本 sprint 结转」——DoD 要求的三项齐。

### 2.6 boundary[0] 的第 ④ 项（reviewer G7）—— skill 如何进容器

DoD 只要求「查清并写明」，不要求改分发机制。dev 的三条结论我**逐条抽验到代码行**（不是抽一条）：

**① 不随镜像打包，是运行时只读 bind mount ⇒ 不需要重建镜像**

```
$ grep -n '^COPY\|^ADD' /Users/zuowei/workspace/ai/wt-warp-hestia/container/Dockerfile
85:COPY agent-runner/package.json agent-runner/bun.lock ./
113:COPY cli-tools.json install-cli-tools.sh /tmp/
152:COPY entrypoint.sh /app/entrypoint.sh
（命中 3 条，无一涉及 skills）

$ sed -n '9,11p' …/container/Dockerfile
# Source is never baked in — /app/src is provided by a shared read-only
# bind mount at runtime (see src/container-runner.ts). Source-only changes
# never require an image rebuild.

$ sed -n '346,349p' …/src/container-runner.ts
  // Shared skills — read-only, symlinks in .claude-shared/skills/ point here.
  const skillsSrc = path.join(projectRoot, 'container', 'skills');
  if (fs.existsSync(skillsSrc)) {
    mounts.push({ hostPath: skillsSrc, containerPath: '/app/skills', readonly: true });
```

⇒ 成立。**spec §9 风险表「重建镜像（若 skills 随镜像打包）」这条尾巴确实可以划掉。**

**② `"skills": "all"` 自动纳入新 skill**

```
$ sed -n '412,426p' …/src/container-runner.ts
 * from `container/skills/` so newly-added upstream skills appear automatically.
 */
function selectedSkillNames(containerConfig: …): string[] {
  if (containerConfig.skills !== 'all') return containerConfig.skills;
  const sharedSkillsDir = path.join(process.cwd(), 'container', 'skills');
  return fs.existsSync(sharedSkillsDir) ? fs.readdirSync(sharedSkillsDir).filter(…) : [];
}
$ grep -n 'function syncSkillSymlinks' …/src/container-runner.ts
371:function syncSkillSymlinks(claudeDir: string, containerConfig: …): void {
```

⇒ 成立（`readdirSync` 实时重算 + 源码注释自陈「newly-added upstream skills appear automatically」）。Warp 组 `container.json` 与 DB 行都是 `"skills": "all"`（见 §2.3 的 DB 输出）⇒ 不需改 skills 字段。

**③ 🔴 `projectRoot = process.cwd()` ⇒ worktree 里的产物容器看不见**

```
$ grep -n 'projectRoot' …/src/container-runner.ts
273:  const projectRoot = process.cwd();
343:  const agentRunnerSrc = path.join(projectRoot, 'container', 'agent-runner', 'src');
347:  const skillsSrc = path.join(projectRoot, 'container', 'skills');
```

⇒ 成立。**先例给出的现成反证我也复现了**（这是本任务最有价值的发现之一）：

```
$ ls /Users/zuowei/workspace/ai/nanoclaw/container/skills/ | wc -l          → 9   （无 warp-research）
$ ls -l …/container/skills/warp-research/                                   → No such file or directory
$ ls …/data/v2-sessions/ag-1782440732275-pqkuxs/.claude-shared/skills/ | wc -l  → 9
$ ls …/.claude-shared/skills/ | grep -c warp                                → 0
$ ls -la …/.claude-shared/skills/ | head -4
lrwxr-xr-x  … agent-browser -> /app/skills/agent-browser
```

⇒ **M3 的先例 skill `warp-research` 此刻在容器里同样不可见**。这不是本任务引入的问题，但它会同样卡住 TASK-005/006 的产物 —— dev 已把这条写进 discovery `interfaces_exposed` 第 3 条交给下游。**这是把「未定的风险」变成「已定的前置条件」，正是 boundary④ 立这条 DoD 的目的。**

### 2.7 error_handling[0] —— 备份与 JSON 校验

```
$ ls -l /Users/zuowei/workspace/ai/nanoclaw/groups/cli-with-warp/
-rw-r--r--@ … CLAUDE.local.md
-rw-r--r--@ … CLAUDE.local.md.bak-2026-09-08
-rw-r--r--@ … container.json
-rw-r--r--@ … container.json.bak-2026-09-08
```

- 备份 `container.json.bak-2026-09-08` **实存**，sha256 `8cd0947…` == 改动前值（§2.3），**逐字节即改动前内容**（`diff -u` 只有新增行可证）。
- 备份路径写进文档 ①节 1.1 ✓。
- `json.load` EXIT=0 ⇒ 未触发「回滚 + `blocked_clarification`」分支，该分支未被执行是**正确**结果。
- 附带：`CLAUDE.local.md` 也做了备份（非 DoD 要求，成本为零而失手不可逆）——合理，不扣分。

### 2.8 non_functional[0]（review）—— 零提交、无 PR 是预期

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw ls-files groups/cli-with-warp
（无输出）  命中条数=0

$ git -C /Users/zuowei/workspace/ai/nanoclaw check-ignore -v \
    groups/cli-with-warp/container.json groups/cli-with-warp/CLAUDE.local.md
.gitignore:15:groups/*	groups/cli-with-warp/container.json
.gitignore:15:groups/*	groups/cli-with-warp/CLAUDE.local.md

$ sed -n '13,20p' /Users/zuowei/workspace/ai/nanoclaw/.gitignore
（14）# Groups - per-installation state, not tracked
（15）groups/*
（18）# per-group memory (CLAUDE.local.md) must never be committed.
（19）**/CLAUDE.local.md

$ git -C /Users/zuowei/workspace/ai/nanoclaw log fork/main..feat/warp-hestia --oneline | wc -l
0
```

⇒ 前提**真成立**：两个改动文件确被 `.gitignore:15` 忽略（`CLAUDE.local.md` 另被 `:19` 二次命中），`git ls-files` 空，需求 Step 5 的「若被跟踪」条件为假。
④节 D 已显式写明「本任务无 nanoclaw 提交，PR 内容为空；产出是 worktree + 两个本机未跟踪文件的改动」，③节 PR 链接标「无」。

⇒ **未按「PR 链接缺失」判 rejected**，并且我确认了这个免判前提本身成立（不是照单全收）。

### 2.9 non_functional[1] —— 交付流程与锚点纪律

**commit 与 scope**：

```
$ git log -1 --format='%s' 134f6351868725b7f1669f0932eb86655c35faf3
docs(TASK-004): M3 nanoclaw feat/warp-hestia worktree + Warp 组队列读写挂载 + CLAUDE.local.md 触发段（交付记录）
$ … | grep -cE '^[a-z]+\(TASK-004\): '
1

$ git show --numstat 134f6351868725b7f1669f0932eb86655c35faf3
507	0	docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md
$ wc -l docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md
507
```

- **唯一文件、纯新增 507/0**，与 `writes` 声明 `./docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md` **完全一致 ⇒ 零越界申报**。
- `git diff --stat 0c3fbdc9ac7b12fcb65d6d2695541f55820cd03c..fe95ac70…` 反向核对同样只此一文件。

**已合入 master（拓扑 + 内容双判据）**：

```
$ git merge-base --is-ancestor 134f6351868725b7f1669f0932eb86655c35faf3 master   → IS-ANCESTOR
$ git show master:docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md | shasum -a 256
667df3338d52117dd614486f74bacb296d7f5853286f0ff693cf10a404933f42
$ git show 134f6351…:docs/…md | shasum -a 256
667df3338d52117dd614486f74bacb296d7f5853286f0ff693cf10a404933f42
$ shasum -a 256 docs/…md（工作树）
667df3338d52117dd614486f74bacb296d7f5853286f0ff693cf10a404933f42
```

三处逐字节相同 ⇒ **拓扑判据与内容判据一致，不是「同内容换了 sha」也不是「空分支假阳」**。

**锚点纪律（DoD 红字：不得写 `HEAD` 或分支名）—— 逐个 grep 检查**：

```
$ grep -n 'HEAD' docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md   → 命中 6 处
161: HEAD is now at d791101 …          ← git 自己的输出
186: $ git … rev-parse --abbrev-ref HEAD ← 命令原文（其输出全 sha 在下一行）
188: $ git … rev-parse HEAD              ← 同上
345: | nanoclaw worktree HEAD **全 sha** | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
347: | 主 checkout 分支 / HEAD 全 sha | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95` |
349: | atlas 基线 HEAD 全 sha | `fe95ac707ea46f9939bb6554261830a95f1b93e1` |

$ grep -oE '\b[0-9a-f]{40}\b' … | sort | uniq -c
   2 aefea6ce5439cfcf15b3dc54ea9bd507c5846c95
   3 d791101e1defcfd3d3d3fcf7ae32a85ec420547b
   1 fe95ac707ea46f9939bb6554261830a95f1b93e1
```

⇒ **6 处 `HEAD` 无一处被当作锚使用**：3 处是命令原文/git 输出，3 处是表格里「HEAD 全 sha」的标签且紧跟全 sha 值。③节给出 nanoclaw worktree / nanoclaw 主 checkout / atlas 三个全 sha，**锚点纪律满足**。

**自证数字统一重采**：文档所有 sha256 / sha / 计数我逐个重跑，**无一不符**（四个文件 sha256、507 行、numstat 507/0、`ls-files` 0 条、9 项 skills ×2、`grep -c warp` 0、COPY/ADD 3 条、`nonMainReadOnly` 0 条）⇒ 「统一重采于最后一次改动之后」这句自述**经得起复算**。

---

## 三、观察（3 条，均不构成 rejected 依据）

### O1（轻微）④节 B 表头混锚：两树被断言为「同」，实际不同

表头写「代码位置（`/Users/zuowei/workspace/ai/nanoclaw`，主 checkout @ `aefea6c…`；worktree `d791101…` **同**）」，但：

```
$ shasum -a 256 {主树,worktree}/src/container-runner.ts   → 两值不同
$ git -C /Users/zuowei/workspace/ai/nanoclaw diff aefea6ce…95 d791101e…7b --stat -- src/container-runner.ts container/Dockerfile
 container/Dockerfile    | 21 +++++++++++++++++++--
 src/container-runner.ts |  9 +++++++++
```

⇒ 两树该文件差 9 行，表内两行各自锚在**不同的树**上：

| 表内引用 | 主树实际 | worktree 实际 |
| --- | --- | --- |
| `:129` materializeContainerJson 调用点 | **129 ✓** | 131 ✗ |
| `:353-356` additionalMounts 消费点 | 351-352 ✗ | **353-354 ✓** |

discovery 的 `line_refs_reverified` 声称「文档与本 discovery 引用的每个 file:line 都在成稿后用 `grep -n` 重新求值并逐条订正过（订正了 6 处）」——**这一行是漏网**：它在某一棵树上求过值，但表头断言的是两棵树都对。

**为何不 reject**：DoD 无「行号需在两树均精确」的条目；结论本身（container.json 非真相源）我已独立复核为真；两处代码在两树**均存在**，偏移仅 2 行，下游读者 `grep -n` 十秒可定位。判 rejected 会换来一整轮返工而零正确性收益。

**建议（不阻断）**：④节 B 表头改为单一树锚（建议钉 worktree `d791101e1defcfd3d3d3fcf7ae32a85ec420547b`，与 ④节 A / C 一致），或删去「worktree 同」这四个字。

### O2（极轻微）④节 B 措辞

「`src/cli/resources/groups.ts` … **没有任何 additional_mounts 子命令**」——该文件确有一处命中：

```
$ grep -n 'additional_mounts\|additionalMounts' …/src/cli/resources/groups.ts
29:    additional_mounts: JSON.parse(row.additional_mounts),
```

但那是**只读序列化**（把 DB 行转成展示对象），写入侧（`config update` 的字段白名单 `provider/model/effort/image_tag/assistant_name/max_messages_per_prompt/cli_scope`）确实**没有** additional_mounts。⇒ **结论成立，措辞可更准**（「没有写入子命令」）。

### O3（记录，非缺陷）散文里的短 sha 回指

`d791101…` / `aefea6c…` 在散文中出现 6 处（行 161/177/178/389/407/417/506），其中 161/177/178 是 git 自己的输出。DoD 与 CLAUDE.md 禁止的是 **`HEAD` / 裸分支名**这类**会过期的符号引用**；缩写 sha 是不可变对象的缩写，只会「歧义」不会「漂移」，且全 sha 在 ③节齐备可回溯。**不判违规**，仅记录。

---

## 四、验证对象漂移（AD-29）

- 判定**开始**时 atlas HEAD = `0c3fbdc9ac7b12fcb65d6d2695541f55820cd03c` == `verify_baseline.head`，discovery sha256 == 基线记录值 ⇒ **零漂移**。
- 判定**过程中** master 前进到 `755016eba82daaebee01721d1b86311f9d50a373`（Leader merge 了 TASK-003）。范围判定：

```
$ git diff --stat 0c3fbdc9ac7b12fcb65d6d2695541f55820cd03c..master
 docs/hestia-m3/TASK-003-loom-spool-source.md | 421 +++++++++++++++++
$ git diff --stat 0c3fbdc9…..master -- docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md
（空）
$ shasum -a 256 .arcforge/discoveries/TASK-004.json
0db59c898ee0f8bf2e30cfbedcaa87cca1492b136588f381235aa5f6df475665   ← 与基线相同
```

⇒ **本任务声明范围内的文件（`./docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md`）在判定期间未发生任何变化**，discovery 亦未漂移。HEAD 前进属并行任务造成的范围外推进，**INFO 级，不需要 `--ack-drift`**。
- nanoclaw 侧同样零漂移：今日全 ref 零提交，两个被改文件的 sha256 与 discovery 记录逐字相同。

---

## 五、结论

**✅ VERIFIED。**

7 条 done_criteria **逐条 PASS，无未覆盖标准，无空洞断言**（本任务无测试套件，DoD 全为 manual/review，判据是可复现的实地证据——我把交付文档里的每一个可验证断言都重新求值了一遍，无一不符）。

超出 DoD 的三项额外发现，均已落进 discovery / 交付文档、对下游有实质价值：

1. **`container.json` 不是真相源**（真相源是 `data/v2.db` 的 `container_configs` 表）—— 若不查明，TASK-006 冒烟会在「配置明明改了却不生效」上空转。生效 SQL 已写进 ④节 B 供人执行，未擅自改运行时库，边界处理正确。
2. **粘贴 allowlist 后须重启 nanoclaw 主进程**（`loadMountAllowlist` 进程内缓存）—— 需求文档只写了重启 selvage，这是对需求的实质订正。
3. **`projectRoot = process.cwd()` ⇒ worktree 里的 skill 容器看不见**，且用 `warp-research` 先例给出了现成反证（非推理）—— 直接决定 TASK-005/006 的产物能否被调到，已写进 `interfaces_exposed`。

三条观察（O1 混锚 / O2 措辞 / O3 短 sha）**不影响任何结论**，建议在后续任务顺手修正 O1，不要求本任务返工。

**下游注意**：worktree `/Users/zuowei/workspace/ai/wt-warp-hestia`（分支 `feat/warp-hestia` @ `d791101e1defcfd3d3d3fcf7ae32a85ec420547b`）**未拆、工作区干净**，TASK-005/006 直接复用，TASK-007 负责拆。
