# Router percentile_step 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 router 实现百分位步进提醒门控（`|当前分位−上次通知分位| ≥ step` 才重新放行），步长**策略级可配**（经 Signal.Metadata 传递、全局值为回退默认），并顺带修复 `cfg.Router` 死配置预存 bug。

**Architecture:** Router 内置门控：confidence/action 过滤为通用前置，分位信号（带 percentile 元数据且有效步长>0）走步进判定（单临界区、不查不更新冷却戳），其余信号走原冷却路径零变化。步长来源：`strategies.{name}.params.percentile_step` → 策略写入 Metadata → router `effectiveStep` 优先采用，回退全局 `router.percentile_step`（经 `app.New()` 从硬编码改为 cfg 映射接线）。

**Tech Stack:** Go 1.21、标准库 testing（表驱动，沿用 router_test.go 现有风格）。

**设计依据（必读）：** `docs/plans/2026-06-12-percentile-step-design.md`（rev4，含策略级步长）

**执行纪律：** 严格 TDD；全部 Task 完成后、最终提交前运行 code-simplifier sub-agent（全局规范）。

---

## Chunk 1: 全部任务

### Task 1: router 步进门控核心

**Files:**
- Modify: `internal/router/router.go`（Config :15-19、Router 结构 :31-38、New :46-56、Route :62-109、passesFilters :151-181）
- Test: `internal/router/router_test.go`

- [ ] **Step 1: 写失败测试**

参照 router_test.go 现有构造方式（`New(cfg, nil, nil)`，nil registry 合法，Route 返回 routed bool）。

```go
func pctSignal(symbol, strat string, action core.Action, pct float64) core.Signal {
	return core.Signal{
		Symbol: symbol, Action: action, Confidence: 0.9, Strategy: strat,
		Metadata: map[string]any{"percentile": pct},
	}
}

func newStepRouter(step float64) *Router {
	cfg := DefaultConfig()
	cfg.PercentileStep = step
	cfg.CooldownDuration = 1 * time.Hour // 显式非零：验证分位路径确实绕过冷却
	return New(cfg, nil, nil) // New 内部对 nil logger 兜底，无需引入 zap import
}

func TestRoute_PercentileStep_BuySide(t *testing.T) {
	r := newStepRouter(5)
	cases := []struct {
		pct  float64
		want bool
	}{
		{49, true},  // 首次：放行并记录 49
		{47, false}, // |47-49|=2 < 5：抑制
		{44, true},  // |44-49|=5 ≥ 5：放行，记录 44
		{46, false}, // |46-44|=2 < 5：抑制
		{49, true},  // 恢复重算：|49-44|=5 ≥ 5：放行（防死锁规则）
	}
	for i, c := range cases {
		routed, err := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, c.pct))
		if err != nil || routed != c.want {
			t.Fatalf("step %d (pct=%v): routed=%v err=%v, want %v", i, c.pct, routed, err, c.want)
		}
	}
}

func TestRoute_PercentileStep_SellSideSymmetric(t *testing.T) {
	r := newStepRouter(5)
	for i, c := range []struct {
		pct  float64
		want bool
	}{{81, true}, {83, false}, {86, true}} {
		routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionSell, c.pct))
		if routed != c.want {
			t.Fatalf("sell step %d (pct=%v): routed=%v, want %v", i, c.pct, routed, c.want)
		}
	}
}

func TestRoute_PercentileStep_KeysIndependent(t *testing.T) {
	r := newStepRouter(5)
	// buy 侧已记录 49
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	// sell 侧独立：首个 sell 信号放行（不受 buy 侧记录影响）
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionSell, 81)); !routed {
		t.Error("sell side must be independent of buy side")
	}
	// 不同策略独立：pe_percentile 首个信号放行（注意元数据键不同）
	sig := pctSignal("600519.SH", "pe_percentile", core.ActionBuy, 50)
	sig.Metadata = map[string]any{"pe_percentile": 50.0}
	if routed, _ := r.Route(sig); !routed {
		t.Error("different strategy must have independent gate key")
	}
	// strong_buy 与 buy 同侧共享 key：strong_buy 47 应被 buy 侧的 49 记录抑制
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionStrongBuy, 47)); routed {
		t.Error("strong_buy shares the buy-side key, |47-49|<5 must suppress")
	}
}

func TestRoute_PercentileStep_BadMetadataFallsBackToCooldown(t *testing.T) {
	r := newStepRouter(5)
	sig := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 0)
	sig.Metadata = map[string]any{"percentile": "not-a-float"}
	if routed, _ := r.Route(sig); !routed {
		t.Fatal("first signal via cooldown path should route")
	}
	// 第二条同标的（仍坏元数据）→ 冷却抑制（1h 内）
	if routed, _ := r.Route(sig); routed {
		t.Error("second signal within cooldown must be suppressed (fell back to cooldown path)")
	}
}

func TestRoute_PercentileStep_PerStrategyOverride(t *testing.T) {
	r := newStepRouter(5) // 全局默认 5
	// 信号自带 percentile_step: 3（策略级配置经元数据传递）→ 按 3 门控
	mk := func(pct float64) core.Signal {
		sig := pctSignal("600519.SH", "pe_percentile", core.ActionBuy, pct)
		sig.Metadata = map[string]any{"pe_percentile": pct, "percentile_step": 3.0}
		return sig
	}
	for i, c := range []struct {
		pct  float64
		want bool
	}{{49, true}, {47, false}, {46, true}} { // |46-49|=3 ≥ 3 放行（全局 5 则会抑制）
		if routed, _ := r.Route(mk(c.pct)); routed != c.want {
			t.Fatalf("override step %d (pct=%v): routed=%v, want %v", i, c.pct, routed, c.want)
		}
	}
	// 全局 step=0 + 信号自带 step=3 → 仍按 3 门控（按策略启用场景，设计 rev4 §4）
	r0 := newStepRouter(0)
	if routed, _ := r0.Route(mk(49)); !routed {
		t.Error("strategy-level step must enable the gate even when global step is 0")
	}
	if routed, _ := r0.Route(mk(48)); routed { // |48-49|=1 < 3：被步进抑制而非进入冷却路径
		t.Error("gate must be active with strategy-level step despite global 0")
	}

	// step 元数据类型异常（string）→ 回退全局 5
	bad := pctSignal("0700.HK", "price_percentile", core.ActionBuy, 49)
	bad.Metadata["percentile_step"] = "3"
	if routed, _ := r.Route(bad); !routed {
		t.Fatal("first should route")
	}
	bad2 := pctSignal("0700.HK", "price_percentile", core.ActionBuy, 45) // |45-49|=4 < 5
	bad2.Metadata["percentile_step"] = "3"
	if routed, _ := r.Route(bad2); routed {
		t.Error("invalid step metadata must fall back to global step 5")
	}
}

func TestRoute_StepDisabled_UsesCooldown(t *testing.T) {
	r := newStepRouter(0) // step=0 禁用：带分位元数据也走冷却
	sig := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49)
	if routed, _ := r.Route(sig); !routed {
		t.Fatal("first should route")
	}
	sig2 := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 30) // 深跌也被冷却抑制
	if routed, _ := r.Route(sig2); routed {
		t.Error("step disabled: cooldown must suppress regardless of percentile delta")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/router/ -run TestRoute_Percentile -v`
Expected: FAIL（`PercentileStep` 字段不存在）

- [ ] **Step 3: 最小实现**

router.go 改动：

```go
// Config 增加字段
	PercentileStep float64 `mapstructure:"percentile_step"` // 0 = disabled

// Router 增加状态（与 cooldowns 共用 r.mu）
	pctGates map[string]float64 // symbol|strategy|side -> last notified percentile
// New() 初始化 pctGates: make(map[string]float64)

// percentileOf extracts the historical percentile from signal metadata.
// Safe to assert float64: signals travel in-memory only (strategy → app →
// router); signalStore is write-only. Revisit if a replay-from-store path
// is ever added.
func percentileOf(sig core.Signal) (float64, bool) {
	for _, key := range []string{"percentile", "pe_percentile"} {
		if v, ok := sig.Metadata[key]; ok {
			if f, ok := v.(float64); ok {
				return f, true
			}
		}
	}
	return 0, false
}

// effectiveStep returns the gate step for a signal: the strategy-carried
// Metadata["percentile_step"] wins when present and positive; otherwise the
// global router config value. <= 0 means the gate is disabled for this signal.
func (r *Router) effectiveStep(sig core.Signal) float64 {
	if v, ok := sig.Metadata["percentile_step"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			return f
		}
	}
	return r.cfg.PercentileStep
}

func sideOf(action core.Action) string {
	if action == core.ActionBuy || action == core.ActionStrongBuy {
		return "buy"
	}
	return "sell"
}

// passPercentileGate reports whether the signal clears the step gate and
// records its percentile when it does. Check and update happen in one
// critical section (no check-then-act race).
func (r *Router) passPercentileGate(sig core.Signal, pct, step float64) bool {
	key := sig.Symbol + "|" + sig.Strategy + "|" + sideOf(sig.Action)
	r.mu.Lock()
	defer r.mu.Unlock()
	last, exists := r.pctGates[key]
	if exists && math.Abs(pct-last) < step {
		return false
	}
	r.pctGates[key] = pct
	return true
}
```

`Route()` 重构（passesFilters 拆分）：

```go
func (r *Router) Route(signal core.Signal) (routed bool, err error) {
	if !r.passesStaticFilters(signal) { // confidence + action（原 passesFilters 前两段）
		... // 原 debug 日志 + return false, nil
	}

	if pct, ok := percentileOf(signal); ok && r.effectiveStep(signal) > 0 {
		// 分位信号：步进门控完全替代冷却（不查、不更新冷却戳，设计 §1/§4）
		// 步长优先用信号自带（策略级配置），回退全局默认（设计 rev4）
		if !r.passPercentileGate(signal, pct, r.effectiveStep(signal)) {
			return false, nil
		}
	} else {
		if !r.passesCooldown(signal) { // 原 passesFilters 第三段
			return false, nil
		}
		// 把现有的 r.cooldowns 更新（原 router.go:81-83）移入本 else 分支内：
		// 冷却戳只在冷却路径盖，分位分支绝不碰——否则 Task 2 的
		// TestRoute_PercentileSignalDoesNotTouchCooldown 必挂
	}
	// 后续：signalStore 持久化、通知（原代码不动）
}
```

（实现要点：`passesFilters` 拆成 `passesStaticFilters` + `passesCooldown` 两个私有方法；`RouteBatch` 的 `passesFilters` 调用同步改为「static + 分流判定」——Task 2 处理。冷却戳更新保持只在冷却路径发生。import `math`。）

- [ ] **Step 4: 运行确认通过（含既有用例不回归）**

Run: `go test ./internal/router/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/router/
git commit -m "feat(router): percentile step gate replacing cooldown for percentile signals"
```

### Task 2: 冷却交互、RouteBatch 与状态管理

**Files:**
- Modify: `internal/router/router.go`（RouteBatch :112-148、ClearCooldown :184-188、ClearAllCooldowns :191-195、GetStats :237-247）
- Test: `internal/router/router_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestRoute_PercentileSignalDoesNotTouchCooldown(t *testing.T) {
	r := newStepRouter(5)
	// 分位信号通知后……
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	// ……同标的无元数据信号（如 ma_crossover）不应被冷却压制（冷却戳未被分位路径更新）
	plain := core.Signal{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.9, Strategy: "ma_crossover"}
	if routed, _ := r.Route(plain); !routed {
		t.Error("percentile signal must not stamp the per-symbol cooldown")
	}
}

func TestClearCooldowns_AlsoClearPercentileGates(t *testing.T) {
	r := newStepRouter(5)
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 40))

	r.ClearCooldown("600519.SH") // 按 symbol| 前缀清除步进 key
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 48)); !routed {
		t.Error("after ClearCooldown the first percentile signal must route again")
	}
	if routed, _ := r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 39)); routed {
		t.Error("other symbols' gates must survive ClearCooldown(600519.SH)")
	}

	r.ClearAllCooldowns()
	if routed, _ := r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 38)); !routed {
		t.Error("after ClearAllCooldowns all gates must reset")
	}
}

func TestRouteBatch_UsesPercentileGate(t *testing.T) {
	r := newStepRouter(5)
	// 批内同 key 两条：第一条放行并更新状态，第二条按更新后状态判定（与连续 Route 等价）
	err := r.RouteBatch([]core.Signal{
		pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49),
		pctSignal("600519.SH", "price_percentile", core.ActionBuy, 47), // |47-49|<5 → 不入批
	})
	if err != nil {
		t.Fatal(err)
	}
	// 间接断言：再 Route 44 应放行（状态为 49 而非 47）
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 44)); !routed {
		t.Error("batch must have recorded 49 (not 47); |44-49|=5 should route")
	}
}

func TestGetStats_IncludesPercentileGate(t *testing.T) {
	r := newStepRouter(5)
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	stats := r.GetStats()
	if stats["percentile_gates_active"] != 1 || stats["percentile_step"] != 5.0 {
		t.Errorf("stats = %+v", stats)
	}
}
```

- [ ] **Step 2: 运行确认失败 → Step 3: 实现**

- `RouteBatch` 的过滤改为与 Route 相同的分流（static 过滤 → 分位信号走 `passPercentileGate` / 其余走 `passesCooldown` + 更新冷却戳），逐条顺序判定；**并补 nil-registry 守卫**（`Route` 在 :86-88 有守卫而 `RouteBatch` 没有——`filtered` 非空时直接调 `r.registry.NotifyAllBatch` 会对 nil registry panic，本任务测试用 nil registry 构造，没有守卫 GREEN 阶段必炸）；其余批处理行为（不写 signalStore 等）不动
- `ClearCooldown(symbol)`：加锁后 `delete(r.cooldowns, symbol)` + 遍历 `pctGates` 删除 `strings.HasPrefix(key, symbol+"|")` 的条目（假设 symbol 不含 `|`，注释注明）
- `ClearAllCooldowns`：同时重建两个 map
- `GetStats`：增加 `percentile_gates_active`（len(pctGates)）与 `percentile_step`

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/router/ -v` → 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/router/
git commit -m "feat(router): percentile gate for RouteBatch, clear ops and stats"
```

### Task 3: 策略级步长参数（设计 rev4）

**Files:**
- Modify: `internal/strategy/price_percentile/strategy.go`（Init 参数读取、Analyze 的 Metadata 构造）
- Modify: `internal/strategy/pe_percentile/strategy.go`（同上）
- Test: 各自 strategy_test.go

- [ ] **Step 1: 写失败测试（两个策略包各一）**

```go
// price_percentile/strategy_test.go
func TestInit_PercentileStepParam(t *testing.T) {
	s := New()
	// 此处测 int 形态；float64 形态由既有 numParam 用例覆盖（双形态 helper 已有测试）
	if err := s.Init(strategy.Config{Params: map[string]any{"percentile_step": 3}}); err != nil {
		t.Fatal(err)
	}
	sigs := analyzeWithExtremeLowPercentile(t, s) // 复用既有造数 helper 触发一条信号
	if len(sigs) == 0 {
		t.Fatal("expected a signal")
	}
	if sigs[0].Metadata["percentile_step"] != 3.0 {
		t.Errorf("metadata percentile_step = %v, want 3.0", sigs[0].Metadata["percentile_step"])
	}
}

func TestAnalyze_NoStepParam_NoStepMetadata(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	sigs := analyzeWithExtremeLowPercentile(t, s)
	if _, ok := sigs[0].Metadata["percentile_step"]; ok {
		t.Error("percentile_step must be absent when not configured (router falls back to global)")
	}
}
```

（`analyzeWithExtremeLowPercentile`：按各自包既有测试的造数方式触发一条 buy 信号的小 helper；pe_percentile 包用 `peCtx(5, ...)` 即可。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/strategy/price_percentile/ ./internal/strategy/pe_percentile/ -run 'PercentileStep|NoStepParam' -v`
Expected: `TestInit_PercentileStepParam` FAIL（Metadata 无 percentile_step 键）；`TestAnalyze_NoStepParam_NoStepMetadata` 是守卫测试，RED 阶段本就 PASS——只有一半红是预期现象

- [ ] **Step 3: 实现**

两个策略各自：结构体加 `percentileStep float64` 字段；`Init` 用既有 `numParam` 双形态 helper 读取 `percentile_step`（≤0 视为未配置）；`Analyze` 构造 Metadata 时 `if s.percentileStep > 0 { md["percentile_step"] = s.percentileStep }`。**保持两包独立实现，不抽公共基类**（与既有结构一致）。

- [ ] **Step 4: 运行确认通过 + 提交**

Run: `go test ./internal/strategy/... -v` → PASS

```bash
git add internal/strategy/price_percentile/ internal/strategy/pe_percentile/
git commit -m "feat(strategy): per-strategy percentile_step carried via signal metadata"
```

### Task 4: 配置接线（修复 cfg.Router 死配置预存 bug）

**Files:**
- Modify: `internal/config/config.go`（RouterConfig :111-114、默认值 :286-289、校验 :337-345；新增 `PercentileStep < 0` 校验沿用 `core.WrapError(core.ErrConfigInvalid, ...)` 风格）
- Modify: `internal/app/app.go`（New 的 routerCfg 构造 :91-96）
- Test: `internal/app/app_test.go`

- [ ] **Step 1: 写失败测试（app_test.go，设计 §6 第 9 条）**

```go
func TestNew_RouterConfigFromCfg(t *testing.T) {
	cfg := config.Defaults() // 实际函数名已核实（config.go:269），返回 *Config
	cfg.Router.CooldownHours = 24
	cfg.Router.MinConfidence = 0.7
	cfg.Router.PercentileStep = 5

	a := New(cfg, nil)
	stats := a.router.GetStats() // router 为私有字段时在包内测试可直接访问；
	                             // 若不可达则通过 GetRouter()/暴露 stats 的现有路径取
	if stats["cooldown_seconds"] != float64(24*3600) {
		t.Errorf("cooldown not wired: %v", stats["cooldown_seconds"])
	}
	if stats["min_confidence"] != 0.7 {
		t.Errorf("min_confidence not wired: %v", stats["min_confidence"])
	}
	if stats["percentile_step"] != 5.0 {
		t.Errorf("percentile_step not wired: %v", stats["percentile_step"])
	}
}
```

（app_test.go 与 app.go 同包，可访问私有字段。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run TestNew_RouterConfig -v`
Expected: **编译错误**（`cfg.Router.PercentileStep` 字段尚不存在）。RED 阶段分两步看：先在 config.go 加 `PercentileStep` 字段（仅字段，不接线）使测试可编译，再跑——此时断言失败、`cooldown_seconds` 恒为 3600，即死配置 bug 的实证；随后做 app.New() 接线转 GREEN。

- [ ] **Step 3: 实现**

config.go：`RouterConfig` 增加 `PercentileStep float64 \`mapstructure:"percentile_step"\``；校验追加 `PercentileStep < 0` 拒绝；默认值块不加该字段（零值 0 = 禁用）。

app.go `New()`：

```go
	routerCfg := router.Config{
		MinConfidence:    cfg.Router.MinConfidence,
		CooldownDuration: time.Duration(cfg.Router.CooldownHours) * time.Hour, // 0 = 禁用冷却（恒放行）
		PercentileStep:   cfg.Router.PercentileStep,
		// EnabledActions 维持现有硬编码（config 无对应字段，设计明确 YAGNI）
		EnabledActions: []core.Action{core.ActionBuy, core.ActionSell, core.ActionStrongBuy, core.ActionStrongSell},
	}
```

注意 `passesCooldown` 对 `CooldownDuration == 0` 的行为：`time.Since(last) < 0` 恒 false → 恒放行，天然满足「0 = 禁用」，无需特判（在 router.go 注释注明）。

⚠️ 存量行为变更（提交信息中注明）：未显式配置的部署冷却 1h→4h、置信阈值 0.5→0.6（config 默认值开始真正生效）。

- [ ] **Step 4: 运行确认通过 + 全量回归**

Run: `go test ./internal/app/ ./internal/router/ ./internal/config/ -v | tail -5` → PASS；`go test ./...` 无回归

- [ ] **Step 5: 提交**

```bash
git add internal/config/ internal/app/
git commit -m "fix(app): wire cfg.Router into router (cooldown/min_confidence/percentile_step)

BREAKING-ish: deployments without explicit router config now get the
documented defaults (cooldown 4h, min_confidence 0.6) instead of the
hardcoded 1h/0.5 that ignored configuration."
```

### Task 5: 配置文件与收尾

**Files:**
- Modify: `configs/percentile-watchlist.yaml`（取消 percentile_step 注释）
- Modify: `configs/config.example.yaml`（router 节补参数）

- [ ] **Step 1: 配置更新**

percentile-watchlist.yaml 四处同步更新：① `# percentile_step: 5` 取消注释（全局默认，行尾注释「策略未配 percentile_step 时的回退值」）；② 两个分位策略的 params 各加 `percentile_step: 5`（行尾注释「可按策略独立调整，如 PE 分位改 3」）；③ 文件头部第 9-10 行「功能就位前的过渡行为」段落删除（功能已就位）；④ `cooldown_hours` 定为 **4**，行尾注释改为「仅约束不带分位元数据的策略（如 ma_crossover）」——24h 是 percentile_step 就位前防刷屏的过渡值，功能就位后回归默认 4h（与设计 rev4 §7 一致）。

config.example.yaml 的 router 节：

```yaml
router:
  min_confidence: 0.6
  cooldown_hours: 4      # 0 = 禁用冷却；仅约束不带分位元数据的策略信号
  percentile_step: 5     # 全局默认步长；策略 params.percentile_step 优先（0 = 禁用）
```

strategies 节的两个分位策略 params 同步补 `percentile_step` 示例行（注释「按策略覆盖全局步长」）。

- [ ] **Step 2: 运行 code-simplifier**（全局规范）

Task tool: `subagent_type: "code-simplifier:code-simplifier"`，prompt 列出全部改动文件：router.go、router_test.go、config.go、app.go、app_test.go、price_percentile/strategy.go(+test)、pe_percentile/strategy.go(+test)。

- [ ] **Step 3: 全量回归 + gitnexus**

```bash
go vet ./... && go test ./... && npx gitnexus analyze
```

- [ ] **Step 4: 最终提交**

```bash
git add -A
git commit -m "feat: percentile step re-alert config rollout

Implements docs/plans/2026-06-12-percentile-step-design.md (rev4)"
```

---

## 验收对照（design §3/§6）

- [ ] 买入序列 49→47→44→46→49 的放行/抑制与设计示例一致（含恢复重算）
- [ ] 卖出侧对称；buy/sell 与双策略 key 独立；strong 档共享同侧 key
- [ ] 分位信号不更新冷却戳（同标的 ma_crossover 不受压制）
- [ ] step=0 / 坏元数据 → 冷却路径回归，原有用例零回归
- [ ] 死配置 bug 修复有实证测试（两段式 RED：先编译错误，加字段后断言失败 cooldown_seconds=3600 即 bug 实证）
- [ ] ClearCooldown/ClearAllCooldowns 同步清理步进状态
- [ ] 策略级步长（rev4）：信号自带 step 覆盖全局、类型异常回退全局、未配置不写元数据三态均有测试；两策略可配不同步长
