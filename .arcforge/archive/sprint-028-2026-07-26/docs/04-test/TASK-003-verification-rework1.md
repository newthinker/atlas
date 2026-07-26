# TASK-003 复验报告（rework-1，桑基模板体系）

- 验证者: test-agent-6
- 本轮承接 assignment_epoch: **2**（rework_count=1）
- 交付 commit: c352c50（首轮）+ **caf8d9f**（rework-1）
- 上轮报告: `.arcforge/docs/04-test/TASK-003-verification.md`（判定 rejected）
- **判定: VERIFIED**

## 1. 上轮两条不通过项的复验

**复用上轮 rejected 时的同一探针**（而非采信 dev 的 rework 说明），逐条复现：

| 上轮不通过项 | 上轮实测 | 本轮实测 | 判定 |
|---|---|---|---|
| (a) `segments[].key` 含大写须加载期 error | `err=<nil>`，`Key=="Cloud"` 被静默接受，以其查 manual 数据得 **0** | `err=invalid sankey template <path>: segments[0].key "Cloud" must be lower case, otherwise it never joins against the manual segments`，`got` 为 nil | **已修复** |
| (b) 两模板同 `company` 须加载期 error | `err=<nil>`，`len(map)=1`，赢家 `SecondMember` 由 `os.ReadDir` 顺序决定 | `err=duplicate company DUP declared in both <a_first.yaml> and <b_second.yaml>`，`got` 为 nil | **已修复** |

**超出要求的两点，我额外验证并确认成立**：
- 错误信息**同时点出两个冲突文件**（我原本只要求报错）。用户不必再翻目录找另一半。
- 大小写不同的 company 也判冲突：探针写 `company: DUPCO` 与 `company: dupco` 两文件 →
  `duplicate company DUPCO declared in both a.yaml and b.yaml`。与「返回 map key 统一大写」口径自洽。

**误伤检查**（新校验最容易出的问题）：探针加载仓库真实模板目录 →
`err=<nil>, len(got)=1`，`configs/prism/templates/msft.yaml` 的三个小写 key 未被新校验误伤。

## 2. 新校验的变异测试（证明测试有咬合力，非纸面修复）

在 `caf8d9f` 的一次性 worktree 上删除两条新校验：

| 变异 | 结果 |
|---|---|
| M1 删除 `case s.Key != strings.ToLower(s.Key)` 分支 | `TestLoadTemplatesValidation/upper_case_segment_key` **FAIL** ✓ |
| M2 删除 `srcOf[symbol]` 冲突检查 | `TestLoadTemplatesDuplicateCompany` **FAIL** ✓ |

还原后复跑 `ok`。**两条修复各有专属测试守护，删掉实现即刻变红。**

## 3. 原 8 条 done_criteria 无回归

`GOTOOLCHAIN=local go test ./internal/prism/sankey/ -count=1 -v` → **9 个顶层测试 / 36 个 PASS 条目全绿**。
上轮已通过的全部用例（`TestLoadTemplatesValid`、`SegmentAxisDefault`、`Validation` 原 8 子例、
`DirCases` 5 子例、`LoadManualSegments` 4 子例、`BadPeriodKey` 5 子例、`FromRepoDir`）**逐个仍在且仍 PASS**，
无一被删改。

**AD-1 复核**：`git show caf8d9f -- go.mod go.sum` **空输出**；`go.mod` direct require 段无任何 yaml 包
（yaml 系全部 `// indirect`）。`gofmt -l` 空、`go vet` 通过、工作树干净。

## 4. 我上轮提的 3 条加固建议的落实情况

| 建议 | 落实 |
|---|---|
| 1. 两处 `Unmarshal` 错误分支（`template.go` 的 loadTemplate / LoadManualSegments）无回归保护 | **已补**：新增 `TestLoadTemplatesValidation/segments_is_not_a_list` 与 `TestLoadManualSegmentsUnparsableFile` 3 子例（invalid yaml / period value is scalar / amount is not a number），均断言 `Error` + `Nil(got)` + 含文件名 |
| 2. `ReadDir`/`Stat` 的权限类错误跨平台难测 | **按我的建议跳过**（正当防御路径，非死代码）——是当前仅剩的 2 个未覆盖块 |
| 3. `dir_without_yaml` 缺 `NotNil` 对偶断言 | **已补**：`assert.NotNil(t, got, "未配置模板是合法状态，应返回可用的空 map")` |

## 5. 覆盖率

```
coverage: 97.1% of statements   （上轮 93.5%）
LoadTemplates 95.0% | loadTemplate 100.0% | validate 100.0% | LoadManualSegments 94.7%
```

未覆盖块仅剩 2 个：`template.go:64`（`os.ReadDir` 非 NotExist 错误）与 `template.go:154`
（`os.Stat` 非 NotExist 错误）——即上轮我判定「跨平台难以稳定构造、接受不覆盖」的两条权限类守卫。
`loadTemplate` 与 `validate` 已达 100%。

## 6. 上轮标记的规格冲突已消解

上轮我标记：`design-spec §3.1.1`「已知限制」把重复 company 校验推给 TASK-011，与追加要求 (b) 矛盾。
本轮核实该表述**已被移除**，§3.1.1 现在明文写「2. **两个模板文件声明同一 `company` → error**」，
与实现一致。文档与代码不再打架。

## 7. 判定

**VERIFIED** —— Leader 追加的 (a)(b) 两条已用同一探针复现确认修复（且错误信息质量超出要求）、
各有变异测试守护；原 8 条 done_criteria 逐个无回归；我上轮的 3 条加固建议按判断全部处置到位；
覆盖率 93.5% → 97.1%。无遗留问题。
