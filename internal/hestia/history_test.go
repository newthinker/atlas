package hestia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M3 的 TASK-001）
// functional[0] history.go 的类型/常量/函数形态          → 由下列各条共同覆盖（编译期 + 行为）
// functional[1] monthly 截断 12 + 降序 + 省略 + FileName  → TestBuildHistoryMonthlyOmitsRecent
// functional[1] h1 双序列                                 → TestBuildHistoryH1CarriesBothSeries
// functional[1] entry 形态 + 确定性                       → TestHistoryEntryShapeAndDeterminism
// functional[1] 落点 + 写失败 err 含 history + 同名覆盖   → TestWriteHistoryLandsInPending
// functional[2] ingest 接线（先侧车后契约）               → ingest_test.go TestIngestWritesHistoryBesideContract
// functional[3] AST 守卫 38 项                            → store_test.go TestPackageExposesNoWriteEntryPoints
// boundary[0]   monthly 省略 / 非 monthly 为 []           → TestBuildHistoryMonthlyOmitsRecent、TestBuildHistoryH1CarriesBothSeries、TestBuildHistoryNonMonthlyKeepsEmptyMonthlyRecent
// boundary[0]   不足 12 期按实有条数、不补空              → TestBuildHistoryH1CarriesBothSeries
// boundary[0]   Data 按 fieldOrder 序、缺失不写 null      → TestHistoryEntryShapeAndDeterminism
// boundary[0]   history.go 无业务字段名字面量             → TestHistorySourceHasNoFieldLiterals
// error_handling[0] Preceding 两处失败                    → TestBuildHistoryPropagatesSameTypeQueryError、TestBuildHistoryPropagatesMonthlyQueryError
// error_handling[0] JSON 失败 / MkdirAll 失败 / 原子写失败 → TestWriteHistoryJSONFailure、TestWriteHistoryLandsInPending（blocker）、TestWriteHistoryAtomicRenameFailure

// seedMonths 往库里放 n 个连续 monthly 观测（2025-01 起）。需要 h1 的用例自己另存。
func seedMonths(t *testing.T, s *Store, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		mm := fmt.Sprintf("%02d", i+1)
		obs := contractObs()
		obs.Meta.Period = "2025-" + mm
		obs.Meta.PublishedAt = "2025-" + mm + "-15"
		obs.Meta.ArticleID = "2025" + mm + "15000000000000000"
		obs.Values[FieldM2] = 300 + float64(i)
		_, err := s.Save(ctx, obs, passing())
		require.NoError(t, err)
	}
}

// monthly：same_type 是前 12 个 monthly（降序、最多 12），monthly_recent 省略。
// 库里放 13 个 monthly（2024-12 + 2025-01…12），目标期 2026-01 ⇒ 取 12 个，2024-12 被截掉。
func TestBuildHistoryMonthlyOmitsRecent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedMonths(t, s, 12)
	older := contractObs()
	older.Meta.Period, older.Meta.PublishedAt, older.Meta.ArticleID = "2024-12", "2025-01-14", "2025011400000000000"
	_, err := s.Save(ctx, older, passing())
	require.NoError(t, err)
	target := contractObs()
	target.Meta.Period, target.Meta.PeriodType = "2026-01", "monthly"

	h, err := BuildHistory(ctx, s, target, "contract@v1")
	require.NoError(t, err)
	assert.Equal(t, "1.0", h.SchemaVersion)
	assert.Equal(t, "2026-01-monthly", h.For)
	assert.Equal(t, "contract@v1", h.GeneratedBy)
	require.Len(t, h.SameType, 12, "最多 12 期")
	assert.Equal(t, "2025-12", h.SameType[0].Meta.Period, "降序，最近在前")
	assert.Equal(t, "2025-01", h.SameType[11].Meta.Period, "2024-12 被截掉")
	assert.Nil(t, h.MonthlyRecent, "monthly 时 monthly_recent 省略")

	b, err := h.JSON()
	require.NoError(t, err)
	assert.NotContains(t, string(b), `"monthly_recent"`)
	assert.Equal(t, "2026-01-monthly.history.json", h.FileName())
}

// h1：same_type 是前几个 h1（可能少于 12），monthly_recent 是最近 12 个 monthly。
func TestBuildHistoryH1CarriesBothSeries(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedMonths(t, s, 12)
	h1 := contractObs()
	h1.Meta.Period, h1.Meta.PeriodType, h1.Meta.PublishedAt, h1.Meta.ArticleID = "2024-06", "h1", "2024-07-15", "2024071500000000000"
	_, err := s.Save(ctx, h1, passing())
	require.NoError(t, err)

	target := contractObs()
	target.Meta.Period, target.Meta.PeriodType = "2025-06", "h1"
	h, err := BuildHistory(ctx, s, target, "contract@v1/replay")
	require.NoError(t, err)
	require.Len(t, h.SameType, 1)
	assert.Equal(t, "2024-06", h.SameType[0].Meta.Period)
	require.Len(t, h.MonthlyRecent, 5, "2025-06 之前的 monthly 只有 2025-01…05")
	assert.Equal(t, "2025-05", h.MonthlyRecent[0].Meta.Period)
}

// 非 monthly 且库里一个 monthly 都没有 ⇒ monthly_recent 是空数组 `[]`，不是 null、不省略。
//
// 🔴 这条钉的是 `omitzero` 而不是 `omitempty`（AD-M3 偏离，见 discovery decisions）：
// omitempty 对 len==0 的 slice 同样省略，那样 annual 侧车就没有 monthly_recent 键，
// 而消费者按「非 monthly 必有此键」写的代码会拿到 undefined 而不是空序列——与
// TestBuildHistoryMonthlyOmitsRecent 要求的「monthly 时必须没有该键」不能靠同一个
// 标签同时满足，故用 omitzero（只省略 nil，空数组保留）。
func TestBuildHistoryNonMonthlyKeepsEmptyMonthlyRecent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	target := contractObs()
	target.Meta.Period, target.Meta.PeriodType = "2025-12", "annual"

	h, err := BuildHistory(ctx, s, target, "contract@v1")
	require.NoError(t, err)
	assert.NotNil(t, h.MonthlyRecent, "非 monthly 恒有该字段，哪怕一条都查不到")
	assert.Empty(t, h.MonthlyRecent)
	assert.Empty(t, h.SameType, "同类型也一条都没有")

	b, err := h.JSON()
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	require.Contains(t, parsed, "monthly_recent", "非 monthly 必须带这个键")
	assert.Equal(t, []any{}, parsed["monthly_recent"], "空数组，不是 null")
	assert.Equal(t, []any{}, parsed["same_type"], "same_type 无 omit 标签，恒为数组")
}

// 每项：Meta 七字段 snake_case + data 按 fieldOrder 序、只含非空；两次生成逐字节相同。
func TestHistoryEntryShapeAndDeterminism(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedMonths(t, s, 3)
	target := contractObs()
	target.Meta.Period = "2025-04"

	h1, err := BuildHistory(ctx, s, target, "contract@v1")
	require.NoError(t, err)
	h2, err := BuildHistory(ctx, s, target, "contract@v1")
	require.NoError(t, err)
	a, err := h1.JSON()
	require.NoError(t, err)
	b, err := h2.JSON()
	require.NoError(t, err)
	assert.Equal(t, a, b)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(a, &parsed))
	first := parsed["same_type"].([]any)[0].(map[string]any)
	meta := first["meta"].(map[string]any)
	for _, k := range []string{"period", "period_type", "published_at", "article_id", "caliber_version", "extractor", "ingested_at"} {
		assert.Contains(t, meta, k)
	}
	data := first["data"].(map[string]any)
	assert.Contains(t, data, FieldM2)
	assert.NotContains(t, data, FieldM0, "缺失字段不出现，不写 null")

	// data 段按 fieldOrder 序（map 的键序会被 encoding/json 排成字典序，那不是 fieldOrder）。
	// 直接在原始字节上取 data 段的键出现次序比对，不经 map。
	assert.Equal(t, fieldOrderOf(t, contractObs().Values), dataKeyOrder(t, a),
		"data 段按 fieldOrder 序，不是字典序")
	assert.True(t, len(a) > 0 && a[len(a)-1] == '\n', "JSON 以换行结尾")
	assert.Contains(t, string(a), "\n  \"schema_version\"", "MarshalIndent 两空格")
}

// fieldOrderOf 按 fieldOrder 序列出 values 里存在的字段名。
func fieldOrderOf(t *testing.T, values map[string]float64) []string {
	t.Helper()
	var out []string
	for _, f := range fieldOrder {
		if _, ok := values[f]; ok {
			out = append(out, f)
		}
	}
	return out
}

// dataKeyOrder 取出侧车 JSON 里第一条 same_type 的 data 段键的出现次序。
//
// 用流式 Decoder 而不是 map：map 的键序被 encoding/json 排成字典序，而这条断言
// 要看的恰恰是原始出现次序——解成 map 再断言等于把被测性质本身丢掉了。
func dataKeyOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		require.NoError(t, err, "侧车里应有 data 段")
		if s, ok := tok.(string); ok && s == "data" {
			break
		}
	}
	tok, err := dec.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), tok)
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, k.(string))
		var v float64
		require.NoError(t, dec.Decode(&v))
	}
	return keys
}

func TestWriteHistoryLandsInPending(t *testing.T) {
	dir := t.TempDir()
	h := ContractHistory{SchemaVersion: "1.0", For: "2026-01-monthly", GeneratedBy: "contract@v1", SameType: []HistoryEntry{}}
	path, err := WriteHistory(dir, h)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "pending", "2026-01-monthly.history.json"), path)
	_, err = os.Stat(path)
	require.NoError(t, err)

	// 同名覆盖（需求 line 97/382 与 spec §7）：修订到达时旧侧车还没被消费，最新的赢。
	// 内容不同的第二份写同一个 dir ⇒ 路径不变、内容被完整替换、不留 .tmp 中间文件。
	h2 := h
	h2.GeneratedBy = "contract@v1/replay"
	h2.SameType = []HistoryEntry{{
		Meta: historyMeta{Period: "2025-12", PeriodType: "monthly"},
		Data: orderedValues{{Key: FieldM2, Value: 321.5}},
	}}
	path2, err := WriteHistory(dir, h2)
	require.NoError(t, err)
	require.Equal(t, path, path2, "同名覆盖：路径不变")
	got, err := os.ReadFile(path2)
	require.NoError(t, err)
	want, err := h2.JSON()
	require.NoError(t, err)
	assert.Equal(t, want, got, "第二份完整覆盖第一份，不是追加、不是残留")
	assert.NotContains(t, string(got), "contract@v1\"", "旧内容不得残留")
	entries, err := os.ReadDir(filepath.Join(dir, "pending"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "只剩最终文件")
	assert.Equal(t, "2026-01-monthly.history.json", entries[0].Name(), "不残留 .tmp")

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	_, err = WriteHistory(blocker, h)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history")
}

// JSON 失败：orderedValues.MarshalJSON 对 NaN 报 unsupported value。侧车覆盖率贴地，
// 这条错误返回没有测试走到会让整包覆盖率跌破 DoD 下限。
func TestWriteHistoryJSONFailure(t *testing.T) {
	h := ContractHistory{SchemaVersion: "1.0", For: "2026-01-monthly", SameType: []HistoryEntry{
		{Data: orderedValues{{Key: FieldM2, Value: math.NaN()}}},
	}}
	_, err := h.JSON()
	require.Error(t, err, "前置：NaN 让 MarshalIndent 失败")

	_, err = WriteHistory(t.TempDir(), h)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history 2026-01-monthly.history.json",
		"错误要点名是哪份侧车序列化不出来")
}

// 原子写失败：把目标文件名预建成目录，让 writeAtomic 的 os.Rename 返回 EISDIR。
// 不依赖权限位——root 下跑也不会假绿（同 TestIngestContractWriteFailureKeepsRowAndSkipsP2）。
func TestWriteHistoryAtomicRenameFailure(t *testing.T) {
	dir := t.TempDir()
	h := ContractHistory{SchemaVersion: "1.0", For: "2026-01-monthly", GeneratedBy: "contract@v1", SameType: []HistoryEntry{}}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pending", h.FileName()), 0o755))

	_, err := WriteHistory(dir, h)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history 2026-01-monthly.history.json")
}

// same_type 查询失败 ⇒ 整个 BuildHistory 失败、错误带 history <for> 前缀。关库制造查询错误。
func TestBuildHistoryPropagatesSameTypeQueryError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.Close())
	target := contractObs()
	target.Meta.Period, target.Meta.PeriodType = "2026-01", "monthly"

	_, err := BuildHistory(ctx, s, target, "contract@v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history 2026-01-monthly")
	assert.Contains(t, err.Error(), "hestia store preceding", "底层错误链保留")
}

// monthly_recent 查询失败 ⇒ 同样整体失败。这是 BuildHistory 里**第二处** Preceding，
// 与上一条不是同一个返回点：同类型那次必须成功，才走得到这里。
//
// 手法：先正常放 h1 与 monthly，再用裸连接把 monthly 那些行的一个数值列改成文本，
// 使 scanObservation 的 sql.NullFloat64 扫描失败。h1 那行不动 ⇒ 第一次查询照旧成功。
func TestBuildHistoryPropagatesMonthlyQueryError(t *testing.T) {
	ctx := context.Background()
	s, path := newTestStoreAt(t)
	seedMonths(t, s, 3)
	h1 := contractObs()
	h1.Meta.Period, h1.Meta.PeriodType, h1.Meta.PublishedAt, h1.Meta.ArticleID = "2024-06", "h1", "2024-07-15", "2024071500000000000"
	_, err := s.Save(ctx, h1, passing())
	require.NoError(t, err)

	db := rawDB(t, path)
	_, err = db.ExecContext(ctx,
		"UPDATE "+TableObservations+" SET "+FieldM2+" = 'not-a-number' WHERE period_type = 'monthly'")
	require.NoError(t, err)

	target := contractObs()
	target.Meta.Period, target.Meta.PeriodType = "2025-06", "h1"
	_, err = BuildHistory(ctx, s, target, "contract@v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history 2025-06-h1")
	assert.Contains(t, err.Error(), "hestia store preceding 2025-06/monthly",
		"失败的是 monthly_recent 那次查询，不是同类型那次")
}

// 业务字段名字面量只允许出现在 fields.go 与 _test.go（Global Constraints）：
// history.go 一律经 fieldOrder 遍历取字段，源码里不得写死任何业务字段名。
func TestHistorySourceHasNoFieldLiterals(t *testing.T) {
	src, err := os.ReadFile("history.go")
	require.NoError(t, err)
	for _, f := range fieldOrder {
		assert.NotContains(t, string(src), `"`+f+`"`,
			"history.go 不得出现业务字段名字面量 %s——字段一律经 fieldOrder 取得", f)
	}
}
