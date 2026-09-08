package hestia

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ContractHistory 是契约的侧车（M3 的 TASK-001）：消费者需要的前 12 期序列。
//
// 不写进契约本体：契约是「这一期是什么」的事实，历史是查询结果，混在一起会让 done/ 里
// 的契约随后续入库而过时。与契约同源（同一次 Save 之后）、同时生成、同名前缀。
type ContractHistory struct {
	SchemaVersion string         `json:"schema_version"`
	For           string         `json:"for"`
	GeneratedBy   string         `json:"generated_by"`
	SameType      []HistoryEntry `json:"same_type"`
	MonthlyRecent []HistoryEntry `json:"monthly_recent,omitzero"` // period_type == monthly 时省略
}

// —— 为什么 monthly_recent 是 omitzero 而不是 omitempty ——
//
// 这个字段要同时满足两件事：monthly 时该键**不出现**（同类型序列就是 monthly 序列，
// 重复一遍没有信息）、非 monthly 时该键**恒出现**（哪怕库里一个 monthly 都没有，消费者
// 拿到的应是空序列 `[]` 而不是 undefined —— 「查不到」与「这个字段不适用」对消费者
// 是两回事）。
//
// omitempty 做不到后一半：它省略的是 len == 0 的 slice，nil 与 `[]HistoryEntry{}`
// 一视同仁。annual 入库而库里还没有任何 monthly 时（首次跑就是这个形态），侧车会
// 干脆没有 monthly_recent 键。omitzero 只省略零值（nil），空 slice 照常序列化成 `[]`
// —— 于是「不适用」由 nil 表达、「查不到」由 `[]` 表达，两者不再同形。
//
// 需求原文写的是 omitempty；原文自己的接线断言 `assert.Contains(t, h, "monthly_recent")`
// 在 annual 夹具（库里无 monthly）下必红，两者不能同时成立。取 omitzero 是因为它让
// 两条断言都成立，而改断言会把「非 monthly 恒有此键」这条对消费者的承诺一并丢掉。
// omitzero 需 Go 1.24+，本仓库 go.mod 是 1.24.4。

// HistoryEntry 是一条 current 行：Meta 七字段（snake_case）+ 非空 Values（按 fieldOrder 序）。
type HistoryEntry struct {
	Meta historyMeta   `json:"meta"`
	Data orderedValues `json:"data"`
}

// historyMeta 与 Meta 逐字段同名同型同序，只有 tag 不同（Meta 不带 json tag，它不进 JSON）。
// Go 的结构体转换忽略 tag，故 toEntries 里可以直接 historyMeta(o.Meta) —— 逐字段抄一遍
// 会让两个同为 string 的字段（如 Extractor / IngestedAt）抄反时静默通过。
// Meta 增删字段或改顺序会让那次转换**编译**报错，这是刻意的：JSON 键序由本结构体定住。
type historyMeta struct {
	Period         string `json:"period"`
	PeriodType     string `json:"period_type"`
	PublishedAt    string `json:"published_at"`
	ArticleID      string `json:"article_id"`
	CaliberVersion string `json:"caliber_version"`
	Extractor      string `json:"extractor"`
	IngestedAt     string `json:"ingested_at"`
}

const historyWindow = 12

// BuildHistory 查库组装侧车。同 period_type 的前 12 期；非 monthly 再附最近 12 个 monthly
// （半年报的「前 12 期」是 12 年，方法论里的「连续第 N 个月」靠后者）。
//
// ⚠️ Preceding 的两条射程限制（见其注释）在这里都不构成问题：侧车交付的是「近期已被
// 接受的同类期次」这个序列本身，不做相邻性推断。消费者若要算环比，得自己核对期次跨度。
func BuildHistory(ctx context.Context, s *Store, obs Observation, generatedBy string) (ContractHistory, error) {
	h := ContractHistory{
		SchemaVersion: "1.0",
		For:           obs.Meta.Period + "-" + obs.Meta.PeriodType,
		GeneratedBy:   generatedBy,
		SameType:      []HistoryEntry{},
	}
	same, err := s.Preceding(ctx, obs.Meta.Period, obs.Meta.PeriodType, historyWindow)
	if err != nil {
		return ContractHistory{}, fmt.Errorf("history %s: %w", h.For, err)
	}
	h.SameType = toEntries(same)
	// "monthly" 是 period_type 不是业务字段名，不受字段名守卫约束（signals.go 已有先例）。
	if obs.Meta.PeriodType != "monthly" {
		recent, err := s.Preceding(ctx, obs.Meta.Period, "monthly", historyWindow)
		if err != nil {
			return ContractHistory{}, fmt.Errorf("history %s: %w", h.For, err)
		}
		h.MonthlyRecent = toEntries(recent)
	}
	return h, nil
}

// toEntries 把观测序列转成侧车项。恒返回非 nil slice —— 空序列要序列化成 `[]`，
// 而 monthly_recent 的「不适用」靠 nil 表达（见上面 omitzero 那段）。
func toEntries(obs []Observation) []HistoryEntry {
	out := make([]HistoryEntry, 0, len(obs))
	for _, o := range obs {
		e := HistoryEntry{Meta: historyMeta(o.Meta), Data: orderedValues{}}
		// 遍历 fieldOrder 而不是 o.Values：既定住键序（map 会被排成字典序，那不是
		// fieldOrder），也让本文件不出现任何业务字段名字面量。
		for _, f := range fieldOrder {
			if v, ok := o.Values[f]; ok {
				e.Data = append(e.Data, keyValue{f, v})
			}
		}
		out = append(out, e)
	}
	return out
}

func (h ContractHistory) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (h ContractHistory) FileName() string { return h.For + ".history.json" }

// WriteHistory 写 <dir>/pending/<for>.history.json，原子、同名覆盖（同 WriteContract）。
func WriteHistory(dir string, h ContractHistory) (string, error) {
	b, err := h.JSON()
	if err != nil {
		return "", fmt.Errorf("history %s: %w", h.FileName(), err)
	}
	pending := filepath.Join(dir, "pending")
	if err := os.MkdirAll(pending, 0o755); err != nil {
		return "", fmt.Errorf("history queue dir %s: %w", dir, err)
	}
	path := filepath.Join(pending, h.FileName())
	if err := writeAtomic(path, b); err != nil {
		return "", fmt.Errorf("history %s: %w", h.FileName(), err)
	}
	return path, nil
}
