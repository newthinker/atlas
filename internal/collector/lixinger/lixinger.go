package lixinger

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

const defaultBaseURL = "https://open.lixinger.com/api"

// defaultRetryDelays is the SKILL.md-mandated backoff schedule for 429/5xx.
var defaultRetryDelays = []time.Duration{
	1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
}

// Lixinger implements FundamentalCollector for the Lixinger open API.
type Lixinger struct {
	apiKey      string
	baseURL     string
	client      *http.Client
	retry       bool            // 429/5xx 退避重试开关
	retryDelays []time.Duration // 退避调度；测试可置零加速
}

// Option configures a Lixinger collector.
type Option func(*Lixinger)

// WithRetry toggles the SKILL.md 429/5xx backoff retry policy.
func WithRetry(enabled bool) Option { return func(l *Lixinger) { l.retry = enabled } }

// New creates a Lixinger collector against the production API. Retry defaults
// to enabled (SKILL.md); pass WithRetry(false) to disable.
func New(apiKey string, opts ...Option) *Lixinger {
	l := newWithBaseURL(apiKey, defaultBaseURL)
	l.retry = true
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewWithBaseURL creates a collector pointed at a custom base URL with retry
// disabled. Used by tests to inject an httptest.Server endpoint.
func NewWithBaseURL(apiKey, baseURL string) *Lixinger {
	return newWithBaseURL(apiKey, baseURL)
}

func newWithBaseURL(apiKey, baseURL string) *Lixinger {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Lixinger{
		apiKey:      apiKey,
		baseURL:     baseURL,
		client:      &http.Client{Timeout: 30 * time.Second},
		retry:       false,
		retryDelays: defaultRetryDelays,
	}
}

func (l *Lixinger) Name() string { return "lixinger" }

func (l *Lixinger) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCNA}
}

func (l *Lixinger) Init(cfg collector.Config) error {
	if cfg.APIKey != "" {
		l.apiKey = cfg.APIKey
	}
	if l.apiKey == "" {
		return fmt.Errorf("lixinger: api_key is required")
	}
	return nil
}

func (l *Lixinger) Start(ctx context.Context) error { return nil }
func (l *Lixinger) Stop() error                     { return nil }

// HasAPIKey returns true if the collector has a valid API key configured
func (l *Lixinger) HasAPIKey() bool {
	return l.apiKey != ""
}

// toLixingerSymbol converts internal symbol format to Lixinger format
// 600519.SH -> 600519, 000001.SZ -> 000001
func (l *Lixinger) toLixingerSymbol(symbol string) string {
	return strings.SplitN(symbol, ".", 2)[0]
}

// requireKey reports the standard error when no API key is configured. Every
// public fetch method guards on this before issuing a request.
func (l *Lixinger) requireKey() error {
	if l.apiKey == "" {
		return fmt.Errorf("lixinger: api_key is required")
	}
	return nil
}

// recentWindow returns a [now-10d, now] range, wide enough to span weekends and
// trading halts when only the latest bar/NAV is needed.
func recentWindow() (start, end time.Time) {
	end = time.Now()
	return end.AddDate(0, 0, -10), end
}

// setChangeFromPrevClose fills PrevClose/Change/ChangePercent on q from the
// previous close, guarding against division by zero. Shared by the stock and
// fund quote paths, which both derive a quote from a newest-first series.
func setChangeFromPrevClose(q *core.Quote, prevClose float64) {
	q.PrevClose = prevClose
	q.Change = q.Price - prevClose
	if prevClose != 0 {
		q.ChangePercent = (q.Price - prevClose) / prevClose * 100
	}
}
