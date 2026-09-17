#!/usr/bin/env bash
# 变异 harness —— dev-m2b-c
#
# 🔴 存在的理由：我在 TASK-006 的第 1 轮和第 3 轮**两次**用 `git checkout -- .` 做还原，
# 而那两次工作树里都有未提交的实现，于是第一个变异跑完就把自己的活冲掉了。
# 第 1 轮之后我把「变异前必须先提交」写进了 checkpoint，**然后第 3 轮又犯了一次**。
# ⇒ 记住规则不产生遵守规则的能力。所以把前置条件写成**会拒绝执行的守卫**，不是写成提醒。
set -uo pipefail

# ⚠️ 在**函数内**读 MUT_FILES，不要在 source 时固化成变量。
# 实撞（写完这个脚本当天）：我 source 之后才 `MUT_FILES=CLAUDE.md`，而 FILES 早就定成了
# 默认的 "."，于是守卫拿整树判脏、把一次**干净树**测试报成「被误拒」。
# 那一刻它看起来像守卫坏了，其实是我用错了——**而下一个人多半会得出同样的错误结论然后删掉它**。
files() { echo "${MUT_FILES:-.}"; }

# 🔴 **它防的是「工作树非干净」，不是「我忘了提交」——这两件事只是在立项那次恰好重合。**
#
# 下次可能不重合：比如你故意留一处未提交的探针去跑变异（我在 TASK-010 就那么干过一次，
# 用临时 test 文件探真实配置的装载行为）。那种情况下「自动提交」会把探针一起提交进去，
# 比冲掉工作更坏——它是**静默地把不该进仓库的东西放进去**。
#
# ⇒ 所以本守卫的行为必须是**拒绝跑**，把决定权交回人手里。
#   **任何把它「优化」成自动 commit / 自动 stash 的改动都是错的**，理由就是上面这段。
#   （Leader 2026-09-17 明确要求把这条理由写进来，免得后人好心办坏事。）
require_clean_tree() {
  local dirty
  dirty=$(git status --porcelain -- $(files) | wc -l | tr -d ' ')
  if [ "${dirty}" -ne 0 ]; then
    echo "REFUSE: 工作树有 ${dirty} 处未提交改动，而本脚本用 'git checkout --' 还原 ⇒ 会把它们冲掉。" >&2
    echo "        先 git commit，再跑变异。（这条守卫的立项依据见文件头注释）" >&2
    git status --porcelain -- $(files) >&2
    return 1
  fi
}

# run_mut <描述> <python 补丁> [测试包...]
run_mut() {
  local desc=$1 patch=$2; shift 2
  local pkgs=${*:-./...}
  python3 -c "${patch}" || { echo "${desc} ⇒ 补丁未命中（变异无效，不计入结果）"; return 1; }
  if ! go build ./... 2>/dev/null; then
    echo "${desc} ⇒ ⚠ 编译闸命中，变异无效（**不是 KILLED**）"
    git checkout -q -- $(files); return 1
  fi
  local out names
  out=$(go test ${pkgs} -count=1 2>&1)
  names=$(echo "${out}" | grep -E '^ *--- FAIL' | sed 's/.*FAIL: //;s/ .*//' | sort -u | tr '\n' ' ')
  printf '%s ⇒ %s\n' "${desc}" "${names:-（无测试变红 —— SURVIVED，不是 KILLED）}"
  git checkout -q -- $(files)
}

verify_restored() {
  local n; n=$(git status --porcelain -- $(files) | wc -l | tr -d ' ')
  printf '还原自证：工作树改动=%s  HEAD=%s\n' "${n}" "$(git rev-parse --short HEAD)"
  [ "${n}" -eq 0 ]
}
