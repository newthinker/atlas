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
	"os"
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
