package hestia

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M2a 的 TASK-004）
// functional[0] EnsureQueueDirs 四目录 + 幂等            → TestEnsureQueueDirsCreatesStateMachine
// functional[0] WriteContract 落 pending/、内容 == JSON()  → TestWriteContractLandsInPending
// functional[0] 同名覆盖、无 .tmp 残留                    → TestWriteContractOverwritesPending
// functional[0] done/ 同名不动                             → TestWriteContractLeavesDoneAlone
// functional[0] dir 是文件 ⇒ mkdir 层报错含 contract       → TestWriteContractFailsLoudly
// functional[1] AST 守卫 34 项                             → store_test.go TestPackageExposesNoWriteFunctions
// boundary[0]   目标预建为目录 ⇒ rename 层报错、无 .tmp 残留 → TestWriteContractFailsWhenTargetIsDir
// boundary[1]   EnsureQueueDirs 对普通文件报错含 contract queue dir → TestEnsureQueueDirsRejectsFile
// boundary[1]   WriteContract 对不存在的 dir 自建 pending/  → TestWriteContractCreatesPendingWithoutEnsure
// boundary[2]   003 残留 (a) 文件末字节是 \n               → TestWriteContractEndsWithNewline
// boundary[2]   003 残留 (b) 全字段在场 ⇒ absent_fields 是 [] → TestWriteContractAbsentFieldsEmptyArray
// boundary[2]   002 残留六条子例                           → signals_test.go TestEvaluateThresholdEdges
// error_handling[0] 红阶段留痕                            → discovery verification.red_phase

// passingContract 是本文件多数用例共用的夹具：一份通过校验的常规契约。队列的用例只关心
// 落盘行为，契约内容不是变量——只有需要改内容的用例（修订、全字段在场）才自己构造。
func passingContract() Contract {
	return BuildContract(ContractInput{Obs: contractObs(), Report: ValidationReport{Passed: true}}, contractCfg())
}

func TestEnsureQueueDirsCreatesStateMachine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "queue", "hestia")
	require.NoError(t, EnsureQueueDirs(dir))
	for _, sub := range []string{"pending", "processing", "done", "failed"} {
		st, err := os.Stat(filepath.Join(dir, sub))
		require.NoError(t, err, sub)
		assert.True(t, st.IsDir())
	}
	require.NoError(t, EnsureQueueDirs(dir), "幂等")
}

func TestWriteContractLandsInPending(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, EnsureQueueDirs(dir))
	c := passingContract()

	path, err := WriteContract(dir, c)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "pending", "2026-08-monthly.json"), path)
	want, _ := c.JSON()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// 同名覆盖：修订到达时旧契约还没被消费，最新的赢。
func TestWriteContractOverwritesPending(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, EnsureQueueDirs(dir))
	first := passingContract()
	_, err := WriteContract(dir, first)
	require.NoError(t, err)

	obs := contractObs()
	obs.Meta.PublishedAt = "2026-09-20"
	second := BuildContract(ContractInput{Obs: obs, Report: ValidationReport{Passed: true}, IsRevision: true, Supersedes: "2026-09-12"}, contractCfg())
	path, err := WriteContract(dir, second)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), `"is_revision": true`)
	entries, _ := os.ReadDir(filepath.Join(dir, "pending"))
	assert.Len(t, entries, 1, "同名只有一份，且没有 .tmp 残留")
}

// done/ 是消费者的地盘：同名文件不动。
func TestWriteContractLeavesDoneAlone(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, EnsureQueueDirs(dir))
	done := filepath.Join(dir, "done", "2026-08-monthly.json")
	require.NoError(t, os.WriteFile(done, []byte("consumed"), 0o644))

	c := passingContract()
	_, err := WriteContract(dir, c)
	require.NoError(t, err)
	got, _ := os.ReadFile(done)
	assert.Equal(t, "consumed", string(got))
}

// 目录不可用 ⇒ 报错；pending/ 里不能留下半个文件。这是 mkdir 层的失败。
func TestWriteContractFailsLoudly(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	c := passingContract()
	_, err := WriteContract(blocker, c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract")
}

// 边界（M2a 的 TASK-004 boundary，AD-4a）：写失败无残留的第二种形态——目标文件名被预建
// 为目录，os.Rename(tmp, path) 在 POSIX 上对「目标是目录、源是文件」返回 EISDIR。这是
// rename 层的失败，与 FailsLoudly 的 mkdir 层失败分开各证一次；判据不依赖权限位（root 下
// chmod 方案会假绿）。TASK-005 的「契约写失败」用例直接复用这个夹具。
func TestWriteContractFailsWhenTargetIsDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, EnsureQueueDirs(dir))
	c := passingContract()
	target := filepath.Join(dir, "pending", c.FileName())
	require.NoError(t, os.MkdirAll(target, 0o755))

	_, err := WriteContract(dir, c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract")
	entries, err := os.ReadDir(filepath.Join(dir, "pending"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "pending/ 里只剩那个预建目录，没有 .tmp 残留")
	assert.Equal(t, c.FileName(), entries[0].Name())
	assert.True(t, entries[0].IsDir())
}

// 边界（M2a 的 TASK-004 boundary[1]）：dir 本身是普通文件 ⇒ 四个子目录一个都建不了，
// 错误带 `contract queue dir` 前缀，让运维一眼知道是队列目录配置的问题。
func TestEnsureQueueDirsRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "queue-is-a-file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	err := EnsureQueueDirs(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract queue dir")
}

// 边界（M2a 的 TASK-004 boundary[1]）：WriteContract 不要求先调 EnsureQueueDirs——
// dir 不存在但可建时自己 MkdirAll(pending) 后写入成功；其余三个状态目录不归它建。
func TestWriteContractCreatesPendingWithoutEnsure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh", "queue")
	c := passingContract()

	path, err := WriteContract(dir, c)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "pending", c.FileName()), path)
	_, err = os.Stat(path)
	require.NoError(t, err)
	for _, sub := range []string{"processing", "done", "failed"} {
		_, err := os.Stat(filepath.Join(dir, sub))
		assert.Truef(t, os.IsNotExist(err), "%s 不归 WriteContract 建", sub)
	}
}

// 003 变异残留 (a)（test-m2a-b 报告 M3）：写出的文件末字节是 \n。JSON() 末尾换行是 003 DoD
// 的明写项，done/ 与回放的逐字节 cmp 依赖它；WriteContract 原样落盘，不得吞掉。
func TestWriteContractEndsWithNewline(t *testing.T) {
	dir := t.TempDir()
	c := passingContract()
	path, err := WriteContract(dir, c)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	assert.Equal(t, byte('\n'), raw[len(raw)-1], "文件末字节必须是换行")
	assert.NotEqual(t, byte('\n'), raw[len(raw)-2], "恰一个换行，不是两个")
}

// 003 变异残留 (b)（test-m2a-b 报告 M6）：Values 覆盖全部 fieldOrder 时写出的 JSON 里
// absent_fields 是 `[]` 而不是 `null`——消费者按数组读，null 会让它多一条分支。
func TestWriteContractAbsentFieldsEmptyArray(t *testing.T) {
	obs := contractObs()
	vals := map[string]float64{}
	for _, f := range fieldOrder {
		vals[f] = 1
	}
	obs.Values = vals
	c := BuildContract(ContractInput{Obs: obs, Report: ValidationReport{Passed: true}}, contractCfg())

	path, err := WriteContract(t.TempDir(), c)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"absent_fields": []`, "全字段在场时 absent_fields 必须是空数组")
	assert.NotContains(t, string(raw), `"absent_fields": null`)
}
