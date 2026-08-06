package policy

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// maxCacheEntries 限制 TTL 缓存条目数，超出时淘汰最旧的一条
// （沿用被本包取代的 CachedCollector 的界，量级相同）。
const maxCacheEntries = 512

// ErrTimeout 表示 fn 自身超过了主题策略的 Timeout。
var ErrTimeout = errors.New("policy: fn timeout")

// Gate 是唯一对外入口：按主题策略施加 TTL 缓存、在途合并、配额预判与节流。
//
// 执行链（设计 §3.2）：
//
//	① 查 TTL 缓存（命中即返回）
//	② singleflight 合并同 key 在途请求
//	③ 配额预判（超额 → ErrQuotaExceeded，不发请求）
//	④ 节流等待到限流域的最小间隔
//	⑤ 执行 fn（各 collector 自己的重试/退避层留在 fn 内部）
//
// ① 在 ④ 之前是刻意的：缓存命中不该再等节流，否则 twelvedata 命中缓存还要
// 白等 8s，缓存等于没加。
//
// 零值不可用，用 New 构造。nil *Gate 是透明的（直接执行 fn），
// 让未接线的调用点安全降级。
type Gate struct {
	table *Table
	quota QuotaStore
	warn  func(msg string, err error)

	domainsMu sync.Mutex
	domains   map[string]*domainState

	cacheMu sync.Mutex
	entries map[string]cacheEntry
	seq     uint64

	sf singleflight.Group
}

// domainState 是一个限流域的闸门状态。刻意在持锁状态下 sleep：并发调用方被
// 依次串行放行，天然形成均匀节奏（与被取代的 yahoo.throttle 行为一致）。
//
// 锁的粒度是**每个域一把**，不是全局一把——twelvedata 的 8s 等待若持有全局锁，
// 会连带卡死进程内所有其他 collector。
type domainState struct {
	mu      sync.Mutex
	lastReq time.Time
}

type cacheEntry struct {
	val      any
	storedAt time.Time
	seq      uint64
}

// Option 配置 Gate。
type Option func(*Gate)

// WithWarn 注入告警回调。配额账本异常时 fail-open 放行并经此上报。
func WithWarn(f func(msg string, err error)) Option {
	return func(g *Gate) { g.warn = f }
}

// New 构造 Gate。table 为 nil 时用内置表；q 为 nil 时不做配额预判。
func New(t *Table, q QuotaStore, opts ...Option) *Gate {
	if t == nil {
		t = NewTable()
	}
	g := &Gate{
		table:   t,
		quota:   q,
		warn:    func(string, error) {},
		domains: make(map[string]*domainState),
		entries: make(map[string]cacheEntry),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Fetch 是带 TTL 缓存的准入控制。
//
// Go 不支持泛型方法，故这是包级函数而非 Gate 的方法（设计 §3.2）。
//
// **返回值所有权**：缓存命中时多个调用方拿到的是同一个值，本函数**不复制**。
// T 为切片/映射时调用方必须自行深拷贝后再交给上层，否则会共享底层数据。
//
// ⚠ `slices.Clone` 只在**元素是 flat value type** 时才等于深拷贝——这个限定
// 来自被本包取代的 cloneOHLCV（「OHLCV is a flat value type, so a shallow
// element copy is a deep copy」）。元素含 map / slice / 指针字段时（如 tushare
// 的 row.values map[string]float64），slices.Clone 复制的是结构体、底层数据仍
// 共享，必须逐元素深拷贝。给错误的隔离手段比不给更糟：照做的人以为自己安全了。
//
// 这句话真正会被违反的地方在各 collector 的 FetchHistory 里，不在这里；写在
// 定义处只能让读到的人受益，**它不构成防线**——防线是各接入任务 DoD 里那条
// 「缓存命中时返回独立切片，调用方修改不污染缓存」的测试。
func Fetch[T any](g *Gate, topic, key string, fn func() (T, error)) (T, error) {
	return fetch(g, topic, key, false, fn)
}

// Do 是无返回值的准入控制，等价于 Fetch 且**强制 TTL=0**：没有结果可缓存时，
// 缓存一个空值会静默吞掉后续调用。
//
// ⚠ Do 虽强制不缓存，但仍走同一个 fetch，**因此照样受 singleflight 合并**。
// 内置表里所有登记主题的 Coalesce 都是 true，所以：
//
//	20 个并发 Do(topic, key, fn) 只会真正发生 1 次副作用。
//
// 拿 Do 去做「必须每次都发生」的调用（计数、审计、写操作）会静默合并且极难
// 排查。需要每次都发生时，请用不同的 key，或为该主题关闭 Coalesce。
func (g *Gate) Do(topic, key string, fn func() error) error {
	_, err := fetch(g, topic, key, true, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// Wait 只施加限流域节流，不碰缓存/合并/配额。
//
// 供 fn **内部的重试循环**复用同一个闸门：Fetch 已在调用 fn 前节流一次，
// 故重试循环只应在 attempt > 0 时调用本方法，否则首次请求会等两倍间隔。
func (g *Gate) Wait(topic string) {
	if g == nil {
		return
	}
	p, ok := g.table.Lookup(topic)
	if !ok {
		return
	}
	g.throttle(p)
}

func fetch[T any](g *Gate, topic, key string, noCache bool, fn func() (T, error)) (T, error) {
	var zero T
	if g == nil {
		return fn()
	}
	p, ok := g.table.Lookup(topic)
	if !ok {
		return fn() // 未登记 = 零策略，直通（设计 §4.1）
	}
	ttl := p.TTL
	if noCache {
		ttl = 0
	}
	// 缓存键与合并键都必须含 topic：Fetch 是泛型的，yahoo.chart 与 yahoo.eps
	// 对同一 symbol 天然同 key，不含 topic 会跨主题串味。
	ck := topic + "|" + key

	if v, hit := g.load(ck, ttl); hit {
		if tv, ok := v.(T); ok {
			return tv, nil
		}
		// 同 key 被不同 T 复用是编程错误；当作未命中重新取，不 panic。
	}

	call := func() (any, error) {
		// 合并窗口内可能已有人写好缓存，二次检查省掉一次请求。
		if v, hit := g.load(ck, ttl); hit {
			if tv, ok := v.(T); ok {
				return tv, nil
			}
		}
		if err := g.takeQuota(topic, p); err != nil {
			return zero, err
		}
		g.throttle(p)
		v, err := runWithTimeout(p.Timeout, fn)
		if err != nil {
			return zero, err // 失败不写缓存、不延长 TTL
		}
		if ttl > 0 {
			g.store(ck, v)
		}
		return v, nil
	}

	var (
		v   any
		err error
	)
	if p.Coalesce {
		v, err, _ = g.sf.Do(ck, call)
	} else {
		v, err = call()
	}
	if err != nil {
		return zero, err
	}
	tv, _ := v.(T)
	return tv, nil
}

// throttle 阻塞到该限流域距上次请求满 MinInterval 为止。
func (g *Gate) throttle(p Policy) {
	if p.MinInterval <= 0 {
		return
	}
	d := g.domainState(p.Domain)
	d.mu.Lock()
	defer d.mu.Unlock()
	if wait := p.MinInterval - time.Since(d.lastReq); wait > 0 {
		time.Sleep(wait)
	}
	d.lastReq = time.Now()
}

func (g *Gate) domainState(domain string) *domainState {
	g.domainsMu.Lock()
	defer g.domainsMu.Unlock()
	d, ok := g.domains[domain]
	if !ok {
		d = &domainState{}
		g.domains[domain] = d
	}
	return d
}

func (g *Gate) load(ck string, ttl time.Duration) (any, bool) {
	if ttl <= 0 {
		return nil, false
	}
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	e, ok := g.entries[ck]
	if !ok || time.Since(e.storedAt) >= ttl {
		return nil, false
	}
	return e.val, true
}

func (g *Gate) store(ck string, v any) {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	if _, exists := g.entries[ck]; !exists && len(g.entries) >= maxCacheEntries {
		g.evictOldest()
	}
	g.seq++
	g.entries[ck] = cacheEntry{val: v, storedAt: time.Now(), seq: g.seq}
}

// evictOldest 删除 seq 最小的条目。调用方须持 cacheMu。
// 每个条目 seq 均非零，故 oldestSeq==0 表示表为空。
func (g *Gate) evictOldest() {
	var oldestKey string
	var oldestSeq uint64
	for k, e := range g.entries {
		if oldestSeq == 0 || e.seq < oldestSeq {
			oldestKey, oldestSeq = k, e.seq
		}
	}
	if oldestSeq != 0 {
		delete(g.entries, oldestKey)
	}
}

// entryCount 是测试辅助。
func (g *Gate) entryCount() int {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	return len(g.entries)
}

// runWithTimeout 让 fn 在超时时间内完成，超时返回 ErrTimeout。
// d <= 0 表示不限时（内置表所有主题的 Timeout 均为 0）。
//
// 超时后 fn 的 goroutine 继续跑完（HTTP client 自带 Timeout 兜底），
// 结果写入**带缓冲**的 channel 后被丢弃，不泄漏 goroutine——channel 若无缓冲，
// 超时后没人再读，那个 goroutine 会永久阻塞在发送处。
func runWithTimeout[T any](d time.Duration, fn func() (T, error)) (T, error) {
	if d <= 0 {
		return fn()
	}
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-timer.C:
		var zero T
		return zero, ErrTimeout
	}
}

// takeQuota 做配额预判。账本异常时 fail-open：放行并告警，绝不因账本损坏
// 阻断降级链（设计 §4.4 / 约束 C7）。
//
// 调用点位于执行链 ③：**查缓存之后、singleflight 内侧、节流之前**。这个位置
// 是契约不是巧合——挪到查缓存之前会让缓存命中吃掉配额，挪到 singleflight 外侧
// 会让 N 个合并请求各消耗一次，两者都由 boundary[2] 的测试钉住。
func (g *Gate) takeQuota(topic string, p Policy) error {
	if p.Quota == nil || g.quota == nil {
		return nil
	}
	ok, err := g.quota.Take(topic, *p.Quota, time.Now())
	if err != nil {
		g.warn("collector quota ledger unavailable, failing open: topic="+topic, err)
		return nil
	}
	if !ok {
		return ErrQuotaExceeded
	}
	return nil
}
