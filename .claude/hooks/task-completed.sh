#!/bin/bash
# TaskCompleted hook — 任务声明范围驱动的硬门禁
# exit 0 = 允许完成, exit 2 = 阻止完成并反馈
#
# 门禁范围 = task JSON 的声明范围,两个口径:packages(覆盖率,宽)驱动 go test/-coverpkg,
# writes(互斥,窄;缺省回落 packages)驱动 scope 漂移与无代码任务的变更判定。
# 绝不使用全局 git diff 推断——共享工作区下其他 Agent 的在途改动(尤其 RED 阶段的预期失败测试)
# 会污染判定(F1)。实际改动超出声明范围按 scope 漂移阻断(评审 R3)。
# 阻断/告警类反馈写 stderr——exit 2 的官方反馈通道是 stderr。
set -uo pipefail

CONFIG_FILE="arcforge.config.json"
TASK_DIR=".arcforge/tasks"
DEV_MIN=$(jq -r '.coverage.dev_minimum // 80' "$CONFIG_FILE" 2>/dev/null || echo 80)
COV_DIR=$(jq -r '.coverage.report_dir // ".arcforge/coverage"' "$CONFIG_FILE" 2>/dev/null || echo ".arcforge/coverage")
TEST_TIMEOUT=$(jq -r '.coverage.test_timeout // "120s"' "$CONFIG_FILE" 2>/dev/null || echo "120s")

# 路径**段**前缀匹配:声明既可以是目录(validator)也可以是文件(CLAUDE.md),故
# 「f 落在 q 之下」= f 与 q 相等,或 f 在 q/ 之下。按**段**而非字符串前缀是硬要求——
# 字符串前缀会把 ./pkg/ab 误判成 ./pkg/a 的子路径。
# 全仓库只此一处口径:docs-only 的「范围内」与 scope 漂移的「范围外」都调它,
# validator 的 pathScopesIntersect 与之对齐(TASK-012)。三方统一,不造第四种(TASK-018)。
# 必须是函数而非命令替换内联:bash 3.2 在 $( ) 内解析 case/;; 有 bug(TASK-005 实撞),
# 函数体在定义时解析,调用点 $(filter_outside ...) 里就没有 case 记号了。
# W7:声明清单必须**行式**读取。原写法 `for p in $2` 让 shell 对声明做词分割 + glob 展开,
# 三种失效各出一次,且都指向「范围内的文件被判越界」:
#   · 含空格的声明(docs/my notes)被拆成 docs/my 与 notes 两条,两条谁都匹配不上;
#   · 含 glob 的声明(pkg/*)按**调用时的 CWD** 展开 —— 同一份 JSON 换个工作目录结论就翻转;
#   · 尾斜杠(validator/)让 "$q"/* 变成 validator//* ,同样匹配不上。
# 前两种由 read 消除(它既不做词分割也不做 glob),第三种由下面剥尾斜杠消除。
# 尾斜杠那种是唯一会**死锁**的:阻断文案要 dev 把越界路径补进 writes,而它已经在里面了
# (只差一个 /),dev 改不出任何东西;而 dev_done 之后 leader 与 dev 都无权再写该字段。
path_under_scope() { # <路径> <声明清单(多行)>;0 = 落在某条声明之下
    local f="${1#./}" p q
    while IFS= read -r p; do
        q="${p#./}"
        # 剥**全部**尾斜杠(docs/// 与 docs/ 同解);剥空则该条声明无意义,跳过。
        while [ -n "$q" ] && [ "$q" != "${q%/}" ]; do q="${q%/}"; done
        [ -n "$q" ] || continue
        case "$f" in
            "$q"|"$q"/*) return 0 ;;
        esac
    done <<EOF
$2
EOF
    return 1
}
filter_outside() { # <路径清单(多行)> <声明清单(多行)> → 打印**不**落在任何声明之下的路径
    local f
    while read -r f; do
        [ -n "$f" ] || continue
        path_under_scope "$f" "$2" || echo "$f"
    done <<EOF
$1
EOF
}

# ---- 1. 从 stdin 解析任务上下文(官方字段 task_id;兼容链兜底) ----
HOOK_INPUT=$(cat)
TASK_ID=$(echo "$HOOK_INPUT" | jq -r '.task_id // .task.id // empty' 2>/dev/null)
if [ -z "$TASK_ID" ]; then
    TASK_ID=$(echo "$HOOK_INPUT" | { grep -oE 'TASK-[0-9]+' || true; } | head -1)
fi

# ---- 2a. 主路径:任务声明的 packages/writes + 无代码任务判定 ----
# writes **字段缺失**时回落 packages(归档里 200+ 个历史任务都没有该字段,结论必须不变);
# **显式 []** 是「本任务不写任何文件」的声明,不回落——故判据是「字段有没有声明」
# (.writes != null,缺失时 .writes 即为 null),而不是 .writes[]? 有没有输出。
PKGS=""
WRITES=""
HAS_WRITES="false"
DOCS_ONLY="false"
if [ -n "$TASK_ID" ] && [ -f "$TASK_DIR/$TASK_ID.json" ]; then
    PKGS=$(jq -r '.packages[]?' "$TASK_DIR/$TASK_ID.json" 2>/dev/null | sort -u)
    HAS_WRITES=$(jq -r '.writes != null' "$TASK_DIR/$TASK_ID.json" 2>/dev/null || echo false)
    WRITES=$(jq -r '.writes[]?' "$TASK_DIR/$TASK_ID.json" 2>/dev/null | sort -u)
    # 无代码任务 = done_criteria 各维度全部条目为对象且 verify_by ∈ {review,manual},
    # 且至少 1 条。字符串条目视同 verify_by:test,自动排除出无代码任务。
    DOCS_ONLY=$(jq -r '[.done_criteria // {} | .[]? | .[]?]
        | (length > 0) and all(type == "object" and (.verify_by == "review" or .verify_by == "manual"))' \
        "$TASK_DIR/$TASK_ID.json" 2>/dev/null || echo false)
fi

# ---- 2a'. 「属于本任务的已提交改动」:C5(漂移) 与 W1(docs-only) 共用的**一个**口径 ----
# 判据锚定到 `<type>(TASK-ID):` 约定前缀。W1:原判据是「正文任意处提到该 ID」,于是被
# **任何**叙述性提及满足 —— 9 个 ID 的真实语料复算(@8167305),旧判据 70 条命中里只有
# 18 条是该任务自己的提交,剔除的 52 条抽查全是 Leader 记账提交
# (`chore(arcforge): TASK-013 dev_done 派验`)与他任务提交正文里的顺带提及。
#
# 这个集合有两个**方向相反**的消费点,故必须精确到「本任务自己的提交」,而不是「某个
# 时间窗内的提交」:W1 判范围**内**有无变更,过宽 ⇒ 误放行(别人的提交替我交了差);
# C5 判范围**外**有无变更,过宽 ⇒ 误阻断(别人的提交算成我的越界)。共享工作区里他人
# 提交推进 HEAD 是常态,任何时间窗口口径都必然混入他人改动,两侧同时受损。
#
# 为什么不用 verify_baseline(QA 的原建议)——实测证伪,不是推理:该字段在
# `dev_done → verifying` 才由写通道写入,而本门禁跑在 `transition dev_done` 之**前**。
# 首轮门禁时刻它 ABSENT;返工轮读到的是**上一轮派验时刻**的陈旧值,比缺失更坏
# (它长得像个可用基线,照它做 diff 会把上一轮已判过的改动和期间全部他人提交算进来)。
# 备选的「从 transitions.jsonl 取该次 assignment_epoch 的派发时刻」同样不可得:
# 本仓库当下的 transitions.jsonl 只有 update 记录、没有 transition 记录。
#
# ⚠ C-1(rev-task-008):**同一个集合不能同时喂这两个消费点**,因为它们的不安全方向相反。
# 初版给 C5 也用了无下界的全历史集合,理由是「多算 = 保守」。那个理由隐含一个前提:
# 多算的代价是「误阻断,dev 改一下就好」。**前提不成立** —— 任务 ID 跨 sprint 复用,于是
# **上一个 sprint 的同号任务碰过的文件**被无条件算成本任务的越界,而 dev 无从消除:
# `git rm` 再提交仍然阻断(`--name-only` 扫的是历史不是树),补进 writes 则是谎报声明。
# 对当时在册的 9 个任务复算:**8 个命中,43 个越界文件 43 个都还在工作树里**
# (故「只保留树中仍存在的文件」这个修法一个都过滤不掉,已实测排除)。
# ⇒ **「宁可多报」只在「多报可被清除」时才是保守的。** 无法被被报者消除的误报不是保守,
#   是拒绝服务。写阻断文案时应逐条自问「照这条做,能不能真的解除?」—— C-1 的两条文案
#   (撤回改动 / 补声明)一条都走不通,这本可以在写文案那一刻就发现。
#
# 修法 = **下界 + 两个消费点各退各的**:
#   · 下界取任务 JSON **已有**的 `last_transition.at`,不新增字段、不改写通道。
#     `dev_done` 的合法前驱唯一是 `in_progress`(出边表保证),故门禁运行那一刻
#     `last_transition.to == "in_progress"`,其 `at` 正是**本轮开工时刻**——brief 想造的
#     `work_baseline` 一直就在那儿。**必须校验 `.to`**:门禁若在别的状态下被手工触发,
#     那个 `at` 指的是别的事件,拿它当开工时刻是错的,此时视同无下界,不猜。
#   · **取不到下界时往哪边退,两侧相反**:
#       W1(判范围**内**):包含不足 ⇒ TASK-005 的死锁回来(硬) ⇒ **不加下界**,行为一字不变
#       C5(判范围**外**):过度包含 ⇒ 不可清除的阻断(硬)     ⇒ **置空 + 出声**
#     初版的错不在「忘了下界」,而在**取不到下界时退错了方向**(退到了最大包含)。
#     同一个文件里「无提交历史」那条降级是对的(放行 + WARN),「无下界」这条却做成了
#     阻断且无法解除 ⇒「放行可以,静默不行」只用了一半。**每个降级点都要单独问往哪边退。**
COMMITTED_MINE=""
COMMITTED_BOUNDED=""
WORK_SINCE=""
NONCONFORMING=""
if [ -n "$TASK_ID" ]; then
    # 两个集合的取数刻意各占**一整行**(而非续行式):变异 harness 按「整行含某字面串」
    # 定位,续行式会让两行共用同一个尾部子串而使锚点不再唯一——实撞过一次
    # (mutation-task-005 的 M5 因此从唯一命中变成命中 2 行而报「锚点失配」)。
    COMMITTED_MINE=$(git log -E --grep="^[a-z]+\(${TASK_ID}\):" --name-only --pretty=format: 2>/dev/null || true)
    # C5 专用的**有下界**集合;下界缺失时置空,**绝不回落到 COMMITTED_MINE**(那就是 C-1)。
    if [ -f "$TASK_DIR/$TASK_ID.json" ]; then
        WORK_SINCE=$(jq -r '.last_transition
                            | select(type == "object" and .to == "in_progress")
                            | .at // empty' "$TASK_DIR/$TASK_ID.json" 2>/dev/null || true)
    fi
    # I1(rev-task-008):**「非空」不等于「可用」**。`git log --since` 走的是 approxidate,
    # 对喂进去的字符串既不报错也不回非零 —— 实测 git 2.50.1(全集 3 条):
    #   'not-a-real-date' / '9999999999' / 'garbage' / '@@@' / '0' → 0 条(过滤成空,方向安全)
    #   **'never' → 3 条(过滤条件被整个忽略)**  ← 直接复现 C-1 本体:全历史进漂移集
    #   **'2021-13-45T99:99:99Z' → 2 条**       ← 月 13 日 45 时 99,被静默强解成某个真实时刻
    # 后两者都会让下界变成**任意值**,而因为走的是「有下界」分支,下面两条为降级写的 WARN
    # **一条都不会打** —— 静默。第一类虽然退出码恰好安全,但同样静默,而「取不到下界」
    # 这件事本身必须出声(本 sprint 口径)。
    # ⇒ 拿到值之后必须自检**可解析性**,失败则走「取不到下界」那条**已有**的降级路径。
    #
    # 自检用严格 RFC3339 + **字段范围**约束,不能只卡形状:`2021-13-45T99:99:99Z` 形状完全合法,
    # 只是月/日/时/分/秒越界。范围约束与 `validator/liveness.go:86` 对**同一个字段**用
    # `time.Parse(time.RFC3339, …)` 的严格校验对齐 —— Go 侧早就这么做了,bash 侧此前没有。
    # 已知局限(如实登记):正则挡不住 2 月 30 日这类「范围内但日历上不存在」的日期,
    # 而 `time.Parse` 挡得住。这类值 git 会强解成 3 月初,偏差以天计而非以年计,
    # 且要伪造它必须绕过写通道 —— 与本条要堵的「下界变成任意值」不同量级。
    RFC3339_RE='^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$'
    if [ -n "$WORK_SINCE" ] && ! printf '%s' "$WORK_SINCE" | grep -qE "$RFC3339_RE"; then
        echo "WARN: ${TASK_ID} 的 last_transition.at 不是可解析的 RFC3339 时间戳(「${WORK_SINCE}」)," >&2
        echo "      **视同取不到下界**——git log --since 对这类值会静默给出任意窗口(实测 'never'" >&2
        echo "      会让过滤条件被整个忽略),据它判漂移等于把全历史算成本任务的越界。" >&2
        WORK_SINCE=""
    fi
    if [ -n "$WORK_SINCE" ]; then
        COMMITTED_BOUNDED=$(git log -E --grep="^[a-z]+\(${TASK_ID}\):" --since="$WORK_SINCE" --name-only --pretty=format: 2>/dev/null || true)
    fi
    # 旧的宽判据不丢弃,降级成**告警探针**:它命中而锚定判据未命中 ⇒ 有提交提到了本任务
    # 却不符约定,那些提交里的改动对下面的漂移检查不可见。放行可以,静默不行。
    #
    # 差集按 **commit hash** 取,不按「主题匹不匹配锚点」判 —— 两者不等价:`git log --grep`
    # 是**逐行**匹配整条 message,故主题为 `chore: x` 而正文里有一行以 `feat(ID):` 开头的
    # 提交**会**进 COMMITTED_MINE,却过不了主题判据,于是被 WARN 说成「不可见」——
    # 一句假话。本仓库真实语料实测该形态 0 次,但告警一旦会说假话就没人再信它,
    # 而按 hash 取差集在结构上就不可能不一致(代价是多一次 git log,毫秒级)。
    MINE_H=$(git log -E --grep="^[a-z]+\(${TASK_ID}\):" --format='%h' 2>/dev/null || true)
    NONCONFORMING=$(git log -E --grep="${TASK_ID}([^0-9]|\$)" --format='%h %s' 2>/dev/null \
                    | MH="$MINE_H" awk 'BEGIN { n = split(ENVIRON["MH"], a, "\n")
                                                for (i = 1; i <= n; i++) if (a[i] != "") mine[a[i]] = 1 }
                                        NF && !($1 in mine)')
fi
# 两条「已提交改动不可见」的出声通路。二者互斥:无提交历史时 NONCONFORMING 结构上为空。
if [ -n "$TASK_ID" ] && ! git rev-parse --verify -q HEAD >/dev/null 2>&1; then
    echo "WARN: ${TASK_ID} 无提交历史(非 git 仓库或尚无提交),已提交改动集合结构上为空;" >&2
    echo "      本次只覆盖未提交改动,已提交的越界改动本次不可见。" >&2
elif [ -n "$NONCONFORMING" ]; then
    NC_N=$(echo "$NONCONFORMING" | { grep -c . || true; })
    echo "WARN: ${TASK_ID} 有 ${NC_N} 条提交提到该 ID,但主题不符 <type>(${TASK_ID}): 约定," >&2
    echo "      不计入本任务的已提交改动 —— 其中已提交的越界改动本次不可见:" >&2
    echo "$NONCONFORMING" | sed 's/^/        /' >&2
    echo "      若确属本任务,可 git commit --amend 改写主题(仅限未推送)。" >&2
fi
# C-1 的两条出声。它们与上面那两条是**不同的不可见来源**,故不合并:上面说的是
# 「哪些提交不算我的」,这里说的是「算我的、但落在本轮开工之前」。
if [ -n "$TASK_ID" ] && [ -z "$WORK_SINCE" ] && [ -n "$COMMITTED_MINE" ]; then
    # ⚠ 措辞刻意避开字面串「scope 漂移」:全套件拿它当**阻断分支的探测标记**
    # (assert_no_grep "scope 漂移"),告警里复用同一个标记会让那些断言失去判别力
    # ——标记必须只属于它标记的那条路径。这条本身就是本任务 §3 那族问题的又一实例。
    echo "WARN: ${TASK_ID} 取不到本轮开工时刻(任务文件无 last_transition,或其 to != in_progress)," >&2
    echo "      **已提交的改动本次一律不参与范围漂移判定**——只覆盖工作树/暂存区/untracked。" >&2
    echo "      这是刻意的降级:无下界时按全历史算,会把历史上同名 TASK-ID 碰过的文件" >&2
    echo "      判成本任务的越界,而 dev 无法消除(git rm 不改历史,补声明是谎报)。" >&2
elif [ -n "$WORK_SINCE" ]; then
    # 算我的、但早于本轮开工的提交:不计入漂移(否则又是不可清除的误报),但必须点名。
    #
    # M1(rev-task-008):**边界闭合性**。`--since=T` 与 `--until=T` 对「恰好在 T 时刻」的
    # 提交**都包含**(实测)。原写法用 `--until` 取「早于开工」,于是边界上那条提交
    # **既进 COMMITTED_BOUNDED 参与漂移判定、又被这里打印成「不参与判定」** —— 告警说了
    # 假话。**这里原有的**那句注释写的是「差集按 hash 取,与上面 NONCONFORMING 同理由」,
    # 而代码走的是 `--until`、并没有那么做(NONCONFORMING 那条倒是名副其实)——
    # **注释写的是意图,代码写的是行为,两者不一致时以代码为准**。现改为真的按 hash 取
    # 差集(MINE − BOUNDED):边界归属由 `--since` 单方面决定,结构上不可能重叠。
    #
    # M2(rev-task-008):只点名**确实碰了声明范围之外文件**的早期提交。否则一条只改了
    # 范围内文件的良性提交也会被列出来,读起来像在暗示「有被忽略的越界改动」——
    # 与 NONCONFORMING 那条一样,**告警不该让人以为发生了没发生的事**。
    BOUNDED_H=$(git log -E --grep="^[a-z]+\(${TASK_ID}\):" --since="$WORK_SINCE" --format='%h' 2>/dev/null || true)
    EARLY_H=$(printf '%s\n' "$MINE_H" \
              | BH="$BOUNDED_H" awk 'BEGIN { n = split(ENVIRON["BH"], a, "\n")
                                             for (i = 1; i <= n; i++) if (a[i] != "") b[a[i]] = 1 }
                                     NF && !($1 in b)')
    if [ -n "$EARLY_H" ]; then
        # 范围口径与 §2c 一致:writes 优先,字段缺失才回落 packages。
        EARLY_SCOPE="$WRITES"; [ "$HAS_WRITES" = "true" ] || EARLY_SCOPE="$PKGS"
        # shellcheck disable=SC2086  # EARLY_H 是换行分隔的十六进制短 hash,词分割是有意的
        EARLY_OUT=$(git show --name-only --pretty=format: $EARLY_H 2>/dev/null \
                    | { grep -vE '^$|^\.arcforge/' || true; } | sed 's#^#./#' | sort -u)
        EARLY_OUT=$(filter_outside "$EARLY_OUT" "$EARLY_SCOPE")
        if [ -n "$(echo "$EARLY_OUT" | tr -d '[:space:]')" ]; then
            EA_N=$(echo "$EARLY_H" | { grep -c . || true; })
            echo "WARN: ${TASK_ID} 有 ${EA_N} 条本任务提交早于本轮开工时刻(${WORK_SINCE})," >&2
            echo "      不参与本次范围漂移判定(可能来自上一轮返工或更早的同号任务);" >&2
            echo "      其中**落在声明范围之外**的文件(本次对漂移检查不可见):" >&2
            echo "$EARLY_OUT" | sed 's/^/        /' >&2
            echo "      涉及的提交:" >&2
            # 先取值再输出:`cmd 2>/dev/null >&2` 的**顺序**会先把 fd2 指向 /dev/null,
            # 随后 `>&2` 复制的就是 /dev/null —— stdout 被静默吞掉。实撞过一次,而且只有
            # 「断言了输出内容」的那条用例发现了它(只断言 WARN 出现与否的话它是绿的)。
            # shellcheck disable=SC2086
            EARLY_LOG=$(git log --no-walk --format='%h %ad %s' --date=short $EARLY_H 2>/dev/null || true)
            echo "$EARLY_LOG" | sed 's/^/        /' >&2
        fi
    fi
fi

# ---- 2b. git 推断的「实际触碰范围」(含 untracked,修 F4);
#          交叉校验与 fallback 共用 ----
CHANGED_FILES=$( { git diff --name-only HEAD; \
                   git diff --name-only --staged; \
                   git ls-files --others --exclude-standard; } 2>/dev/null )
# 漂移口径 = 变更的**文件路径本身**,不再只筛 .go(TASK-018):只筛 .go 时 markdown 结构上
# 永远进不了 DRIFT,而 writes 恰恰是拿来管文档/产物类任务的——本 sprint 真实发生过的那次
# .md 越界是当事人自己申报的,不是被拦下的。文件粒度也让阻断文案能点名到具体文件。
# .arcforge/ 整棵树排除:它由 arcforge-write.sh + 单写者矩阵专管(比「声明」更强的机制),
# 且 transition dev_done 那一刻本任务的 tasks/discoveries JSON 必然是脏的——不排除的话
# 每一次 dev_done 都会被自己阻断,而没有人会把框架状态目录写进 writes。
# C5:漂移集必须含**已提交**的改动。原口径只取工作树/暂存区/untracked,而
# 「先 git commit 再 transition dev_done」恰恰是正常流程 —— 单变量对照(文件内容一字不改,
# 只多跑一次 git commit)下,越界从 BLOCKED 变成 exit 0 而越界文件仍原样躺在那里。
# 并入的是 COMMITTED_MINE(§2a',本任务自己的提交)而非时间窗内的全部提交,理由见 §2a'。
# **只并进 ACTUAL_FILES,不并进 CHANGED_FILES**:后者还喂着 ACTUAL_PKGS(2d fallback 的
# go test 作用域),并进去会连带改变 fallback 行为,那是 TASK-005 划下的边界。
# ⚠ 这里并的是 **COMMITTED_BOUNDED**(有下界)而非 COMMITTED_MINE(无下界)——见 §2a' 的 C-1。
# 无下界时 COMMITTED_BOUNDED 为空 ⇒ 本行退化为改动前的行为(只看未提交),并已在上面出声。
ACTUAL_FILES=$(printf '%s\n%s\n' "$CHANGED_FILES" "$COMMITTED_BOUNDED" \
               | { grep -vE '^$|^\.arcforge/' || true; } \
               | sed 's#^#./#' | sort -u)
# 2d fallback 专用,仍是 .go 的**包目录**:它会被直接喂给 go test,喂文件路径会走 file 模式。
ACTUAL_PKGS=$(echo "$CHANGED_FILES" | { grep '\.go$' || true; } \
              | xargs -n1 dirname 2>/dev/null | sort -u | sed 's#^#./#')
# 其他在途任务**会写**的路径(他人的在途改动不算到本任务头上,修 F1 污染)。
# 同属 writes 口径:漂移判定是两步相减(实际改动 − OTHERS − 我的 writes),两步必须同口径。
# 第一步若仍按他人 packages 相减,「别人只读消费的目录」就成了免罪金牌——我改了它,
# 却因它出现在别人的覆盖率口径里而被抵消掉,静默放行且无任何 WARN。
OTHERS=$(find "$TASK_DIR" -name '*.json' ! -name "${TASK_ID:-__none__}.json" -exec \
    jq -r 'select(.status | IN("assigned","in_progress","dev_done","verifying","blocked_clarification"))
           | (if .writes == null then .packages else .writes end)[]?' {} \; \
    2>/dev/null | sort -u)
# 两步相减一律走段前缀(原为 comm 的字符串精确比对):声明目录、实际改其子路径下的文件是
# 合法的,精确比对会把它误报成漂移;而他人声明**文件路径**时,目录口径的相减又减不掉。
MINE_FILES=$(filter_outside "$ACTUAL_FILES" "$OTHERS")
MINE_ACTUAL=$(filter_outside "$ACTUAL_PKGS" "$OTHERS")

if [ -n "$PKGS" ]; then
    # ---- 2c. 交叉校验:实际改动 ⊆ **writes**(评审 R3,防 scope 漂移逃逸) ----
    # 用 writes 而非 packages:只读消费的包算在覆盖率口径里,但改了它就是越界。
    [ "$HAS_WRITES" = "true" ] || WRITES="$PKGS"
    DRIFT=$(filter_outside "$MINE_FILES" "$WRITES")
    if [ -n "$(echo "$DRIFT" | tr -d '[:space:]')" ]; then
        WRITES_N=$(echo "$WRITES" | { grep -c . || true; })
        echo "BLOCKED: ${TASK_ID:-unknown} 检测到任务声明范围之外的改动(scope 漂移):" >&2
        echo "$DRIFT" | sed 's#^#  越界: #' >&2
        echo "当前声明范围(writes,共 ${WRITES_N} 项):" >&2
        if [ "$WRITES_N" -eq 0 ]; then
            echo "  (显式空——本任务声明「不写任何文件」)" >&2
        else
            echo "$WRITES" | sed 's#^#  - #' >&2
        fi
        echo "上面每条越界路径都不在任何一条声明之下。二选一修正,且**必须在 transition dev_done" >&2
        echo "之前**做完——dev_done 之后 leader 与 dev 都无权再写该字段:" >&2
        echo "  ① 补声明(经写通道,不能直接编辑 JSON):" >&2
        echo "     bash .claude/hooks/arcforge-write.sh --as <你的实例名> \\" >&2
        echo "       task ${TASK_ID:-TASK-xxx} update --json-field 'writes=[\"...\",\"<越界路径>\"]'" >&2
        echo "     若该路径也该算进覆盖率统计,同时补进 packages。" >&2
        echo "  ② 撤回该改动:git checkout -- <路径>(已跟踪)或删除(untracked)。" >&2
        exit 2
    fi
else
    # ---- 2d. fallback:仅限 task JSON 缺失/未声明的异常场景。
    #          validator 的 scope-empty 规则保证常态下在途任务必有声明(评审 R4)。----
    echo "WARN: ${TASK_ID:-unknown} 无声明 packages,退化为 git 推断门禁。" >&2
    PKGS="$MINE_ACTUAL"
    # 同 2c 的回落,只是此处 packages 口径本身来自 git 推断
    [ "$HAS_WRITES" = "true" ] || WRITES="$PKGS"
fi

# 过滤不存在的路径(无代码任务可声明文件或目录,故用 -e;被删文件的 dirname 亦被排除)
PKGS=$(echo "$PKGS" | while read -r p; do [ -n "$p" ] && [ -e "$p" ] && echo "$p"; done)

if [ -z "$PKGS" ]; then
    if [ -z "$TASK_ID" ]; then
        # 非 arcforge 任务完成事件(解析不出 TASK_ID)且无实际 .go 改动:不误拦
        echo "WARN: 无任务上下文且无代码改动,跳过门禁。" >&2
        exit 0
    fi
    echo "BLOCKED: ${TASK_ID} 的声明范围为空或声明路径全部不存在。" >&2
    echo "无代码任务也必须显式声明:packages 指向文档/产物路径,且全部 done_criteria 标注 verify_by: review|manual。" >&2
    exit 2
fi

if [ "$DOCS_ONLY" = "true" ]; then
    # 无代码任务分支:验证声明范围内确有实际变更,跳过 Go 门禁。
    # 变更来源 = 未提交(CHANGED_FILES) ∪ 「属于本任务的已提交」(COMMITTED_MINE,§2a')。
    # W1:COMMITTED_MINE 的判据由「正文任意处提到 <TASK-ID>」收紧为锚定 `<type>(ID):`
    # 约定前缀,见 §2a' —— 原判据让**别人的提交**(尤其 Leader 的记账提交)足以替本任务
    # 交差,实测 70 条命中里 52 条不是本任务的。CHANGED_FILES 仍不含已提交项(§2c 的漂移
    # 由 ACTUAL_FILES 单独并入 COMMITTED_MINE,两条通路分开,TASK-005 边界原样保留)。
    # 约定被违反(提交主题不符 `<type>(ID):`)时判据失效 → 保持 BLOCKED,即宁可漏放行
    # 不误放行;下方 BLOCKED 提示给出改写提交信息的纠偏路径。无提交历史/TASK_ID
    # 解析失败时 git log 失败 → 集合为空 → 退化为只看未提交变更(§2a' 已就此出声)。
    # 用 here-doc 让 while 在当前 shell 执行(绕开 bash 3.2 命令替换内 case/;; 解析 bug)。
    # 「范围内」同属 writes 口径:问的是「**我**改了东西没有」,而只读消费的包被别人改
    # 一下不该算成本任务的产出(否则本任务一个字没写也能放行)。
    # 判「范围**内**有无变更」与 2c 判「范围**外**有无变更」是两件独立的事,但共用
    # path_under_scope 这一个口径 —— 共用是结构性的,不是约定俗成的(TASK-018 boundary[0])。
    # 取数写成具名变量,与 §2b 的 ACTUAL_FILES 对称:两个消费点各自从**哪个**集合取数,
    # 是 C-1 的全部要害,不该藏在 here-doc 里(藏着就既读不出来、也没法单行变异)。
    # ⚠ 这里用的是 **COMMITTED_MINE(无下界)**,与漂移侧的 COMMITTED_BOUNDED 刻意不同 ——
    # 见 §2a':W1 侧包含不足会让 TASK-005 修的死锁复发,不安全方向与 C5 相反。
    DOCS_SOURCES=$(printf '%s\n%s\n' "$CHANGED_FILES" "$COMMITTED_MINE")
    CHANGED_IN_SCOPE=""
    while read -r f; do
        [ -n "$f" ] || continue
        path_under_scope "$f" "$WRITES" && CHANGED_IN_SCOPE="$f"
    done <<EOF
$DOCS_SOURCES
EOF
    if [ -z "$CHANGED_IN_SCOPE" ]; then
        echo "BLOCKED: 无代码任务 ${TASK_ID} 声明范围内没有任何实际变更。" >&2
        echo "已检查:未提交改动(工作树/暂存区/untracked),以及提交信息含 ${TASK_ID} 的历史提交。" >&2
        echo "若变更已提交:请确认提交信息含 ${TASK_ID}(约定 <type>(${TASK_ID}): <描述>)——" >&2
        echo "  未含时可 git commit --amend 改写(仅限未推送),或补一次含 ${TASK_ID} 的提交。" >&2
        echo "若尚未改动:请先完成 ${TASK_ID} 声明范围内文件的实际修改。" >&2
        exit 2
    fi
    echo "无代码任务(全部 verify_by=review|manual),范围内变更已确认,跳过 Go 门禁。"
    exit 0
fi

# 非 Go 项目(无 go.mod)跳过 Go 专用门禁。**只跳过 Go 那一段**:上面的 scope 漂移校验、
# 空 scope 拒绝、无代码任务判定都与 Go 无关,却曾因这句早退而在框架自身仓库里一次都没跑过
# (TASK-018)——本框架仓库根就没有 go.mod(go.work 在根、validator 在子目录),于是
# writes 这个安全属性对最需要它的那类任务(文档/产物)完全失效。
if [ ! -f go.mod ]; then
    echo "No go.mod found; skipping Go coverage gate."
    exit 0
fi

# ---- 3. 仅对声明范围跑测试 + 覆盖率,产物按任务隔离(F2) ----
mkdir -p "$COV_DIR"
COVERPKG=$(echo "$PKGS" | paste -sd, -)
COVERPROFILE="$COV_DIR/${TASK_ID:-adhoc-$$}.out"
echo "=== Gate scope (${TASK_ID:-fallback}) ==="
echo "$PKGS"
# shellcheck disable=SC2086  # PKGS 按行分包,word-splitting 是有意的
TEST_OUTPUT=$(go test $PKGS -timeout "$TEST_TIMEOUT" \
              -coverpkg="$COVERPKG" -coverprofile="$COVERPROFILE" 2>&1)
TEST_EXIT=$?
if [ $TEST_EXIT -ne 0 ]; then
    echo "BLOCKED: Tests failed in task scope. Fix before marking complete:" >&2
    echo "$TEST_OUTPUT" | tail -30 >&2
    exit 2
fi

TOTAL=$(go tool cover -func="$COVERPROFILE" 2>/dev/null \
        | { grep "total:" || true; } | awk '{print $NF}' | sed 's/%//')
if [ -z "$TOTAL" ]; then
    echo "WARNING: Could not determine coverage. Proceeding."
    exit 0
fi
# 任务级 coverage_floor 覆盖全局 dev_minimum（历史包袱重的包按其既有水位设定）。
# 必须放在这里而非文件头：TASK_ID 在第 26 行才解析出来。
TASK_FLOOR=$(jq -r '.coverage_floor // empty' ".arcforge/tasks/${TASK_ID}.json" 2>/dev/null)
if [ -n "$TASK_FLOOR" ]; then
    echo "Task-level coverage_floor=${TASK_FLOOR} overrides dev_minimum=${DEV_MIN}"
    DEV_MIN="$TASK_FLOOR"
fi

if [ "${TOTAL%.*}" -lt "$DEV_MIN" ]; then
    echo "BLOCKED: Task-scope coverage ${TOTAL}% < dev_minimum ${DEV_MIN}%." >&2
    go tool cover -func="$COVERPROFILE" | grep -v "100.0%" >&2
    exit 2
fi

echo "Task scope passes. Coverage: ${TOTAL}% (dev_minimum: ${DEV_MIN}%)"
exit 0
