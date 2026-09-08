# TASK-003 交付记录：loom `spool.archive` 加 `source` 白名单参数

> Arcforge 任务 **TASK-003** ≡ 需求文档 **TASK-002**（`hestia/docs/superpowers/plans/2026-09-06-hestia-m3-warp-hestia.md` 第 438–585 行）。
> 两套编号并存是预期的：代码注释用需求编号（`M3 TASK-002` / `M3 的 TASK-002`），atlas commit subject 用 Arcforge 编号（`docs(TASK-003):`）。
>
> **跨仓库任务**：真实代码在 **loom**，atlas 的 git 看不见它。本文件是证据载体；
> **真正的质量闸是验证者按第 ③ 节的锚点去 loom 实跑**，不要以本文件写着「绿」为准。
> 本文件所有数字均采于**最后一次改动之后**（锚点 `6d6c38f96901dc60f516ef2154283a213782ad5e`），与该 commit 同源。

---

## ① 改动清单

锚点 commit：`6d6c38f96901dc60f516ef2154283a213782ad5e`（loom，分支 `feat/spool-source-param`，父提交 `0466116cc8c15c14ab8632d30d9be542fca5068c`）

`git show --numstat --format='' 6d6c38f96901dc60f516ef2154283a213782ad5e`：

```
6	1	configs/README.md
1	0	configs/config.yaml
16	3	internal/executors/spool/archive.go
76	0	internal/executors/spool/archive_test.go
12	8	internal/executors/spool/taint.go
22	10	internal/executors/spool/taint_test.go
```

合计 6 文件、133 增、22 删（`6+1+16+76+12+22 = 133` / `1+0+3+0+8+10 = 22`）。

| 文件 | 增/删 | 改了什么 |
|---|---|---|
| `internal/executors/spool/taint.go` | 12/8 | 删 `taintSource` 常量；加 `defaultSource = "web-research"` 与闭集 `allowedSources`；`injectTaint(content, source string)`；打标行改为 `"source: "+source` |
| `internal/executors/spool/archive.go` | 16/3 | `hasFrontmatter` 检查**之后**读 `params["source"]` → 空则缺省 → 非白名单 `return "DENIED", "source not allowed: "+source`；`injectTaint(content, source)` |
| `internal/executors/spool/archive_test.go` | 76/0 | 新增 3 条测试（含 2 条缺省回落子测试） |
| `internal/executors/spool/taint_test.go` | 22/10 | 新增 1 条测试；既有 10 处调用点改为 `injectTaint(in, "web-research")` |
| `configs/config.yaml` | 1/0 | `spool_write_allow` 加 `- Wiki/Macro/PBOC` |
| `configs/README.md` | 6/1 | 字段表那一行改写；Reed 段之前新增 `source` 参数说明段 |

**`injectTaint(` 全部出现处**（锚点树上 13 处 = 1 定义 + 1 生产调用 + 11 测试调用；
基线是 12 处，本次新增 1 处测试调用。⚠️ DoD 措辞的「12 处调用点」实为 `grep 'injectTaint('`
的 12 个命中，其中 1 个是**函数定义行**，真正的既有调用点是 11 个 —— 10 个在 `taint_test.go`
改为 `injectTaint(in, "web-research")`，1 个在 `archive.go` 改为 `injectTaint(content, source)`）：

```
internal/executors/spool/taint.go:61:func injectTaint(content, source string) string {
internal/executors/spool/taint_test.go:33:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:49:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:72:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:103:		out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:119:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:129:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:146:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:195:	out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:226:		out := injectTaint(in, "web-research")
internal/executors/spool/taint_test.go:262:	out := injectTaint("---\n---\n", "web-research") // must not panic
internal/executors/spool/taint_test.go:270:	out := injectTaint("---\ntype: summary\ncreated: 2026-09-12\nsource: evil\n---\n# hi\n", "hestia")
internal/executors/spool/archive.go:44:	content = injectTaint(content, source)
```

零处仍为单参：`grep -rn 'injectTaint([^,)]*)' internal/ | grep -vc 'func injectTaint'` → `0`。

### 关键代码（锚点树原样）

`internal/executors/spool/taint.go` 1–20 行：

```go
package spool

import "strings"

// Trust markers forced onto every archived note's frontmatter. reviewed is always
// false: a note written through Spool is unreviewed by definition, whatever the
// payload declares. source names the producing pipeline and is chosen by the
// caller from allowedSources (M3 TASK-002) — it is provenance, not trust.
const (
	taintReviewed = "reviewed: false"
	defaultSource = "web-research"
)

// allowedSources is the closed set a caller may stamp as `source`.
var allowedSources = map[string]bool{"web-research": true, "hestia": true}

// splitFrontmatter locates the first YAML frontmatter block and returns its body
// (the lines between the fences, no delimiters), the rest of the document from
// the closing fence onward (kept verbatim), and whether a well-formed block was
// found. The closing fence must be a FULL LINE equal to "---" (CRLF tolerated),
```

`internal/executors/spool/archive.go` 25–45 行：

```go
	if !hasFrontmatter(content) {
		return "DENIED", "content must start with YAML frontmatter (--- ... type/created ...)"
	}
	// M3 的 TASK-002: source names the producing pipeline. It is optional
	// (defaulting to web-research) but must come from the closed allowedSources
	// set — an unknown value is rejected before anything is written. A non-string
	// params["source"] fails the type assertion to "" and takes the default.
	source, _ := params["source"].(string)
	if source == "" {
		source = defaultSource
	}
	if !allowedSources[source] {
		return "DENIED", "source not allowed: " + source
	}
	// Stamp trust markers (source: <caller's choice>, reviewed: false) into the
	// frontmatter before it ever hits disk — this is the G-1 injection-chain
	// mitigation, applied only on the write path (spool.archive). reviewed is
	// overwritten unconditionally for every source: source is provenance, not
	// trust.
	content = injectTaint(content, source)
```

`configs/config.yaml` 17–19 行：

```yaml
spool_write_allow:
  - Wiki/Loom-Research           # Plan 2 Warp 调研正式落点
  - Wiki/Macro/PBOC              # Hestia M3 解读笔记落点
```

---

## ② 实测输出

以下全部在 loom worktree `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-loom-TASK-003`（分支 `feat/spool-source-param` @ `6d6c38f96901dc60f516ef2154283a213782ad5e`）执行。
退出码单独取，**不跨管道**。

### 2.1 TDD RED（error_handling[0]）—— 可在锚点上复现

RED 阶段的失败输出是在锚点上**确定性重放**得到的（原始 RED 跑在实现落盘之前；
为了让验证者拿到逐字节相同的输出，这里给出可复现配方）：

```bash
git -C /Users/zuowei/workspace/go/src/github.com/newthinker/loom \
    worktree add --detach /tmp/red-repro 6d6c38f96901dc60f516ef2154283a213782ad5e
cd /tmp/red-repro
git checkout 0466116cc8c15c14ab8632d30d9be542fca5068c -- internal/executors/spool/taint.go internal/executors/spool/archive.go
go test ./internal/executors/spool/ -run 'TestArchiveSource|TestTaintSourceParameterized'
```

退出码 **1**，输出原样：

```
# github.com/newthinker/loom/internal/executors/spool [github.com/newthinker/loom/internal/executors/spool.test]
internal/executors/spool/taint_test.go:33:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:49:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:72:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:103:26: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:119:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:129:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:146:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:195:25: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:226:26: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:262:35: too many arguments in call to injectTaint
	have (string, string)
	want (string)
internal/executors/spool/taint_test.go:262:35: too many errors
FAIL	github.com/newthinker/loom/internal/executors/spool [build failed]
FAIL
```

⚠️ 输出里 `too many arguments in call to injectTaint` **只有 10 行**，不是 11 行——
Go 编译器每包最多报 10 个错就以 `too many errors` 截断（末尾那行 `taint_test.go:262:35: too many errors`
即截断标记）。**「10」是编译器的上限，不是调用点计数**，不要拿它去核对调用点数量；
调用点数量以第 ① 节的 `grep` 清单为准。

### 2.2 四条新测试全 PASS（functional[0]）+ 两条边界子测试（boundary[0]）

```bash
go test ./internal/executors/spool/ -run 'TestArchiveSource|TestTaintSourceParameterized' -v
```

退出码 **0**，输出原样：

```
=== RUN   TestArchiveSourceDefaultsToWebResearch
=== RUN   TestArchiveSourceDefaultsToWebResearch/empty-string
=== RUN   TestArchiveSourceDefaultsToWebResearch/non-string
--- PASS: TestArchiveSourceDefaultsToWebResearch (0.00s)
    --- PASS: TestArchiveSourceDefaultsToWebResearch/empty-string (0.00s)
    --- PASS: TestArchiveSourceDefaultsToWebResearch/non-string (0.00s)
=== RUN   TestArchiveSourceHestiaAllowed
--- PASS: TestArchiveSourceHestiaAllowed (0.00s)
=== RUN   TestArchiveSourceUnknownDenied
--- PASS: TestArchiveSourceUnknownDenied (0.00s)
=== RUN   TestTaintSourceParameterized
--- PASS: TestTaintSourceParameterized (0.00s)
PASS
ok  	github.com/newthinker/loom/internal/executors/spool	0.259s
```

### 2.3 DENIED 不落盘的断言（boundary[0]）

`TestArchiveSourceUnknownDenied` 的断言用 `os.Stat` + `os.IsNotExist`，锚点树原文：

```go
func TestArchiveSourceUnknownDenied(t *testing.T) {
	s := newSpool(t)
	st, res := s.Handle(context.Background(), "spool.archive", map[string]any{
		"path": "Projects/Loom/.warp-out/x.md", "content": goodContent, "mode": "create", "source": "reviewed-by-me",
	})
	if st != "DENIED" || !strings.Contains(res.(string), "source") {
		t.Fatalf("want DENIED about source, got %s %v", st, res)
	}
	if _, err := os.Stat(filepath.Join(s.VaultRoot, "Projects/Loom/.warp-out/x.md")); !os.IsNotExist(err) {
		t.Fatalf("denied archive must not write")
	}
}
```

上面 2.2 的 `--- PASS: TestArchiveSourceUnknownDenied` 即该断言通过。

### 2.4 spool 全包（non_functional[0]）

```bash
go test ./internal/executors/spool/ -v
```

退出码 **0**；顶层 `--- PASS` **39** 条；任意层 `--- FAIL` **0** 条。尾部：

```
PASS
ok  	github.com/newthinker/loom/internal/executors/spool	0.687s
```

不带 `-v`（`-count=1` 绕开测试缓存，确保是真跑不是缓存命中）：

```
$ go test -count=1 ./internal/executors/spool/
ok  	github.com/newthinker/loom/internal/executors/spool	0.439s
退出码=0
```

### 2.5 测试条数复核 —— 两把独立的尺

尺 1（静态，`grep -c '^func Test'`）：

```
internal/executors/spool/archive_test.go:21
internal/executors/spool/commit_test.go:6
internal/executors/spool/taint_test.go:12
```

合计 **39**。

尺 2（动态，`go test -v` 的顶层 `--- PASS` 计数）：**39**。

两把尺同值 **39** = 基线 35（archive 18 / commit 6 / taint 11）+ 新增 4（archive +3 / taint +1）。
⚠️ 尺 2 的 `=== RUN` 是 48 条（含 `t.Run` 子测试），**不能**拿它与尺 1 相减——两者口径不同。

### 2.6 全仓库 `go test ./...`（non_functional[0]）

退出码 **0**，`tail -5`：

```
ok  	github.com/newthinker/loom/internal/gauge	(cached)
ok  	github.com/newthinker/loom/internal/lockfile	(cached)
ok  	github.com/newthinker/loom/internal/selvage	(cached)
ok  	github.com/newthinker/loom/internal/vaultgit	2.214s
ok  	github.com/newthinker/loom/scripts	8.858s
```

全文件 `^FAIL` 行数：**0**。

### 2.7 gofmt / vet / 依赖

| 命令 | 输出 | 退出码 |
|---|---|---|
| `gofmt -l internal/executors/spool` | （零输出） | 0 |
| `go vet ./internal/executors/spool/...` | （零输出） | 0 |
| `git diff --numstat 0466116cc8c15c14ab8632d30d9be542fca5068c 6d6c38f96901dc60f516ef2154283a213782ad5e -- go.mod go.sum` | （零输出，0 行） | 0 |

无新增 Go 依赖。

### 2.8 code-simplifier（non_functional[0]）

按全局规范 + DoD 要求跑了 `code-simplifier:code-simplifier`，约束已随 prompt 下发
（不放松 `reviewed: false` 无条件覆写、不改 `allowedSources` 闭集、不改既有测试断言、
不新增依赖、只碰那四个 Go 文件）。**其回复不可采信，以 `git diff` 为准**，实际两处改动：

| 它改的 | 我的处置 | 理由 |
|---|---|---|
| `taint.go`：把 `defaultSource` 并进既有 `const (...)` 块，而非另起一个 `const` 语句 | **采纳** | 语义完全等价，两个常量本就同族；需求原文写成独立 `const` 只是行文，不是约束 |
| `archive_test.go`：把边界子测试的表从 `[]struct{name,src}` 改成 `map[string]any` | **驳回，已还原为切片** | Go map 迭代序随机 ⇒ 子测试执行顺序不确定、失败输出不可复现。切片是 Go 表驱动测试的惯用形态。已在代码里留注释说明 |

三条硬约束**均未被触碰**：`taintReviewed = "reviewed: false"` 与其无条件 append 原样；
`allowedSources` 仍是两键闭集；既有 10 处测试调用点只加了第二个实参，断言一字未动。
还原之后**全部数字已统一重采**（本节 2.2–2.7 皆采于 `6d6c38f96901dc60f516ef2154283a213782ad5e`，即最后一次改动之后）。

---

## ③ 锚点

| 项 | 值 |
|---|---|
| 目标仓库 | loom `/Users/zuowei/workspace/go/src/github.com/newthinker/loom` |
| 分支 | `feat/spool-source-param` |
| **commit 全 sha** | **`6d6c38f96901dc60f516ef2154283a213782ad5e`** |
| 父提交（基线）全 sha | `0466116cc8c15c14ab8632d30d9be542fca5068c`（`main` 的当时 HEAD） |
| PR | https://github.com/newthinker/loom/pull/13 |
| 开发用 worktree | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-loom-TASK-003`（交付后拆除） |

验证者复跑配方（**锚已钉全 sha，不含 `HEAD` / 分支名这类会过期的符号引用**）：

```bash
git -C /Users/zuowei/workspace/go/src/github.com/newthinker/loom \
    worktree add --detach /tmp/verify-TASK-003 6d6c38f96901dc60f516ef2154283a213782ad5e
cd /tmp/verify-TASK-003
go test ./internal/executors/spool/ -v            # 期望 39 条顶层 PASS，退出码 0
go test ./...                                     # 期望全绿，退出码 0
gofmt -l internal/executors/spool                 # 期望零输出
go vet ./internal/executors/spool/...             # 期望零输出
grep -c '^func Test' internal/executors/spool/*_test.go   # 期望 21 / 6 / 12，合计 39
git -C /Users/zuowei/workspace/go/src/github.com/newthinker/loom \
    worktree remove --force /tmp/verify-TASK-003
```

⚠️ 若 PR 已被 squash/rebase 合并，`6d6c38f96901dc60f516ef2154283a213782ad5e` 可能不在 `main` 的祖先链上但**对象仍在**，
上面的 `worktree add --detach 6d6c38f96901dc60f516ef2154283a213782ad5e` 仍然可用；请勿改用 `main` 或 `HEAD` 复跑。

---

## ④ 未做与理由

### 4.1 `configs/config.local.yaml` 未改（gitignored 本机文件）

- **现状（我实读，未改）**：`/Users/zuowei/workspace/go/src/github.com/newthinker/loom/configs/config.local.yaml` 第 6–7 行仍是

  ```yaml
  spool_write_allow:
    - Wiki/Loom-Research
  ```

- **为什么没改**：该文件被 loom `.gitignore` 忽略，**不存在于 worktree 里**，改它就得动 loom 本机工作区，
  而 Leader 明确要求「不碰本机工作区」；需求原文 Step 4 与 DoD description 也都写「gitignored，**由人改**，记入 TASK-006 Step 0」。
  DoD functional[2] 字面写的是「两处都加」，与上面两句冲突，我已就此向 Leader 提问并按「不改」执行；
  若 Leader 裁定要我直接改，补上是一行的事。
- **后果**：**本机 selvage 在这一行补上之前，写 `Wiki/Macro/PBOC` 仍会 `DENIED "path outside write allowlist"`。**
  这是 M3 集成冒烟的硬前置。

### 4.2 selvage 未重启（agent 不执行）

`launchctl kickstart -k gui/$(id -u)/com.loom.selvage` 是**人执行**的（需求 TASK-006 Step 0），
我没有执行、也不会执行。config 改动在重启前不生效。

### 4.3 `Wiki/Macro/PBOC/` 目录是否被 Spool 自动创建 —— **未实证**

- **已核实的事实**：vault 根 `/Users/zuowei/Obsidian/ClawdVault` 下 `Wiki/` 存在，
  而 **`Wiki/Macro/` 与 `Wiki/Macro/PBOC/` 都不存在**（`ls -d` 退出码 1）。
- 上游 spec §2「总体形态」表声称「`Wiki/Macro/PBOC/` 由 Spool 首次写入时自动建」——
  **那是设计意图，不是实测**（spec §0.3 自己也说「vault 里的目标目录都不存在」）。
  本 sprint **没有**做过端到端写入，故这条**未实证**，属结转的冒烟项（需求 TASK-006）。
- 仅作为**代码阅读**（不是运行结果）：`archive.go` 在写白名单校验通过之后有一句无条件的
  `os.MkdirAll(parent, 0o755)`，随后对 `EvalSymlinks(parent)` 再做一次 root/白名单复检。
  ⇒ 机制上倾向于「会自动建」，但真实 vault 上的权限、iCloud 同步、符号链接、
  selvage 是否已加载新 config 这些条件**一个都没验过**，不要把这段读码当成验证通过。

### 4.4 PR 未合并

loom PR #13 已开、**未合并**（合并由人决定）。atlas 侧本文件的 merge 由 Leader 串行执行。

### 4.5 两套任务编号并存（不是笔误）

代码注释里写 `M3 TASK-002` / `M3 的 TASK-002`（需求编号，读者是三仓库开发者），
atlas commit subject 写 `docs(TASK-003):`（Arcforge 编号，门禁按它认领改动）。
「`M3 TASK-002`」（无「的」）出自需求原文给的代码样例，照抄保持原样；新写的注释用「`M3 的 TASK-002`」。
Leader 已裁决两种写法都接受，验证者**不得**因这个「的」字判不符。

### 4.6 交付过程中踩到的一个坑（记给后来人）

在 loom worktree 里 `cd` 着调用 `bash .claude/hooks/arcforge-write.sh`，
相对路径会解析到 **loom 自己的那份**写通道脚本，checkpoint 因此落进了
**loom worktree 的 `.arcforge/checkpoints/`**，**退出码 0、零告警**
（loom 那份脚本没有 linked-worktree DENY）。
⇒ 跨仓库任务里，写通道**一律用 atlas 绝对路径调用**。
误写的残留文件被 loom `.gitignore` 忽略、不会进任何提交，已随 worktree 拆除一并消失。
