# TASK-006 验证报告：Warp 触发脚本（先判队列再唤起）

- 验证者：test-m4-a
- 判定对象：master @ `717c627d09f35251c247ad0fc5f2d0f79322745c`（= verify_baseline.head；判定时主仓库 HEAD 相同，`git diff --stat <baseline>..HEAD -- scripts/ops/` 为空）
- discovery sha256：`2892fb145bd5be6c0172b8f2ee8d7b78a0f206a79ffee7fe5756d4a82cd77b06`（与 baseline 一致）
- 被测文件 sha256：trigger `a2db3533…40298f05`、test `77932f9d…8dde8ff30f`（与 discovery 声明一致）
- 运行环境：独立 worktree `../wt-verify-TASK-006`（detached @ 上述全 sha）；GNU bash 3.2.57；shellcheck 0.11.0
- 结论：**VERIFIED**（8/8 条 done_criteria 通过；3 条非阻断观察见末节）

## 一、验证者亲自运行的证据

| 检查 | 命令 | 结果 |
|---|---|---|
| 自测 | `bash scripts/ops/hestia-warp-trigger-test.sh` | `9 passed, 0 failed`，rc=0 |
| 语法 | `bash -n` 两个脚本 | 均 rc=0 |
| 可执行位 | `ls -l` | 两者 `-rwxr-xr-x` |
| shellcheck | `shellcheck -S error …` 与默认级别 | 均 rc=0、零输出 |
| mv/rm | `grep -cE '\b(mv|rm)\b' scripts/ops/hestia-warp-trigger.sh` | 0 |
| 锁文件 | `grep -ciE 'lock' scripts/ops/hestia-warp-trigger.sh` | 0 |

真实 runtime 队列 `/Users/zuowei/workspace/runtime/atlas/queue/hestia`：验证前 pending/processing 均 0 件；变异 harness 前后 `os.walk` 目录树快照一致（True）；本人未往其中放任何文件；全部运行都带 TRIGGER_CMD 桩，变异运行额外把 `NANOCLAW_DIR` 指向不存在目录、PATH 前置记录型假 pnpm 作兜底（兜底调用记录 0 次）。

## 二、done_criteria 覆盖矩阵

| # | 完成标准 | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 自测 `N passed, 0 failed` 且 exit 0，N≥7 覆盖全部形态 | 亲跑 9 passed/0 failed rc=0；形态逐一对应下列各行 | PASS |
| functional[1] | 四目录全空 ⇒ exit 0、`queue empty`、桩未调用 | 用例「空队列」（rc 0 / 含 queue empty / calls 0）；变异 M10（改文案）被杀 | PASS |
| functional[2] | pending 1 件、processing 空 ⇒ `triggering`、桩恰 1 次 | 用例「有活」断言 calls==1（记录文件行数）；M7（`-le 1`）、M13 被杀 | PASS |
| boundary[0] | 两者非空 ⇒ exit 0、`busy`、未调用；只有 processing ⇒ 未调用 | 用例「agent 在跑」「只有 processing」；M2（busy 后去 exit 0）被杀 | PASS |
| boundary[1] | pending 只有子目录/点文件/`.tmp` ⇒ 视为空、未调用 | 用例「非计件条目」（空子目录 + .DS_Store + a.json.tmp）；M9/M9b/M9c 分别去掉三个过滤条件各自被杀；补充直测：非空子目录 ⇒ queue empty、calls 0 | PASS |
| error_handling[0] | pending 缺失、processing 缺失 ⇒ 非零、stderr 含路径、未调用 | 用例两例；M6（去 set -e）、M8（stderr→stdout）、M16（exit 0）均被杀 | PASS |
| non_functional[0] | RED 留痕、可执行位、bash -n、shellcheck 无 error | discovery `verification.red_phase` 含 9 例全 FAIL 原始输出（`0 passed, 9 failed`，成因为脚本尚不存在 rc=127）；其余见第一节 | PASS |
| non_functional[1] | 无锁/无 mv·rm；头注释写明两条理由；默认目录必须等于指定路径；不设 TRIGGER_CMD 走 `cd "$NANOCLAW_DIR" && pnpm run chat … \|\| true` | 读源码：第 4–6 行「先判后唤起」理由、第 10–13 行「别加超时自动清理」理由；第 21 行默认值逐字等于 `/Users/zuowei/workspace/runtime/atlas/queue/hestia`，并核对 `deploy/launchd/com.newthinker.atlas.serve.plist` WorkingDirectory=`/Users/zuowei/workspace/runtime/atlas` + `configs/hestia.yaml:41` `dir: queue/hestia`；第 53 行默认唤起形态吻合；M11（去 `|| true`）、M15（去 cd）被杀 | PASS |

## 三、验证者独立变异（隔离副本，mktemp，主工作区 sha 未变）

对照组 9/0。15 个变异，每个先 `bash -n`（均 rc=0）并打印 diff 核对：

- KILLED 12：M2、M6、M7、M8、M9、M9b、M9c、M10、M11、M13、M15、M16（杀死用例见矩阵）
- SURVIVED 3：
  - **M17 默认目录改为 `…/queue/hestia2`**：自测「默认队列目录」用 `[[ "$out$err" == *"$DEFAULT_QUEUE_DIR"* ]]` 子串匹配，`…/hestia2/pending` 含 `…/hestia` 前缀而通过。discovery 所称「默认目录截掉 /hestia 被杀」属实，但**延长/加后缀方向的错误不被该用例捕获**。DoD 此条为 review，已由读源码第 21 行逐字核对通过，故不阻断。
  - M18 交换 pending/processing 两个判定的顺序：「只有 processing」时文案由 queue empty 变 busy，DoD 对该形态只要求桩未调用 ⇒ DoD 意义上等价变异。
  - M19 去掉 `-maxdepth 1`：夹具中的子目录为空，嵌套文件未被覆盖。实现本身正确（补充直测非空子目录 ⇒ queue empty、calls 0），属自测覆盖缺口，非实现缺陷。

## 四、补充直测（直接调用被测脚本，均带桩）

| 形态 | 结果 |
|---|---|
| pending 仅含非空子目录 `sub/inner.json` | rc 0，queue empty，calls 0 |
| pending 仅含 symlink `link.json` | rc 0，queue empty，calls 0 |
| 只有 processing | rc 0，queue empty，calls 0 |
| pending 1 件 + processing 仅 `.DS_Store` | rc 0，triggering，calls 1 |
| 文件名含空格 | rc 0，triggering，calls 1 |
| pending 与 processing 都缺 | rc 1，stderr 含 pending 路径，calls 0 |
| pending 权限 000 | rc 1，stderr 为 find Permission denied，calls 0 |

## 五、非阻断观察（建议后续处理，不影响本次判定）

1. 自测「默认队列目录」断言为子串匹配，无法捕获默认路径被延长（M17）。可改为匹配 `(${DEFAULT_QUEUE_DIR})` 或 `${DEFAULT_QUEUE_DIR}/pending` 这类带边界的形态。
2. 「非计件条目」夹具的子目录为空，去掉 `-maxdepth 1` 不被捕获（M19）。可在子目录内放一个 `.json`。
3. symlink 口径与 `QueueHealthOf` 不一致：`internal/hestia/queue_health.go:37` 只排除 `IsDir()`/点文件/`.tmp`，symlink 会被计件；脚本 `-type f` 不计 symlink。atlas 经 `writeAtomic` 写普通文件，实际不产生 symlink，故无现实影响；DoD 列出的三类判据两者一致。

## 六、补充（Leader 转达 dev-m4-c 意见，2026-09-17，判定后追加，不改变结论）

dev-m4-c 指出：自测「默认队列目录」一例只断言「stdout+stderr 含默认路径」且桩调用 ≤1，其通过与否不取决于真实目录状态；若实现把默认路径硬编码进 echo 而实际读别的目录，该例查不出 ⇒ 它**不能证明默认值真的被使用**。

验证者核对：该判断与本报告第三节 M17 的实测一致且更强——M17（默认值改为 `…/hestia2`，此时真实目录不存在）自测仍 9/0 通过，即「目录不存在」这一形态确实被该用例放过。故 DoD non_functional[1] 的默认值判定**不依赖该自测**，依据是：读 `scripts/ops/hestia-warp-trigger.sh` 第 21 行，`QUEUE_DIR="${HESTIA_QUEUE_DIR:-/Users/zuowei/workspace/runtime/atlas/queue/hestia}"` 逐字等于要求路径；第 27 行 `count_of` 用的就是同一个 `$QUEUE_DIR`，决策行 echo 里的路径也取自这个变量，不是硬编码。第五节观察 1 的补强建议同样适用。
