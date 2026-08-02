# M3.5a 设计规格(指针文档)

正式设计规格在仓库文档中,本文件仅作指针与摘要,避免双写漂移:

- **Spec**: `docs/superpowers/specs/2026-08-02-prism-m3.5-design.md`
  - §0 实测记录(Stooq 出局、tushare 能力面、baostock 需建桥)
  - §2 降级链总表(市场×能力)——**本 Sprint 的验收基准**
  - §3 M3.5a 组件设计(三客户端 + 编排层)
  - §5 风险与对策 / §6 验收概要
- **实施计划**: `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`
  - 含 Task 1(tushare)完整参考实现与测试代码
  - 含 Task 3(bridge.py)完整参考实现
  - File Structure 一节 = 本 Sprint 全部预计改动文件

## 接口契约(任务间依赖的关键接口,来自计划,已 Self-Review 核对一致)

```go
// tushare(TASK-001 产出,TASK-005 消费)
func New(token string) *Client
func NewWithBaseURL(token, baseURL string) *Client
type ValuationPoint struct{ Date time.Time; PETTM, PB, PSTTM float64 }
type PricePoint struct{ Date time.Time; Close float64 }
func (c *Client) FetchDailyBasic(symbol string, start, end time.Time) ([]ValuationPoint, error)
func (c *Client) FetchIndexDaily / FetchHKDaily / FetchDaily (...) ([]PricePoint, error)
var ErrNoPermission = errors.New("tushare: no api permission") // 40203 永久性

// twelvedata(TASK-002 产出,TASK-005 消费)
func New(apiKey string) *Client
func NewWithBaseURL(apiKey, baseURL string) *Client
func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error)

// baostock(TASK-003 产出,TASK-005 仅注册到 collector registry)
func New(baseURL string) *Client
func (c *Client) FetchDaily(symbol string, start, end time.Time) ([]core.OHLCV, error)
// symbol Atlas 形态 "600519.SH" → 桥内 "sh.600519"

// 编排层(TASK-005)
type TushareClient interface { FetchDailyBasic; FetchIndexDaily; FetchHKDaily }
type TwelvedataClient interface { FetchHistory }
func Refresh(cfg, store, lix, us, ak, ed, ts TushareClient, td TwelvedataClient, now) Report
// nil = 未配置该跳,行为与改动前完全一致
```
