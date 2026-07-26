# TASK-003 验证报告（桑基模板体系）

- 验证者: test-agent-6
- 交付 commit: c352c50
- 承接时 assignment_epoch: 1
- **判定: REJECTED**（JSON 8 条全部通过；Leader 追加的 (a)(b) 两条不满足）

## 1. 实跑证据

```
GOTOOLCHAIN=local go test ./internal/prism/sankey/ -count=1 -cover -v
7 个顶层测试 / 23 个子测试 全部 PASS
ok  github.com/newthinker/atlas/internal/prism/sankey  0.195s  coverage: 93.5% of statements
```

函数级覆盖（go tool cover -func）：

| 函数 | 覆盖率 |
|---|---|
| LoadTemplates | 92.9% |
| loadTemplate | 91.7% |
| validate | 100.0% |
| LoadManualSegments | 89.5% |
| total | 93.5% |

`git show --stat c352c50`：7 文件 +506 行；`git show c352c50 -- go.mod go.sum` **空输出**（零变更）。

## 2. done_criteria 逐条覆盖矩阵（JSON 内 8 条）

| # | 完成标准 | 对应测试 | 对偶护栏 | 判定 |
|---|---|---|---|---|
| F0 | 合法模板解析 company/cik/segment_axis/version + segments 逐字段 | `TestLoadTemplatesValid`（Segment 结构体整体相等断言，含 name_zh/name_en/xbrl_member）；`TestLoadTemplatesSegmentAxisDefault`（缺省回填 DefaultSegmentAxis） | `require.Len(got,1)` 挡多余条目；`require.Len(Segments,2)` 挡列表截断 | **PASS** |
| F1 | 校验分支：key 重复 / xbrl_member 空 / company 空 → error | `TestLoadTemplatesValidation` 8 个子用例（含 DoD 未要求的 dup xbrl_member、empty key、empty cik、non-numeric cik） | 每例 `require.Error` + `assert.Nil(got)` + error 含关键词 + **含文件名** | **PASS** |
| F2 | AD-16 三种目录情况必须区分 | `TestLoadTemplatesDirCases` 5 子用例：dir missing / dir without yaml / invalid yaml / **invalid among valid** / dup_key file | dir missing 断言 `NoError`+`Empty`+`NotNil`（正负成对）；invalid 断言 `Error`+`Nil`+含 `broken.yaml`；混放场景断言 nil，证明不返回部分结果 | **PASS** |
| F3 | LoadManualSegments 两层 map / 文件不存在空 map+nil / fiscal_period 正则校验 | `TestLoadManualSegments`（file exists 精确 map 相等，含小数 13020000000.5；大小写 symbol；file missing；dir missing）+ `TestLoadManualSegmentsBadPeriodKey`（2025Q5/2025-Q1/FY25/2025/Q1 + invalid yaml） | 正向覆盖 `2006Q1` 与 `FY2006` 两种合法格式；负向 6 例均 `Error`+`Nil`+含文件名；file/dir missing 断言 `NoError`+`Empty`+`NotNil` | **PASS** |
| F4 | 仓库真实模板冒烟 + msft 3 个 xbrl_member | `TestLoadTemplatesFromRepoDir` 直接加载 `configs/prism/templates/`，逐字段断言 cik=789019、axis、version=1、3 个 member **按序**相等、3 个 key、每 segment name_zh/name_en 非空 | 用 `assert.Equal([]string{...})` 而非 Contains，顺序与集合同时锁定 | **PASS** |
| B0 | 返回 map key 统一大写 symbol；cik 非空且纯数字 | `TestLoadTemplatesValid`（testdata 里 `company: acme` 小写 → 断言 `got["ACME"]` 存在 **且** `tmpl.Company=="acme"`，同时证明 key 归一、值不动）；`TestLoadTemplatesValidation/empty_cik`、`/non_numeric_cik` | 正负成对，设计良好 | **PASS** |
| N0 (review) | viper+mapstructure（AD-1）、go.mod direct 无新增 yaml、模板头注释说明 AD-5 | 人工 review：`template.go:15` import `github.com/spf13/viper`，全字段 `mapstructure` tag，无 yaml import；`go.mod` direct require 段无任何 yaml 包（yaml 仅 3 条 indirect，且 commit 对 go.mod/go.sum 零变更）；`configs/prism/templates/msft.yaml:1-5` 头注释写明主干流由 periods 引擎从 fundamental_q 构建、不进模板的理由 | — | **PASS** |
| N1 (test) | go test 全绿 | 见 §1 实跑输出 | — | **PASS** |

### 关于 packages 收窄的判断

dev 把 `packages` 从 `["./internal/prism/sankey","configs/prism/templates"]` 收窄为前者。
**该收窄未掩盖任何未验证交付物**：`configs/prism/templates/msft.yaml` 已随 c352c50 提交
（`git show --stat` 确认），其全部字段由 `TestLoadTemplatesFromRepoDir` 加载真实目录逐字段断言，
AD-5 头注释我已人工 review 通过。且 `-coverpkg` 本就无法度量非 Go 数据目录，保留该条目只会让门禁 setup failed。

## 3. viper 契约独立复现（不采信 dev 结论）

写临时探针 `zz_probe_verify_test.go`（运行后已删除，`git status` 确认工作树无残留），
对混合大小写 YAML 直接调 `viper.AllSettings()`：

```
AllSettings = map[
  "company":"MSFT",                                  # 值原样保留大写
  "segment_axis":"StatementBusinessSegmentsAxis",    # 键 Segment_Axis → segment_axis
  "fy2025":map["cloud":1, "mixedcase":2],            # 键 FY2025→fy2025, Cloud→cloud, MixedCase→mixedcase
  "2025q1":map["devices":3],
  "segments":[]{ map["key":"Cloud", "xbrl_member":"CloudMember"] }  # 列表内 map 键 Key→key 也小写化；值 "Cloud" 保留
]
```

**结论：dev 的自述属实，且我补充了一条更强的观察** —— viper 的小写化是**递归的**，
连**列表元素内部的 map key** 也会小写化（`Key` → `key`、`XBRL_Member` → `xbrl_member`），
而**任何层级的 value 都原样保留**。这正是 `Segment` 的 mapstructure tag 能匹配的原因，
也意味着 TASK-007/008/009 可以放心依赖 design-spec §3.1.1 的推论 1 与推论 2。

## 4. Leader 追加的两条验收要求（不在 JSON DoD 内，与 8 条同等效力）

> 说明：这两条是 **Leader 定义 DoD 时的遗漏，不是 dev 的质量问题**。dev 在 discovery
> 与 `Segment.Key` 注释里已主动识别出 (a) 的风险并写成约定，只是没有把约定升级为加载期校验。

### (a) `segments[].key` 含大写字符必须在加载期 error —— **FAIL**

探针实测（`TestProbeUpperCaseSegmentKey`）：

```
模板 segments: [{key: Cloud, ...}]  → LoadTemplates err=<nil>，Segments[0].Key = "Cloud"（静默接受）
手工数据 FY2025: {Cloud: 42}        → LoadManualSegments 返回 {"FY2025":{"cloud":42}}
以模板 key "Cloud" 查手工数据        → 结果 0（静默零值，无任何错误与日志）
```

失败模式与 Leader 描述完全一致：**运行期静默空数据而非加载期显式失败**。
当前 `template.go:24-26` 只有注释约定（"Key must be lower case"），`validate()` 无对应校验。

### (b) 两个模板文件声明同一 `company` 必须在加载期 error —— **FAIL**

探针实测（`TestProbeDuplicateCompany`）：两个文件均 `company: DUP`，

```
LoadTemplates err=<nil>, len(map)=1, 最终 DUP.Segments[0].Member = "SecondMember"
```

后加载者静默覆盖先加载者（`template.go:73` 直接 `out[...] = tmpl`），赢家取决于
`os.ReadDir` 顺序，跨平台不保证稳定。

> **⚠ 需 Leader 裁决的规格冲突**：`design-spec.md §3.1.1` 末尾「已知限制」明文写着
> 「未校验『两个模板文件声明同一 company』…… **本期不修，归属 TASK-011**」。
> 即当前设计文档把 (b) 显式推给了 TASK-011，与本次追加要求相矛盾。
> 无论 (b) 如何归属，**(a) 单独已足以构成 rejected**。请 Leader 二选一：
> 把 (b) 补进 TASK-003 的同时删掉 §3.1.1 的「本期不修」表述，或维持原设计只要求 dev 补 (a)。

## 5. 其他发现（非 DoD 强制，加固建议）

1. **两条真实可测但未覆盖的错误分支**（coverage profile 定位）：
   - `template.go:85-87` `loadTemplate` 的 `v.Unmarshal` 失败分支
   - `template.go:150-152` `LoadManualSegments` 的 `v.Unmarshal` 失败分支

   我实测这两条路径**行为正确**（error 含文件路径）：`segments: notalist` →
   `parsing sankey template <path>: ... 'segments[0]' expected a map or struct, got "string"`；
   `2025Q1: 123` → `parsing manual segments <path>: '[2025q1]' expected type 'map[string]float64'`；
   `cloud: notanumber` → `cannot parse value as 'float64'`。
   但**无测试锁定**——现有 `bad_yaml` 用例只覆盖 YAML **语法**错误（`ReadInConfig` 分支），
   未覆盖语法合法但 **schema 不匹配** 的情形。建议补 2 个用例。

2. 另两条未覆盖语句（`template.go:63`、`template.go:142`）是 `ReadDir`/`Stat` 的
   非 `ErrNotExist` 错误（权限类），跨平台难以稳定构造，**接受不覆盖**。

3. `TestLoadTemplatesDirCases/dir_without_yaml` 比同组的 `dir missing` 少一条
   `assert.NotNil(got)`，对偶护栏轻微不对称（1 行可补）。

## 6. 判定

**REJECTED** —— 理由：仅缺 Leader 追加的 (a)(b) 两条加载期校验，
JSON 内 8 条 done_criteria 已逐条验证通过，**已完成部分无需返工**。
