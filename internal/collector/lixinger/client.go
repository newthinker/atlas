package lixinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent mirrors a recent Chrome UA as required by the Lixinger skill docs.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

// envelope is the common Lixinger response wrapper. Success is code==1; any
// other code (notably 0) is a business error carrying an `error` object.
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   *struct {
		Name     string `json:"name"`
		Message  string `json:"message"`
		Messages []struct {
			Message string `json:"message"`
		} `json:"messages"`
	} `json:"error"`
}

// request POSTs payload as JSON to baseURL/endpoint and returns the raw body
// after validating the Lixinger envelope (code==1). It applies the SKILL.md
// backoff retry policy for 429/5xx when l.retry is enabled; 4xx never retries.
func (l *Lixinger) request(endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", l.baseURL, endpoint)

	maxAttempts := 1
	if l.retry {
		maxAttempts = len(l.retryDelays) + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		raw, status, derr := l.doOnce(url, body)
		switch {
		case derr != nil: // 传输层错误：可重试
			lastErr = derr
		case status == http.StatusTooManyRequests || status >= 500:
			lastErr = fmt.Errorf("lixinger: retryable HTTP status %d", status)
		case status != http.StatusOK:
			// 4xx 不重试。若 body 是合法信封带 error，透出其 message；否则回退到状态码错误。
			if _, perr := parseEnvelope(raw); perr != nil {
				return nil, perr
			}
			return nil, fmt.Errorf("lixinger: unexpected HTTP status %d", status)
		default:
			return parseEnvelope(raw)
		}

		if attempt < maxAttempts-1 {
			time.Sleep(l.retryDelays[attempt])
		}
	}
	return nil, lastErr
}

func (l *Lixinger) doOnce(url string, body []byte) (raw []byte, status int, err error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lixinger: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("lixinger: read body: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// parseEnvelope validates the Lixinger envelope and returns the raw body on
// success (code==1) so callers can parse the data array themselves.
func parseEnvelope(raw []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("lixinger: decode envelope: %w", err)
	}
	if env.Code == 1 {
		return raw, nil
	}
	if env.Error != nil {
		if env.Error.Message != "" {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Message)
		}
		if len(env.Error.Messages) > 0 {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Messages[0].Message)
		}
		if env.Error.Name != "" {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Name)
		}
	}
	return nil, fmt.Errorf("lixinger: API error code %d: %s", env.Code, env.Message)
}
