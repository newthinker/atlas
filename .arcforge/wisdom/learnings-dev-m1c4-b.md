
## [dev-m1c4-b / TASK-004 / 2026-09-01] 「已交付等 merge」是流水线上唯一无活性保障的状态

**实撞**：TASK-004 交付后等 merge **286 分钟**，其间 idle hook 唤醒我数十次，解锁文案恒为「把名下任务推进到 dev_done」——**方向与 AD-4 恰好相反**（AD-4：merge 之前转 dev_done，门禁会在没有我代码的树上报绿）。

**为什么没有任何机制发现**：
- `in_progress` 上 validator 的 `stale-dispatch` **刻意不设阈值**（正常 dev 干活 p90 就有 65 分钟，设阈值会把长任务淹在告警里）
- idle hook 只知 status ∈ {assigned, in_progress, review_fix}，看不出「代码已提交、在等别人 merge」
- ⇒ 「已交付等 merge」与「还在写代码」**在文件层完全同形**

**dev 侧只能做三件事**（都做了）：内容判据查 merge（不是拓扑判据）、临时 worktree 预演 merge 排除自己这侧的问题、催办。**不能因 hook 催就转 dev_done。**

### 🔴 对 `merged_into_master` 的一处订正（我查实的，与转述有出入）

Leader 转述我说「有人曾把 merge 状态记进**任务文件**」——**实际不是**。查实：

- 全仓 `grep -rl merged_into_master` 命中 **6 个文件，全部在 `discoveries/`**（sprint-038 两份、sprint-039 四份），形态是全 sha：`"merged_into_master": "0a05c4e43c514efb2c516692d480a5c00999f07c"`
- 归档 324 份**任务文件**的顶层字段全集里**没有** `merged_into_master`；与 merge 沾边的只有 `commit`（21 次）和 `pr`（14 次）
- `commit` / `pr` / `impl_ref` / `verified_at` / `dev_done_at` / `coverage_minimum` 在 `.claude/hooks/*.sh` 里的命中数**全是 0** —— 零消费者字段

**这处订正不是细节**：它恰好解释了为什么那个好主意没变成机制。写进 discovery 的东西**没有任何调度机制会读**（validator 读 task 文件、hook 读 task 文件）。前人已经想到了要记 merge sha，但把它写在了一个**只有人会读、而且是事后才会读**的载体上 ⇒ 想法留下了，活性保障没有留下。

⇒ 与[载体强度排序]同一族：`done_criteria` > `questions[].answer` > `discovery.decisions` > inbox。**要让某件事被机制执行，就得写进机制会读的字段**；写进 discovery 只等于写给未来的人看。

⇒ 若将来要做，正确形态是 task 文件的顶层字段 + validator 规则（属 write-matrix / validator 契约变更 = 运行时资产，须走人类确认，dev 不能自己加）。

### 副产品：Leader 数 `fieldOrder` 数出 78（真值 76）

`grep -oE 'Field[A-Za-z0-9]+' | wc -l` 数的是「文本里出现的 `Field*` 标识符」，不是「`fieldOrder` 的元素数」——被我写的块注释里两个 `Field` 抓进去了。**它在基线上给 54 是对的，这正是它能活下来的原因**：「我以前用它没出过错」只说明没用到它会坏的粒度上。正确仪器是让 Go 自己 `len()`。

### 🔴 订正上一条的证据：我数「零消费者」用的是子串计数，不是字段引用（2026-09-01，leader 指出，我复算确认）

我上面写「`commit`/`pr`/`impl_ref`/`verified_at`/`dev_done_at` 在 `.claude/hooks/*.sh` 里命中数**全是 0**」。**错了两个**。我自己复算：

```
merged_into_master   0   ✓
impl_ref             0   ✓
verified_at          0   ✓
dev_done_at          0   ✓
coverage_minimum     0   ✓
commit              13   ✗
pr                  97   ✗
```

`pr` 的 97 次全是子串误配（`printf` 40 / `sprint` 19 / `in_progress` 8 / `no_progress` 6 / `progress` 5 / `project` 4 / `progress_note` 3 / `pretty` 3 …），`commit` 的 13 次是 `git commit` 这类命令。

⇒ **裸 `grep -c <字段名>` 数的是「文本里出现这个字串」，不是「hook 读取这个任务文件字段」。** 与同一天 leader 用 `grep -oE 'Field[A-Za-z0-9]+'` 把 `fieldOrder` 数成 78 是**同一形状**：拿高度相关的可观测量代替性质本身，近似失效时不报错、只给一个像样的数。⚠️ **我在同一条消息里点评了他那个错，然后自己用了同一形状的仪器** —— 指出别人的仪器问题不会让我的仪器变好。

**换精确仪器**（提取 hooks 里 `jq` 程序引用的字段）：

```bash
grep -ohE "jq [^|]*'[^']*\.[a-z_]+" .claude/hooks/*.sh | grep -oE '\.[a-z_]+' | sort | uniq -c | sort -rn
```

结论**更强**了：不是「这几个字段没人读」，而是「**hooks 读到的字段集合里根本没有 merge 这回事**」——不依赖我把字段名拼对。

⚠️ **但这把新尺也有已知局限，写成判别式而不是结论**：它把三类东西混在一起数了——
① 真的任务文件字段（`.status` `.writes` `.packages` `.assigned_to` `.verify_baseline` `.assignment_epoch` `.done_criteria` `.rework_count` `.verifier` `.reason_class` `.id` `.last_transition`）、
② **hook 输入负载**字段（`.tool_input` `.tool_name` `.file_path` `.command` `.teammate_name` `.task_id`）、
③ **路径/配置片段**（`.arcforge` `.json` `.rules` `.report_dir` `.coverage_floor` `.dev_minimum` `.test_timeout` `.owner_table` `.tokens`）。
leader 报的是**手工筛过的 9 条**，我跑同一条命令拿到 **35 条**。

⇒ **它的误差方向是「多算」，而多算在这个问题上是安全的**：连**过量**的集合里都没有任何 merge 相关字段，「无消费者」的结论只会更硬。**引用它时必须带上这句方向说明**——同一把尺若用来证明「某字段有人读」就完全不成立（`.json` 显然不是谁在读任务文件的 `json` 字段）。

⇒ 通则：**报「某某为 0」之前，先问「这把尺在什么条件下会给出一个像样的错数」**，并且**说清误差方向**——朝安全方向偏的可以用，朝危险方向偏的必须换尺。
