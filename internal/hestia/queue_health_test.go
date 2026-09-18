package hestia

// Context Checkpoint: done_criteria → test mapping（TASK-001）
// functional[0]     件数与最旧年龄（pending 取最旧、processing 取最旧）→ TestQueueHealthCountsAndAges
// functional[1]     复用 queueStates + switch default 报错               → 验证者 review（queue_health.go）
// functional[2]     AST 守卫登记 QueueHealthOf、reflect 守卫零改动         → TestPackageExposesNoWriteFunctions / TestStoreExposesNoWriteMethods
// boundary[0]       四个目录都在且都空 ⇒ 零件数、零年龄、无错             → TestQueueHealthEmptyIsNotAnError
// boundary[1]       子目录 / 点文件 / .tmp 不计件、不参与年龄              → TestQueueHealthIgnoresNonItems
// error_handling[0] 四个子目录各删一次 ⇒ 报错含目录名、返回零值            → TestQueueHealthMissingDirIsAnError
// error_handling[1] 子目录权限 000 ⇒ 报错（root 跳过）                     → TestQueueHealthUnreadableDirIsAnError
// non_functional[0] 纯文件系统读（C1）                                     → 验证者 review（queue_health.go）

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newQueue(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, EnsureQueueDirs(dir))
	return dir
}

func put(t *testing.T, dir, state, name string, age time.Duration) {
	t.Helper()
	p := filepath.Join(dir, state, name)
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o644))
	at := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(p, at, at))
}

func TestQueueHealthCountsAndAges(t *testing.T) {
	dir := newQueue(t)
	put(t, dir, "pending", "a.json", 3*time.Hour)
	put(t, dir, "pending", "b.json", 1*time.Hour)
	put(t, dir, "failed", "c.json", 0)

	got, err := QueueHealthOf(dir)
	require.NoError(t, err)
	require.Equal(t, 2, got.PendingCount)
	require.Equal(t, 1, got.FailedCount)
	require.Equal(t, 0, got.ProcessingCount)
	require.Equal(t, 0, got.DoneCount)
	// 取**最旧**那个，不是最新——「堆了多久」问的是最早那份等了多久
	require.WithinDuration(t, time.Now().Add(-3*time.Hour), got.OldestPending, time.Minute)

	put(t, dir, "processing", "d.json", 2*time.Hour)
	put(t, dir, "processing", "e.json", 30*time.Minute)
	put(t, dir, "done", "f.json", 0)
	got, err = QueueHealthOf(dir)
	require.NoError(t, err)
	require.Equal(t, 2, got.ProcessingCount)
	require.Equal(t, 1, got.DoneCount)
	require.WithinDuration(t, time.Now().Add(-2*time.Hour), got.OldestProcessing, time.Minute)
}

// TestQueueHealthEmptyIsNotAnError：四个目录都在但都空，是正常状态。
func TestQueueHealthEmptyIsNotAnError(t *testing.T) {
	got, err := QueueHealthOf(newQueue(t))
	require.NoError(t, err)
	require.Zero(t, got.PendingCount)
	require.Zero(t, got.ProcessingCount)
	require.Zero(t, got.DoneCount)
	require.Zero(t, got.FailedCount)
	require.True(t, got.OldestPending.IsZero(), "空目录的年龄是零值，由调用方决定不输出该指标")
	require.True(t, got.OldestProcessing.IsZero())
}

// TestQueueHealthMissingDirIsAnError 是本任务最重要的一条（C3）。
//
// `deploy.sh` 删队列那次的形态是「目录还在、里面空了」，而**「正常空」与「被删空」
// 在件数上完全同形**。所以目录读不到必须是**可告警的事实**，不能退化成零件数——
// 退化的话，队列整个没了会表现得和一切正常一模一样。
//
// 四个状态各删一次：只删 pending 时，「只读了 pending」的实现照样能过。
func TestQueueHealthMissingDirIsAnError(t *testing.T) {
	for _, state := range []string{"pending", "processing", "done", "failed"} {
		t.Run(state, func(t *testing.T) {
			dir := newQueue(t)
			// 每个目录都放一件：否则删掉后面的目录时，已扫过的前面几个件数仍是 0，「返回部分结果」看不出来
			for _, s := range []string{"pending", "processing", "done", "failed"} {
				put(t, dir, s, "keep.json", 0)
			}
			require.NoError(t, os.RemoveAll(filepath.Join(dir, state)))

			got, err := QueueHealthOf(dir)
			require.Error(t, err)
			require.Contains(t, err.Error(), state, "报错要说清是哪个子目录，否则四个目录要挨个试")
			require.Equal(t, QueueHealth{}, got, "出错时不返回部分结果")
		})
	}
}

func TestQueueHealthUnreadableDirIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视权限位")
	}
	dir := newQueue(t)
	sub := filepath.Join(dir, "processing")
	require.NoError(t, os.Chmod(sub, 0o000))
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) }) // 否则 TempDir 清不掉

	got, err := QueueHealthOf(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "processing")
	require.Equal(t, QueueHealth{}, got)
}

// TestQueueHealthIgnoresNonItems：子目录、点文件、.tmp 都不是队列项，既不计件也不参与年龄。
// .tmp 是 writeAtomic 在 pending/ 的中间态；判据与触发脚本（TASK-006）一致。
func TestQueueHealthIgnoresNonItems(t *testing.T) {
	dir := newQueue(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "pending", "tmp"), 0o755))
	put(t, dir, "pending", ".DS_Store", 5*time.Hour)
	put(t, dir, "pending", "a.json.tmp", 4*time.Hour)

	got, err := QueueHealthOf(dir)
	require.NoError(t, err)
	require.Zero(t, got.PendingCount)
	require.True(t, got.OldestPending.IsZero())
}

// —— 以下为 QA 返工（W-1 / W-2 / W-6）追加的用例 ——
// boundary[1]       非常规文件（symlink 三例）不计件，且与触发脚本同判 → TestQueueHealthIgnoresIrregularFiles
// functional[1]     ByState() 的键恰为 queueStates                      → TestQueueHealthByStateCoversAllStates
// error_handling[0] 条目在 ReadDir 与 Info 之间消失 ⇒ 不是错误          → TestQueueHealthEntryVanishingIsNotAnError

// TestQueueHealthIgnoresIrregularFiles：符号链接等非常规文件不是队列项（QA W-1）。
//
// 🔴 **判据必须与触发脚本的 `find -type f` 一致**，否则指标说「有 N 件」而触发器说
// 「queue empty」：24h 后 hestia_queue_stuck 亮起，文案却指向「触发器没跑」，把值班人引偏。
// os.ReadDir 的 DirEntry.Type() 来自 lstat，symlink 的 IsDir() 恒 false ⇒ 不排非常规文件
// 就会把 symlink 计成一件。
//
// 三例都要：指向普通文件的（lstat 说 symlink、stat 说普通文件）、悬空的（stat 会失败）、
// 指向目录的（IsDir() 仍是 false，最容易漏）。
func TestQueueHealthIgnoresIrregularFiles(t *testing.T) {
	dir := newQueue(t)
	target := filepath.Join(dir, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("{}"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "pending", "to-file.json")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "gone.json"), filepath.Join(dir, "pending", "dangling.json")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "done"), filepath.Join(dir, "pending", "to-dir.json")))

	got, err := QueueHealthOf(dir)
	require.NoError(t, err, "悬空 symlink 也不该让整次读失败——它根本不是队列项")
	require.Zero(t, got.PendingCount)
	require.True(t, got.OldestPending.IsZero())

	// 同一夹具喂给触发脚本：两侧同判才算口径一致（只比对各自的断言不够——那只证明
	// 两边各自符合我写下的期望，不证明它们彼此一致）。
	script, err := filepath.Abs("../../scripts/ops/hestia-warp-trigger.sh")
	require.NoError(t, err)
	called := filepath.Join(t.TempDir(), "called")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"HESTIA_QUEUE_DIR="+dir,
		"TRIGGER_CMD=echo called >> "+called) // 桩：确保测试绝不唤起真 agent
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "触发脚本应 exit 0，输出：%s", out)
	require.Contains(t, string(out), "queue empty", "触发脚本也必须认为队列是空的")
	require.NoFileExists(t, called, "队列空时不得唤起 agent")
}

// TestQueueHealthByStateCoversAllStates：ByState() 是 collector 的单一口径（QA W-2）。
//
// 🔴 断言的是**键集合与 queueStates 相等**，不是「包含我列举的四个」：加第五个状态
// 而 ByState 没跟上时，后者恒绿——那正是 W-2 要堵的失效（hestia 侧改齐、消费侧漏跟，
// go test ./... 全绿而新状态的件数静默消失）。
func TestQueueHealthByStateCoversAllStates(t *testing.T) {
	dir := newQueue(t)
	put(t, dir, "pending", "a.json", time.Hour)
	put(t, dir, "pending", "b.json", time.Hour)
	put(t, dir, "failed", "c.json", 0)

	got, err := QueueHealthOf(dir)
	require.NoError(t, err)
	by := got.ByState()

	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, queueStates, keys, "ByState 的键必须恰为 queueStates —— 多一个少一个都是口径分叉")
	require.Equal(t, map[string]int{"pending": 2, "processing": 0, "done": 0, "failed": 1}, by)
}

// TestQueueHealthEntryVanishingIsNotAnError：条目在 ReadDir 与 Info() 之间消失不是错误（QA W-6）。
//
// os.ReadDir 先返回条目名、e.Info() 才 lstat。契约从 pending/ rename 到 processing/ 若发生在
// 这两步之间，Info() 返 ErrNotExist。把它当致命错误的代价：该轮 queue_up=0、队列指标整组
// 缺失，stuck/failed 的 for 计时清零 ⇒ 真告警最多晚 10 分钟。
//
// 这个竞态在真实文件系统上不可稳定复现，故经包内 seam 注入一个「Info() 说文件不在了」
// 的条目——测的是 QueueHealthOf 对该错误的处置，不是 os.ReadDir 本身。
func TestQueueHealthEntryVanishingIsNotAnError(t *testing.T) {
	dir := newQueue(t)
	put(t, dir, "pending", "stays.json", time.Hour)

	orig := queueReadDir
	t.Cleanup(func() { queueReadDir = orig })
	queueReadDir = func(name string) ([]os.DirEntry, error) {
		entries, err := orig(name)
		if err != nil || filepath.Base(name) != "pending" {
			return entries, err
		}
		return append(entries, vanishedEntry{name: "renamed-away.json"}), nil
	}

	got, err := QueueHealthOf(dir)
	require.NoError(t, err, "正常流转的 rename 不是故障")
	require.Equal(t, 1, got.PendingCount, "消失的那件不计数，留下的那件照常计")
	require.WithinDuration(t, time.Now().Add(-time.Hour), got.OldestPending, time.Minute)
}

// vanishedEntry 模拟「ReadDir 看见了它，随后它被 rename 走」的条目：Type() 说是普通文件，
// Info() 报 fs.ErrNotExist。
type vanishedEntry struct{ name string }

func (e vanishedEntry) Name() string               { return e.name }
func (e vanishedEntry) IsDir() bool                { return false }
func (e vanishedEntry) Type() os.FileMode          { return 0 }
func (e vanishedEntry) Info() (os.FileInfo, error) { return nil, fs.ErrNotExist }

// TestQueueHealthOtherInfoErrorsAreFatal：只有「不存在」被放过，其它 Info 错误仍响亮失败。
func TestQueueHealthOtherInfoErrorsAreFatal(t *testing.T) {
	dir := newQueue(t)
	orig := queueReadDir
	t.Cleanup(func() { queueReadDir = orig })
	queueReadDir = func(name string) ([]os.DirEntry, error) {
		entries, err := orig(name)
		if err != nil || filepath.Base(name) != "pending" {
			return entries, err
		}
		return append(entries, brokenEntry{name: "broken.json"}), nil
	}

	got, err := QueueHealthOf(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "broken.json")
	require.Equal(t, QueueHealth{}, got)
}

type brokenEntry struct{ name string }

func (e brokenEntry) Name() string               { return e.name }
func (e brokenEntry) IsDir() bool                { return false }
func (e brokenEntry) Type() os.FileMode          { return 0 }
func (e brokenEntry) Info() (os.FileInfo, error) { return nil, fs.ErrPermission }
