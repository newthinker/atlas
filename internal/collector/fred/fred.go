// Package fred fetches observation series from the FRED API
// (https://fred.stlouisfed.org/docs/api/). Free tier allows ~120 req/min —
// far above this module's 6 series/day, so no client-side rate limiting.
package fred

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.stlouisfed.org/fred"

// Observation is one dated value of a series. Missing observations
// (value ".") are filtered out by FetchSeries.
type Observation struct {
	Date  string // YYYY-MM-DD
	Value float64
}

type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
	backoff time.Duration // 重试退避基准；测试注入 1ms
}

func New(apiKey string) *Client { return NewWithBaseURL(apiKey, defaultBaseURL) }

// NewWithBaseURL is for tests injecting an httptest server.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
		backoff: 2 * time.Second,
	}
}

// FetchSeries returns the observations of seriesID within [start, end]
// (either may be empty for FRED's defaults). Transport errors and 5xx are
// retried up to 3 attempts with exponential backoff (design §4.3); 4xx fails
// immediately.
func (c *Client) FetchSeries(ctx context.Context, seriesID, start, end string) ([]Observation, error) {
	q := url.Values{}
	q.Set("series_id", seriesID)
	q.Set("api_key", c.apiKey)
	q.Set("file_type", "json")
	if start != "" {
		q.Set("observation_start", start)
	}
	if end != "" {
		q.Set("observation_end", end)
	}
	reqURL := c.baseURL + "/series/observations?" + q.Encode()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff << (attempt - 1)):
			}
		}
		obs, retryable, err := c.fetchOnce(ctx, reqURL)
		if err == nil {
			return obs, nil
		}
		lastErr = err
		if !retryable {
			return nil, fmt.Errorf("fred %s: %w", seriesID, err)
		}
	}
	return nil, fmt.Errorf("fred %s after retries: %w", seriesID, lastErr)
}

func (c *Client) fetchOnce(ctx context.Context, reqURL string) ([]Observation, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		// url.Error.Error() embeds the full request URL, whose query carries
		// the api_key; strip it so transport failures logged to launchd stderr
		// never leak the key. Keep only the inner cause; still retryable.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, true, fmt.Errorf("transport: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 5xx is transient and worth retrying; 4xx is not.
		return nil, resp.StatusCode >= 500, fmt.Errorf("http %d", resp.StatusCode)
	}
	var body struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("decoding response: %w", err)
	}
	out := make([]Observation, 0, len(body.Observations))
	for _, o := range body.Observations {
		if o.Value == "." {
			continue
		}
		v, err := strconv.ParseFloat(o.Value, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parsing %s value %q: %w", o.Date, o.Value, err)
		}
		out = append(out, Observation{Date: o.Date, Value: v})
	}
	return out, false, nil
}
