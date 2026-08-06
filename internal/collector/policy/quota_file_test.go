package policy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[0]     跨进程配额累计（两个独立实例指向同一账本）
//                       → TestFileStoreQuotaSurvivesProcessRestart（设计 §7.3 / 验收标准 3）
// functional[1]     窗口翻篇归零 / 主题隔离
//                       → TestFileStoreResetsOnWindowRollover / TestFileStoreIsolatesTopics
// functional[2]     文件不存在从空账本起算 / 父目录自动创建
//                       → TestFileStoreMissingFileStartsEmpty / TestFileStoreCreatesParentDir
// boundary[0]       被拒的 Take 不写入计数
//                       → TestFileStoreRejectedTakeDoesNotIncrement
// boundary[1]       并发 Take 在 flock 下不超发
//                       → TestFileStoreConcurrentTakesRespectLimit
// error_handling[0] 分句1 账本损坏时 fail-open、不 panic 不阻断
//                       → TestFileStoreFailsOpenOnCorruptLedger（带 defer recover）
// error_handling[0] 分句2 **且必须能自愈** —— 下一次 Take 重建账本并恢复正常计数
//                       → TestFileStoreSelfHealsAfterCorruption
// non_functional[0] 原子写：Take 后目录中除目标账本（与锁文件）外无残留临时文件
//                       → TestFileStoreLeavesNoTempFiles
// non_functional[1] flock 的 fd 在 Take 返回前关闭（verify_by: review；此处做间接验证）
//                       → TestFileStoreDoesNotLeakFDs
//
// ⚠ error_handling[0] 是复合句，两个分句的**证据方向相反**：分句 1 证明「坏事发生时
//   系统仍走下去」，分句 2 证明「坏事发生后状态真的恢复」。只验分句 1 是不够的——
//   若每次读到坏 JSON 都只返回 err 而从不重写，配额将永久静默失效，而分句 1 的断言
//   全程为真。
//
// ⚠ 分句 1 属「不该 panic」型：必须用 defer recover 把 panic 转成断言失败，否则裸
//   panic 会中断整个测试二进制，同一次运行里排在后面的测试根本跑不到。

func quotaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "collector-quota.json")
}

// TestFileStoreQuotaSurvivesProcessRestart 是配额设计的立身之本（设计 §7.3、
// 验收标准 3）：两个独立 Gate 实例指向同一账本文件，模拟 launchd 的两次启动。
func TestFileStoreQuotaSurvivesProcessRestart(t *testing.T) {
	path := quotaPath(t)

	newGate := func() *Gate {
		tbl := &Table{policies: make(map[string]Policy)}
		tbl.Set("tushare.daily_basic", Policy{
			Quota: &Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC},
		})
		return New(tbl, NewFileStore(path))
	}

	// 第一次「启动」：用掉全部 5 次
	first := newGate()
	for i := 1; i <= 5; i++ {
		if _, err := Fetch(first, "tushare.daily_basic", "600519.SH", func() (int, error) { return i, nil }); err != nil {
			t.Fatalf("第一个实例第 %d 次: %v", i, err)
		}
	}

	// 第二次「启动」：全新 Gate、全新内存，首次 Take 就该被拒
	second := newGate()
	fnCalls := 0
	_, err := Fetch(second, "tushare.daily_basic", "600519.SH", func() (int, error) {
		fnCalls++
		return 0, nil
	})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("跨进程配额未生效: err = %v, want ErrQuotaExceeded", err)
	}
	if fnCalls != 0 {
		t.Errorf("超额请求不得发出: fn 调用 %d 次, want 0", fnCalls)
	}
}

func TestFileStoreResetsOnWindowRollover(t *testing.T) {
	path := quotaPath(t)
	loc := time.UTC
	q := Quota{Limit: 2, Window: 24 * time.Hour, Loc: loc}

	// 手工写一份「昨天已用满」的账本
	yesterday := time.Date(2026, 8, 5, 0, 0, 0, 0, loc)
	writeTestLedger(t, path, map[string]ledgerEntry{
		"t": {WindowStart: yesterday, Count: 2},
	})

	s := NewFileStore(path)
	today := time.Date(2026, 8, 6, 9, 0, 0, 0, loc)
	ok, err := s.Take("t", q, today)
	if err != nil || !ok {
		t.Fatalf("窗口翻篇后应放行: (%v, %v)", ok, err)
	}

	got := readTestLedger(t, path)
	if got["t"].Count != 1 {
		t.Errorf("翻篇后计数应归零再计一: Count = %d, want 1", got["t"].Count)
	}
	if !got["t"].WindowStart.Equal(time.Date(2026, 8, 6, 0, 0, 0, 0, loc)) {
		t.Errorf("window_start 未推进: %v", got["t"].WindowStart)
	}
}

// TestFileStoreFailsOpenOnCorruptLedger 覆盖 error_handling[0] 的**分句 1**：
// 账本损坏时放行并报错，不 panic 不阻断（约束 C7）。
func TestFileStoreFailsOpenOnCorruptLedger(t *testing.T) {
	// 「不该 panic」型断言必须 recover：裸 panic 会中断整个测试二进制。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("账本损坏不得 panic: %v", r)
		}
	}()

	path := quotaPath(t)
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}

	ok, err := s.Take("t", q, time.Now())
	if !ok {
		t.Error("账本损坏必须 fail-open 放行（设计 §4.4）")
	}
	if err == nil {
		t.Error("账本损坏必须同时报错以便告警")
	}
}

// TestFileStoreSelfHealsAfterCorruption 覆盖 error_handling[0] 的**分句 2**：
// 损坏后必须能自愈。
//
// 与分句 1 的证据方向相反 —— 分句 1 证明「坏事发生时系统仍走下去」，这里要证明
// 「坏事发生后状态真的恢复」。若实现每次读到坏 JSON 都只返回 err 而从不重写，
// 分句 1 的断言全程为真，但配额将**永久静默失效**（反审 A10），只在日志留一行 Warn。
//
// 所以判据不是「不再报错」，而是**配额功能确实恢复**：账本重建为合法 JSON、
// 计数从新窗口起算、并且能在到达 Limit 时正确拒绝。
func TestFileStoreSelfHealsAfterCorruption(t *testing.T) {
	path := quotaPath(t)
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	q := Quota{Limit: 2, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	// 第 1 次：fail-open 放行 + 报错，同时应把账本重建掉
	if ok, err := s.Take("t", q, now); !ok || err == nil {
		t.Fatalf("首次遇到坏账本应 (放行, 报错): (%v, %v)", ok, err)
	}

	// 账本必须已是合法 JSON（readTestLedger 在非法 JSON 时会 Fatalf）
	got := readTestLedger(t, path)
	if got["t"].Count != 1 {
		t.Errorf("损坏后应重建账本并计一次: Count = %d, want 1", got["t"].Count)
	}

	// 第 2 次：不再报错，正常计数
	if ok, err := s.Take("t", q, now); !ok || err != nil {
		t.Errorf("自愈后应恢复正常: (%v, %v), want (true, nil)", ok, err)
	}

	// 第 3 次：已达 Limit=2，必须被拒 —— 这才证明配额功能真的恢复了，
	// 而不只是「不再报错」。
	if ok, err := s.Take("t", q, now); ok || err != nil {
		t.Errorf("自愈后配额应正常生效: (%v, %v), want (false, nil)", ok, err)
	}
}

// ——— I/O 层 fail-open（约束 C7）———
//
// TestFileStoreFailsOpenOnCorruptLedger 只覆盖了 **JSON 解析失败** 这一条路径。
// 下面三条覆盖 I/O 故障：目录建不了、账本写不进去、账本读不出来。
//
// 这三类在生产里**比 JSON 损坏常见得多**（磁盘满、权限变更、只读挂载）。任一处
// 返回 (false, err)，prism refresh 就会因为**账本写不进去**而拒绝发请求——配额机制
// 本是为保护降级链，反而成了阻断它的原因。
//
// 我上一轮在 discovery 里写「这些分支无法在测试中可靠构造」，是错的：下面每一种
// 都只用 t.TempDir() + 文件权限即可，不需要 root，macOS/Linux 通用。

// TestFileStoreFailsOpenOnDirError 构造 lock() 里 MkdirAll 失败：把父路径做成
// **文件**而不是目录。
func TestFileStoreFailsOpenOnDirError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 父路径是个文件 → MkdirAll 必失败
	s := NewFileStore(filepath.Join(blocker, "sub", "collector-quota.json"))
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}

	ok, err := s.Take("t", q, time.Now())
	if !ok || err == nil {
		t.Errorf("目录建不了时必须 fail-open: (%v, %v), want (true, 非nil)", ok, err)
	}
}

// TestFileStoreFailsOpenOnWriteError 构造 write() 里 CreateTemp 失败：预热之后把
// 目录改成不可写。预热是必要的——lock 文件与账本要先存在，否则失败会发生在更早的
// lock() 而非 write()。
func TestFileStoreFailsOpenOnWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector-quota.json")
	s := NewFileStore(path)
	q := Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	if ok, err := s.Take("t", q, now); err != nil || !ok {
		t.Fatalf("预热 Take: (%v, %v)", ok, err)
	}

	if err := os.Chmod(dir, 0o500); err != nil { // r-x：可读可进入，不可写
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // 还原，否则 TempDir 清理失败

	// 先确认权限确实生效（root 会无视权限位）
	if probe, err := os.CreateTemp(dir, "probe-*"); err == nil {
		probe.Close()
		os.Remove(probe.Name())
		t.Skip("当前用户无视目录权限位（root?），无法构造 write 失败")
	}

	ok, err := s.Take("t", q, now)
	if !ok || err == nil {
		t.Errorf("账本写不进去时必须 fail-open: (%v, %v), want (true, 非nil)", ok, err)
	}
}

// TestFileStoreFailsOpenOnReadError 构造 read() 里 ReadFile 失败（**非** JSON 解析
// 失败）：账本文件存在但不可读。
func TestFileStoreFailsOpenOnReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector-quota.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	// 先确认权限确实生效（root 会无视权限位）
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("当前用户可读 0o000 文件（root?），无法构造 read I/O 错误")
	}

	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}

	ok, err := s.Take("t", q, time.Now())
	if !ok || err == nil {
		t.Errorf("账本读不出来时必须 fail-open: (%v, %v), want (true, 非nil)", ok, err)
	}
}

func TestFileStoreMissingFileStartsEmpty(t *testing.T) {
	s := NewFileStore(quotaPath(t)) // 文件不存在
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	ok, err := s.Take("t", q, time.Now())
	if err != nil || !ok {
		t.Fatalf("账本首次创建应放行: (%v, %v)", ok, err)
	}
}

func TestFileStoreCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "collector-quota.json")
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	if ok, err := s.Take("t", q, time.Now()); err != nil || !ok {
		t.Fatalf("应自动建目录: (%v, %v)", ok, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("账本文件未落盘: %v", err)
	}
}

func TestFileStoreRejectedTakeDoesNotIncrement(t *testing.T) {
	path := quotaPath(t)
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	if ok, _ := s.Take("t", q, now); !ok {
		t.Fatal("首次应放行")
	}
	for i := 0; i < 3; i++ {
		if ok, _ := s.Take("t", q, now); ok {
			t.Fatal("超额应被拒")
		}
	}
	if got := readTestLedger(t, path)["t"].Count; got != 1 {
		t.Errorf("被拒的请求不得计数: Count = %d, want 1", got)
	}
}

func TestFileStoreIsolatesTopics(t *testing.T) {
	s := NewFileStore(quotaPath(t))
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()
	if ok, _ := s.Take("a", q, now); !ok {
		t.Fatal("a 首次应放行")
	}
	if ok, _ := s.Take("b", q, now); !ok {
		t.Error("不同主题账本互不影响")
	}
}

// TestFileStoreConcurrentTakesRespectLimit 覆盖 boundary[1]：并发 Take 不超发。
//
// ⚠ **它守护的是「同进程并发不超发」，而不是 flock 本身。** 互斥由两道各自独立
// 充分的机制提供——同进程的 f.mu 与跨进程的 flock，实测：
//
//	去掉 f.mu（留 flock 排他） → 绿
//	flock 改共享锁（留 f.mu）  → 绿
//	两者同时失效              → 红（放行 50 次）
//
// 所以单独删掉任一道，这条测试都测不出。真正的跨进程 flock 保护需要多进程才能
// 验证，单个 go test 进程内做不到；DoD 措辞里的「在 flock 保护下」在此只能理解为
// 「在现有互斥机制下」。
func TestFileStoreConcurrentTakesRespectLimit(t *testing.T) {
	path := quotaPath(t)
	s := NewFileStore(path)
	q := Quota{Limit: 10, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	const n = 50
	var mu sync.Mutex
	granted := 0
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if ok, err := s.Take("t", q, now); err == nil && ok {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if granted != 10 {
		t.Errorf("并发下放行 %d 次, want 10", granted)
	}
	if got := readTestLedger(t, path)["t"].Count; got != 10 {
		t.Errorf("账本计数 = %d, want 10", got)
	}
}

// TestFileStoreLeavesNoTempFiles 覆盖 non_functional[0]：原子写不留残留。
//
// DoD 原措辞「进程中途被杀不留半截 JSON」在 go test 内无法可靠复现，改为可判定的
// 等价断言：Take 执行完毕后，目录里除目标账本与锁文件外不应有别的东西。
// temp+rename 实现下临时文件已被 rename 掉；若改成「直接覆写目标文件」则没有临时
// 文件但会留半截 JSON，那由 readTestLedger 的合法性检查在别处兜住。
func TestFileStoreLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector-quota.json")
	s := NewFileStore(path)
	q := Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	for i := 0; i < 5; i++ {
		if ok, err := s.Take("t", q, now); err != nil || !ok {
			t.Fatalf("第 %d 次 Take: (%v, %v)", i, ok, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var unexpected []string
	for _, e := range entries {
		switch e.Name() {
		case "collector-quota.json", "collector-quota.json.lock":
			// 目标账本与锁文件是预期内的常驻物
		default:
			unexpected = append(unexpected, e.Name())
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("Take 后目录中有残留临时文件: %v", unexpected)
	}
}

// nextFD 返回下一个可用的 fd 号。POSIX 保证 open 返回**最小可用** fd，因此泄漏
// N 个 fd 会让这个数增大约 N。
//
// 比读 /proc/self/fd（Linux 专有）或 /dev/fd（macOS 上 go test 进程内实测读不到）
// 都可移植。
func nextFD(t *testing.T) int {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("探测 fd 号: %v", err)
	}
	defer f.Close()
	return int(f.Fd())
}

// TestFileStoreDoesNotLeakFDs 验证 non_functional[1]：flock 的 fd 在 Take 返回前关闭。
//
// **直接观测 fd，不用「撞上限」的间接办法**：本机 ulimit -n 是 1048576，泄漏几百个
// fd 根本撞不到上限；且 os.File 带 finalizer，GC 会把泄漏的 fd 悄悄收掉——间接办法
// 在这两点下都是「看起来在守护、实际不设防」。实测：删掉释放函数里的 lf.Close()，
// 间接版（跑 300 次看是否报 too many open files）仍然全绿。
func TestFileStoreDoesNotLeakFDs(t *testing.T) {
	s := NewFileStore(quotaPath(t))
	q := Quota{Limit: 0, Window: 24 * time.Hour, Loc: time.UTC} // Limit<=0 = 不设上限
	now := time.Now()

	// 先跑一次让懒初始化（建目录、建锁文件）落定，再取基线。
	if ok, err := s.Take("t", q, now); err != nil || !ok {
		t.Fatalf("预热 Take: (%v, %v)", ok, err)
	}
	base := nextFD(t)

	const n = 50
	for i := 0; i < n; i++ {
		ok, err := s.Take("t", q, now)
		if err != nil {
			t.Fatalf("第 %d 次 Take: %v", i, err)
		}
		if !ok {
			t.Fatalf("第 %d 次 Take 被拒，但 Limit<=0 应不设上限", i)
		}
	}

	// 容差 5：测试进程自身也可能在此期间开关文件。每次 Take 泄漏一个 fd 的话
	// 增量会是 n=50，远超容差。
	if after := nextFD(t); after > base+5 {
		t.Errorf("Take 泄漏 fd: %d 次调用后下一个可用 fd 号 %d → %d（flock 的 fd 须在 Take 返回前关闭）",
			n, base, after)
	}
}

// TestFileStoreMatchesMemStoreSemantics 守护「两个后端语义一致」这个跨实现约束。
//
// FileStore 与 MemStore 共用 take() 与 windowStart() 两个纯逻辑函数，正是为了不让
// 窗口语义在两处各写一遍而漂移。若将来有人在某一侧「就地优化」窗口判定，这条会红。
func TestFileStoreMatchesMemStoreSemantics(t *testing.T) {
	q := Quota{Limit: 3, Window: 24 * time.Hour, Loc: time.UTC}
	day1 := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)

	steps := []time.Time{day1, day1, day1, day1, day2}
	want := []bool{true, true, true, false, true} // 第 4 次超 Limit=3；第 5 次跨日重置

	mem := NewMemStore()
	file := NewFileStore(quotaPath(t))
	for i, now := range steps {
		gotMem, errMem := mem.Take("t", q, now)
		gotFile, errFile := file.Take("t", q, now)
		if errMem != nil || errFile != nil {
			t.Fatalf("第 %d 步出错: mem=%v file=%v", i, errMem, errFile)
		}
		if gotMem != want[i] || gotFile != want[i] {
			t.Errorf("第 %d 步: MemStore=%v FileStore=%v, want %v（两个后端语义必须一致）",
				i, gotMem, gotFile, want[i])
		}
	}
}

func writeTestLedger(t *testing.T, path string, l map[string]ledgerEntry) {
	t.Helper()
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestLedger(t *testing.T, path string) map[string]ledgerEntry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var l map[string]ledgerEntry
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("账本不是合法 JSON: %v (%s)", err, raw)
	}
	return l
}
