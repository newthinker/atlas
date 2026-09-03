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

	res, err := saveSnapshot(dir, "a1", v2, snapNow)
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
