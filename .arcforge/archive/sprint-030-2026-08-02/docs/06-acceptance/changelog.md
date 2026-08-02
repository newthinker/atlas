# Changelog — Sprint-030 / Prism M3.5a

范围 `dde9639..d012927`(11 commits,25 files,+2342/-66)

## Added

- **`internal/collector/tushare`** — Tushare Pro POST JSON 客户端。`FetchDailyBasic`(A 股估值
  pe_ttm/pb/ps_ttm)、`FetchIndexDaily`、`FetchHKDaily`、`FetchDaily`;200ms 节流兜底;
  40203 按 msg 拆分为 `ErrRateLimited`(临时,判据只认「频率超限」四字、与窗口口径无关)与
  `ErrNoPermission`(永久)。base URL 为 https。
- **`internal/collector/twelvedata`** — time_series 客户端,美股价格备源。8s 最小间隔节流
  (免费层 8 req/min);apikey 经 `Authorization` 请求头传递;`end+1` 兑现 `[start,end]`
  闭区间(TD 的 end_date 排他,原样透传会静默丢掉最新一根收盘价)。
- **`scripts/baostock/` + `internal/collector/baostock`** — A 股行情第三跳侧车桥。独立 venv
  (baostock 0.8.9 冻结)、stdlib http.server 仅绑 127.0.0.1:8181、adjustflag=3 不复权;
  上游 `error_code != "0"` 显式转 500(防静默 `200 []` 把故障伪装成无数据)。
- **`deploy/launchd/com.newthinker.atlas.baostock.plist`** — 桥常驻服务(KeepAlive),
  已接入 `scripts/ops/{deploy,install-services}.sh`。
- **`PrismConfig.BaostockBaseURL`** — 默认 `http://127.0.0.1:8181`。
- 降级跳:A 股公司估值 akshare→tushare `daily_basic`(分位本地算)、A 股指数链尾 tushare
  仅价格、美股价格 yahoo→twelvedata、港股 akshare→`hk_daily` 仅价格。每跳记 `Report.Degraded`。
- collector registry 登记 tushare(`FetchDaily`)与 baostock 作 A 股行情二/三跳备源。

## Fixed

- **twelvedata apikey 外泄(CRITICAL)** — 传输层失败时 `*url.Error` 携带含 apikey 的完整 URL,
  经 `Report.Failed` 流向日志与 Telegram 告警。凭证移出 URL + `wrapErr` 唯一 error 出口脱敏,
  且不保留 error 链(防 `errors.Unwrap` 取回未脱敏原文)。
- **tushare 估值兜底跳写全 NaN 行毒化增量水位(CRITICAL)** — 全 NaN 行会推进 `MAX(d)` 水位,
  致次日 `start=latest+1`、那些日期永不重访且报 fallback ok。加「至少一点三值非全 NaN」守卫
  (非 PE-only 判据,后者会误伤 `pe_ttm` 为 null 而 `pb`/`ps` 有值的亏损标的)。
- **仅价格跳吞掉 upsert 写失败** — 价格是该跳的全部交付物,写失败却报 fallback ok 且零落库。
  改为上抛 error。
- **兜底跳零行结果被判成功** — `hk_daily` 对 `0700.HK` 返回 `code=0` 但 items 为空(静默空),
  判成功会天天上报 fallback ok 却零写入。改为零行判失败。
- **`buildCollectors` 默认值时序** — 跑在 `ApplyDefaults()` 之前且操作副本,导致
  `BaostockBaseURL` 为空串、三跳静默缺席。
- **A 股指数链尾形态** — `index_dailybasic` 实测 40203 无权限,定为仅价格(非估值)。

## Changed

- `configs/config.example.yaml` 增 `collectors.tushare/twelvedata` 空 key 条目与密钥卫生注释。
- 港股用例语义修正:接线测试改名并声明不构成生产可用性证据;新增缺口锁定用例
  (按上游契约以 5 位建键,锁住「4 位取不到数」)。
- 文档:design §9 M3.5a 状态表与后续工作、§10 七条风险(含 40203 语义重载、tushare 容量边界)、
  spec §2 降级链总表(容量注记、港股不可用标注、跳序未实现);文档内代码引用由行号改函数名。
- ADR 就地修订:#3(40203 语义重载与拆分)、#8(symbol 形态对港股不成立,待修订)、
  #10(三跳三故障域路由层未实现);#2/#4 补实施结果;#7 补密钥卫生的**机制**而非仅结果。

## Known Limitations

- baostock 真桥取数未实证(上游 10030 本机不可达)。
- 港股 `hk_daily` 取数未实证(4/5 位形态未归一 + 限频),该跳当前不构成可用兜底。
- tushare 估值兜底受 1 次/分钟限频 × 串行遍历所限,批量断源时覆盖率约 1/3,只能救个别标的。
- `ErrRateLimited` 目前仅用于 Degraded 文案分叉,**尚无退避重试**。
- 以上四项均并入后续任务「限频感知退避」(design §9 已登记四面,顺序不可颠倒)。
