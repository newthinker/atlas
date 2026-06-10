#!/bin/bash
# TaskCompleted hook — 任务声明范围驱动的硬门禁
# exit 0 = 允许完成, exit 2 = 阻止完成并反馈
#
# 门禁范围 = task JSON 的 packages 字段(声明范围)。绝不使用全局 git diff
# 推断——共享工作区下其他 Agent 的在途改动(尤其 RED 阶段的预期失败测试)
# 会污染判定(F1)。实际改动超出声明范围按 scope 漂移阻断(评审 R3)。
# 阻断/告警类反馈写 stderr——exit 2 的官方反馈通道是 stderr。
set -uo pipefail

CONFIG_FILE="arcforge.config.json"
TASK_DIR=".arcforge/tasks"
DEV_MIN=$(jq -r '.coverage.dev_minimum // 80' "$CONFIG_FILE" 2>/dev/null || echo 80)
COV_DIR=$(jq -r '.coverage.report_dir // ".arcforge/coverage"' "$CONFIG_FILE" 2>/dev/null || echo ".arcforge/coverage")
TEST_TIMEOUT=$(jq -r '.coverage.test_timeout // "120s"' "$CONFIG_FILE" 2>/dev/null || echo "120s")

# 非 Go 项目（无 go.mod）跳过本 hook 的 Go 专用门禁
if [ ! -f go.mod ]; then
    echo "No go.mod found; skipping Go coverage gate."
    exit 0
fi
mkdir -p "$COV_DIR"

# ---- 1. 从 stdin 解析任务上下文(官方字段 task_id;兼容链兜底) ----
HOOK_INPUT=$(cat)
TASK_ID=$(echo "$HOOK_INPUT" | jq -r '.task_id // .task.id // empty' 2>/dev/null)
if [ -z "$TASK_ID" ]; then
    TASK_ID=$(echo "$HOOK_INPUT" | { grep -oE 'TASK-[0-9]+' || true; } | head -1)
fi

# ---- 2a. 主路径:任务声明的 packages ----
PKGS=""
if [ -n "$TASK_ID" ] && [ -f "$TASK_DIR/$TASK_ID.json" ]; then
    PKGS=$(jq -r '.packages[]?' "$TASK_DIR/$TASK_ID.json" 2>/dev/null | sort -u)
    # 任务级覆盖基线覆盖全局 dev_minimum(Leader 裁决写入;
    # 场景:package main 等含大量任务范围外存量样板的包,整包 80% 不可行)
    TASK_MIN=$(jq -r '.coverage_minimum // empty' "$TASK_DIR/$TASK_ID.json" 2>/dev/null)
    if [ -n "$TASK_MIN" ]; then
        echo "NOTE: $TASK_ID 使用任务级覆盖基线 ${TASK_MIN}%（Leader 裁决，全局默认 ${DEV_MIN}%）"
        DEV_MIN="$TASK_MIN"
    fi
fi

# ---- 2b. git 推断的「实际触碰范围」(含 untracked,修 F4);
#          交叉校验与 fallback 共用 ----
CHANGED_FILES=$( { git diff --name-only HEAD; \
                   git diff --name-only --staged; \
                   git ls-files --others --exclude-standard; } 2>/dev/null )
ACTUAL=$(echo "$CHANGED_FILES" | { grep '\.go$' || true; } \
         | xargs -n1 dirname 2>/dev/null | sort -u | sed 's#^#./#')
# 其他在途任务声明的 packages(他人的在途改动不算到本任务头上,修 F1 污染)
OTHERS=$(find "$TASK_DIR" -name '*.json' ! -name "${TASK_ID:-__none__}.json" -exec \
    jq -r 'select(.status | IN("in_progress","dev_done","verifying","verified","accepted")) | .packages[]?' {} \; \
    2>/dev/null | sort -u)
MINE_ACTUAL=$(comm -23 <(echo "$ACTUAL") <(echo "$OTHERS"))

if [ -n "$PKGS" ]; then
    # ---- 2c. 交叉校验:实际改动 ⊆ 声明范围(评审 R3,防 scope 漂移逃逸) ----
    DRIFT=$(comm -23 <(echo "$MINE_ACTUAL") <(echo "$PKGS"))
    if [ -n "$(echo "$DRIFT" | tr -d '[:space:]')" ]; then
        echo "BLOCKED: 检测到任务声明范围之外的改动(scope 漂移):" >&2
        echo "$DRIFT" >&2
        echo "请先更新 $TASK_DIR/$TASK_ID.json 的 packages 字段(防护性写入),再标记完成。" >&2
        exit 2
    fi
else
    # ---- 2d. fallback:仅限 task JSON 缺失/未声明的异常场景。
    #          validator 的 scope-empty 规则保证常态下在途任务必有声明(评审 R4)。----
    echo "WARN: ${TASK_ID:-unknown} 无声明 packages,退化为 git 推断门禁。" >&2
    PKGS="$MINE_ACTUAL"
fi

# 过滤不存在的目录(被删除文件的 dirname 可能已不存在)
PKGS=$(echo "$PKGS" | while read -r p; do [ -n "$p" ] && [ -d "$p" ] && echo "$p"; done)

if [ -z "$PKGS" ]; then
    echo "No packages in scope for ${TASK_ID:-unknown task}. Proceeding."
    exit 0
fi

# ---- 3. 仅对声明范围跑测试 + 覆盖率,产物按任务隔离(F2) ----
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
if [ "${TOTAL%.*}" -lt "$DEV_MIN" ]; then
    echo "BLOCKED: Task-scope coverage ${TOTAL}% < dev_minimum ${DEV_MIN}%." >&2
    go tool cover -func="$COVERPROFILE" | grep -v "100.0%" >&2
    exit 2
fi

echo "Task scope passes. Coverage: ${TOTAL}% (dev_minimum: ${DEV_MIN}%)"
exit 0
