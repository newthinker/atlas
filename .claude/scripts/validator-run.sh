#!/bin/bash
# validator-run.sh — validator CLI 统一解析链(单一真相源,Sprint E 修订①)。
# 用法: validator-run.sh <validate|progress> [args...]
# 解析顺序(首个可用者胜出):
#   1. $HOME/.arcforge/bin/arcforge-<tool> 存在且可执行 → **先比对版本**,一致才执行;
#      业务退出码(0..125)原样透传;执行层失败(rc ≥ 126,如架构不符)降级下一级。
#   2. 仓库内存在 ./validator/ 且有 Go → go build 到临时文件再执行(dogfooding,依赖根 go.work)。
#      不用 go run:go run 会把子程序的非零退出码一律塌缩为 1,破坏 0/1/2 透传契约;
#      先 build 再直接执行则退出码原样透传,与第 1 级二进制行为一致。
#   3. 都不成 → 按「压根没装」还是「装了但坏了/陈旧」给**不同**的退出码,见下表。
# CWD 恒为调用时目录(须从仓库根调用,discovery 相对路径语义天然正确)。
# ⚠️ 解析逻辑只此一处;各命令文档仅一行引用,禁止复述(防三处漂移,Sprint C 教训)。
#
# ============================ 退出码表 ============================
# TASK-009 据此区分「不适用」与「不可用」。**判别只看退出码,不解析 stderr 文案**
# ——文案会漂,退出码是契约。改动本表必须同步 tests/scripts/test-validator-run.sh。
#
#   码    | 含义                                             | T9 语义
#   ------|--------------------------------------------------|------------------
#   0..125| 被调工具的业务码,**原样透传**                    | 工具**可用**,这是它的判定结论
#         |   validate: 0 通过 / 1 有阻断级问题 / 2 目录不可读|
#         |   (今天两个工具只发 0/1/2,但透传面是 0..125)   |
#   64    | 用法错误:未知工具名,或未给工具名                 | 调用方的错,不是环境问题
#   124   | **不可用**:二进制或源码存在,但执行/构建/临时文件 | 出声 + **阻断**
#         | 失败 —— 本该能跑却跑不了                          |
#   125   | **不可用**:二进制版本陈旧且无源码可降级          | 出声 + **阻断**,处置=重装
#   127   | **不适用**:压根没装(无二进制,且无 ./validator   | 出声 + **放行**
#         | 或无 Go 工具链)                                  |
#
# ⚠ 三个自产码 124/125/127 与业务码 0..125 在**理论上**可能撞码(若将来某个工具
#   真的 exit 124/125)。今天 arcforge-validate 只发 0/1/2、arcforge-progress 只发 0/2,
#   故不撞;若新增业务码,必须绕开 64/124/125/127 或改本表。
# ⚠ 126 与 128+N **不会**从本脚本泄漏出去:两级的 rc ≥ 126 都被判为「执行层失败」,
#   level-1 降级、level-2 转 124。理由见 level-2 那段注释。
# ==================================================================
set -uo pipefail

# 未给工具名同样是**用法错误**,与「未知工具名」同类走 64。
# 不能沿用 `${1:?...}` 的 exit 1:那会让 1 既是业务码又是用法错误,下游无从分辨
# (审查实测:裸调用本脚本得到 rc=1)。退出码是契约,不能有二义。
if [ $# -eq 0 ]; then
  echo "错误: 未给工具名。用法: validator-run.sh <validate|progress> [args...]" >&2
  exit 64
fi
TOOL="$1"
shift
case "$TOOL" in
  validate|progress) : ;;
  *) echo "错误: 未知工具「${TOOL}」(可选 validate|progress)" >&2; exit 64 ;;
esac

BIN="$HOME/.arcforge/bin/arcforge-$TOOL"
STALE=0        # 1 = 二进制陈旧,不使用它
BROKEN=0       # 1 = 「本该能跑却跑不了」(执行层失败/构建失败),即**不可用**而非未安装
# 二进制**在那儿但不可执行**(权限位丢了)与「压根没装」是两件事,而 `[ -x ]` 把它们抹平。
# 不置位 BROKEN 的话,它会一路走到最后落 127「不适用」→ T9 放行,而实际是**装了但坏了**。
# 这是 MAJOR-1 那个语义错误的另一个形态(re-review 补的):同一个洞、不同的入口。
# 触发面不罕见:`git checkout` 丢可执行位、`cp` 没带 `-p`、解压/rsync 丢权限都会造出它。
# `-L` 那半边捎带盖住**悬空符号链接**——`-e` 跟随链接,断链时为假,而断链同样是「装了但坏了」。
if { [ -e "$BIN" ] || [ -L "$BIN" ]; } && [ ! -x "$BIN" ]; then
  BROKEN=1
  echo "WARN: ${BIN} 存在但不可执行(权限位丢失或符号链接断裂),这不是「未安装」。" >&2
  echo "      处置: chmod +x «该文件»,或重跑 installer/install.sh。" >&2
fi

if [ -x "$BIN" ]; then
  # W2:二进制无版本比对会让「✓ 校验通过」成为**空真通过**(sprint-004 全程如此:安装版落后
  # HEAD 39 个提交,同语料少报 1 条,而 rc 恒 0、无人比对)。
  #
  # 对照物必须是「这个二进制由哪份源码编译而来」,分两种形态:
  #   - dogfooding 仓库(CWD 内有 ./validator)→ 对照 CWD 的 describe。sprint-004 的事故正是
  #     二进制落后**工作仓库**,对照 $HOME/.arcforge 检不出来(install.sh 给两者同赋同值)。
  #   - 消费项目(CWD 无 ./validator)→ 对照 $HOME/.arcforge,那才是 install.sh 编译二进制的源。
  #     若在消费项目里对照 CWD,拿到的是该项目自己的版本号,**恒不相等** → 每次运行都告警,
  #     真信号会被噪声淹没(CLAUDE.md:全量告警会让确认退化成条件反射)。
  #
  # `--dirty` 是**非对称**的,两边各有各的理由,别统一:
  #   - dogfooding 侧带 --dirty:sprint 进行中工作树恒是脏的,而 describe 只看 commit
  #     ——「validator 源码刚被改过、二进制还是改之前那版」正是 sprint-005 各 agent 的日常
  #     形态,不带 --dirty 就检不出来(实景冒烟实测:改完 main.go 仍走了旧二进制)。
  #     这一侧误判的代价可控:`./validator` 按定义就在手边,降级源码永远走得通。
  #   - 消费项目侧**不带** --dirty,与 installer/install.sh 的 ARC_VERSION 逐字相同
  #     (git describe --tags --always)。install.sh 烧进二进制的就是这个表达式的值;
  #     若这边多带 --dirty,~/.arcforge 一旦有任何本地改动就恒不相等,而那一侧**没有**源码
  #     可降级 → 直接 exit 125,框架整个变成不可用。两个口径不同的值比出来的不是版本。
  if [ -d "./validator" ]; then
    SRC_DIR="."; DESCRIBE_ARGS=(--tags --always --dirty)
  else
    SRC_DIR="$HOME/.arcforge"; DESCRIBE_ARGS=(--tags --always)
  fi
  BIN_VER=$("$BIN" --version 2>/dev/null | head -1)
  SRC_VER=""
  if command -v git >/dev/null 2>&1 && [ -d "$SRC_DIR" ]; then
    SRC_VER=$(git -C "$SRC_DIR" describe "${DESCRIBE_ARGS[@]}" 2>/dev/null)
  fi
  if [ -z "$BIN_VER" ] || [ -z "$SRC_VER" ]; then
    # 比对不可行(无 git / 非 git 仓库 / 二进制不支持 --version)——**放行可以,静默不行**。
    # 消费项目里没有 .git 是正常形态,阻断会把框架变成不可用。
    echo "WARN: 无法比对 ${BIN} 的版本(二进制「${BIN_VER:-空}」/ 源码 ${SRC_DIR} 「${SRC_VER:-空}」);" >&2
    echo "      放行,但本次校验结论**未经版本校验**。" >&2
    # 已知盲区(审查 MINOR-4):**老到不支持 `--version`** 的二进制会把 `--version` 当成
    # tasks-dir,报错进 stderr 被吞、stdout 为空 ⇒ 落进本分支被放行 —— 而它恰恰是最陈旧、
    # 最该拦的那一种。这里做不到自动分辨(空输出既可能是老二进制、也可能是权限问题),
    # 故只把这句话摆到用户眼前,让人能自己判。
    [ -n "$BIN_VER" ] || echo "      注意: 若该二进制不支持 --version,说明它早于版本注入功能,请直接重装。" >&2
  elif [ "$BIN_VER" != "$SRC_VER" ]; then
    STALE=1
    if [ "$BIN_VER" = "${SRC_VER%-dirty}" ]; then
      # 差异**纯由未提交改动造成**(commit 相同,只多一个 -dirty 后缀)。
      # 这在 sprint 进行中是常态,而「重装」在这种情况下**照做也不会好**——重装后二进制
      # 仍不带 -dirty,工作树只要还脏就依然不相等。给一条恒真且无效的建议,会让真正需要
      # 反应的信号(commit 真的落后)被淹没,正是用例 13 自己立的那条原则要防的。
      # 故这里降为一行 INFO 且**不给重装建议**;仍然不使用二进制——那正是 --dirty 的目的。
      echo "INFO: 工作树有未提交改动(${SRC_VER}),二进制反映的是提交态 ${BIN_VER};本次改用源码构建。" >&2
    else
      echo "WARN: ${BIN} 的版本 ${BIN_VER} 与源码 ${SRC_DIR} 的版本 ${SRC_VER} 不一致," >&2
      echo "      陈旧二进制的「校验通过」可能是空真通过,故不使用它。" >&2
      echo "      处置: bash installer/install.sh --update 重装(git pull 不会重编译)。" >&2
    fi
  fi
fi

if [ -x "$BIN" ] && [ "$STALE" -eq 0 ]; then
  "$BIN" "$@"
  RC=$?
  if [ "$RC" -lt 126 ]; then
    exit "$RC"
  fi
  # 二进制**存在**却执行不了(架构不符/损坏)。这是「不可用」,不是「没装」——若后面也
  # 降级不成,必须落 124 而不是 127,否则 T9 会把「装了但坏了」当「根本没装」放行。
  BROKEN=1
  echo "WARN: $BIN 执行失败(rc=$RC,可能架构不符/二进制损坏),降级源码运行" >&2
fi

if [ -d "./validator" ] && command -v go >/dev/null 2>&1; then
  # 源码在手却建不出/跑不了,同样是「不可用」而非「未安装」。先置位,成功路径会 exit 掉。
  BROKEN=1
  BUILD=$(mktemp "${TMPDIR:-/tmp}/arcforge-$TOOL.XXXXXX") \
    || { echo "错误: mktemp 失败(无法准备构建产物)" >&2; exit 124; }
  trap 'rm -f "$BUILD"' EXIT
  # -buildvcs=false:linked worktree 的 `.git` 是**文件**,go 的 VCS stamping 不认它、继续向上
  # latch 到第一个含 `.git` **目录**的祖先;那个祖先若不是真仓库,`git status` 就吐 exit 128,
  # 整个 build 失败 → 第 2 级恒不可用 → 链路塌到 127。而 CLAUDE.md 规定每个 dev 都在
  # `../wt-<TASK-ID>` 这样的 linked worktree 里干活,等于 dev 在自己的工作区跑不了任何门禁。
  # 本包压根不消费 VCS stamp(版本经 install.sh 的 -ldflags 注入,源码直跑恒为 dev),关掉无损。
  if go build -buildvcs=false -o "$BUILD" "./validator/cmd/arcforge-$TOOL"; then
    "$BUILD" "$@"
    RC=$?
    # 与 level-1 **同一条规则**:rc ≥ 126 不是业务码,是「执行层失败」。
    # 此前这里是裸 `exit $?`,会把 126(noexec TMPDIR 下内核拒绝执行)与 128+N(被信号杀)
    # 原样漏出去 —— 那两个码不在退出码表里,下游按业务码解读就是错的。此处收敛为 124。
    if [ "$RC" -lt 126 ]; then
      exit "$RC"   # 业务退出码原样透传(不经 go run 塌缩)
    fi
    echo "错误: 源码构建产物执行失败(rc=$RC,如 TMPDIR 挂 noexec 或进程被信号终止)。" >&2
    exit 124
  fi
  echo "WARN: go build 失败,源码不可用" >&2
fi

if [ "$STALE" -eq 1 ]; then
  # 与 127 刻意分开:127 是「没装」,125 是「装了但结论不可信且无从降级」。两者的处置动作
  # 不同(前者装工具/回退手工统计,后者重跑 install.sh),混用一个码会让上游做错决策。
  echo "错误: ${BIN} 版本落后于源码,且无源码可降级 —— 拒绝以陈旧二进制出具校验结论。" >&2
  echo "      处置: bash installer/install.sh --update" >&2
  exit 125
fi

if [ "$BROKEN" -eq 1 ]; then
  # **不可用**:东西是装了的(二进制存在,或 ./validator 在手),只是跑不起来。
  # 这条分支是审查 MAJOR-1 的修复点 —— 此前它与「压根没装」共用 127,
  # 导致 T9 无法区分,会把「装了但坏了」静默放行。
  echo "错误: validator 存在但不可用(二进制执行失败或源码构建失败),拒绝给出结论。" >&2
  echo "      这不是「未安装」——请修复环境后重试,或重跑 installer/install.sh。" >&2
  exit 124
fi

echo "validator 未分发(安装时无 Go 工具链或未重跑 install.sh),请回退手工统计" >&2
exit 127
