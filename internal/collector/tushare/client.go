// Package tushare 直连 Tushare Pro HTTP API(POST JSON,token 鉴权,积分制)。
// 2026-08-02 实测(基础积分档):daily/index_daily/daily_basic/hk_daily ✅;
// us_daily/income ❌(40203)。角色 = A/H 估值+行情备源(spec §2)。
// token 只入 runtime configs/config.yaml(gitignored),不入仓不入日志。
package tushare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// token 走 POST body,必须 https:明文 http 会让凭证对路径上任意中间节点可见。
const defaultBaseURL = "https://api.tushare.pro"

// code 40203 是**重载码**:无权限与限频共用它,只能靠 msg 区分。两者的处置相反,
// 故拆成两个哨兵错误——分错不会走错执行路径(该跳本就只调一次),但会让降级链的
// Degraded 文案把「等一会儿就好」写成「去改配置」,误导运维。
var (
	// ErrNoPermission:积分/权限不足——**永久性**,改配置前重试无意义,
	// Degraded 文案应注明配置性问题(降级链原则,spec §2)。
	ErrNoPermission = errors.New("tushare: no api permission")
	// ErrRateLimited:访问频率超限——**临时性**,窗口过后自愈,
	// 调用方可按可重试/下一跳处理,不应提示改配置。
	ErrRateLimited = errors.New("tushare: rate limited")
)

// rateLimitMarker 是 tushare 限频 msg 的判别子串(实测原文形如
// 「抱歉,您访问接口(daily_basic)频率超限(1次/分钟)」;窗口口径会自升级到 1 次/小时,
// 故只认「频率超限」四字,不匹配具体窗口)。
const rateLimitMarker = "频率超限"

type Client struct {
	token, baseURL string
	hc             *http.Client
	gate           *policy.Gate
}

// topicPrefix + api_name 构成策略主题(如 tushare.daily_basic)。
// 200ms 基础档限频兜底与 daily_basic 的 5 次/天配额均登记在 policy 策略表里。
const topicPrefix = "tushare."

func New(token string) *Client { return NewWithBaseURL(token, defaultBaseURL) }

// NewWithBaseURL 构造一个 client。
//
// **gate 在此处快照 policy.Default(),不是每次调用现取**:任何要替换默认闸门的
// 调用方(尤其测试)必须在构造 client **之前** SetDefault,否则拿到的是旧闸门。
// 这条顺序依赖不会让编译失败,也不会确定性地让测试转红——错序时闸门只是变回
// 生产策略,表现为「偶尔慢/偶尔挂」,极易被当成 flaky 掩盖过去。
func NewWithBaseURL(token, baseURL string) *Client {
	return &Client{
		token:   token,
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 60 * time.Second},
		gate:    policy.Default(),
	}
}

type ValuationPoint struct {
	Date             time.Time
	PETTM, PB, PSTTM float64
}
type PricePoint struct {
	Date  time.Time
	Close float64
}

// row 是一行返回数据:date 取自 trade_date,values 按 fields 列名索引。
type row struct {
	date   time.Time
	values map[string]float64
}

// call 经策略闸门发一次 api 并返回按 fields 列名索引的行(按日期升序)。
//
// 返回的 []row 在缓存命中时由多个调用方共享(policy.Fetch 命中缓存时不复制返回值),
// 且 row.values 是 map ——**浅拷贝隔离不了它,slices.Clone 也不行**(复制的是 row
// 结构体,每个副本的 values 仍是同一个 map header 指向同一块底层数据)。
// 这里选择不复制,代价是本包所有消费者**必须只读 row**:FetchDailyBasic 与
// fetchClose 都只读 values 并立即构造新的值类型切片。
// 该不变量由 TestCachedRowsNotMutatedByConsumers 守护——**它不是一句注释约定**:
// 任何写 r.values[k] 的新消费者都会污染共享的缓存条目,使第二次(命中缓存的)
// 调用返回不同结果而转红。要改成复制的话必须逐行新建 map,不能用 slices.Clone。
func (c *Client) call(apiName string, params map[string]string, fields string) ([]row, error) {
	rows, err := policy.Fetch(c.gate, topicPrefix+apiName, callKey(params, fields), func() ([]row, error) {
		return c.callHTTP(apiName, params, fields)
	})
	// 本地配额预判与服务端限频语义一致:临时性、窗口过后自愈。映射成本包既有
	// 哨兵错误,让 prism 的降级链一行不改(设计 §5.1)——policy 包的错误绝不外泄
	// 到 prism 层。行为上只是从「撞墙后降级」提前为「撞墙前降级」。
	// **绝不可映射成永久性的 ErrNoPermission**:那会让降级链把「等窗口即可」报成
	// 「去改配置」,运维照着查积分档而问题根本不在那儿。
	if errors.Is(err, policy.ErrQuotaExceeded) {
		return nil, fmt.Errorf("%w: %s (本地配额预判，未发出请求)", ErrRateLimited, apiName)
	}
	return rows, err
}

// callKey 是缓存/合并键。map 遍历顺序随机,必须按键名排序后拼接,
// 否则同一次查询会散落到多个缓存槽、命中率静默归零。
func callKey(params map[string]string, fields string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
		b.WriteByte('&')
	}
	b.WriteString("fields=")
	b.WriteString(fields)
	return b.String()
}

// callHTTP 是 call 的网络层:POST 一次 api 并解析响应。不经闸门。
func (c *Client) callHTTP(apiName string, params map[string]string, fields string) ([]row, error) {
	body, _ := json.Marshal(map[string]any{
		"api_name": apiName, "token": c.token, "params": params, "fields": fields,
	})
	resp, err := c.hc.Post(c.baseURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tushare: %s: %w", apiName, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tushare: %s: read body: %w", apiName, err)
	}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Fields []string `json:"fields"`
			Items  [][]any  `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("tushare: %s: decode: %w", apiName, err)
	}
	if env.Code == 40203 {
		// 限频分支优先:同码不同义,msg 是唯一判据。
		if strings.Contains(env.Msg, rateLimitMarker) {
			return nil, fmt.Errorf("%w: %s (%s)", ErrRateLimited, apiName, env.Msg)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrNoPermission, apiName, env.Msg)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("tushare: %s: code %d: %s", apiName, env.Code, env.Msg)
	}
	idx := map[string]int{}
	for i, f := range env.Data.Fields {
		idx[f] = i
	}
	di, ok := idx["trade_date"]
	if !ok {
		return nil, fmt.Errorf("tushare: %s: no trade_date field", apiName)
	}
	rows := make([]row, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		if di >= len(item) {
			continue
		}
		ds, _ := item[di].(string)
		d, err := time.Parse("20060102", ds)
		if err != nil {
			continue
		}
		values := map[string]float64{}
		for f, i := range idx {
			values[f] = math.NaN() // 非数值列(如 ts_code)与缺列一律 NaN
			if i < len(item) {
				if v, ok := item[i].(float64); ok {
					values[f] = v
				}
			}
		}
		rows = append(rows, row{date: d, values: values})
	}
	// items 为倒序,统一转升序
	sort.Slice(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })
	return rows, nil
}

func dateParams(symbol string, start, end time.Time) map[string]string {
	return map[string]string{"ts_code": symbol,
		"start_date": start.Format("20060102"), "end_date": end.Format("20060102")}
}

// FetchDailyBasic 返回 A 股每日估值指标(官方计算口径)。
func (c *Client) FetchDailyBasic(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	rows, err := c.call("daily_basic", dateParams(symbol, start, end),
		"ts_code,trade_date,pe_ttm,pb,ps_ttm")
	if err != nil {
		return nil, err
	}
	out := make([]ValuationPoint, len(rows))
	for i, r := range rows {
		out[i] = ValuationPoint{
			Date:  r.date,
			PETTM: r.values["pe_ttm"], PB: r.values["pb"], PSTTM: r.values["ps_ttm"],
		}
	}
	return out, nil
}

func (c *Client) fetchClose(apiName, symbol string, start, end time.Time) ([]PricePoint, error) {
	rows, err := c.call(apiName, dateParams(symbol, start, end), "ts_code,trade_date,close")
	if err != nil {
		return nil, err
	}
	out := make([]PricePoint, len(rows))
	for i, r := range rows {
		out[i] = PricePoint{Date: r.date, Close: r.values["close"]}
	}
	return out, nil
}

func (c *Client) FetchIndexDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("index_daily", symbol, start, end)
}
func (c *Client) FetchHKDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("hk_daily", symbol, start, end)
}
func (c *Client) FetchDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("daily", symbol, start, end)
}
