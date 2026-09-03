# TASK-008 验证报告 · docs-only 收口：采锚、全量核对、真语料回归、CONTRACTS §A/§B、code-simplifier 终检

- 验证者：test-m1d-a　承接 epoch：1　判定：**VERIFIED**
- verify_baseline.head：`f3d6eb282c83e0ca730b1713907f5220114ee86b`（master，与验证时 HEAD 同值）；discovery sha256 与基线一致 `2a8736c2…b6b0`
- dev commit：`ba8e69b48dd63e860f69e220f205bd86f7b18b2c`（父 `540e84a0eee6e37c5a85cb4743a189a1744aae64`；merge `f3d6eb2`），`git show --stat` 只含 `internal/hestia/CONTRACTS.md` 68+/0-
- §B 采样锚：`540e84a0eee6e37c5a85cb4743a189a1744aae64`；`git diff --stat 540e84a f3d6eb2 -- '*.go' configs deploy go.mod go.sum` 0 行 ⇒ 锚上的数字对 master HEAD 仍有效
- 验证树：`git worktree add --detach ../wt-verify-TASK-008-m1d 540e84a0eee6e37c5a85cb4743a189a1744aae64`（复算 §B）；真语料回归在**主仓库**跑（`data/` 不进 git），产物 `scratchpad/m1d-reg-test-m1d-a/`

## 1. 结论

全部 DoD 为 `verify_by: review|manual`，按人工核对逐条通过。§B 每个数字都在锚 `540e84a` 上独立复算相符，且新增测试计数用两把独立的尺互验；真语料回归五组数在主仓库复跑逐字相同、与 dev 的输出逐行一致；§A1–A3 与需求模板一致（A2 两处偏离均有依据）；§A4 与 TASK-004 验证报告 §4 一致；提交只含 CONTRACTS.md、锚 `docs(TASK-008):`；code-simplifier 终检对应的代码范围零改动。

## 2. Done Criteria 覆盖矩阵（review/manual，无测试对应，列证据）

| # | verify_by | 完成标准 | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | review | 采锚前置：代码范围 `git status --short` 空；`ANCHOR` 全 sha；写 CONTRACTS 前 numstat 空 | discovery `verification.anchor` 记 `540e84a…`（采前采后同值、代码范围 status 空、numstat 空）；验证时主仓库 `git status --short -- internal/hestia cmd/atlas configs deploy go.mod go.sum` 0 行；`git diff --stat 540e84a f3d6eb2 -- '*.go' configs deploy go.mod go.sum` 0 行；§B 表头锚为该全 sha | PASS |
| functional[1] | review | 全量核对（gofmt / vet / 覆盖率 ≥ 96.3 / cmd 绿 / 五不动文件 / 三守卫 PASS 且导出面 want 未改） | 树 `540e84a`（TASK-007 二验时同一 sha 已跑过 gofmt 仅两欠账、vet rc=0、两包 ok）；本次复算：`-cover` 96.4%（coverprofile 2438/2528 = 96.44%，`go tool cover -func` total 96.4%）；`git diff --stat 4916106 540e84a -- {store,validate,parse,extract,fields}.go` 0 行；`TestPackageExposesNoWriteFunctions`、`TestFieldNamesAppearOnlyInFieldsGo`、`TestShippedConfigLoadsAndIsCalibrated` PASS；want 字面量 22 项，`go doc -all ./internal/hestia \| grep -c '^func '` = 22；`store_test.go` 自 `ae088eb` 0 行 diff | PASS |
| functional[2] | **manual** | 真语料回归五组数与 M1c-4 §B 逐字相等，exit 0 | 主仓库 HEAD `f3d6eb2`，`go build -o scratchpad/m1d-reg-test-m1d-a/atlas ./cmd/atlas`，`backfill load --dir <主仓库>/data/hestia-backfill-2026-08-14 --db …/reg.db --allow-incomplete` ⇒ **exit=0**，`load.txt` 394 行；`语料总篇数 218 / 待解析 217 / 本迭代不解析 1 / 解析成功 213 / 解析失败 4 / 合并后观测 97（单篇 28 + 合并组 69）/ 入权威表 76 / 落 pending 21 / 字段冲突（预期 0，共 0）/ 口径路由核对 … 预期违反 0，共 0 违反`；与 M1c-4 §B（锚 `bdc8252`）逐字相等；与 dev 的 `scratchpad/m1d-reg-dev-m1d-b/load.txt` 同一 grep 的 10 行 `diff` 为空 | PASS |
| functional[3] | review | CONTRACTS 新开 `## Sprint M1d …` 含 §A1–A3、§A4、§B 表；新增测试行按实际 `git diff` 核对并注明差因 | 段落存在；§A1/§A3 与需求 Step 5 模板（L1442-1461）逐字相同；§A2 两处偏离：`（TASK-003）`→`（M1d 的 TASK-003）`（Global Constraints「注释带 milestone 前缀」）、追加一句 TASK-002 幂等规则（discovery `decisions[2]`，内容与 `snapshot.go` 事实一致）；§A4 与 `04-test/TASK-004-verification.md` §4 一致（阈值 ≥ 1e6、`1776000 → 1.776e+06`、真钉法 `assert.Equal(t, "1776000", fmtNum(1776000))` 挂账 M2、004 不改）。**新增测试 42**：尺① `git diff ae088eb 540e84a -- '*_test.go' \| grep -c '^+func Test'` = 42、`^-func Test` = 0；尺② `go test -list 'Test.*'` 两树差集 = hestia +34 / atlas +8 = 42、删除 0；分文件 config 3 · snapshot 8 · notify 8 · ingest 15（5+7+3）· discover 0 · cmd 8，与 §B 行「snapshot 8 · notify 8 · ingest 5+7+3 · config 3 · cmd 8」逐项相符；逐任务表 001=3/002=8/003=5/004=8/005=7/006=3/007=8 与我验各任务时读到的 diff 一致（002 discovery 报 8、003 报 5、007 首轮 6 + 返工 2）；§B「`$BASE` 上 2358/2448 = 96.32%」复算相符 | PASS |
| boundary[0] | review | code-simplifier 终检，预期无改动；有建议不自改、走澄清环 | discovery：11 个改动文件、回复无改动建议、`git status` + numstat 核实；验证：代码范围 status 0 行、`540e84a..f3d6eb2` 对 `*.go` 0 行、审计里无 `blocked_clarification`；「11 个」= 5 个 .go + 6 个 _test.go（含 discover_test.go）与 `git diff --stat ae088eb 540e84a -- '*.go'` 文件数一致 | PASS |
| error_handling[0] | review | 自证纪律：数字同锚；提交只含 CONTRACTS.md；锚 `docs(TASK-008):`；不写占位符 | `git show --stat ba8e69b` 只有 `internal/hestia/CONTRACTS.md`；提交信息 `docs(TASK-008): M1d …` 匹配 `^[a-z]+\(TASK-008\):`；§B 无 `<填>`/`<ANCHOR>` 占位符；discovery `measured_after_last_change` 成立（唯一改动是非代码文件） | PASS |
| non_functional[0] | review | docs-only 交付流程：主仓库分支、merge 后 dev_done、discovery 挂指针 | 链 `540e84a → ba8e69b → f3d6eb2`（merge 11:31 之前 dev_done 11:31:10Z，审计 `dev_done → verifying` 11:31:32Z）；discovery `files_modified: [internal/hestia/CONTRACTS.md]`，`update discovery` 审计行 11:31:10Z；无 `wt-TASK-008` 残留 | PASS |

**越界申报核对**：`git show --stat ba8e69b` ⇒ 恰 `internal/hestia/CONTRACTS.md`，与 `writes` 一致。

## 3. 备注

- discovery `decisions[4]`：首个提交 `4df0709` 因 trailer 笔误 amend 为 `ba8e69b`，未合并；merge `f3d6eb2` 第二父确为 `ba8e69b`（`git log --graph` 核过）。
- 验证者自我更正：我在 002/004 派验回执里写「TASK-004 实际 9 条测试」是错的，`notify_test.go` 新增 `func Test` 为 **8**（本报告尺①②与 §B 一致），Leader 原报的 8 正确。
- 沿用 M1c-4 §B 的告警：本节数字采于 `540e84a`，该 sha 之后 master 只多了本任务的 docs 提交，代码范围无变化（已核）。

## 4. 复现命令（锚全 sha）

```bash
# §B 复算
git worktree add --detach ../wt-verify-TASK-008-m1d 540e84a0eee6e37c5a85cb4743a189a1744aae64
cd ../wt-verify-TASK-008-m1d
GOTOOLCHAIN=local go test ./internal/hestia/ -coverprofile=/tmp/c.out -count=1 && go tool cover -func=/tmp/c.out | tail -1
git diff ae088eb253b64b36e10558a02587e3fa657f5f3e HEAD -- '*_test.go' | grep -c '^+func Test'    # 42
# 真语料回归（主仓库）
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
S=<scratchpad>/m1d-reg-<实例名>; mkdir -p $S; rm -f $S/reg.db*
GOTOOLCHAIN=local go build -o $S/atlas ./cmd/atlas
$S/atlas hestia backfill load --dir "$PWD/data/hestia-backfill-2026-08-14" --db $S/reg.db --allow-incomplete > $S/load.txt 2>&1; echo exit=$?
grep -E "语料总篇数|待解析|本迭代不解析|解析成功|解析失败|合并后观测|入权威表|落 pending|字段冲突|口径路由核对" $S/load.txt
```
