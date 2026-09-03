package hestia

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M1d 的 TASK-002）
// functional[0]     首次落盘路径固定、内容逐字节相同        → TestSaveSnapshotWritesNewFile
// functional[1]     目录不存在时 MkdirAll 自行创建            → TestSaveSnapshotCreatesDir
// functional[2]     同 id 同字节 ⇒ Unchanged，不重写、mtime 不变 → TestSaveSnapshotUnchangedKeepsMtime
// boundary[0]       同 id 不同字节 ⇒ Diverged，另存不覆盖     → TestSaveSnapshotDivergedKeepsBothVersions
// error_handling[0] dir 路径上是普通文件 ⇒ 响亮报错           → TestSaveSnapshotFailsLoudlyWhenDirUnusable
// error_handling[0] (R-002) writeAtomic 的 WriteFile 失败分支  → TestSaveSnapshotFailsWhenDirReadOnly
// error_handling[0] 读既有文件出错且非 ErrNotExist ⇒ snapshot read → TestSaveSnapshotFailsWhenExistingPathIsDir
// error_handling[0] rename 失败 ⇒ 清理 tmp 并返回 snapshot rename → TestWriteAtomicRenameFailureRemovesTmp
// review_fix A3   改稿后重抓同字节 ⇒ 命中时间戳副本 ⇒ Unchanged，不再落第三份 → TestSaveSnapshotDivergedTwiceIsUnchanged
// review_fix A3   副本查重的 glob / 读副本失败分支                    → TestSaveSnapshotFailsOnUnglobbableID / TestSaveSnapshotFailsWhenCopyIsDir
// review_fix A8   now 传非 UTC 时区，文件名仍 …T083015Z（钉 now.UTC()） → TestSaveSnapshotDivergedKeepsBothVersions
// non_functional[0] 不新增导出函数、不含业务字段名字面量      → 既有 TestPackageExposesNoWriteFunctions /
//                                                                TestFieldNamesAppearOnlyInFieldsGo 保持绿

var snapNow = time.Date(2026, 9, 4, 8, 30, 15, 0, time.UTC)

// 首次落盘：路径固定为 <dir>/<article_id>.html，内容逐字节相同。
func TestSaveSnapshotWritesNewFile(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("<html>2026-08</html>")

	res, err := saveSnapshot(dir, "2026091412345678901", raw, snapNow)
	require.NoError(t, err)
	assert.Equal(t, snapshotWritten, res.Kind)
	assert.Equal(t, filepath.Join(dir, "2026091412345678901.html"), res.Path)

	got, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	assert.Equal(t, raw, got, "落盘必须逐字节等于抓到的原文——快照的全部价值是回溯")
}

// 目录不存在时自行创建（与 NewStore 的 MkdirAll 同约定）。
func TestSaveSnapshotCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "snapshots")
	_, err := saveSnapshot(dir, "a1", []byte("x"), snapNow)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "a1.html"))
	require.NoError(t, err)
}

// 同 id、同字节：跳过，且**不改 mtime**——重跑不该让文件看起来「被动过」。
func TestSaveSnapshotUnchangedKeepsMtime(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("same")
	first, err := saveSnapshot(dir, "a1", raw, snapNow)
	require.NoError(t, err)

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(first.Path, old, old))

	res, err := saveSnapshot(dir, "a1", raw, snapNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, snapshotUnchanged, res.Kind)
	assert.Equal(t, first.Path, res.Path)

	st, err := os.Stat(first.Path)
	require.NoError(t, err)
	assert.True(t, st.ModTime().Equal(old), "字节相同就不该重写；mtime 变了说明重写了")
}

// 同 id、不同字节：**不覆盖**，另存带 UTC 时间戳的文件；第一版原样。
func TestSaveSnapshotDivergedKeepsBothVersions(t *testing.T) {
	dir := t.TempDir()
	v1 := []byte("version one")
	v2 := []byte("version two (央行改稿)")
	first, err := saveSnapshot(dir, "a1", v1, snapNow)
	require.NoError(t, err)

	// 传东八区的 now：文件名必须仍是 UTC 的 083015Z，而不是本地钟面的 163015（review_fix A8）。
	cst := snapNow.In(time.FixedZone("CST", 8*3600))
	res, err := saveSnapshot(dir, "a1", v2, cst)
	require.NoError(t, err)
	assert.Equal(t, snapshotDiverged, res.Kind)
	assert.Equal(t, filepath.Join(dir, "a1.20260904T083015Z.html"), res.Path,
		"改稿版的文件名带 UTC 时间戳，秒级；同一秒内第二次改稿在真实站点上不会发生")

	got1, err := os.ReadFile(first.Path)
	require.NoError(t, err)
	assert.Equal(t, v1, got1, "第一版必须原样——「不覆盖」是这条规则的全部内容")
	got2, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	assert.Equal(t, v2, got2)
}

// 目录不可用（路径上是个普通文件）⇒ 返回错误，而不是静默跳过。
func TestSaveSnapshotFailsLoudlyWhenDirUnusable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	_, err := saveSnapshot(blocker, "a1", []byte("x"), snapNow)
	require.Error(t, err, "快照写不进去必须响亮失败——它是 DoD 项，不是可选副作用")
}

// 目录存在但只读 ⇒ writeAtomic 的 WriteFile 分支报错（reviewer R-002：
// internal/hestia 覆盖率与门槛零余量，未覆盖的错误分支足以把它压破）。
// 首次落盘与改稿另存两条路径都经这一分支，各断言一次。root 不受目录权限约束，此时跳过。
func TestSaveSnapshotFailsWhenDirReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 不受目录权限约束，无法触发写失败")
	}
	dir := t.TempDir()
	_, err := saveSnapshot(dir, "a1", []byte("v1"), snapNow)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err = saveSnapshot(dir, "a2", []byte("x"), snapNow)
	require.Error(t, err, "首次落盘写不进去")
	assert.Contains(t, err.Error(), "snapshot write")

	_, err = saveSnapshot(dir, "a1", []byte("v2"), snapNow)
	require.Error(t, err, "改稿另存写不进去")
	assert.Contains(t, err.Error(), "snapshot write")
}

// <dir>/<id>.html 位置上是个目录 ⇒ ReadFile 报 EISDIR（不是 ErrNotExist），
// 必须走 snapshot read 分支报错，而不是当成「不存在」去写。
func TestSaveSnapshotFailsWhenExistingPathIsDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "a1.html"), 0o755))

	_, err := saveSnapshot(dir, "a1", []byte("x"), snapNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot read")
}

// rename 失败（目标是已存在的目录，POSIX 规定 EISDIR）⇒ 清掉 <path>.tmp 再报错，
// 不留下一个孤儿临时文件。
func TestWriteAtomicRenameFailureRemovesTmp(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.Mkdir(target, 0o755))

	err := writeAtomic(target, []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot rename")
	_, statErr := os.Stat(target + ".tmp")
	assert.ErrorIs(t, statErr, os.ErrNotExist, "rename 失败后临时文件必须被清掉")
}

// 改稿后再重抓（--force 或次日重抓）拿到的仍是改稿版：必须命中时间戳副本 ⇒ Unchanged，
// 不能每次都再落一份相同字节的副本并打 diverged（review_fix A3，QA 实测 v1/v2/v2/v2 ⇒ 4 文件）。
func TestSaveSnapshotDivergedTwiceIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	v1, v2 := []byte("version one"), []byte("version two")

	r1, err := saveSnapshot(dir, "a1", v1, snapNow)
	require.NoError(t, err)
	assert.Equal(t, snapshotWritten, r1.Kind)

	r2, err := saveSnapshot(dir, "a1", v2, snapNow)
	require.NoError(t, err)
	assert.Equal(t, snapshotDiverged, r2.Kind)

	r3, err := saveSnapshot(dir, "a1", v2, snapNow.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, snapshotUnchanged, r3.Kind, "同字节的改稿版已在副本里，不该再落一份")
	assert.Equal(t, r2.Path, r3.Path, "Unchanged 的 Path 指向命中的那份副本")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "目录里恰 2 文件：<id>.html 与一份时间戳副本")
}

// articleID 含 glob 元字符且已有不同字节的 <id>.html ⇒ 副本查重的 Glob 报 ErrBadPattern，
// 必须响亮返回，不能当成「没有副本」继续落盘。
func TestSaveSnapshotFailsOnUnglobbableID(t *testing.T) {
	dir := t.TempDir()
	_, err := saveSnapshot(dir, "a[1", []byte("v1"), snapNow)
	require.NoError(t, err, "首次落盘不经 Glob，正常写入")

	_, err = saveSnapshot(dir, "a[1", []byte("v2"), snapNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot glob")
}

// <id>.<x>.html 位置上是个目录 ⇒ 读副本报 EISDIR，必须走 snapshot read 分支报错。
func TestSaveSnapshotFailsWhenCopyIsDir(t *testing.T) {
	dir := t.TempDir()
	_, err := saveSnapshot(dir, "a1", []byte("v1"), snapNow)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "a1.bogus.html"), 0o755))

	_, err = saveSnapshot(dir, "a1", []byte("v2"), snapNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot read")
}
