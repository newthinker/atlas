# TASK-006 验证报告 — 部署、断源演练与文档(docs-only/产物类)

验证者: test-agent-15 | 日期: 2026-08-02 | 判定: **VERIFIED (PASS)**

## 验证方式说明
本任务 done_criteria 全部为 `review`/`manual`,无 `verify_by: test` 条目,故**不适用**
go test 覆盖矩阵。核对方式 = 文件实证(亲跑 lint/语法检查/只读库查询)+ 记录自洽性
(discovery ↔ 计划文档 ↔ 环境实际状态三方交叉)。断源演练按 **Leader 定案的降格口径**
判定,未重跑(重跑会再耗 tushare 限频额度并再次改动生产库)。

## 完成标准覆盖矩阵

| # | 完成标准 | verify_by | 亲跑核对与证据 | 判定 |
|---|---|---|---|---|
| functional[0] | baostock plist 照 aktools 结构(Label/ProgramArguments/日志路径/KeepAlive),不含密钥;deploy.sh 与 install-services.sh 装载该 plist | review | 见下「① plist 与 ops 脚本」 | PASS |
| functional[1] | docs/deployment.md 增 baostock 桥部署段(setup/装载/日志/排障) | review | 见下「② deployment.md」 | PASS |
| functional[2] | design §10 增五条风险,§9 更新 M3.5a 状态 | review | 见下「③ 设计文档」 | PASS |
| non_functional[0] | 断源演练:akshare 断源 → tushare 恢复 + Degraded 上报;演练后还原 | manual | 见下「④ 断源演练(降格口径)」 | PASS |
| non_functional[1] | 正常 refresh 27 ok 0 failed;密钥哨兵终检 `git log -p --all -S` 为空;验收结果记入计划文档 | manual | 见下「⑤ refresh 与哨兵」 | PASS |

---

## ① plist 与 ops 脚本(functional[0])

**语法**:`plutil -lint deploy/launchd/com.newthinker.atlas.baostock.plist` → `OK`(exit 0)。

**结构逐键对照 aktools plist**(`com.newthinker.atlas.aktools.plist`),两者键集完全一致:

| 键 | aktools | baostock | 判定 |
|---|---|---|---|
| Label | com.newthinker.atlas.aktools | com.newthinker.atlas.baostock | ✅ |
| ProgramArguments | `scripts/akshare/.venv/bin/python -m aktools --host --port 8180` | `scripts/baostock/.venv/bin/python` + `bridge.py` | ✅ 指向 venv 解释器 + bridge.py,与 DoD 一致 |
| WorkingDirectory | runtime 根 | runtime 根(同值) | ✅ |
| RunAtLoad / KeepAlive | true / true | true / true | ✅ |
| ThrottleInterval | 10 | 10 | ✅ |
| StandardOut/ErrorPath | logs/aktools.{out,err}.log | logs/baostock.{out,err}.log | ✅ 日志路径正确 |

差异仅在 ProgramArguments 形态(baostock 无 CLI 旗标,端口硬编码在 bridge.py),
plist 内已有注释说明该差异及「改端口须同时改 bridge.py 与 runtime config」。

**无密钥**:plist 中 `api_key|apikey|token|secret|password|passwd` 关键字命中 **0**;
密钥哨兵(runtime 两个 key 前 8 位)命中 **0**。`scripts/baostock/` 源文件
(排除 `.venv`)同样命中 **0**;`.venv` 已由 `.gitignore:54` 忽略。

**ops 脚本**:`bash -n scripts/ops/deploy.sh` exit=0、`bash -n scripts/ops/install-services.sh`
exit=0。`install-services.sh:31` 的装载循环已含 `com.newthinker.atlas.baostock`,
循环体内做 `plutil -lint` → `cp` → `bootout` 等待 teardown → `bootstrap`(带 3 次 EIO 重试),
即新 plist 走的是与其余 9 个服务完全相同的幂等装载路径。头注释 `:13` 同步加了 baostock
说明行(注明需先跑 `scripts/baostock/setup.sh`)。`deploy.sh:49` 加
`--exclude='/scripts/baostock/.venv/'`(紧邻既有的 akshare 同款排除),头注释 `:7` 同步更新
——**runtime 侧独立安装的 venv 受 `--delete` 保护不被清除**,这正是 DoD 要点。
`kickstart serve` 提示在 `deploy.sh:66`(既有行,未被破坏)。

## ② docs/deployment.md(functional[1])

服务清单表加一行 baostock(常驻,127.0.0.1:8181,注明「失败只影响该跳」);
新增「Baostock 桥部署要点」整段(+51 行),含 DoD 要求的 setup/装载/日志/排障四类内容:
投递、建 venv(幂等/lock/`--upgrade`/默认 python3.11)、装载常驻、配置要点、验证 curl、
日志路径、复权口径(`adjustflag=3` 不复权)。

**Leader 点名要核的 4 条实测排障全部在位**,且每条都带可执行判别方法:
1. **bs.login 阻塞 ~30s** —— 明写「启动慢是正常的,不是卡死」,`bs.login()` 模块加载时执行,
   上游不可达时阻塞约 30 秒才轮到 `HTTPServer` 监听。
2. **健康检查探端口** —— 明写「健康检查必须探 8181 端口(`lsof -nP -i TCP:8181`)而非
   『进程存在』」,并给出理由(进程起来不等于端口就绪)。
3. **KeepAlive 反复重启告警** —— 明写真正触发重启循环的是**端口被占**
   (`OSError: Address already in use`),症状是 err.log 反复同一 traceback,给出
   `lsof -i :8181` + `launchctl kickstart -k` 处置;并**对比**说明上游挂了桥**不会**崩溃循环
   (转 500 而非静默 200,以便降级链感知)。
4. **curl --noproxy** —— 明写 `--noproxy '*'` 不能省,因本机 `http_proxy=127.0.0.1:7897`
   会让 curl 把 127.0.0.1:8181 也走代理,**桥未就绪时返回代理的 502 而非真实错误**,
   足以把排障带偏(2026-08-02 实测踩过)。

## ③ 设计文档 §10 / §9(functional[2])

**§10 风险表新增 7 条**,DoD 列的 5 条**全部在位**,另 2 条为 Leader 批准的超集:

| # | 风险条目 | 归属 |
|---|---|---|
| 1 | Stooq 全站 PoW 反爬(方案作废,记录防重踩) | DoD 五条之一 |
| 2 | Twelve Data 免费层 800 次/天(成分股不走 TD) | DoD 五条之一 |
| 3 | tushare 积分边界(能力面以实测为准,指数链尾降为仅价格) | DoD 五条之一 |
| 4 | **tushare 40203 语义重载**(同码不同义,临时限频被判永久) | Leader 批准超集 |
| 5 | N-PORT 标识映射与 ~2 月滞后(M3.5b 预告) | DoD 五条之一 |
| 6 | 密钥卫生(tushare token / TD key,含哨兵终检口径) | DoD 五条之一 |
| 7 | **baostock 桥成为新常驻服务**(私有 TCP 10030,代理无效) | Leader 批准超集 |

第 4 条与第 7 条与 discovery `decisions[0]` 申报的超集理由一致(40203 为 Leader 明确要求;
baostock 常驻服务为 spec §5 列出但 DoD 未点名,是本 sprint 唯一新增常驻服务)。第 4 条
还带了完整复现证据(同 token 连续探针:正常 → 1次/分钟 → 约 75 秒后 1次/小时)与判别线索
(msg 含「频率超限」即临时),并显式标注**修复未做、留待决策**——如实而非粉饰。

**§9 M3.5a 状态表**:7 行(tushare / twelvedata / baostock 桥 / 配置接线 / 编排层降级链 /
部署与断源演练 / 港股跳),每行带「状态 + 实测证据或缺口」。两处**未实证项主动标黄**
(baostock 上游未实证、港股跳未实证取到数据),未被包装成 ✅。Stooq 以删除线标注出局
并给出替代;M3.5b 预告段在位。

## ④ 断源演练(non_functional[0],按 Leader 定案的降格口径判定)

Leader 定案:实证标准降格为「演练完整执行 + 接线正确(tushare 跳被调用 + Degraded 上报)
+ 结论如实记录」;「A 股经 tushare 恢复数据」未实证系三条外部根因,已批准按 PARTIAL 处理。
验证者据此核对四项:

**(a) 演练完整执行且记录自洽** ✅
discovery `drill_timeline` 8 个时间点(13:25 备份 → 13:25 基线 → 13:26 断源首跑 →
13:28 回滚重跑 → 13:30 构造窗口重跑 → 13:31 还原 → 13:32 复验 → 13:33 复探+哨兵)与
计划文档「验收记录」6 节**逐项对应、数字全部一致**:`27 ok/0 failed/0 degraded`(基线与复验)、
`27 ok/3 failed/0 degraded`(首跑)、`23 ok/7 failed/4 degraded`(回滚后)、茅台三行
`19.96/20.58/20.41`、偏差 `~2e-8`、WAL `4.1MB`。两份记录无一处冲突。

**(b) 接线正确已实证** ✅
计划文档贴出的原始 refresh 输出显示 tushare 跳**确被调用**(`600519.SH: akshare: ...
connection refused; tushare fallback: ...` 四个标的均有 `tushare fallback:` 段)且
Degraded **确被上报**(`ℹ️ Prism 主源降级(已兜底):` 段)。这正是降格口径要的两点。

**(c) 三条外部根因证据齐备** ✅
① 40203 语义重载:输出中 Degraded 文案「权限不足,配置性问题,不重试」与同行 tushare msg
「频率超限(1次/分钟)」并排贴出,**矛盾自证**,非空口断言;② 限频容量:实测 600519 首个
通过、紧随的 600036/000423 全部撞限频,推出「串行遍历 ⇒ 单次 refresh 只有第一个 A 股标的
能兜底」,并记录窗口自升级(1次/分钟 → 1次/小时);③ 周末零行:演练日 2026-08-02 为周日、
水位 7/31 周五 ⇒ 窗口 [08-01,08-02] 全非交易日,并解释两源口径差异(akshare 取整段本地过滤
vs tushare 精确取)。三条均为外部依赖限制,**不是代码缺陷**,与 Leader 定案一致。
dev 还为绕开根因③ 主动删茅台三行构造含交易日窗口重跑(仍被根因② 阻断),
即三条根因是逐一排除后剩下的,不是一次失败的事后解释。

**(d) 环境还原:验证者独立实证(不采信 dev 记录)** ✅

| 核查项 | 方法 | 结果 |
|---|---|---|
| runtime config 还原 | 直读 `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml:290` | `akshare_base_url: "http://127.0.0.1:8180"` ✅;全文件 `18999` 命中 **0** ✅ |
| 生产库完整性 | `sqlite3 -readonly ... "PRAGMA integrity_check"` | `ok` ✅ |
| 库总行数 | `SELECT COUNT(*) FROM valuation_daily` | **58717**,与 discovery `env_restoration` 记录**逐字相同** ✅ |
| 演练中删除的茅台三行 | 只读查询 600519.SH 的 2026-07-29..07-31 | `19.96 / 20.58 / 20.41` 三值**逐一复现**,与记录相同 ✅ |
| 茅台序列有无缺口 | 查最近 5 个日期 | 07-28/29/30/31/08-01 连续,无空洞 ✅ |
| 演练残留进程 | `pgrep -fl bridge.py` | 无 ✅(aktools 8180 正常在跑) |

**排除一处疑似异常(避免误报)**:全库水位分布为「4 个标的 08-01、23 个标的 07-31」,
而这 4 个恰是演练涉及的 600519.SH/600036.SH/000423.SZ/0700.HK,初看像演练残留。
逐层下钻后**排除**:08-01 是**周六**,该日有行的标的按 market/type 分布为
`CN_A stock 3 + HK stock 1`,与**上一个周六 07-25 的分布完全相同**(同为 CN_A stock 3 +
HK stock 1,无 index、无 US);且该库历史上共有 **1840** 行周末数据,茅台的周末行可
连续回溯至 07-05。⇒ 系 akshare「取近一年整段序列」的既有数据源行为(指数与美股源不返回
周末行),**非演练残留**。

**(e) 美股断源** ✅ 未做 live,以单测 `TestRefreshUSPriceFallsBackToTwelvedata` 覆盖为准,
计划文档 Step 2 原文即允许(yahoo 不可控),且演练记录中已按 DoD 要求**注明**。
TD 复权口径已由 TASK-002 live 实证(我在 TASK-002 验证中独立核对过该结论)。

## ⑤ 正常 refresh 与密钥哨兵终检(non_functional[1])

**正常 refresh `27 ok, 0 failed`**(按 Leader 口径不重跑,以记录 + 库状态自洽为准)✅
库侧交叉印证:`SELECT COUNT(*) FROM instrument` = **27**,且
`COUNT(DISTINCT instrument_id) FROM valuation_daily` = **27**,即 27 个标的全部有估值数据
——与「27 ok, 0 failed」的标的基数**完全吻合**,不存在「报 27 ok 但库里只有 20 个标的」
这类不自洽。

**密钥哨兵终检:验证者亲自复跑** ✅
SENT1/SENT2 = runtime `configs/config.yaml` 第 30/34 行 tushare/twelvedata 两个 api_key 的
前 8 字符(现取,长度各 8,全程未打印):

| 扫描范围 | 命中 |
|---|---|
| `git log -p --all -S "$SENT1"`(**全历史全分支**) | **0** |
| `git log -p --all -S "$SENT2"`(**全历史全分支**) | **0** |
| `git diff --cached` | 0 |
| `git diff`(工作区) | 0 |
| baostock plist | 0 |
| `scripts/`(全目录) | 0 |
| `docs/`(全目录) | 0 |
| 全部未跟踪新文件 | 0 |

**反向对照(排除假阴性)**:同一对哨兵在 `configs/config.yaml` 中命中 **2** 次,
证明 grep 与 `-S` 检索本身有效。

**验收结果记入计划文档** ✅ `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`
Task 6 的 Step 1-3 复选框已勾选,并新增「验收记录(2026-08-02,dev-agent-34)」小节,
含 ①正常基线 ②断源演练(附原始输出与三条根因) ③环境还原与复验(含 WAL 坑) ④密钥哨兵终检
⑤baostock 上游复探 ⑥遗留未实证项汇总,共 6 节。该文件在 `writes` 补报内,越界已获批准。

## 越界申报核对
声明 `packages=[docs/deployment.md, docs/prism, deploy/launchd, scripts/ops]`、
`writes` 6 项。`git status` + `git diff --stat` 实际改动:
`deploy/launchd/com.newthinker.atlas.baostock.plist`(新建)、`scripts/ops/deploy.sh`(+3/-1)、
`scripts/ops/install-services.sh`(+3/-1)、`docs/deployment.md`(+51)、
`docs/prism/atlas_prism_design.md`(+28/-2)、
`docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`(+47/-4)——
**6 个文件全部落在声明范围内,无越界未申报**。
(另有未跟踪目录 `docs/collector/`,文件时间戳 2026-07-26 21:15,**早于本 sprint**,
与本任务无关,不计入。)

## 观察(非阻断,不影响判定,但交付时需注意)
1. **runtime 侧 `scripts/baostock/` 尚未投递**(deploy.sh 未跑),故 plist 的
   `ProgramArguments` 指向的解释器路径当前**不存在**。若此刻直接跑 `install-services.sh`,
   baostock 服务会因解释器缺失而 KeepAlive 失败循环。这**不是缺陷**——deployment.md 已
   文档化正确顺序(投递 → runtime 侧执行 setup.sh → 装载),但交付/部署时须按序执行。
2. baostock 上游 10030 持续不可达,该跳真实取数至今未实证;discovery `residual_risk`
   与 §9/§10 均已如实标注,并入了遗留项。
3. 40203 语义重载**未修**,已入 §10 并留待 Leader 决策立后续任务——这是本次演练暴露的
   最有价值的生产风险,建议不要遗漏。

## 判定
5 条 done_criteria(3 review + 2 manual)**逐条通过**。所有 review 条目均经亲跑
lint/语法检查与逐键结构对照实证;两条 manual 条目按 Leader 定案口径核对,记录三方自洽
(discovery ↔ 计划文档 ↔ 环境实际状态),环境还原经验证者**独立只读查询实证**
(integrity ok / 58717 行 / 茅台三行值逐一复现 / config 无残留),疑似异常水位已下钻排除;
密钥哨兵全历史全分支 0 命中且有反向对照;无越界申报。→ **VERIFIED**
