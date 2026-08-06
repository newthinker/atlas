package yahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
)

// defaultBaseURL is the production Yahoo Finance chart endpoint. It is the
// default for New; tests inject an httptest server URL via NewWithBaseURL.
const defaultBaseURL = "https://query1.finance.yahoo.com/v8/finance/chart"

// validSymbol matches stock symbols (AAPL, 600519.SH, 0700.HK),
// index symbols (^GSPC), futures symbols (GC=F) and Yahoo currency
// symbols (JPY=X).
// Validation is purely syntactic and intentionally decoupled from the
// phase-1 coverage list (see design §2.1).
var validSymbol = regexp.MustCompile(`^(\^[A-Za-z0-9]{1,10}|[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?|[A-Za-z]{1,6}=[FX])$`)

// validateSymbol checks if a symbol has valid format
func validateSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	if len(symbol) > 20 {
		return fmt.Errorf("symbol too long: %s", symbol)
	}
	if !validSymbol.MatchString(symbol) {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	return nil
}

// 突发限流参数(设计 §9 M2.1):日总量安全 ≠ 突发安全——2026-07-24 v1.4.0
// 首跑 20 家背靠背拉 10Y 行情即触发 429,且 engine fallback 同依赖 yahoo。
// 取包内常量不进 config(无按环境调参需求)。
//
// 相邻请求最小间隔已迁到 policy 策略表的 yahoo 限流域(500ms 不变),
// 三个主题同域共享同一个闸门。
const (
	retryBackoffBase  = time.Second      // 指数退避基数:1s→2s→4s
	maxRetryAttempts  = 3                // 单请求最多重试次数
	retryBudgetPerRun = 20               // 实例级重试预算(极端持续限流下快速失败)
	maxRetryAfterWait = 60 * time.Second // Retry-After 上界:服务端给极大值时不阻塞调度窗口
)

// 策略主题名。chart/eps/quote 同属 yahoo 限流域,TTL 各自独立。
const (
	topicChart = "yahoo.chart"
	topicEPS   = "yahoo.eps"
	topicQuote = "yahoo.quote"
)

// Yahoo implements the Yahoo Finance collector
type Yahoo struct {
	client     *http.Client
	config     collector.Config
	baseURL    string
	epsBaseURL string

	// gate 提供限流/缓存/合并/配额;退避与重试预算仍是本包自己的事(设计 §3.2)。
	gate *policy.Gate

	// 退避状态(见 do);字段可在测试中覆盖以缩短用例耗时。
	mu            sync.Mutex
	backoffBase   time.Duration
	maxRetries    int
	retryBudget   int
	maxRetryAfter time.Duration
}

// New creates a new Yahoo collector pointing at the production endpoints.
func New() *Yahoo {
	return NewWithBaseURLs(defaultBaseURL, defaultEPSBaseURL)
}

// NewWithBaseURL creates a Yahoo collector with a custom chart endpoint, using
// the same URL for both chart and EPS endpoints. It keeps existing tests that
// only wire a single httptest server working unchanged.
func NewWithBaseURL(baseURL string) *Yahoo {
	return NewWithBaseURLs(baseURL, baseURL)
}

// NewWithBaseURLs creates a Yahoo collector with independent chart and EPS
// endpoints. It is intended for tests that point each at an httptest server.
func NewWithBaseURLs(chartURL, epsURL string) *Yahoo {
	return &Yahoo{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:       chartURL,
		epsBaseURL:    epsURL,
		gate:          policy.Default(),
		backoffBase:   retryBackoffBase,
		maxRetries:    maxRetryAttempts,
		retryBudget:   retryBudgetPerRun,
		maxRetryAfter: maxRetryAfterWait,
	}
}

// newRequest builds a GET request carrying browser-like headers. Yahoo's
// unofficial endpoints return 403 without a User-Agent, so every code path
// (quote, history, EPS) must share these headers.
func (y *Yahoo) newRequest(reqURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return req, nil
}

// do sends req through the retry/backoff loop. Only idempotent GETs with nil
// body go through here (newRequest 只构造这种请求),所以同一 *http.Request
// 可安全重发。重试耗尽或预算用尽时返回最后一个响应,交由调用点既有的
// unexpected status 路径报错——错误语义零变更。
//
// 节流由 Gate 负责:首发请求已在 policy.Fetch 进入 fn 前节流过一次,故这里
// 只对**重试**(attempt > 0)补一次 Wait,避免首发等两倍间隔。
func (y *Yahoo) do(topic string, req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			y.gate.Wait(topic)
		}
		resp, err := y.client.Do(req)
		if err != nil {
			return nil, err // 网络层错误不重试,与既有行为一致
		}
		if !retryableStatus(resp.StatusCode) || attempt >= y.maxRetries || !y.takeRetryToken() {
			return resp, nil
		}
		wait := retryAfterWait(resp, y.backoffBase<<attempt, y.maxRetryAfter)
		_, _ = io.Copy(io.Discard, resp.Body) // 排干并关闭,允许连接复用
		_ = resp.Body.Close()
		time.Sleep(wait)
	}
}

// takeRetryToken 消耗一次实例级重试预算;预算耗尽后不再重试(快速失败,
// 不拖长调度窗口),超预算的标的按既有语义走 fallback/Failed。
func (y *Yahoo) takeRetryToken() bool {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.retryBudget <= 0 {
		return false
	}
	y.retryBudget--
	return true
}

// retryableStatus: 429 与 5xx 值得重试;4xx 其余(403/404 等)是确定性失败。
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// retryAfterWait 优先尊重 Retry-After 响应头(整数秒形式;http-date 形式
// yahoo 不用,解析失败即回退指数退避)。头给出的等待封顶 maxWait,避免服务端
// 返回极大值时单次重试阻塞过久拖长调度窗口。
func retryAfterWait(resp *http.Response, fallback, maxWait time.Duration) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return min(time.Duration(secs)*time.Second, maxWait)
		}
	}
	return fallback
}

func (y *Yahoo) Name() string {
	return "yahoo"
}

func (y *Yahoo) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketHK, core.MarketEU}
}

func (y *Yahoo) Init(cfg collector.Config) error {
	y.config = cfg
	return nil
}

func (y *Yahoo) Start(ctx context.Context) error {
	return nil
}

func (y *Yahoo) Stop() error {
	return nil
}

// toYahooSymbol converts internal symbol format to Yahoo format
func (y *Yahoo) toYahooSymbol(symbol string) string {
	// Shanghai stocks: 600519.SH -> 600519.SS
	if strings.HasSuffix(symbol, ".SH") {
		return strings.TrimSuffix(symbol, ".SH") + ".SS"
	}
	return symbol
}

// FetchQuote fetches real-time quote
// TTL=0(实时语义),但仍走 Gate 以共享 yahoo 限流闸门并合并同 symbol 的在途请求。
// mapPolicyErr 把 policy 包的哨兵错误换成本包的普通错误，防止它外泄给上层。
//
// **必须在每个 policy.Fetch 的返回值处调用，不能在更外层统一 catch**：更外层
// 分不清「policy 产生的 timeout」与「HTTP 客户端自己的 timeout」，统一 catch 会
// 把两类不同来源的错误压成一类。
//
// 用 %v 而非 %w 是**故意断链**：留了链，errors.Is 照样能取回 policy 哨兵，映射
// 等于没做。fn 自身产生的错误原样返回，不碰 —— 本函数只处理 Gate 产生的那些。
//
// ⚠ **临时性绝不可映射成永久性**：ErrTimeout/ErrQuotaExceeded 都是可重试的临时
// 故障，措辞若像「无此标的/无数据」，上层会停止重试并把错误结果落库 —— 那是一次
// 静默的数据损坏，不是一次失败。故文本里显式写 retryable，并保留原始原因备排障。
func mapPolicyErr(err error) error {
	if errors.Is(err, policy.ErrTimeout) || errors.Is(err, policy.ErrQuotaExceeded) {
		return fmt.Errorf("yahoo: temporary gate failure (retryable): %v", err)
	}
	return err
}

func (y *Yahoo) FetchQuote(symbol string) (*core.Quote, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	q, err := policy.Fetch(y.gate, topicQuote, symbol, func() (*core.Quote, error) {
		return y.fetchQuote(symbol)
	})
	if err != nil {
		return nil, mapPolicyErr(err)
	}
	// 合并时多个调用方共享同一指针,返回副本避免相互污染。
	out := *q
	return &out, nil
}

func (y *Yahoo) fetchQuote(symbol string) (*core.Quote, error) {
	yahooSymbol := y.toYahooSymbol(symbol)
	reqURL := fmt.Sprintf("%s/%s?interval=1d&range=1d", y.baseURL, url.PathEscape(yahooSymbol))

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.do(topicQuote, req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for symbol: %s", symbol)
	}

	r := result.Chart.Result[0]
	meta := r.Meta

	// Calculate change from previous close
	change := meta.RegularMarketPrice - meta.ChartPreviousClose

	return &core.Quote{
		Symbol:        symbol,
		Market:        y.detectMarket(symbol),
		Price:         meta.RegularMarketPrice,
		Open:          meta.RegularMarketOpen,
		High:          meta.RegularMarketDayHigh,
		Low:           meta.RegularMarketDayLow,
		PrevClose:     meta.ChartPreviousClose,
		Change:        change,
		ChangePercent: meta.RegularMarketChangePercent,
		Volume:        int64(meta.RegularMarketVolume),
		Time:          time.Unix(int64(meta.RegularMarketTime), 0),
		Source:        "yahoo",
	}, nil
}

// FetchHistory fetches historical OHLCV data
// 缓存键把 start/end 截断到分钟,让上层「以 time.Now() 为 end」的抖动仍能命中
// 同一缓存槽(沿用被取代的 CachedCollector.cacheKey 的口径)。
func (y *Yahoo) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d|%d|%s",
		symbol, start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix(), interval)
	data, err := policy.Fetch(y.gate, topicChart, key, func() ([]core.OHLCV, error) {
		return y.fetchHistory(symbol, start, end, interval)
	})
	if err != nil {
		return nil, mapPolicyErr(err)
	}
	// Gate 不复制返回值:缓存命中时多个调用方共享同一底层数组,故在此 clone。
	// core.OHLCV 今天全是值类型,浅元素拷贝即深拷贝——**这个前提由 core/types.go
	// 保证,不是这里**;若将来给它加了 map/slice/指针字段,这里必须改成逐元素深拷贝。
	return slices.Clone(data), nil
}

func (y *Yahoo) fetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	yahooSymbol := y.toYahooSymbol(symbol)
	yahooInterval := y.toYahooInterval(interval)

	reqURL := fmt.Sprintf("%s/%s?interval=%s&period1=%d&period2=%d",
		y.baseURL, url.PathEscape(yahooSymbol), yahooInterval, start.Unix(), end.Unix())

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.do(topicChart, req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for symbol: %s", symbol)
	}

	r := result.Chart.Result[0]
	timestamps := r.Timestamp
	quotes := r.Indicators.Quote[0]

	// n guards against jagged slices: use the shortest length across all fields.
	n := len(timestamps)
	for _, length := range []int{len(quotes.Open), len(quotes.High), len(quotes.Low), len(quotes.Close), len(quotes.Volume)} {
		if length < n {
			n = length
		}
	}

	data := make([]core.OHLCV, 0, n)
	for i := 0; i < n; i++ {
		ts := timestamps[i]
		// Skip bars with any missing field (Yahoo returns partial old bars).
		if quotes.Open[i] == nil || quotes.High[i] == nil || quotes.Low[i] == nil ||
			quotes.Close[i] == nil || quotes.Volume[i] == nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     *quotes.Open[i],
			High:     *quotes.High[i],
			Low:      *quotes.Low[i],
			Close:    *quotes.Close[i],
			Volume:   int64(*quotes.Volume[i]),
			Time:     time.Unix(int64(ts), 0),
		})
	}

	return data, nil
}

func (y *Yahoo) toYahooInterval(interval string) string {
	switch interval {
	case "1m":
		return "1m"
	case "5m":
		return "5m"
	case "1h":
		return "1h"
	case "1d":
		return "1d"
	default:
		return "1d"
	}
}

func (y *Yahoo) detectMarket(symbol string) core.Market {
	if strings.HasSuffix(symbol, ".HK") {
		return core.MarketHK
	}
	if strings.HasSuffix(symbol, ".SH") || strings.HasSuffix(symbol, ".SZ") {
		return core.MarketCNA
	}
	return core.MarketUS
}

// Yahoo API response types
type chartResponse struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type chartResult struct {
	Meta       chartMeta  `json:"meta"`
	Timestamp  []int      `json:"timestamp"`
	Indicators indicators `json:"indicators"`
}

type chartMeta struct {
	Symbol                     string  `json:"symbol"`
	RegularMarketPrice         float64 `json:"regularMarketPrice"`
	RegularMarketVolume        int     `json:"regularMarketVolume"`
	RegularMarketTime          int     `json:"regularMarketTime"`
	RegularMarketOpen          float64 `json:"regularMarketOpen"`
	RegularMarketDayHigh       float64 `json:"regularMarketDayHigh"`
	RegularMarketDayLow        float64 `json:"regularMarketDayLow"`
	ChartPreviousClose         float64 `json:"chartPreviousClose"`
	RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
}

type indicators struct {
	Quote []quoteIndicator `json:"quote"`
}

type quoteIndicator struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*int     `json:"volume"`
}
