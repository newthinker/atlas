#!/bin/bash
# arcforge-write.sh — .arcforge/ 状态文件唯一写入通道(防线 2,ISSUE-4)
# 校验「声明身份 × 权限矩阵 × 状态迁移」后原子写,并维护 last_transition 审计字段。
#
# 用法(内容一律走 stdin):
#   arcforge-write.sh --as <me> task <TASK-ID> create                 # 新任务 JSON(status 必须 pending)
#   arcforge-write.sh --as <me> task <TASK-ID> transition <new-status> [--expect-epoch N] [--field k=v ...] [--json-field k=json ...]
#   arcforge-write.sh --as <me> task <TASK-ID> update [--field k=v ...] [--json-field k=json ...]
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
# dev_done 门禁(发现#1):transition dev_done 在锁外前置执行同目录 task-completed.sh,
# exit 非 0 → 拒绝整个迁移;门禁脚本缺失 → WARN 放行(fail-open)。限制:仓库根无
# go.mod 时门禁走 task-completed.sh 的 skip 分支(exit 0)放行——生产级 Go 门禁仅在
# 有 go.mod 的目标仓库触发(框架自身仓库根无 go.mod,validator 在子目录)。
set -uo pipefail

ARC=".arcforge"
MATRIX="$ARC/write-matrix.json"
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
LOCK="$(dirname "$SELF")/with-task-lock.sh"

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

# C1/C2:状态机受控字段不得经 --field/--json-field 旁路写入(否则 update/transition 可绕过迁移授权)。
# status/last_transition/assignment_epoch 一律禁写;assigned_to/verifier 仅 leader 在 transition(派发/指派)时可写。
# 普通业务字段(progress_note/questions/discovery/reject_reason/fix_items 等)放行。
# 依赖调用处 $ME/$MODE 在作用域内(--locked-task-write 内部模式)。
guard_field_key() { # <key>
    case "$1" in
        status|last_transition|assignment_epoch)
            deny "字段「$1」由状态机/脚本管理,禁止经 --field/--json-field 写" \
                "状态变更只能走 task transition;审计字段与 epoch 由脚本自动维护" ;;
        assigned_to|verifier)
            { [ "$ME" = "leader" ] && [ "$MODE" = "transition" ]; } || \
                deny "角色绑定字段「$1」仅允许 leader 在 transition(派发/指派)时写" \
                    "其它情况禁止改绑定;改派由 leader 经 transition 操作" ;;
    esac
}

# wisdom #6:--field 一律落字符串,数值语义字段误用会产生 "1" 这类字符串值,
# 破坏 validator 数值断言与 epoch/rework 运算。数值字段必须走 --json-field。
guard_numeric_field() { # <key>
    case "$1" in
        rework_count|wave)
            deny "字段「$1」是数值语义,--field 会写成字符串" \
                "改用 --json-field $1=<数字>" ;;
    esac
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

SUBAGENT_HINT="若你是子代理:禁止写状态文件,把结论写进最终回复带回父 agent,由其落盘"

# ---------- 内部模式:任务锁临界区内的读-校验-写(经 with-task-lock 自调用,勿直接使用) ----------
if [ "${1:-}" = "--locked-task-write" ]; then
    shift
    ME="$1"; FILE="$2"; MODE="$3"; NEW="$4"; shift 4   # MODE=transition|update;update 时 NEW="-"
    CUR=$(jq -r '.status' "$FILE") || fail "读取 $FILE 失败"

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
    if [ "$MODE" = "transition" ]; then
        AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        PROG="$PROG | .status=\$new | .last_transition={from:\$cur,to:\$new,by:\$me,at:\$at}"
        ARGS+=(--arg new "$NEW" --arg cur "$CUR" --arg me "$ME" --arg at "$AT")
        # F5:每次(重)派 epoch +1
        [ "$NEW" = "assigned" ] && PROG="$PROG | .assignment_epoch=((.assignment_epoch // 0)+1)"
    fi
    EXPECT_EPOCH=""
    N=0
    while [ $# -gt 0 ]; do
        case "$1" in
            --expect-epoch)
                EXPECT_EPOCH="${2:?--expect-epoch 需要一个数字}"
                echo "$EXPECT_EPOCH" | grep -qE '^[0-9]+$' || fail "--expect-epoch 必须是非负整数" ;;
            --field)
                K="${2%%=*}"; V="${2#*=}"; guard_field_key "$K"; guard_numeric_field "$K"
                PROG="$PROG | .[\$k$N]=\$v$N"
                ARGS+=(--arg "k$N" "$K" --arg "v$N" "$V") ;;
            --json-field)
                K="${2%%=*}"; V="${2#*=}"; guard_field_key "$K"
                PROG="$PROG | .[\$k$N]=\$jv$N"
                ARGS+=(--arg "k$N" "$K" --argjson "jv$N" "$V") ;;
            *) fail "未知参数 $1" ;;
        esac
        N=$((N+1)); shift 2
    done

    # F5:epoch 断言在锁临界区内、重读 assignment_epoch 之后、jq 落盘之前——
    # 重读-校验-写的原子性由 with-task-lock 保证,竞态窗口为零。持有的 epoch 与
    # 文件当前 epoch 不符 = 任务已被重派,放弃本次写入(把「Dev 持 epoch 写回」从自觉变机制)。
    if [ -n "$EXPECT_EPOCH" ]; then
        CUR_EPOCH=$(jq -r '.assignment_epoch // 0' "$FILE")
        [ "$CUR_EPOCH" = "$EXPECT_EPOCH" ] || deny \
            "epoch 断言失败:文件 assignment_epoch=$CUR_EPOCH ≠ 你持有的 $EXPECT_EPOCH(任务已被重派)" \
            "放弃本次写入,回到任务扫描循环重新认领(F5 认领协议)"
    fi

    TMP=$(mktemp "$ARC/tasks/.tmp.XXXXXX") || fail "mktemp 失败"
    if ! jq "${ARGS[@]}" "$PROG" "$FILE" > "$TMP"; then rm -f "$TMP"; fail "jq 更新失败"; fi
    mv "$TMP" "$FILE" || { rm -f "$TMP"; fail "原子替换失败"; }
    # 进度可视化:迁移成功后追加审计行(原子性来自 O_APPEND + 单行 < PIPE_BUF;五字段均经
    # 上游白名单校验,不含引号/换行/反斜杠,printf 拼 JSON 安全)。追加失败仅 WARN 不回滚——
    # 审计是 best-effort,主事务(任务文件)已原子落盘(S2)。
    if [ "$MODE" = "transition" ]; then
        if ! printf '{"task":"%s","from":"%s","to":"%s","by":"%s","at":"%s"}\n' \
            "$(basename "$FILE" .json)" "$CUR" "$NEW" "$ME" "$AT" \
            >> "$ARC/tasks/transitions.jsonl" 2>/dev/null; then
            echo "WARN: transitions.jsonl 追加失败,审计缺行(主事务已落盘,不回滚)" >&2
        fi
    fi
    exit 0
fi

# ---------- 主模式 ----------
[ -f "$MATRIX" ] || deny "$MATRIX 缺失——无授权依据,拒绝所有写(fail-closed)" \
    "从框架模板重新落盘: cp ~/.arcforge/templates/write-matrix.json $ARC/"

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
        # 授权终判仍在锁内(--locked-task-write 的角色绑定校验,一行不动),此处仅在锁外惰性
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
        exec bash "$LOCK" "$TASK_ID" "$SELF" --locked-task-write "$ME" "$FILE" transition "$NEW" "$@"
        ;;
      update)
        [ -f "$FILE" ] || deny "$FILE 不存在" "先由 leader 执行 task $TASK_ID create"
        exec bash "$LOCK" "$TASK_ID" "$SELF" --locked-task-write "$ME" "$FILE" update - "$@"
        ;;
      *) fail "未知 task 动作「$ACTION」" ;;
    esac
    ;;
  plan)
    read_stdin
    check_path_writer "docs/03-progress/plan.md"
    atomic_write "$ARC/docs/03-progress/plan.md"
    ;;
  verdict)
    F="${1:?用法: verdict <filename.md>}"
    shift
    reject_unsafe_path "$F"
    read_stdin
    check_path_writer "docs/05-review/$F"
    atomic_write "$ARC/docs/05-review/$F"
    ;;
  discovery)
    T="${1:?用法: discovery <TASK-ID>}"
    validate_task_id "$T"   # C6:discovery 分支复用同一 TASK_ID 格式校验
    shift
    TF="$ARC/tasks/${T}.json"
    [ -f "$TF" ] || deny "$TF 不存在" "discovery 必须对应已存在任务"
    OK=0
    [ "$ME" = "leader" ] && OK=1
    [ "$(jq -r '.assigned_to // empty' "$TF")" = "$ME" ] && OK=1
    [ "$(jq -r '.verifier // empty' "$TF")" = "$ME" ] && OK=1
    [ "$OK" = 1 ] || deny "「${ME}」不是 $T 的 owner(assigned_to/verifier)" "discovery 由任务 owner 完成时写入"
    read_stdin
    atomic_write "$ARC/discoveries/${T}.json"
    ;;
  wisdom)
    TYPE="${1:?用法: wisdom <learnings|decisions|digest>}"
    shift
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
    read_stdin
    atomic_write "$ARC/checkpoints/${ME}-checkpoint.md"   # 命名对齐 agent 定义中的 {instance}-checkpoint.md
    ;;
  doc)
    REL="${1:?用法: doc <docs/ 下相对路径>}"
    shift
    reject_unsafe_path "$REL"
    case "$REL" in docs/*) : ;; *) deny "doc 只接受 docs/ 下相对路径" "如 docs/01-design/design-spec.md" ;; esac
    read_stdin
    check_path_writer "$REL"
    atomic_write "$ARC/$REL"
    ;;
  *) fail "未知子命令「$CMD」" ;;
esac
exit 0
