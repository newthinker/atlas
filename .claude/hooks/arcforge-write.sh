#!/bin/bash
# arcforge-write.sh — .arcforge/ 状态文件唯一写入通道(防线 2,ISSUE-4)
# 校验「声明身份 × 权限矩阵 × 状态迁移」后原子写,并维护 last_transition 审计字段。
#
# 用法(内容一律走 stdin):
#   arcforge-write.sh --as <me> task <TASK-ID> create                 # 新任务 JSON(status 必须 pending)
#   arcforge-write.sh --as <me> task <TASK-ID> transition <new-status> [--expect-epoch N] [--field k=v ...] [--json-field k=json ...]
#     · transition rejected/review_fix 必带 --field reason_class=<task_defect|dod_defect|env_infra|no_progress>(A-2)
#     · rejected->assigned / review_fix->in_progress 时脚本自动 rework_count+1(env_infra 豁免);rework_count 由脚本管理,禁经 --field/--json-field 写
#   arcforge-write.sh --as <me> task <TASK-ID> update [--expect-status S] [--field k=v ...] [--json-field k=json ...]
#     · TASK-014:带字段的 update 也向 tasks/transitions.jsonl 追加一行审计
#       {task,op:"update",by,at,keys,changes};changes 对字符串数组(packages/writes)记
#       added/removed,其余只记 {type,len} 摘要 —— **绝不记字段值全文**
#   arcforge-write.sh --as <me> plan                                  # docs/03-progress/plan.md
#   arcforge-write.sh --as <me> verdict <filename.md>                 # docs/05-review/<filename>
#   arcforge-write.sh --as <me> discovery <TASK-ID>                   # discoveries/<TASK-ID>.json
#   arcforge-write.sh --as <me> wisdom <learnings|decisions|digest>   # learnings/decisions 只追加
#   arcforge-write.sh --as <me> checkpoint                            # checkpoints/<me>-checkpoint.md
#   arcforge-write.sh --as <me> doc <docs/ 下相对路径>
#   arcforge-write.sh --as leader matrix set-token <instance>       # token 明文走 stdin,存 sha256(仅 leader)
#     重置已登记实例须经 ARCFORGE_TOKEN/--token 携带其当前 token(放大链收口);首次登记免旧 token。
#     残留局限:leader 本体不设防,冒名 leader 仍可为未登记实例首次登记 token(彻底封堵需外部注入 leader token,范围外)。
# 退出码: 0 成功;2 校验拒绝(stderr 含纠偏指引);1 环境/IO 错误
#
# 参数守卫(TASK-002,atlas 实测四次事故):
#   · **本脚本没有 --append/--file 参数**。多余参数一律 DENY 并原样列出,不再静默忽略
#     (`doc <path> --append` 曾覆盖整份 ADR;`discovery <id> --file x` 曾静默卡到超时)。
#     doc/discovery/checkpoint/plan/verdict 都是全量覆盖写;wisdom learnings|decisions 本就是追加。
#   · --field/--json-field 的 key 必须是合法**顶层**字段名([A-Za-z0-9_-]+):jq 程序是
#     .[$k]=$v,`questions[0].answer` 只会造出同名顶层垃圾字段;嵌套改动用 --json-field 整体替换。
#   · verified/accepted 后禁止覆盖既有 discovery(判定依据不得被静默替换);首写仍放行。
#   · **`packages`/`writes` 禁止随 `transition` 写**(TASK-015):迁移审计行只记 from/to,
#     那条路径会让范围变更完全不留痕。拆两步——先 `update --json-field packages=...`,再 transition。
#
# 跨 worktree 误写防线(TASK-015,实测复现):
#   · `.arcforge/` 是已跟踪目录,每个 linked worktree 里都有一份独立副本。在 worktree 里调用
#     写通道会写进副本、主仓库真相源不变**且 exit 0**。故主模式入口即判别
#     `git rev-parse --git-dir` vs `--git-common-dir`,不等 → DENY 并给出主仓库路径。
#   · 判别不出来(非 git 仓库 / git 不可用 / rev-parse 报错)→ **放行**(fail-open):误拒会
#     阻断唯一通道上的全部写入。⚠ 这是**代价权衡,不是「不漏防」**——三种成因里只有
#     「非 git 仓库」蕴含 worktree 不存在,另两种可发生在已存在的 worktree 里(详见
#     assert_main_worktree 头注释)。故补与 git 无关的兜底探针,仅在主判别式无法决断时生效。
#
# 验证对象漂移告警(TASK-007,atlas 实测:判定对象与最终交付物不是同一个 commit):
#   · `dev_done -> verifying` 时脚本自动写入 **`verify_baseline`** = {head, discovery_sha256, at}
#     ——承接那一刻的 `git rev-parse HEAD` 与 `discoveries/<ID>.json` 的 sha256。
#     该字段由脚本管理,**禁经 --field/--json-field 写**(同 assignment_epoch;能被一行命令
#     抹掉的守卫不是守卫)。`verifying -> verifying`(leader 逃生边)**不刷新**基线。
#   · `verifying -> verified` 时比对:一致放行;不一致则打出差异并**要求显式确认**——
#       transition verified --ack-drift <当前 HEAD 全 sha> [--ack-discovery-drift <当前 sha256>]
#     确认值取**当前值**而非记录值:必须真去看过现状才填得出。两个参数只在这条边合法,
#     用在别的边上 DENY(不沉默吞无意义参数)。
#   · 判据**收窄为声明范围内**(writes 优先,缺失回落 packages,与 task-completed.sh 同口径)
#     的文件变了:多 dev 并行时他人提交推进 HEAD 是常态,全量告警会退化成条件反射。
#   · `verifying -> rejected` **不受此约束**;非 git 仓库/无提交时基线记空,转 verified 时
#     跳过校验但打 WARN(有声跳过,不静默)。
#
# 单入口(TASK-001 sprint-005,取消 `--locked-task-write` 平行写通道):
#   · 本脚本**只有一个入口**——主模式。task transition/update 由主模式**原地取锁**
#     (acquire_task_lock)后调用 locked_task_write,不再 exec 回自己。
#   · 改动前的 `--locked-task-write` 在文件顶部分发,早于 worktree 判别、早于 token 读取、
#     早于 dev_done 门禁,一次绕过三道防线;且它是「被锁调用的那一端」,直接调用时
#     `.TASK-xxx.lock` 从未建立 ⇒ 「竞态窗口为零」在该路径下不成立。修法取「取消入口」
#     而非「给入口加校验」:加校验(如一次性 nonce)是在承认第二条通道存在的前提下补丁,
#     校验本身又要靠环境变量传递,而环境变量会随 exec 泄漏给下游进程;没有入口才没有旁路。
#   · with-task-lock.sh 保留不动:它仍是 agent 包裹自己「读-校验-写」的通用工具。
#   · ARCFORGE_LOCK_TIMEOUT(秒,默认 30)只缩短等锁时间,不放宽任何校验。
#
# dev_done 门禁(发现#1):transition dev_done 在锁外前置执行同目录 task-completed.sh,
# exit 非 0 → 拒绝整个迁移;门禁脚本缺失 → WARN 放行(fail-open)。限制:仓库根无
# go.mod 时门禁走 task-completed.sh 的 skip 分支(exit 0)放行——生产级 Go 门禁仅在
# 有 go.mod 的目标仓库触发(框架自身仓库根无 go.mod,validator 在子目录)。
set -uo pipefail

ARC=".arcforge"
MATRIX="$ARC/write-matrix.json"
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"

deny() { # <原因> <纠偏指引>
    echo "DENY: $1" >&2
    echo "  → $2" >&2
    exit 2
}
fail() { echo "ERROR: $1" >&2; exit 1; }

# sha256 十六进制摘要(stdin → stdout;macOS 用 shasum,Linux 用 sha256sum)
sha256_hex() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum | cut -d' ' -f1
    else shasum -a 256 | cut -d' ' -f1; fi
}

# TASK-007(atlas 实测):**判定对象与最终交付物不是同一个 commit**。TASK-018 的 dev 在 dev_done
# 之后 6 分钟又提交了一版,verifier 87 秒后判 VERIFIED —— 没有任何机制会告警。那次实质无碍
# (diff 仅注释),但若改的是断言,VERIFIED 就失效了而无人知晓。故承接(dev_done->verifying)时对
# 「HEAD + discovery 摘要」拍快照,转 verified 时比对。与 epoch、与 discovery 时机守卫同思路:
# **不禁止变化,只保证变化不会被静默吞掉**(堵在验证者侧,不堵 dev —— dev_done 后修正一条错误的
# 理由注释本身是对的,堵 dev 会误伤这类有价值的追加)。
# 下面三个取值器一律**不报错只返回空串**(git 不可用/非 git 仓库/无提交的 fresh init/文件不存在),
# 由调用方**有声**跳过 —— 取不到值绝不能变成整条写入失败。
current_head() {
    command -v git >/dev/null 2>&1 || return 0
    # 必须带 --verify:裸 `git rev-parse HEAD` 在 fresh init(无提交)时会把**字面串 "HEAD"**
    # 打到 stdout 后才失败,基线于是记成 "HEAD" 而非空,boundary[1] 的跳过分支永远进不去。
    git rev-parse --verify HEAD 2>/dev/null || true
}
discovery_sha() { # <TASK-ID>;文件不存在返回空串(与「存在且内容为 X」可区分)
    local f="$ARC/discoveries/$1.json"
    [ -f "$f" ] && sha256_hex < "$f" || true
}
# 声明范围:writes(互斥/变更口径)优先,**字段缺失**才回落 packages(覆盖率口径)——
# 与 task-completed.sh 的 `if .writes == null then .packages else .writes end` 逐字同口径,
# 两处口径分叉会让「改了什么算越界」在门禁与漂移告警之间自相矛盾。
task_scope_paths() { jq -r '(if .writes == null then .packages else .writes end)[]?' "$1" 2>/dev/null; }

# C1/C2:状态机受控字段不得经 --field/--json-field 旁路写入(否则 update/transition 可绕过迁移授权)。
# status/last_transition/assignment_epoch 一律禁写;assigned_to/verifier 仅 leader 在 transition(派发/指派)时可写。
# 普通业务字段(progress_note/questions/discovery/reject_reason/fix_items 等)放行。
# 依赖调用处 $ME/$MODE/$NEW 在作用域内(locked_task_write 不声明 local,三者全局可见);$2 为写入来源(field|json-field)。
guard_field_key() { # <key> <src: field|json-field>
    case "$1" in
        packages|writes)
            # TASK-015(TASK-014 交付时如实登记的残留缺口 R1,test-agent-49 复现确认):
            # transition 的审计行只有 from/to,于是 `transition <s> --json-field packages='[...]'`
            # **扩围成功而毫无痕迹** —— 一个 flag 就绕过 TASK-014 刚建立的全部范围变更审计。
            # 而 packages/writes 就是门禁作用域本身(task-completed.sh 据它决定跑哪些包、
            # validator 据它做 scope 互斥),TASK-008 之后 writes 还同时驱动漂移判据:
            # **声明与实际写入不一致 = 范围外的真漂移不会告警**,故这是安全属性而非文档洁癖。
            # 它也不是对抗场景专属 —— 把状态推进与范围声明写进一条命令是会被**误撞**的正常写法。
            # 修法取「拒绝」而不是「transition 也追加一行审计」:后者会改变 jsonl 的**行流**,
            # 打红 UB0 的「一次 transition 只产出一行」与 TASK-001 的行数断言(M4 变异实测);
            # 而拒绝是 guard_field_key 式的,一行审计都不新增,与 boundary[0] 零冲突。
            [ "$MODE" != "transition" ] || \
                deny "声明范围字段「$1」禁止随 transition 写(迁移审计行只记 from/to,这条路径会让范围变更完全不留痕)" \
                    "拆成两步:先 task <ID> update --json-field $1='[...]'(留下 added/removed 审计),再 task <ID> transition <目标状态>" ;;
        status|last_transition|assignment_epoch|rework_count|verify_baseline)
            deny "字段「$1」由状态机/脚本管理,禁止经 --field/--json-field 写" \
                "状态变更走 task transition;审计/epoch/返工计数/漂移基线由脚本自动维护" ;;
        assigned_to|verifier)
            { [ "$ME" = "leader" ] && [ "$MODE" = "transition" ]; } || \
                deny "角色绑定字段「$1」仅允许 leader 在 transition(派发/指派)时写" \
                    "其它情况禁止改绑定;改派由 leader 经 transition 操作" ;;
        reason_class)
            # C-2/W-3:reason_class 唯一合法写路径 = --field 随 transition rejected|review_fix(过下方 A-2 枚举校验)。
            # 堵两类走私:①--json-field 同名键后写覆盖内联影子变量落盘 env_infra 绕熔断;②update 直接写旁路 A-2 校验。
            { [ "$2" = "field" ] && [ "$MODE" = "transition" ] && \
              { [ "$NEW" = "rejected" ] || [ "$NEW" = "review_fix" ]; }; } || \
                deny "reason_class 只能经 --field 随 transition rejected|review_fix 写" \
                    "去掉 --json-field/update 写法;返工分类用 --field reason_class=<task_defect|dod_defect|env_infra|no_progress> 随熔断迁移提交" ;;
    esac
}

# TASK-006(sprint-005;本文件另一处 `TASK-006 实测:派验后 53 秒…` 是上个 sprint 的同号任务,
# 任务 ID 随归档重置,两者无关)error_handling[0]:guard_field_key 只判「谁能写这个 key」,不看**值**。
# 于是 assigned_to/verifier 可被写成任意字符串(含换行/引号/路径符),而这两个字段正是
# 角色绑定判据(locked_task_write 的 `[ "$BOUND" = "$ME" ]`)与 stale-dispatch 的 owner。
# 写进去的垃圾不会当场报错,只会让该任务此后**没有任何实例能通过绑定校验**(--as 侧的
# validate_instance 拒绝同形的名字),即一条静默的自锁。故在入口对值做同一套整串校验。
#
# 空串是**合法**的:`transition verifying --field verifier=` 是文档化的「两步式先置空收回」
# 第一步(CLAUDE.md 逃生边;test-arcforge-write.sh 的 "E4: 两步式先置空收回" 钉着它)。
# 判据因此是「非空则须在册」,不是「必须在册」。
guard_instance_field_value() { # <key> <已解码为字符串的值>
    case "$1" in
        assigned_to|verifier)
            [ -n "$2" ] || return 0
            validate_instance "$2" "派发/改派只能写在册实例名;要收回请写空值(--field $1=)" ;;
    esac
}

# wisdom #6:--field 一律落字符串,数值语义字段误用会产生 "1" 这类字符串值,
# 破坏 validator 数值断言与 epoch/rework 运算。数值字段必须走 --json-field。
# 注:rework_count 已升格为脚本管理字段(见 guard_field_key),两种写法都被拦,此处仅余 wave。
guard_numeric_field() { # <key>
    case "$1" in
        wave)
            deny "字段「$1」是数值语义,--field 会写成字符串" \
                "改用 --json-field $1=<数字>" ;;
    esac
}

# 第 9 项(atlas 实测):--field/--json-field 的 jq 程序是 .[$k]=$v,$k 是**字面顶层 key**,
# 不是路径表达式。传 questions[0].answer 会造出一个名为 "questions[0].answer" 的顶层垃圾字段——
# 数据进了文件但在错误位置,且 validator 不校验未知字段故全程绿(atlas TASK-011 至今留着
# questions[0].answer 与 coverage_floor: 两个这样的字段)。这比静默失败更糟,故入口即拒。
# 白名单整串校验(与 validate_task_id/validate_instance 同范式;case 的 * 含换行,故换行/引号/
# 路径符一并挡掉)。
guard_field_name() { # <key> <整个 k=v 参数> <src: field|json-field>
    local hint="字段名只含字母/数字/下划线/连字符;嵌套结构请用 --json-field <顶层字段>=<完整 JSON> 整体替换(如 --json-field questions='[{\"q\":\"..\",\"answer\":\"..\"}]')"
    case "$2" in
        *=*) : ;;
        *) deny "「--$3 $2」缺少 =(应为 <字段名>=<值>;缺 = 会把字段名与值解析成同一串)" "$hint" ;;
    esac
    [ -n "$1" ] || deny "「--$3 $2」字段名为空" "$hint"
    case "$1" in
        *[!A-Za-z0-9_-]*) deny "字段名「$1」不是合法顶层字段名(含 . [ ] : 空格等字符)" "$hint" ;;
    esac
}

# 第 5 项(atlas 实测四次事故):各子命令 shift 掉位置参数后从不检查 $@ 剩余 ⇒ 对不认识的输入
# 一律沉默。`doc <path> --append` 因此覆盖了整份 ADR(580 行 26 条决策 → 29 行 1 条):--append
# 在本脚本里根本不存在,而 doc 是全量覆盖写;`discovery <id> --file <path>` 则静默卡到超时。
# --append 之所以被当成通用语法,是因为它在 wisdom learnings(本就是 >> 追加)上「成功」过——
# 多余参数被忽略而结果恰好符合预期。故:多余参数一律 DENY 并原样列出。
reject_extra_args() { # <子命令用法名> <剩余参数...>
    local cmd="$1"; shift
    [ $# -eq 0 ] && return 0
    deny "子命令「${cmd}」不接受多余参数: $*" \
        "内容一律走 stdin(如 cat f | arcforge-write.sh --as <me> ${cmd});本脚本没有 --append/--file 参数——doc/discovery/checkpoint/plan/verdict 都是全量覆盖写,wisdom learnings|decisions 本就是追加"
}

# W2:文件名/相对路径参数拒绝路径穿越(.. 或以 / 开头);无此二者即保证规范化后仍落在目标目录内。
reject_unsafe_path() { # <路径参数>
    case "$1" in
        /*)    deny "路径「$1」不能以 / 开头(必须是相对 .arcforge 的路径)" "用相对路径,如 docs/05-review/x.md" ;;
        *..*)  deny "路径「$1」含 ..(路径穿越)" "禁止用 .. 逃出目标目录" ;;
    esac
}

# C6(backlog-c #7):TASK_ID 全局格式约束——task/discovery 分支入口共用此校验。
# 格式外字符(引号/空格/../ /$ /换行 等)入口即 DENY,不落盘、不进 printf,根除注入面。
# 用单个 case 对整串做 glob 匹配(等价 ^TASK-[A-Za-z0-9_-]+$),不用 echo|grep -qE:
# grep 按行匹配,带换行的 ID 首行合法即整体放行(QA review #1 换行走私绕过);
# case 的 * 含换行,故 TASK- 后夹带的换行/回车/路径符会命中 [!A-Za-z0-9_-] 被拒。
validate_task_id() { # <TASK-ID>
    local hint="任务 ID 只含字母/数字/连字符/下划线,且不含换行;禁止引号、空格、路径字符"
    case "$1" in
        TASK-*[!A-Za-z0-9_-]*)  # TASK- 开头但夹带非法字符(含换行/回车/路径符)
            deny "TASK_ID「$1」格式非法(含非法字符,须整串匹配 ^TASK-[A-Za-z0-9_-]+\$)" "$hint" ;;
        TASK-?*) : ;;           # TASK- 开头且后有内容(等价正则的 +),合法
        *)                      # 缺 TASK- 前缀,或 TASK- 后无内容
            deny "TASK_ID「$1」格式非法(须整串匹配 ^TASK-[A-Za-z0-9_-]+\$)" "$hint" ;;
    esac
}

# R1(review_fix):实例名整串校验,对齐 validate_task_id 范式,堵 echo|grep -qE 换行走私——
# grep 按行匹配,带换行的名字首行合法即整体放行,而 ME 会流入 checkpoints/${ME}、
# wisdom/learnings-${ME} 路径与 tokens key,INSTANCE 流入 tokens key。case 的 * 含换行,
# 故换行/回车/路径符/大写夹带命中 [!a-z0-9-] 被拒。结构等价 write-matrix.instances 正则
# ^(leader|(dev|test|qa|ops)-[a-z0-9-]+)$。
validate_instance() { # <name> <deny-hint>
    case "$1" in
        *[!a-z0-9-]*) deny "「$1」含非法字符(实例名只含小写字母/数字/连字符,不含换行/回车/路径符)" "$2" ;;
    esac
    case "$1" in
        leader|dev-?*|test-?*|qa-?*|ops-?*) : ;;
        *) deny "「$1」不是在册实例(须匹配 ^(leader|(dev|test|qa|ops)-[a-z0-9-]+)\$)" "$2" ;;
    esac
}

# TASK-014(test-agent-48 在验证 TASK-005 时发现):transitions.jsonl 的审计追加原本被
# `if [ "$MODE" = "transition" ]` 包住 ⇒ **`update` 模式的 --field/--json-field 完全不留痕**。
# 而 packages/writes **就是门禁作用域本身**(task-completed.sh 用它决定跑哪些包、validator
# 用它做 scope 互斥),扩大它等于削弱门禁 —— 最该留痕的一类写偏偏是唯一不留痕的。
# 实撞:验证者想核实「dev 只加了这一个、没扩别的」时无从查起,只能靠「声明恰好 N 项 =
# 提交恰好 N 个文件」的零 slack 对齐间接判定,而那只在「声明 ⊆ 实际」时成立。
#
# 三条口径:
#  ① **只记「改了哪些 key」+「字符串数组增删了哪几项」,绝不记字段值全文**。description /
#     done_criteria 动辄数千字,全量入 jsonl 会把它撑爆;字符串/数组/对象一律降为
#     {type,len} 摘要,数值/布尔才记 value(有界)。整行超 4096 字节再降级为只记 key。
#  ② **用 jq -c 生成而非 printf**。上面 transition 的 printf 拼 JSON 之所以安全,是因为
#     它的每个插值都过了白名单校验;而 --field 的**值**不受任何约束(guard_field_name 只
#     管 key),可含引号/换行/反斜杠 —— printf 会造出非法/多行的行,而 ParseTransitions
#     是**按行解析**的,一行写坏就是一条审计凭空消失。
#  ③ **不带 from/to**。ParseTransitions 的阶段耗时按 .to 分派(validator/progress/
#     transitions.go 的 switch),留空即不匹配任何阶段,不干扰 Dev/Verify 耗时统计。
update_audit_line() { # <旧文件快照> <新文件> <TASK-ID> <me> <at> <被写的 key...>
    local old="$1" new="$2" task="$3" by="$4" at="$5"; shift 5
    jq -c -n --slurpfile old "$old" --slurpfile new "$new" \
        --arg task "$task" --arg by "$by" --arg at "$at" '
        def summ: if . == null then {type:"absent"}
            elif type=="string" then {type:"string",len:length}
            elif type=="array"  then {type:"array",len:length}
            elif type=="object" then {type:"object",len:(keys|length)}
            else {type:type,value:.} end;
        def strarr: type=="array" and all(.[]; type=="string");
        ($old[0]) as $o | ($new[0]) as $n |
        [ $ARGS.positional[] as $k | ($o[$k]) as $a | ($n[$k]) as $b |
          if ($b|strarr) and (($a==null) or ($a|strarr))
          then {key:$k, added:[($b - ($a // []))[] | .[0:200]], removed:[(($a // []) - $b)[] | .[0:200]]}
          else {key:$k, before:($a|summ), after:($b|summ)} end ] as $ch |
        {task:$task,op:"update",by:$by,at:$at,keys:$ARGS.positional,changes:$ch} |
        if (tojson|length) > 4096 then del(.changes)+{changes_omitted:true} else . end
        ' --args "$@"
}

SUBAGENT_HINT="若你是子代理:禁止写状态文件,把结论写进最终回复带回父 agent,由其落盘"

# ---------- 任务锁:主模式原地取锁(TASK-001,sprint-005) ----------
# 改动前 transition/update 走 `exec with-task-lock.sh <ID> <self> --locked-task-write …`,
# 于是脚本**必须**暴露一个内部入口。而那个入口在文件顶部分发,早于 assert_main_worktree、
# 早于 TOKEN 读取、也早于 dev_done 门禁 —— 谁直接调它就一次绕过三道防线;更要命的是它是
# 「被锁调用的那一端」,直接调用时根本没有调用方去建 `.TASK-xxx.lock`,「竞态窗口为零」
# 在该路径下不成立。⇒ **取消双入口**:锁在主模式原地取,内部入口不复存在,没有入口就没有旁路。
#
# with-task-lock.sh 本身保留不动(CLAUDE.md 教 agent 用它包裹自己的读-校验-写),这里内联的是
# 它的 flock/mkdir 双路径。清理路径刻意**只留一条**(EXIT trap):deny/fail/jq 失败都经 exit
# 收敛到它。若函数内再各自 rmdir,就会出现「trap 已跑过、函数又删一次」——第二次 rmdir 删的
# 可能是**别人刚建立的同名锁目录**,把互斥直接拆掉。
#
# 超时取 30s,与改动前 with-task-lock.sh 逐字相同。ARCFORGE_LOCK_TIMEOUT 只缩短等待
# (fail-closed 方向:等得越短越倾向拒绝),不放宽任何校验;它的存在是为了让「取锁失败必须
# 出声拒绝」这条边可以在 2 秒内被测到,而不是只能等满 30 秒。
TASK_LOCK_DIR=""
release_task_lock() {
    [ -n "$TASK_LOCK_DIR" ] && rmdir "$TASK_LOCK_DIR" 2>/dev/null
    TASK_LOCK_DIR=""
}
acquire_task_lock() { # <TASK-ID>
    local base="$ARC/tasks/.$1.lock" secs tries i
    secs="${ARCFORGE_LOCK_TIMEOUT:-30}"
    case "$secs" in ''|*[!0-9]*) fail "ARCFORGE_LOCK_TIMEOUT 必须是非负整数,实得「$secs」" ;; esac
    if command -v flock >/dev/null 2>&1; then
        # fd 形态(util-linux):锁随**进程退出**由内核释放,没有清理路径可竞争,故不挂 trap。
        exec 9>"${base}file" || fail "无法创建锁文件 ${base}file(目录不可写?),拒绝无锁写入"
        flock -w "$secs" 9 || fail "获取 $1 锁超时(${secs}s),本次写入未落盘 —— 疑似有长事务或死锁残留: ${base}file"
        return 0
    fi
    # 无 flock(macOS):mkdir 自旋锁(mkdir 是原子的)。trap 先挂再 mkdir:反序则
    # 「mkdir 成功后立刻被 kill」会漏清理;TASK_LOCK_DIR 为空时 trap 是无害 no-op。
    trap 'release_task_lock' EXIT
    tries=$((secs * 10))
    for i in $(seq 1 "$tries"); do
        if mkdir "$base" 2>/dev/null; then TASK_LOCK_DIR="$base"; return 0; fi
        sleep 0.1
    done
    fail "获取 $1 锁超时(${secs}s),本次写入未落盘(疑似死锁残留: $base)
       确认无活动 agent 进程后手动清理: rmdir $base"
}

# ⚠ 给后续在这条控制流上加东西的人(TASK-001 起本文件只剩一条入口,改动都会落在这里):
#   · **EXIT trap 已被 mkdir 分支占用**。要加自己的收尾动作请**追加**(把 release_task_lock
#     也写进新 trap),`trap '…' EXIT` 直接覆盖会把锁清理踢掉 —— 锁目录留在 .arcforge/tasks/,
#     下一个人自旋满 30 秒后报「疑似死锁残留」,而现场没有任何线索指向是谁覆盖的。
#   · **fd 9 已被 flock 分支占用**,别再拿它做重定向。
#
# ---------- 任务锁临界区内的读-校验-写 ----------
# 由主模式在 acquire_task_lock 之后**原地调用**(不再是可被外部触达的第二个入口)。
# ME/FILE/MODE/NEW 刻意**不声明 local**:下游(审计行、validator 挂载)按全局可见消费它们。
locked_task_write() { # <me> <task-file> <transition|update> <new-status|-> [args...]
    ME="$1"; FILE="$2"; MODE="$3"; NEW="$4"; shift 4   # MODE=transition|update;update 时 NEW="-"
    CUR=$(jq -r '.status' "$FILE") || fail "读取 $FILE 失败"
    TID=$(basename "$FILE" .json)   # 任务 ID:漂移基线取 discovery 摘要与告警文案共用

    if [ "$MODE" = "transition" ]; then
        WRITERS=$(jq -c --arg e "${CUR}->${NEW}" '.transitions[$e] // empty' "$MATRIX")
        [ -n "$WRITERS" ] || deny "非法迁移 ${CUR}->${NEW}(当前状态以文件为准,可能已被并发更新)" \
            "合法边见 write-matrix.json 的 transitions;若你认为该推进,发 inbox 通知对应 owner"
    else
        WRITERS=$(jq -c --arg s "$CUR" '.owner_table[$s] // empty' "$MATRIX")
        [ -n "$WRITERS" ] || deny "状态 ${CUR} 无 owner_table 条目" "矩阵未登记该状态的合法写者"
    fi

    MATCH=0
    for W in $(echo "$WRITERS" | jq -r '.[]'); do
        case "$ME" in $W) MATCH=1; break ;; esac
    done
    [ "$MATCH" = 1 ] || deny "「${ME}」无权执行 ${CUR}$( [ "$MODE" = transition ] && echo "->${NEW}" ) 写入(合法写者: $WRITERS)" \
        "Leader 专属动作请发 inbox 通知 leader;$SUBAGENT_HINT"

    # 角色绑定:dev 只能写派给自己的任务,test 只能写自己验证的任务
    BIND=$(jq -r --arg me "$ME" \
        '.bindings | to_entries[] | select(.key as $p | $me | test("^" + ($p | gsub("\\*"; ".*")) + "$")) | .value' \
        "$MATRIX" | head -1)
    if [ -n "$BIND" ]; then
        BOUND=$(jq -r --arg f "$BIND" '.[$f] // empty' "$FILE")
        [ "$BOUND" = "$ME" ] || deny "该任务 ${BIND}=「${BOUND}」,不是你($ME)" "只能写绑定到你实例的任务"
    fi

    PROG='.'
    ARGS=()
    AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)   # TASK-014:update 审计行也要 at,故上提到公共位置(transition 行格式不变)
    if [ "$MODE" = "transition" ]; then
        PROG="$PROG | .status=\$new | .last_transition={from:\$cur,to:\$new,by:\$me,at:\$at}"
        ARGS+=(--arg new "$NEW" --arg cur "$CUR" --arg me "$ME" --arg at "$AT")
        # F5:每次(重)派 epoch +1
        [ "$NEW" = "assigned" ] && PROG="$PROG | .assignment_epoch=((.assignment_epoch // 0)+1)"
        # A-2:返工自动计数——重派回 Dev 时脚本 +1(env_infra 豁免),计数从 Leader 自觉变机制。
        if { [ "$CUR" = "rejected" ] && [ "$NEW" = "assigned" ]; } || \
           { [ "$CUR" = "review_fix" ] && [ "$NEW" = "in_progress" ]; }; then
            RC_PREV=$(jq -r '.reason_class // ""' "$FILE")
            [ "$RC_PREV" = "env_infra" ] || PROG="$PROG | .rework_count=((.rework_count // 0)+1)"
        fi
        # TASK-007:承接即拍快照,作为「验证者判定对象」的基线。
        # **只在 dev_done->verifying 这一条边记录**:verifying->verifying(leader 逃生边改派 verifier)
        # 刻意不刷新 —— 继承更早的基线只会更容易告警、不会漏报(保守方向),且不给 leader 留一条
        # 「自环即可清掉漂移证据」的杠杆。新 verifier 为承接前就发生的漂移确认一次,成本极低。
        if [ "$CUR" = "dev_done" ] && [ "$NEW" = "verifying" ]; then
            VB_HEAD=$(current_head)
            VB_DSHA=$(discovery_sha "$TID")
            [ -n "$VB_HEAD" ] || echo "WARN: 非 git 仓库或 HEAD 不存在(fresh init 无提交),verify_baseline.head 记为空;转 verified 时将跳过 HEAD 漂移校验" >&2
            PROG="$PROG | .verify_baseline={head:\$vbh,discovery_sha256:\$vbd,at:\$at}"
            ARGS+=(--arg vbh "$VB_HEAD" --arg vbd "$VB_DSHA")
        fi
    fi
    EXPECT_EPOCH=""
    EXPECT_STATUS=""
    ACK_HEAD=""
    ACK_DSHA=""
    REASON_CLASS=""
    # TASK-014:本次被写的 key,供 update 审计用。用**空格分隔字符串**而非数组 ——
    # macOS 自带 bash 3.2 下 `set -u` + 空数组展开 `${A[@]}` 会报 unbound variable。
    # key 已被 guard_field_name 限定为 [A-Za-z0-9_-]+,故展开时的词分割安全、无 glob 面。
    UPD_KEYS=""
    N=0
    while [ $# -gt 0 ]; do
        case "$1" in
            --expect-epoch)
                EXPECT_EPOCH="${2:?--expect-epoch 需要一个数字}"
                echo "$EXPECT_EPOCH" | grep -qE '^[0-9]+$' || fail "--expect-epoch 必须是非负整数" ;;
            --expect-status)
                # M1c-4:epoch 挡「你还是不是 owner」,挡不住「状态在你手上变了」——二者正交,
                # 而只有前者有锁。实撞:dev 读到 dev_done、拼装 done_criteria、写入时已是
                # verifying(派验切在读与写之间,窗口 4 分钟),而 update 没有乐观锁可用。
                EXPECT_STATUS="${2:?--expect-status 需要一个状态名}"
                echo "$EXPECT_STATUS" | grep -qE '^[a-z_]+$' || fail "--expect-status 必须是小写状态名(如 in_progress)" ;;
            --ack-drift)
                ACK_HEAD="${2:?--ack-drift 需要一个 commit sha}" ;;
            --ack-discovery-drift)
                ACK_DSHA="${2:?--ack-discovery-drift 需要一个 sha256}" ;;
            --field)
                K="${2%%=*}"; V="${2#*=}"
                guard_field_name "$K" "$2" field; guard_field_key "$K" field; guard_numeric_field "$K"
                guard_instance_field_value "$K" "$V"
                [ "$K" = "reason_class" ] && REASON_CLASS="$V"
                UPD_KEYS="$UPD_KEYS $K"
                PROG="$PROG | .[\$k$N]=\$v$N"
                ARGS+=(--arg "k$N" "$K" --arg "v$N" "$V") ;;
            --json-field)
                K="${2%%=*}"; V="${2#*=}"
                guard_field_name "$K" "$2" json-field; guard_field_key "$K" json-field
                # 同一条判据必须覆盖两条写路径:只守 --field 等于把 --json-field 留成旁路
                # (guard_field_key 对 assigned_to/verifier 两条路径同等放行 leader)。
                # 值是原始 JSON,先解码成字符串再走同一个校验;非字符串(数字/数组/null)直接拒。
                case "$K" in
                    assigned_to|verifier)
                        # ⚠ `${K}` / `${V}` 的花括号**不可省**:它们后面紧跟多字节的「」,而 bash 3.2
                        # 在 UTF-8 locale 下会把该字符的首字节并进变量名,`set -u` 当场
                        # `K<高位字节>: unbound variable` 退出(exit 127)——DENY 变成了崩溃。
                        # 本仓库的 DENY 文案全是中文、几乎每条都紧跟 $VAR,是这个陷阱的高发区。
                        JV=$(printf '%s' "$V" | jq -er 'if type=="string" then . else empty end' 2>/dev/null) \
                            || deny "角色绑定字段「${K}」的值必须是 JSON 字符串,实得「${V}」" \
                                "改用 --field ${K}=<实例名>(普通写法),或传合法 JSON 字符串如 --json-field ${K}='\"dev-agent-1\"'"
                        guard_instance_field_value "$K" "$JV" ;;
                esac
                UPD_KEYS="$UPD_KEYS $K"
                PROG="$PROG | .[\$k$N]=\$jv$N"
                ARGS+=(--arg "k$N" "$K" --argjson "jv$N" "$V") ;;
            *) fail "未知参数 $1" ;;
        esac
        N=$((N+1)); shift 2
    done

    # A-2:rejected/review_fix 必带熔断原因分类(与 validator 枚举一字不差)。
    # 校验在锁临界区内、jq 落盘之前:缺失/非法即 DENY,任务文件完全无变更。
    if [ "$MODE" = "transition" ]; then
        case "$NEW" in
        rejected|review_fix)
            case "$REASON_CLASS" in
                task_defect|dod_defect|env_infra|no_progress) ;;
                "") deny "transition $NEW 必须携带 --field reason_class=<task_defect|dod_defect|env_infra|no_progress>" \
                    "按判定表选择(见 test-agent/qa-agent 定义):测试失败→task_defect;DoD 矛盾→dod_defect;限流/超时/环境→env_infra;反复无进展→no_progress" ;;
                *) deny "非法 reason_class「${REASON_CLASS}」" \
                    "合法值: task_defect|dod_defect|env_infra|no_progress" ;;
            esac ;;
        esac
    fi

    # M1c-4:status 断言与 epoch 断言同位(锁内、落盘前),但回答的是**另一个**问题:
    # epoch 答「任务还是不是派给我的」,status 答「我读到的那个状态还在不在」。
    # CUR 是本函数开头在锁内读出的当前状态,与调用方读到的值比对即可闭合读-校验-写。
    # ⚠ transition 不需要它:非法迁移边本身就会被 transitions 表拒掉;update 没有那层保护。
    if [ -n "$EXPECT_STATUS" ]; then
        [ "$MODE" = "update" ] || deny \
            "--expect-status 只用于 update(transition 的状态断言由迁移边本身完成)" \
            "transition 请用 --expect-epoch;若你要断言起始状态,说明该边可能已被并发推进"
        [ "$CUR" = "$EXPECT_STATUS" ] || deny \
            "status 断言失败:文件 status=$CUR ≠ 你读到的 $EXPECT_STATUS(状态已在你读与写之间变化)" \
            "放弃本次写入,重读任务文件确认当前 owner 后再决定——你可能已不是合法写者"
    fi

    # F5:epoch 断言在锁临界区内、重读 assignment_epoch 之后、jq 落盘之前——
    # 重读-校验-写的原子性由 with-task-lock 保证,竞态窗口为零。持有的 epoch 与
    # 文件当前 epoch 不符 = 任务已被重派,放弃本次写入(把「Dev 持 epoch 写回」从自觉变机制)。
    if [ -n "$EXPECT_EPOCH" ]; then
        CUR_EPOCH=$(jq -r '.assignment_epoch // 0' "$FILE")
        [ "$CUR_EPOCH" = "$EXPECT_EPOCH" ] || deny \
            "epoch 断言失败:文件 assignment_epoch=$CUR_EPOCH ≠ 你持有的 $EXPECT_EPOCH(任务已被重派)" \
            "放弃本次写入,回到任务扫描循环重新认领(F5 认领协议)"
    fi

    # ---- TASK-007:验证对象漂移告警。同样在锁临界区内、jq 落盘之前,与 epoch 断言同位。----
    # 只作用于 verifying->verified 这一条边:**rejected 完全不查** —— 判为不通过时交付物变没变
    # 都不影响结论,强加确认只会拖慢返工。
    IS_VERIFY_EDGE=0
    { [ "$MODE" = "transition" ] && [ "$NEW" = "verified" ] && [ "$CUR" = "verifying" ]; } && IS_VERIFY_EDGE=1
    # 确认参数用在别的边上是无意义输入 —— TASK-002 口径:不认识/不生效的输入一律 DENY,
    # 不得沉默(`--append` 之所以被当成通用语法,正是因为它在某处被静默忽略而「碰巧成功」过)。
    if [ "$IS_VERIFY_EDGE" = 0 ] && [ -n "${ACK_HEAD}${ACK_DSHA}" ]; then
        deny "--ack-drift/--ack-discovery-drift 只用于 verifying->verified(本次是 ${CUR}$( [ "$MODE" = transition ] && echo "->${NEW}" ))" \
            "漂移确认只对「验证判定的对象变没变」有意义;rejected 不受漂移约束,不需要也不接受确认参数"
    fi
    if [ "$IS_VERIFY_EDGE" = 1 ]; then
        VB=$(jq -c '.verify_baseline // empty' "$FILE")
        DRIFT_DETAIL=""; ACK_NEEDED=""; ACK_USED=0
        if [ -z "$VB" ]; then
            echo "WARN: $TID 无 verify_baseline(本机制上线前就已进入 verifying 的存量任务),跳过漂移校验" >&2
        else
            B_HEAD=$(echo "$VB" | jq -r '.head // ""')
            B_DSHA=$(echo "$VB" | jq -r '.discovery_sha256 // ""')
            # --- HEAD 漂移 ---
            NOW_HEAD=$(current_head)
            if [ -z "$B_HEAD" ]; then
                echo "WARN: $TID 的 verify_baseline.head 为空(承接时非 git 仓库/无提交),跳过 HEAD 漂移校验" >&2
            elif [ -z "$NOW_HEAD" ]; then
                echo "WARN: 现在取不到 HEAD(git 不可用/非 git 仓库),跳过 HEAD 漂移校验" >&2
            elif [ "$NOW_HEAD" != "$B_HEAD" ]; then
                # boundary[0]:判据**收窄为声明范围内的文件变了**,而不是「HEAD 变了」。
                # 多 dev 并行时 HEAD 因他人提交而前进是常态;若每次都要确认,确认就退化成条件反射,
                # 真漂移的信号被噪声淹没(告警疲劳)。收窄后 DENY 一响就一定与本任务的交付物有关。
                SCOPE=()
                while IFS= read -r p; do [ -n "$p" ] && SCOPE+=("$p"); done < <(task_scope_paths "$FILE")
                if ! git cat-file -e "${B_HEAD}^{commit}" 2>/dev/null; then
                    # 基线 commit 不可达(分支被 rebase/amend/强推):无法机械核对 ⇒ 按漂移处置(fail-closed)
                    DRIFT_STAT="    承接时记录的 commit 已不可达(分支被 rebase / amend / 强推?),无法机械核对差异"
                elif [ ${#SCOPE[@]} -gt 0 ]; then
                    DRIFT_STAT=$(git diff --stat=200,180 "$B_HEAD" "$NOW_HEAD" -- "${SCOPE[@]}" 2>/dev/null | sed 's/^/    /')
                else
                    DRIFT_STAT=$(git diff --stat=200,180 "$B_HEAD" "$NOW_HEAD" 2>/dev/null | sed 's/^/    /')   # 无声明范围 ⇒ 保守取全树
                fi
                if [ -z "$DRIFT_STAT" ]; then
                    echo "INFO: HEAD 已从 ${B_HEAD:0:12} 前进到 ${NOW_HEAD:0:12},但声明范围内无变更,判定对象未漂移" >&2
                elif [ -n "$ACK_HEAD" ] && [ "$ACK_HEAD" = "$NOW_HEAD" ]; then
                    echo "INFO: HEAD 漂移已由 $ME 显式确认(--ack-drift $NOW_HEAD)" >&2; ACK_USED=1
                else
                    # error_handling[0]:说清「承接时是 X,现在是 Y,中间这些文件变了」,而非只说「HEAD 变了」
                    DRIFT_DETAIL="${DRIFT_DETAIL}  你承接 $TID 时的 HEAD: $B_HEAD
  现在的 HEAD          : $NOW_HEAD
  这期间**声明范围内**变更的文件(git diff --stat ${B_HEAD:0:12}..${NOW_HEAD:0:12} -- <声明范围>):
$DRIFT_STAT
"
                    ACK_NEEDED="$ACK_NEEDED --ack-drift $NOW_HEAD"
                fi
            fi
            # --- discovery 漂移(与 HEAD 同处置)。TASK-006 实测:派验后 53 秒 discovery 被改写,
            # 改动恰好落在 verification.suite —— DoD 自证字段,最敏感的一类。TASK-002 的守卫堵的是
            # 判定**之后**(verified/accepted 禁覆盖),这里堵的是判定**之中**(verifying 窗口)。---
            NOW_DSHA=$(discovery_sha "$TID")
            # `[ -n "$B_DSHA" ]` 是**刻意的不误伤**,与 TASK-002 的「首写放行」逐条同口径:
            # 判据是「有没有可被替换的判定依据」,不是「文件有没有变化」。承接时 discovery 尚不
            # 存在(dev 事后补记 —— 写通道明确支持,atlas 用到过;集成冒烟链也是先派验后写)时,
            # 不存在「验证者看过的那一份」,新出现的 discovery 没有替换掉任何东西,故不算漂移。
            # 反向(承接时有、现在没了)仍算漂移:判定依据凭空消失比被改写更严重。
            if [ -n "$B_DSHA" ] && [ "$NOW_DSHA" != "$B_DSHA" ]; then
                if [ -n "$ACK_DSHA" ] && [ "$ACK_DSHA" = "$NOW_DSHA" ]; then
                    echo "INFO: discovery 漂移已由 $ME 显式确认(--ack-discovery-drift $NOW_DSHA)" >&2; ACK_USED=1
                elif [ -z "$NOW_DSHA" ]; then
                    # 现已不存在:无当前 sha 可确认,故不给 ack 出路 —— 恢复文件或改走 rejected
                    DRIFT_DETAIL="${DRIFT_DETAIL}  你承接 $TID 时的 discovery sha256: $B_DSHA
  现在: $ARC/discoveries/$TID.json **已不存在** —— 判定依据凭空消失
"
                    ACK_NEEDED="$ACK_NEEDED (无法确认:请先恢复该文件,或改走 transition rejected)"
                else
                    DRIFT_DETAIL="${DRIFT_DETAIL}  你承接 $TID 时的 discovery sha256: $B_DSHA
  现在的 discovery sha256          : $NOW_DSHA
  文件: $ARC/discoveries/$TID.json(只记了摘要,无法还原承接时的内容;逐行比对: git diff -- $ARC/discoveries/$TID.json)
"
                    ACK_NEEDED="$ACK_NEEDED --ack-discovery-drift $NOW_DSHA"
                fi
            fi
        fi
        if [ -n "$ACK_NEEDED" ]; then
            printf '%s' "$DRIFT_DETAIL" >&2
            deny "验证对象漂移:$TID 在你承接之后变了(见上方差异)——你判定的对象与现在的交付物不是同一个" \
                "先核对上方差异是否影响你的判定;确认无碍后原命令追加:${ACK_NEEDED};若判定已因此失效,改走 transition rejected(rejected 不受漂移约束)"
        fi
        if [ -n "${ACK_HEAD}${ACK_DSHA}" ] && [ "$ACK_USED" = 0 ]; then
            echo "WARN: 提供了漂移确认参数但未检测到对应漂移(判定对象未变),该参数无效果" >&2
        fi
    fi

    # TASK-014:update 审计要给出「改了什么」而不只是「改过」,故须留住改动前的值。
    # 原子替换后旧内容就没了,所以在替换前拍一份快照。
    # 触发条件收在**一处**:transition 已由上面的审计块留痕(且它的行格式被 ParseTransitions
    # 与 transition-audit 按行依赖,不能多出一行);无字段的 update 没有任何变更可记。
    AUDIT_UPD=0
    [ "$MODE" = "update" ] && [ -n "$UPD_KEYS" ] && AUDIT_UPD=1
    AUDIT_OLD=""
    if [ "$AUDIT_UPD" = 1 ]; then
        AUDIT_OLD=$(mktemp "$ARC/tasks/.old.XXXXXX") && cp "$FILE" "$AUDIT_OLD" || AUDIT_OLD=""
    fi
    TMP=$(mktemp "$ARC/tasks/.tmp.XXXXXX") || fail "mktemp 失败"
    # W8:`"${ARGS[@]}"` 在 **bash < 4.4** 的 `set -u` 下,数组为空时报 `ARGS[@]: unbound variable`
    # 并当场退出——`task <ID> update` 不带任何 --field/--json-field 正是这一形态(ARGS 保持空)。
    # 退出发生在 mktemp **之后**,故还会把 .tmp.XXXXXX 留在 .arcforge/tasks/ 里。macOS 自带
    # bash 是 3.2.57,是本项目的硬下限。`${A[@]+"${A[@]}"}` 是 3.2 下唯一安全的展开写法
    # (非空时逐字等价于 "${A[@]}",空时整体消失)。上面的 UPD_KEYS 用空格分隔字符串绕开了
    # 同一个坑,这里绕不开——jq 参数含用户提供的值,必须保持逐参数不分词。
    if ! jq ${ARGS[@]+"${ARGS[@]}"} "$PROG" "$FILE" > "$TMP"; then rm -f "$TMP"; fail "jq 更新失败"; fi
    mv "$TMP" "$FILE" || { rm -f "$TMP"; fail "原子替换失败"; }
    # 进度可视化:迁移成功后追加审计行(原子性来自 O_APPEND + 单行 < PIPE_BUF)。
    # 追加失败仅 WARN 不回滚——审计是 best-effort,主事务(任务文件)已原子落盘(S2)。
    # S-1:补记落盘终值 reason_class 与 rework_count,为返工熔断留痕。
    #
    # C3(TASK-006,sprint-005 实测):此处原用 printf 拼 JSON,注释断言「字段均经上游白名单
    # 校验,不含引号/换行/反斜杠,printf 拼 JSON 安全」。**该断言为假**——逐字段核对:
    #   · task / by      = TID / $ME,确经 validate_task_id / validate_instance 整串校验 ✓
    #   · from / to      = 迁移边须在矩阵 .transitions 里查得到才放行,取值受矩阵约束 ✓
    #   · at             = date -u 生成 ✓
    #   · reason_class   = 从**任务文件**读回,不是从本次命令行读。--field 路径确有 A-2 枚举
    #                      校验,但 `task <ID> create` 的 stdin 只校验 `.status == "pending"`,
    #                      其余字段原样落盘 ⇒ 文件里的值可以是**任意字符串**。✗
    #   · rework_count   = 同上,且它填的是**不带引号**的 %s 位:一个字符串值即可闭合本对象、
    #                      换行、再开一个语法完全合法的新对象。✗
    # 实测 PoC:create 时把 rework_count 写成
    #   "0}\n{\"task\":\"TASK-904\",\"from\":\"FORGED\",\"to\":\"accepted\",\"by\":\"ghost\",
    #    \"at\":\"1970-01-01T00:00:00Z\",\"reason_class\":\"\",\"rework_count\":0"
    # 之后随便走一条合法 transition,transitions.jsonl 就凭空多出一行伪造迁移,退出码 0、
    # 无任何告警;那个 1970 的 at 让 arcforge-progress 的「已运行」变成 495920h44m。
    #
    # ⇒ 改用 jq -cn 构造(与下方 update_audit_line 同范式)。值一律由 jq 负责转义、行数恒为 1,
    #   **不依赖任何上游校验是否到位**——这才是「安全」可以被机械判定的形式。
    #   reason_class/rework_count 走 --argjson + `jq -c`(其输出恒为合法 JSON),故正常值
    #   (字符串枚举 / 数字)产出的字节与改动前逐字节相同,异常值则被降为转义后的字符串而不是
    #   拼进语法结构。⚠ 别再把「上游都校验过」当既有结论去改别处——它当时就不成立。
    if [ "$MODE" = "transition" ]; then
        RC_FINAL=$(jq -c '.reason_class // ""' "$FILE")
        RW_FINAL=$(jq -c '.rework_count // 0' "$FILE")
        # TASK-009:逃生开关的留痕**并进本行**,不另起一行 —— C3 的不变量是「一次 transition
        # 只增加一行审计」(它是防审计行伪造的核心判据),另起一行会把那条不变量撞破。
        # $sv == 0 时对象逐字节与改动前相同:`+ {}` 不产生任何键,既有断言与既有产物不受影响。
        AUDIT_LINE=$(jq -cn --arg task "$TID" --arg from "$CUR" --arg to "$NEW" \
            --arg by "$ME" --arg at "$AT" --argjson rc "$RC_FINAL" --argjson rw "$RW_FINAL" \
            --argjson sv "${SKIP_VALIDATE_USED:-0}" \
            '{task:$task,from:$from,to:$to,by:$by,at:$at,reason_class:$rc,rework_count:$rw}
             + (if $sv == 1 then {skip_validate:true} else {} end)')
        if [ -z "$AUDIT_LINE" ] || ! printf '%s\n' "$AUDIT_LINE" \
            >> "$ARC/tasks/transitions.jsonl" 2>/dev/null; then
            echo "WARN: transitions.jsonl 追加失败,审计缺行(主事务已落盘,不回滚)" >&2
        fi
    fi
    # TASK-014:update 的字段写入同样留痕。与上面 transition 审计逐条同口径 ——
    # best-effort:jq 生成失败或追加失败都只 WARN,主事务(任务文件)已原子落盘,不回滚。
    # $UPD_KEYS 刻意不加引号:它是空格分隔的 key 列表,要靠词分割拆成 update_audit_line 的多个参数。
    if [ "$AUDIT_UPD" = 1 ]; then
        UPD_LINE=$(update_audit_line "$AUDIT_OLD" "$FILE" "$TID" "$ME" "$AT" $UPD_KEYS)
        if [ -z "$UPD_LINE" ] || ! printf '%s\n' "$UPD_LINE" >> "$ARC/tasks/transitions.jsonl" 2>/dev/null; then
            echo "WARN: transitions.jsonl 追加失败,审计缺行(主事务已落盘,不回滚)" >&2
        fi
        [ -n "$AUDIT_OLD" ] && rm -f "$AUDIT_OLD"
    fi
    exit 0
}

# TASK-015(RED 实测复现,2026-07-27):`git worktree` 只共享 `.git`,而 `.arcforge/` 是
# **已跟踪目录**,会按 checkout 的 commit 在每个 worktree 里铺开一份独立副本。在 linked
# worktree 里调用写通道,jq 原子写落在那份副本上、审计追加也落在那里,而**主仓库的真相源
# 一个字节没变、退出码仍是 0**。实测:worktree 里跑 `task TASK-015 update --field
# progress_note=…` 后,progress_note 只出现在副本里,主仓库文件 sha256 不变。
# 这比它要解决的并发问题更隐蔽——「文件系统是唯一真相源」这条根基被**静默**架空。
# (副本还可能是陈旧残缺的:按更早的 commit 建的 worktree 里,后来才落盘的任务文件根本不存在。)
#
# 判别式:主工作区 `--git-dir` 与 `--git-common-dir` 相等(都是 `.git`);
# linked worktree 前者是 `<root>/.git/worktrees/<name>`、后者是 `<root>/.git`。
#
# **判别不出来时放行(fail-open)**,与上面「矩阵缺失 fail-closed」刻意相反,理由不是
# 「宽松一点」而是代价的不对称:矩阵是授权依据,缺了就无从判权,必须拒;而本判别只回答
# 「我在哪」,一次误判就阻断唯一写入通道上的**全部**写入,整个 sprint 当场停摆。
#
# ⚠ **fail-open 的射程要如实记:它确实会漏掉一部分本该拦下的场景。**
# 本注释早先断言那三种成因「都蕴含 linked worktree 不可能存在」,**那是错的**
# (TASK-015 返工,test-agent-49 造出两个反例):
#   · 非 git 仓库    —— 成立,worktree 不可能存在。
#   · git 不在 PATH  —— **不成立**:worktree 是**过去**由一个能用的 git 建的,与**此刻**
#                       git 能不能用无关;实测在已存在的 worktree 里静默写进副本且 exit 0。
#   · rev-parse 报错 —— **不成立**:孤儿 worktree(主仓库 .git 已被移走)正是这一档。
# 错因是把「**我现在探测不到**」当成了「**它不存在**」。fail-open 这个选择保留(代价不对称
# 的论证不受影响),但补一道与 git 无关的兜底探针,把后两档捞回来。
#
# 兜底探针:linked worktree 的 `.git` 是**文件**(内容 `gitdir: …`),主工作区的是**目录**。
# ⚠ **已知假阳,且探针本身分不开**:git **子模块**的 `.git` 同样是 `gitdir:` 开头的文件
# (实测 `gitdir: ../.git/modules/<name>`)。故它**只在主判别式无法决断时**跑,不无条件跑——
# 有 git 时子模块由主判别式正确放行,根本走不到这里;只有「**git 也用不了的子模块**」这一
# 罕见组合会被保守误拒。这是有意取舍:该组合下无从区分,而误拒可由人工 cd 解决,误放则静默写坏。
looks_like_linked_worktree() { [ -f .git ] && head -c 7 .git 2>/dev/null | grep -q '^gitdir:'; }
assert_main_worktree() {
    local gd gcd main
    # 两者一个可能相对、一个可能绝对(git 的输出形态随 cwd 变),且 macOS 的 /var 是符号
    # 链接,故统一成**物理绝对路径**再比,避免把主工作区误判成 worktree。
    if command -v git >/dev/null 2>&1 \
       && gd=$(git rev-parse --git-dir 2>/dev/null) \
       && gcd=$(git rev-parse --git-common-dir 2>/dev/null) \
       && [ -n "$gd" ] && [ -n "$gcd" ] \
       && gd=$(cd "$gd" 2>/dev/null && pwd -P) \
       && gcd=$(cd "$gcd" 2>/dev/null && pwd -P); then
        [ "$gd" = "$gcd" ] && return 0
        main=$(dirname "$gcd")
        deny "检测到当前在 linked worktree(--git-dir=$gd ≠ --git-common-dir=$gcd):这里的 $ARC/ 只是一份 checkout 副本,写进去不会更新真相源,而且不会报错" \
            "cd $main 回主仓库再调用写通道;worktree 只用来编辑代码 / 跑测试 / 跑变异,提交在自己 worktree(分支 task/<TASK-ID>),合并回 main 由 Leader 串行执行"
    fi
    # 主判别式无法决断(非 git 仓库 / git 不在 PATH / rev-parse 报错)——走兜底探针。
    looks_like_linked_worktree || return 0
    deny "git 判别式不可用,但当前目录的 .git 是「gitdir:」形态的文件——极可能是 linked worktree(孤儿 worktree 或 git 不可用时的 worktree),这里的 $ARC/ 只是一份 checkout 副本,写进去不会更新真相源" \
        "cd 回主仓库根(含真实 .git 目录那一层)再调用写通道;若这里其实是 git 子模块而非 worktree,说明 git 当前不可用——先修好 git,主判别式会正确放行"
}

# ---------- 主模式 ----------
assert_main_worktree   # TASK-015:先于矩阵检查——worktree 里那份矩阵副本是存在的,放后面就晚了
[ -f "$MATRIX" ] || deny "$MATRIX 缺失——无授权依据,拒绝所有写(fail-closed)" \
    "从框架模板重新落盘: cp ~/.arcforge/templates/write-matrix.json $ARC/"

# TASK-001:`--locked-task-write` 已取消。留一条**指名道姓**的 DENY 而不是让它落进下面
# 那句「缺少 --as」——照抄旧命令的人需要知道的是「这条入口没了、为什么没了」,而不是一句
# 会让人以为「补个 --as 就行」的通用报错。位置在 worktree 判别与矩阵检查**之后**:在
# worktree 里照抄旧命令时,该先告诉他人在错的目录,那才是更靠外的一层。
#
# 写成 `if … then … fi` 而不是 `[ … ] && deny …`。理由**不是**「将来补了 `set -e` 会炸」——
# 那条说法**实测为假**,别再据它改别处:`set -e` 明文豁免「`&&`/`||` 列表中除最后一个之外的
# 命令」,故条件为假时 `[ … ]` 的失败不触发退出。实测(把本文件 `set -uo pipefail` 改成
# `set -euo pipefail` 后跑正常调用)改前改后同为 `rc=0`;最小复现
# `bash -c 'set -euo pipefail; [ a = b ] && echo hit; echo 到达'` 也照常走到最后。
#
# 真正的理由只有一条,小但确实:该 AND 列表在**正常路径**(条件为假)上整句退出状态是 1,
# 即这条语句不是状态中性的。今天它后面还有别的语句,所以无害;而本文件从此只剩这一条控制流,
# 后续改动都会落在这附近 —— 一旦它变成某个函数或脚本的**最后一条**语句,那个 1 就成了返回值。
# `if` 形态没有这个尾巴,成本是三个关键字。
if [ "${1:-}" = "--locked-task-write" ]; then
    deny "--locked-task-write 已取消:它曾是一条平行写通道,早于 worktree 判别/token 校验/dev_done 门禁分发,且直接调用时任务锁根本不建立" \
        "改走主模式(锁已由主模式原地获取): $(basename "$0") --as <me> task <TASK-ID> <transition|update> …"
fi
[ "${1:-}" = "--as" ] || deny "缺少 --as <instance> 身份声明" "所有状态写入必须声明在册身份;$SUBAGENT_HINT"
ME="${2:-}"
[ -n "$ME" ] || deny "--as 身份为空" "$SUBAGENT_HINT"
shift 2

# 修订③:token 以 ARCFORGE_TOKEN 环境变量为主;--token 兜底(显式传参优先)。
# spawn 模板只教环境变量——命令行参数会进 ps/shell 历史/transcript,暴露面更大。
TOKEN="${ARCFORGE_TOKEN:-}"
if [ "${1:-}" = "--token" ]; then
    TOKEN="${2:?--token 需要一个值}"
    shift 2
fi

validate_instance "$ME" "$SUBAGENT_HINT"   # R1:整串校验(替代 echo|grep -qE,堵换行走私)

# --- 2b token 防线:主模式身份校验后、子命令分发前统一验 token,覆盖全部子命令 ---
# (否则冒名者绕开 task 仍可污染 plan/verdict。)tokens[ME] 未登记或字段缺失 → 不启用
# (旧项目/存量 sprint 平滑过渡);leader 本体暂不纳入(自举无解,如实记录的局限)。
# token 验「你是谁」,epoch 验「你还是不是 owner」,二者正交。
TOKEN_DENY_MSG="token 缺失/不符:你可能是子代理或被误路由——子代理禁写状态,把结论带回本体"
REG_HASH=$(jq -r --arg i "$ME" '.tokens[$i] // empty' "$MATRIX")
if [ -n "$REG_HASH" ]; then
    [ -n "$TOKEN" ] || deny "$TOKEN_DENY_MSG" "$SUBAGENT_HINT"
    [ "$(printf '%s' "$TOKEN" | sha256_hex)" = "$REG_HASH" ] || deny "$TOKEN_DENY_MSG" "$SUBAGENT_HINT"
fi

CMD="${1:-}"
[ -n "$CMD" ] || fail "缺少子命令"
shift

read_stdin() { CONTENT=$(cat); [ -n "$CONTENT" ] || fail "stdin 内容为空"; }

# 按 rules 表匹配 <相对 .arcforge 的路径>:命中且有权 → return 0;命中无权/未登记 → deny
check_path_writer() {
    local rel="$1" rule pat writers W
    while read -r rule; do
        pat=$(echo "$rule" | jq -r '.path')
        pat="${pat//\{me\}/$ME}"
        case "$rel" in
        $pat)
            writers=$(echo "$rule" | jq -c '.writers')
            [ "$writers" = '"self"' ] && return 0   # {me} 已绑定路径,身份即授权
            for W in $(echo "$writers" | jq -r 'if type=="array" then .[] else empty end'); do
                case "$ME" in $W) return 0 ;; esac
            done
            deny "「${ME}」无权写 $rel(合法写者: $writers)" "该文件归属其它角色;需要变更请 inbox 通知其 owner"
            ;;
        esac
    done < <(jq -c '.rules[]' "$MATRIX")
    deny "$rel 未在权限矩阵登记(deny-by-default)" "新文件类型需先在 write-matrix.json 显式登记"
}

atomic_write() { # <目标绝对/相对路径>,内容取 $CONTENT
    mkdir -p "$(dirname "$1")"
    local tmp
    tmp=$(mktemp "$(dirname "$1")/.tmp.XXXXXX") || fail "mktemp 失败"
    if ! printf '%s\n' "$CONTENT" > "$tmp"; then rm -f "$tmp"; fail "写入临时文件失败"; fi
    mv "$tmp" "$1" || { rm -f "$tmp"; fail "原子替换 $1 失败"; }
}

case "$CMD" in
  matrix)
    ACTION="${1:?用法: matrix set-token <instance>(token 明文走 stdin)}"
    [ "$ACTION" = "set-token" ] || fail "未知 matrix 动作「$ACTION」"
    INSTANCE="${2:?用法: matrix set-token <instance>}"
    shift 2
    reject_extra_args "matrix set-token $INSTANCE" "$@"
    [ "$ME" = "leader" ] || deny "仅 leader 可登记实例 token(spawn 前置步骤)" "$SUBAGENT_HINT"
    validate_instance "$INSTANCE" "先确认 spawn 实例命名再登记"   # R1:整串校验(堵换行走私)
    # R1 放大链收口:重置已登记实例的 token 须携带并校验其当前 token(ARCFORGE_TOKEN/--token),
    # 否则单一未设防的 leader 身份可覆盖任意已登记 token→伪造全树(QA 实证)。首次登记(未登记)免旧 token。
    # 残留局限(本轮不堵,如实记录):leader 本体不设防 ⇒ 冒名 leader 仍可为「未登记」实例首次登记 token
    # (占位/抢注),且改 --as leader 平移绕过主模式 token 校验;彻底封堵需 leader token 外部注入(范围外)。
    # 此环只堵已登记实例的未授权重置覆盖。
    OLD_HASH=$(jq -r --arg i "$INSTANCE" '.tokens[$i] // empty' "$MATRIX")
    if [ -n "$OLD_HASH" ]; then
        [ -n "$TOKEN" ] || deny "重置已登记实例「${INSTANCE}」的 token 须携带其当前 token(放大链收口:防冒名 leader 覆盖)" \
            "经 ARCFORGE_TOKEN 或 --token 传入该实例现 token;仅首次登记免旧 token"
        [ "$(printf '%s' "$TOKEN" | sha256_hex)" = "$OLD_HASH" ] || \
            deny "重置「${INSTANCE}」的旧 token 不符,拒绝覆盖(放大链收口)" \
            "leader 不设防,故对已登记实例的重置强制校验旧 token;传入正确的当前 token 再试"
    fi
    read_stdin   # token 明文;只存 sha256,明文不落盘
    HASH=$(printf '%s' "$CONTENT" | sha256_hex)
    TMP=$(mktemp "$ARC/.tmp.XXXXXX") || fail "mktemp 失败"
    if ! jq --arg i "$INSTANCE" --arg h "$HASH" \
        '.tokens[$i] = $h' "$MATRIX" > "$TMP"; then   # jq 自动创建缺失的 .tokens 对象
        rm -f "$TMP"; fail "jq 更新矩阵失败"
    fi
    mv "$TMP" "$MATRIX" || { rm -f "$TMP"; fail "原子替换矩阵失败"; }
    ;;
  task)
    TASK_ID="${1:?用法: task <TASK-ID> <create|transition|update> ...}"
    validate_task_id "$TASK_ID"   # C6:入口即校验格式,ACTION 解析/落盘/门禁均在其后
    ACTION="${2:?用法: task <TASK-ID> <create|transition|update> ...}"
    shift 2
    FILE="$ARC/tasks/${TASK_ID}.json"
    case "$ACTION" in
      create)
        reject_extra_args "task $TASK_ID create" "$@"
        [ "$ME" = "leader" ] || deny "仅 leader 可创建任务(任务拆分是 Leader 职责)" "$SUBAGENT_HINT"
        [ -f "$FILE" ] && deny "$FILE 已存在" "已存在任务请用 transition/update"
        read_stdin
        echo "$CONTENT" | jq -e '.status == "pending"' >/dev/null 2>&1 \
            || deny "新任务 status 必须是 pending" "初始状态由状态机规定(2.9)"
        atomic_write "$FILE"
        ;;
      transition)
        NEW="${1:?用法: task <ID> transition <new-status> ...}"
        shift
        [ -f "$FILE" ] || deny "$FILE 不存在" "先由 leader 执行 task $TASK_ID create"
        # 发现#1(试点):原生 TaskCompleted 事件在文件写通道下不触发,门禁内建到
        # dev_done 迁移前置,且在锁临界区外执行——go test 可长达 test_timeout,不占任务锁;
        # 门禁只看测试/覆盖率,与任务文件状态无 TOCTOU 耦合。
        # 缺件语义区分:门禁脚本缺失 = 封口层缺件 → WARN 放行(fail-open,与 fail-closed
        # 的矩阵缺失相反);矩阵是授权依据,缺则无从判权,门禁只是附加防线,缺不阻断主功能。
        # 限制:本仓库根无 go.mod 时门禁走 task-completed.sh 既有 skip 分支(exit 0)放行,
        # 生产级触发需运行时同步后在有 go.mod 的目标仓库发生(B8 文档如实注明)。
        #
        # C6(backlog-c #6):门禁前绑定预检——未授权实例不得消耗门禁(go test 可长达 test_timeout)。
        # 授权终判仍在锁内(locked_task_write 的角色绑定校验,一行不动),此处仅在锁外惰性
        # early-DENY 明显未授权的门禁触发;预检读 task JSON 失败即 fail-closed DENY(不把损坏文件送进门禁)。
        if [ "$NEW" = "dev_done" ] && [ "$ME" != "leader" ]; then
            BOUND=$(jq -r '.assigned_to // empty' "$FILE" 2>/dev/null) || \
                deny "预检读取 $FILE 失败(文件损坏?)" "修复任务文件后重试(fail-closed)"
            [ "$BOUND" = "$ME" ] || deny "该任务 assigned_to=「${BOUND}」,不是你($ME)——门禁未执行" \
                "只能对绑定到你实例的任务执行 dev_done"
        fi
        if [ "$NEW" = "dev_done" ]; then
            GATE="$(dirname "$SELF")/task-completed.sh"
            if [ -f "$GATE" ]; then
                if ! printf '{"task_id":"%s"}\n' "$TASK_ID" | bash "$GATE" >&2; then
                    deny "dev_done 门禁未通过(task-completed 详情见上方 stderr)" \
                        "修复声明范围内的测试/覆盖率后重试 transition dev_done"
                fi
            else
                echo "WARN: $GATE 缺失,dev_done 门禁跳过(封口层缺件 fail-open,不阻断写通道)。" >&2
            fi
        fi
        # ---------- TASK-009 刹车:leader 派发路径先跑 validator ----------
        # 挂在**派发边**上(pending/rejected/blocked_human → assigned、dev_done → verifying):
        # 那是任务图状态真正改变、下游 agent 据以开工的时刻,也是 scope 互斥冲突第一次变得
        # 可观测的时刻。挂在 dev 认领或 test 判定上没有意义 —— 那时任务图已经定了。
        #
        # ⚠ 判别**只看 validator-run.sh 的退出码,不解析 stderr 文案**。该契约由
        # project-template/scripts/validator-run.sh 的退出码表定义(表里就写着「TASK-009
        # 据此区分不适用与不可用」)。按文案判会在改一句提示时静默失效,而退出码是契约。
        #
        # ⚠ **刹车拦的是任务图问题,不是代码缺陷**。它查的是 DAG 成环 / wave 序 / scope
        # 互斥 / epoch 不变量这类**声明层**的错,对 C2、C3 那类实现层漏洞一无所知。
        # 装了刹车不等于不会再出那种洞 —— 别让下一个读者据此放松代码审查。
        if [ "$ME" = "leader" ] && { [ "$NEW" = "assigned" ] || [ "$NEW" = "verifying" ]; }; then
            if [ "${ARCFORGE_SKIP_VALIDATE:-}" = "1" ]; then
                # 人类 2026-08-03 裁决:保留本逃生开关。裁决的前提是「留痕这一半真的成立」,
                # 所以出声与落盘**两件事都要做** —— 只出声不落盘的开关就是后门:
                # stderr 会滚走,而 transitions.jsonl 是能被事后查的那一份。
                echo "WARN: ARCFORGE_SKIP_VALIDATE=1 —— 本次派发跳过 validator 任务图校验。" >&2
                echo "      这是逃生开关,不是常规路径;本次绕过将记入 transitions.jsonl 该行的 skip_validate 字段。" >&2
                # 留痕**不在这里写**:另起一行会撞破 C3 的「一次 transition 只增加一行审计」
                # 这条不变量(它是防审计行伪造的核心)。改为置标记,由 locked_task_write 构造
                # 迁移审计行时并进同一行。⇒ 留痕与「一行」两个约束同时成立,不必二选一。
                SKIP_VALIDATE_USED=1
            else
                # ARCFORGE_VALIDATOR_RUN 覆写**仅供测试注入**(与 ARCFORGE_WRITE_SCRIPT /
                # ARCFORGE_MATRIX_TPL 同范式):默认值即生产路径,CI 与生产行为不变。
                VRUN="${ARCFORGE_VALIDATOR_RUN:-$(dirname "$SELF")/../scripts/validator-run.sh}"
                if [ ! -f "$VRUN" ]; then
                    # 封口层缺件 fail-open + 出声,与 dev_done 门禁缺失同档。
                    echo "WARN: $VRUN 缺失,派发跳过任务图校验(封口层缺件 fail-open)。" >&2
                else
                    # bash 3.2 没有 timeout(1),自己轮询。粒度 1 秒足够:阈值是 30 秒量级。
                    VTO="${ARCFORGE_VALIDATE_TIMEOUT:-30}"
                    VOUT_F=$(mktemp) || deny "mktemp 失败" "检查临时目录可写后重试"
                    bash "$VRUN" validate .arcforge/tasks > "$VOUT_F" 2>&1 &
                    # 「本次是否超时」用独立标志记,不从 (VRC==124 ∧ VWAIT≥VTO) 反推:124 是
                    # 退出码表里 validator **自己也会发**的码,两者处置同档(都判不可用)但成因
                    # 不同,而只有成因决定下面那行 ERROR 该不该打。反推还会让 VWAIT 泄漏出循环
                    # 当第二判据 —— 桩恰好在第 VTO 秒自发 exit 124 时它就误报「执行超时」。
                    VPID=$!; VWAIT=0; VTIMEDOUT=0
                    while kill -0 "$VPID" 2>/dev/null; do
                        if [ "$VWAIT" -ge "$VTO" ]; then
                            kill -TERM "$VPID" 2>/dev/null
                            sleep 1; kill -KILL "$VPID" 2>/dev/null
                            VTIMEDOUT=1; break
                        fi
                        sleep 1; VWAIT=$((VWAIT + 1))
                    done
                    wait "$VPID" 2>/dev/null; VRC=$?
                    [ "$VTIMEDOUT" -eq 1 ] && VRC=124   # 超时并入「不可用」档,与退出码表一致
                    VOUT=$(cat "$VOUT_F"); rm -f "$VOUT_F"
                    case "$VRC" in
                        0) : ;;   # 任务图通过,放行
                        127)      # **不适用**:压根没装 —— 消费项目的正常形态 ⇒ 出声 + 放行
                            echo "INFO: validator 未安装(消费项目的正常形态),派发跳过任务图校验。" >&2
                            ;;
                        124|125)  # **不可用**:本该能跑却跑不了 / 版本陈旧无源码可降级 ⇒ 阻断
                            [ "$VTIMEDOUT" -eq 1 ] && \
                                echo "ERROR: validator 执行超过 ${VTO}s,视为不可用。" >&2
                            printf '%s\n' "$VOUT" | sed 's/^/    /' >&2
                            deny "validator **不可用**(exit=$VRC:安装损坏、版本陈旧或执行超时),派发已阻断" \
                                 "这与「未安装」是两回事(未安装会放行);请重装或修复 validator 后重试,确需绕过用 ARCFORGE_SKIP_VALIDATE=1(会留痕)"
                            ;;
                        *)        # 业务码:1 = 有阻断级问题,2 = 目录不可读,64 = 用法错误
                            printf '%s\n' "$VOUT" | sed 's/^/    /' >&2
                            deny "validator 判定任务图存在阻断级问题(exit=$VRC),本次派发已阻断" \
                                 "按上方输出修正 .arcforge/tasks 后重试;确需绕过用 ARCFORGE_SKIP_VALIDATE=1(会留痕)"
                            ;;
                    esac
                fi
            fi
        fi
        # TASK-001:原地取锁 + 原地调用。锁在门禁**之后**才取(门禁的 go test 可长达
        # test_timeout,不该占着任务锁),这一先后关系与改动前 exec 的位置逐字一致。
        acquire_task_lock "$TASK_ID"
        locked_task_write "$ME" "$FILE" transition "$NEW" "$@"
        ;;
      update)
        [ -f "$FILE" ] || deny "$FILE 不存在" "先由 leader 执行 task $TASK_ID create"
        acquire_task_lock "$TASK_ID"
        locked_task_write "$ME" "$FILE" update - "$@"
        ;;
      *) fail "未知 task 动作「$ACTION」" ;;
    esac
    ;;
  plan)
    reject_extra_args plan "$@"
    read_stdin
    check_path_writer "docs/03-progress/plan.md"
    atomic_write "$ARC/docs/03-progress/plan.md"
    ;;
  verdict)
    F="${1:?用法: verdict <filename.md>}"
    shift
    reject_extra_args "verdict $F" "$@"
    reject_unsafe_path "$F"
    read_stdin
    check_path_writer "docs/05-review/$F"
    atomic_write "$ARC/docs/05-review/$F"
    ;;
  discovery)
    T="${1:?用法: discovery <TASK-ID>}"
    validate_task_id "$T"   # C6:discovery 分支复用同一 TASK_ID 格式校验
    shift
    reject_extra_args "discovery $T" "$@"   # 早于 read_stdin:`discovery <id> --file x` 曾静默卡到超时
    TF="$ARC/tasks/${T}.json"
    [ -f "$TF" ] || deny "$TF 不存在" "discovery 必须对应已存在任务"
    OK=0
    [ "$ME" = "leader" ] && OK=1
    [ "$(jq -r '.assigned_to // empty' "$TF")" = "$ME" ] && OK=1
    [ "$(jq -r '.verifier // empty' "$TF")" = "$ME" ] && OK=1
    [ "$OK" = 1 ] || deny "「${ME}」不是 $T 的 owner(assigned_to/verifier)" "discovery 由任务 owner 完成时写入"
    # 第 10 项:discovery 原本与 status 无关,verified/accepted 之后仍可全量覆盖 ⇒ 验证者读到的
    # 判定依据可能与它验证时看到的不是同一份。堵**时机**而非堵人:判定作出后任何角色(含 leader
    # 与 owner)都不得覆盖既有 discovery;文件尚不存在时的首写仍放行——没有可被替换的判定依据,
    # 事后补记不被误伤。discovery 是单个 JSON 对象,「只追加」不成立,故取拒绝而非追加。
    case "$(jq -r '.status // ""' "$TF")" in
      verified|accepted)
        [ -f "$ARC/discoveries/${T}.json" ] && deny \
            "$T 已 verified/accepted,discovery 是该判定的依据,禁止覆盖写(防判定依据被静默替换)" \
            "确需修订:请 leader 先把任务退回(verified->review_fix)再由 owner 重写;仅作补充说明请写 wisdom/learnings-${ME}.md 或发 inbox" ;;
    esac
    read_stdin
    atomic_write "$ARC/discoveries/${T}.json"
    ;;
  wisdom)
    TYPE="${1:?用法: wisdom <learnings|decisions|digest>}"
    shift
    reject_extra_args "wisdom $TYPE" "$@"
    read_stdin
    case "$TYPE" in
      digest)
        check_path_writer "wisdom/_digest.md"
        atomic_write "$ARC/wisdom/_digest.md"
        ;;
      learnings|decisions)
        mkdir -p "$ARC/wisdom"
        printf '%s\n' "$CONTENT" >> "$ARC/wisdom/${TYPE}-${ME}.md"   # 实例自有文件,单写者,只追加
        ;;
      *) fail "未知 wisdom 类型「$TYPE」" ;;
    esac
    ;;
  checkpoint)
    reject_extra_args checkpoint "$@"
    read_stdin
    atomic_write "$ARC/checkpoints/${ME}-checkpoint.md"   # 命名对齐 agent 定义中的 {instance}-checkpoint.md
    ;;
  doc)
    REL="${1:?用法: doc <docs/ 下相对路径>}"
    shift
    reject_extra_args "doc $REL" "$@"   # `doc <path> --append` 曾覆盖整份 ADR:doc 是全量覆盖写
    reject_unsafe_path "$REL"
    case "$REL" in docs/*) : ;; *) deny "doc 只接受 docs/ 下相对路径" "如 docs/01-design/design-spec.md" ;; esac
    read_stdin
    check_path_writer "$REL"
    atomic_write "$ARC/$REL"
    ;;
  *) fail "未知子命令「$CMD」" ;;
esac
exit 0
