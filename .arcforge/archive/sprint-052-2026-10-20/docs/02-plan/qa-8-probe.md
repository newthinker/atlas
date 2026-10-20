# QA-8 探针源码（test-pw-a，验 TASK-004 @ 80a56f38888e6770d01db2ff8824e164d8d82039；复用 sync_test.go 的 fakeProvider/series/newSyncer；原实现两个都过、S29/S30 变异分别使其红）

```go
package pricewatch

import (
	"testing"
	"time"
)

// A（okx 语义）只有最近 400 天；B 有 3000 天但对老段全部报错（瞬时故障）。
// 正确行为：老段「A 成功 0 根 + B 报错」不能当成「未上线」，不得记上线日。
func TestProbeListingNotRecordedOnError(t *testing.T) {
	aFrom := today.AddDate(0, 0, -400)
	a := &fakeProvider{name: "A", sem: okxSem, data: series(aFrom, syncNow, dayDur, 1)}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(today.AddDate(0, 0, -3000), syncNow, dayDur, 2),
		errIf: func(s, e time.Time) bool { return s.Before(aFrom.AddDate(0, 0, -300)) }}
	sy := newSyncer(t, a, b)
	_ = sy.Sync(t.Context(), "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV("listing:BTCUSDT:1d"); v != "" {
		t.Fatalf("listing recorded despite errors: %q", v)
	}
}

// 老段所有源都报错，之后的段有数据：报错段不能被当成「未上线」。
func TestProbeListingNotRecordedAllError(t *testing.T) {
	cut := today.AddDate(0, 0, -400)
	bad := func(s, e time.Time) bool { return s.Before(cut) }
	a := &fakeProvider{name: "A", sem: okxSem, data: series(cut, syncNow, dayDur, 1), errIf: bad}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(cut, syncNow, dayDur, 2), errIf: bad}
	sy := newSyncer(t, a, b)
	_ = sy.Sync(t.Context(), "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV("listing:BTCUSDT:1d"); v != "" {
		t.Fatalf("listing recorded despite all-source errors: %q", v)
	}
}
```
