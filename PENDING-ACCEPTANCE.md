# 待验收（业务）

**这份文件只放「等真实数据到达才能判定的验收项」，不放机制变更（那是 `PENDING-MECHANISMS.md`）、
不放挂账（那是 CONTRACTS §D）。** 一项验收完就从这里删掉，文件空了就只剩这段说明。

⚠️ 它存在的理由：CONTRACTS 已 252 KB，验收项埋在 §F 里没人会主动翻到；而「等一周后再做」
这类事**没有任何机制会提醒**——`.arcforge/` 归档后运行时目录是空的，validator 也不管业务待办。

---

## Hestia 2026-08 月报首期增量验收

| | |
|---|---|
| 窗口 | **2026-09-09 ~ 09-15**（按 51 期 monthly 的发布日分布推算） |
| 前置 | ✅ 全部就绪：运行时已于 2026-09-04 切换（`db5890e`），链路四件事实测通过 |
| 阻塞什么 | **M1 关闭**的最后一项；M4（crisis 双时态迁移）明写「M1 验收后启动」 |
| 不阻塞什么 | M1.5 / M2 / M3 的**开发**——文档里没有一处说它们依赖 M1 验收 |

**不需要人守着**：月报到达时 launchd 自动处理并发 Telegram，验收是事后核对。

```bash
RT=/Users/zuowei/workspace/runtime/atlas
grep -E "^2026-08 |FAILED|diverged" $RT/logs/hestia-ingest.out.log | tail -5
tail -2 $RT/logs/hestia-ingest.err.log        # 基线 62 行，切换后未被写过
ls -l $RT/data/hestia-snapshots/              # 新增 <article_id>.html 即抓到了
```

判定四态见 vault `Projects/Hestia/README.md` 顶部同名小节（含每态要记的证据）。
一句话版：**入权威表或落 pending 都算通过，但都要有 Telegram 消息为证；
err.log 增长而 Telegram 静默 = 不通过**（本迭代最想抓的失效模式）。

🔴 **验收完成前不要向运行时部署新二进制**——`deploy.sh` 投递的是同一个 `bin/atlas`，
M1.5 的 `hestia_runs` 表会改到 ingest 路径，部署后 2026-08 就不是在 M1d 的交付物上处理的了
（与 sprint 里 `verify_baseline` 防的是同一件事）。M1.5 可以照常开发合并，只是别 deploy。

**验收后**：结果写进 `internal/hestia/CONTRACTS.md` 的 `## Sprint M1d`（新开 §G 记首期增量），
删掉本节与 vault README 顶部那节，并把「连续 3 期」判据（2026-09、2026-10）留在 §F 挂账到 M2 前。
