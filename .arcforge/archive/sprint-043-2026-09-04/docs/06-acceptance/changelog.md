# Changelog · Sprint M1d（Sprint 043）· 2026-09-03

**范围**：`internal/hestia`、`cmd/atlas`、`configs/hestia.yaml`、`deploy/launchd/com.newthinker.atlas.hestia-ingest.plist`、`internal/hestia/CONTRACTS.md`
**起点** `ae088eb253b64b36e10558a02587e3fa657f5f3e` → **终点** `290370feaa7e69b1eb533ca3c0a0683000853de4`

## 新增
- `storage.snapshot_dir`（默认 `data/hestia-snapshots`，显式空串拒绝）；`configs/hestia.yaml` 显式写出，`config_version` → `2026-09-03`（TASK-001）
- `internal/hestia/snapshot.go`：`saveSnapshot` 三态幂等——不存在写入 / 同字节跳过不改 mtime / 不同字节另存 `<id>.<UTC 时间戳>.html` 不覆盖；临时文件 + rename（TASK-002）
- `ingestOne` 在 Fetch 之后、Parse 之前落盘快照；写盘失败 ⇒ 该期失败（`snapshot` 阶段）；改稿打一行 `snapshot diverged`（TASK-003）
- `internal/hestia/notify.go`：`Sender` 窄接口 + `renderP0/P1/P2` 纯函数（TASK-004）
- `IngestDeps.Notify`：落 pending ⇒ P0，入权威表 ⇒ P2（任何 Verdict），失败 ⇒ P1；发送失败并进错误链（`notifyError`）且不级联；空跑 0 条（TASK-005）
- `IngestDeps.OnlyPeriod`：只与 Force 同用，Discover 之后过滤，0 匹配响亮失败（TASK-006）
- cmd：`hestia ingest --only-period YYYY-MM`（需 `--force`，开库前校验）；`buildHestiaSender` 从主配置 `notifiers.telegram` 构造；打印 `notify: telegram|disabled`；plist `ProgramArguments` 加 `--config /Users/zuowei/workspace/runtime/atlas/configs/config.yaml`（TASK-007）
- CONTRACTS `## Sprint M1d` §A（A1–A4）+ §B（TASK-008）

## QA 终审后的返工（review_fix 第 1 轮，4 条）
- TASK-002 A3：`saveSnapshot` 改稿后与已有 `<id>.*.html` 副本逐个比对（`findSnapshotCopy`），命中即 Unchanged——修复「每次重抓再落一份相同字节副本」的幂等失效；A8 用非 UTC 时区断言文件名仍带 `Z`
- TASK-004 A7：`renderP2` 对 Duplicate 改写为「已在库（本次抽取值未写入）」——Force 重跑时 Values 实际不写库，原措辞会误导运维
- TASK-005 A4：`Ingest` 汇总行分子改用 `len(failedPeriods)`（原 `len(errs)` 在「解析失败 + 通知也失败」时打出 `2/1 期失败`）；补 `send P0` 失败用例与 `snapshot` 阶段名前缀断言
- TASK-007 A5：`buildHestiaSender() (hestia.Sender, error)`——主配置**装不上**时响亮失败，不再与「未配置」一样静默返回 nil

## 不变
- `store.go` / `validate.go` / `parse.go` / `extract.go` / `fields.go` diff 0 行；导出面 22 项未改；无新增依赖；`Meta` 七字段不动

## 其它提交
- `688c24c chore(arcforge)`：上游 PR #6 运行时同步（`--expect-status`、派验回执一句）——人类会话外所做，Leader 提交
- `2f5ad51 chore(gitnexus)`：索引再生（skill 目录迁移）——Leader 误跑，可 revert

## 结转（人类）
- 需求 TASK-009 运行时切换 / TASK-010 首期增量验收（09-09～09-15）/ TASK-011 文档回写与语料副本
