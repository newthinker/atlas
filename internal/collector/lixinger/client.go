package lixinger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
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
// after validating the Lixinger envelope (code==1). 退避重试策略留在 fn 内部
// （requestHTTP），Gate 只负责 TTL 缓存与在途合并。
//
// 这是设计 §1.3 缺陷的修复点：lixinger 的两条身份（eastmoney 的内部 fallback、
// Valuation/Fundamental source）都汇流到这里，闸门放在这里两条同时被覆盖。
func (l *Lixinger) request(endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// 缓存键含完整请求体：同一 endpoint 不同标的/字段必须落到不同槽。
	// 信封校验刻意留在 fn 内部——放到 Fetch 外面的话，HTTP 200 + code:0 的业务
	// 错误会被当成功 body 写进缓存，后续同 key 调用永远拿到它且不再发请求。
	raw, err := policy.Fetch(l.gate, "lixinger."+endpoint, string(body), func() ([]byte, error) {
		return l.requestHTTP(endpoint, body)
	})
	if err != nil {
		return nil, mapPolicyErr(err)
	}
	// Gate 命中缓存时不复制返回值，多个调用方拿到同一个切片；此处必须返回副本，
	// 否则任一调用方改写就会污染缓存。[]byte 的元素是 flat value type，
	// 浅复制即深复制。
	return bytes.Clone(raw), nil
}

// mapPolicyErr 把 policy 包的哨兵错误换成本包的普通错误，防止它外泄给上层。
//
// **必须在 policy.Fetch 的返回值处调用，不得在更外层统一 catch**：更外层分不清
// 「policy 产生的 timeout」与「HTTP 客户端自己的 timeout」。fn 自身产生的错误
// 原样返回、不碰。
//
// 本包**没有哨兵错误**（实测 0 个 var Err / errors.New），故走「断链 + 本包前缀」
// 这一支：用 %v 而非 %w。**用 %w 包一层前缀是挡不住的** —— 消息变了，但
// errors.Is(err, policy.ErrTimeout) 仍然成立，policy 错误照样在调用方可见的链上。
// （有哨兵的包如 tushare 走另一支：映射成本包哨兵并保留 %w，因为 refresh.go
// 的降级链靠 errors.Is 分叉。）
//
// ⚠ **临时性绝不可映射成永久性**：ErrTimeout/ErrQuotaExceeded 都是可重试的临时
// 故障，文案若写成「不支持/无此数据」会让运维照着去改配置，而问题只是要等窗口。
// 故显式写 retryable 并保留原始原因备排障。
func mapPolicyErr(err error) error {
	if errors.Is(err, policy.ErrTimeout) || errors.Is(err, policy.ErrQuotaExceeded) {
		return fmt.Errorf("lixinger: temporary gate failure (retryable): %v", err)
	}
	return err
}

// requestHTTP 发一次（或按退避调度多次）真实 HTTP 请求并校验信封。
// It applies the SKILL.md backoff retry policy for 429/5xx when l.retry is
// enabled; 4xx never retries.
func (l *Lixinger) requestHTTP(endpoint string, body []byte) ([]byte, error) {
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
