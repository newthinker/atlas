package policy

import (
	"sync"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[0]     Default() 懒初始化返回内置表 Gate / 重复调用同一实例 / SetDefault 生效
//                       → TestDefaultIsLazyAndUsesBuiltinTable / TestSetDefaultReplaces
// error_handling[0] SetDefault(nil) 不 panic
//                       → TestSetDefaultNilDoesNotPanic（「不该 panic」型，带 defer recover）
// error_handling[0] 并发调用 Default() 不 panic、初始化并发安全
//                       → TestDefaultIsConcurrencySafe（-race 下检出竞争；同时断言单例性）
//
// ⚠ 本文件的测试都会动进程内全局单例，必须用 t.Cleanup 还原，否则会污染同一次
//   运行里的其他测试。Go 的包内测试默认串行，故不加 t.Parallel。

func TestDefaultIsLazyAndUsesBuiltinTable(t *testing.T) {
	g := Default()
	if g == nil {
		t.Fatal("Default() 不得返回 nil —— 未接线的调用点也要能拿到内置策略")
	}
	if p, ok := g.table.Lookup("yahoo.chart"); !ok || p.MinInterval != 500*time.Millisecond {
		t.Errorf("Default 应使用内置表, got (%+v, %v)", p, ok)
	}
	if g2 := Default(); g2 != g {
		t.Error("Default() 必须返回同一实例")
	}
}

func TestSetDefaultReplaces(t *testing.T) {
	orig := Default()
	t.Cleanup(func() { SetDefault(orig) })

	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("x.y", Policy{MinInterval: time.Second})
	custom := New(tbl, nil)
	SetDefault(custom)

	if Default() != custom {
		t.Error("SetDefault 未生效")
	}
}

// TestSetDefaultNilDoesNotPanic 覆盖 error_handling[0] 的前半句。
//
// 「不该 panic」型断言必须 recover：裸 panic 会中断整个测试二进制。
// 判据不只是「没崩」——SetDefault(nil) 之后 Default() 必须重新懒构造出一个**可用**
// 的 Gate，否则调用方会拿到 nil 而在别处崩。
func TestSetDefaultNilDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetDefault(nil) 后不得 panic: %v", r)
		}
	}()

	orig := Default()
	t.Cleanup(func() { SetDefault(orig) })

	SetDefault(nil)

	g := Default()
	if g == nil {
		t.Fatal("Default() 永不返回 nil：SetDefault(nil) 后应重新懒构造")
	}
	if _, ok := g.table.Lookup("yahoo.chart"); !ok {
		t.Error("重新懒构造的 Gate 应使用内置表")
	}
}

// TestDefaultIsConcurrencySafe 覆盖 error_handling[0] 的后半句：并发 Default()
// 安全。先 SetDefault(nil) 把单例清空，让 N 个 goroutine 去竞争懒初始化。
//
// 两个判据缺一不可：-race 检出数据竞争（读写 defaultGate 无保护时报），以及
// **所有 goroutine 拿到同一实例**（有锁但写成「每次都新建」时前者不报、后者会红）。
func TestDefaultIsConcurrencySafe(t *testing.T) {
	orig := Default()
	t.Cleanup(func() { SetDefault(orig) })
	SetDefault(nil)

	const n = 50
	got := make([]*Gate, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) { defer wg.Done(); got[i] = Default() }(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if got[i] == nil {
			t.Fatalf("goroutine %d 拿到 nil", i)
		}
		if got[i] != got[0] {
			t.Errorf("goroutine %d 拿到了不同实例：懒初始化没有保证单例", i)
		}
	}
}
