package hestia

import "encoding/json"

// Contract 是契约 JSON 的内存形态（方案报告 5.2；M2a 的 TASK-003）。
//
// 用结构体而不是 map：键序固定，同一观测两次生成逐字节相同，done/ 里的文件能与
// 重放结果 cmp。data 与 absent_fields 按 fieldOrder 序。
//
// 字段顺序就是契约顶层键序（spec §2.2），由 TestContractJSONTopLevelKeyOrder 钉住——
// 重排字段会静默改输出。
type Contract struct {
	SchemaVersion         string             `json:"schema_version"`
	Period                string             `json:"period"`
	PeriodType            string             `json:"period_type"`
	PublishedAt           string             `json:"published_at"`
	SourceURL             string             `json:"source_url"`
	ArticleID             string             `json:"article_id"`
	CaliberVersion        string             `json:"caliber_version"`
	IsRevision            bool               `json:"is_revision"`
	SupersedesPublishedAt *string            `json:"supersedes_published_at"`
	ExtractedAt           string             `json:"extracted_at"`
	Extractor             string             `json:"extractor"`
	GeneratedBy           string             `json:"generated_by"`
	AbsentFields          []string           `json:"absent_fields"`
	Units                 map[string]string  `json:"units"`
	Validation            ContractValidation `json:"validation"`
	Thresholds            ContractThresholds `json:"thresholds"`
	Data                  orderedValues      `json:"data"`
}

// ContractValidation 是 validation 段：Passed 与逐项 checks（回放时 Checks 为空数组，不是 null）。
type ContractValidation struct {
	Passed bool            `json:"passed"`
	Checks []ContractCheck `json:"checks"`
}

// ContractCheck 是一条闸门结论；Value 无意义时写 null，Reason 只在 skipped 时出现。
type ContractCheck struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Value  *float64 `json:"value"`
	Reason string   `json:"reason,omitempty"`
}

// ContractThresholds 是阈值快照：Loom 从这里读，不读 configs/hestia.yaml（方案报告 5.3）。
// temp_scale 在这一层写一份，Signals 里的同名字段是 json:"-"（TASK-001，AD-6）。
type ContractThresholds struct {
	ConfigVersion string  `json:"config_version"`
	TempScale     string  `json:"temp_scale"`
	Signals       Signals `json:"signals"`
}

// orderedValues 按 fieldOrder 序输出的 data 段。map 会按键排序，那不是 fieldOrder。
type orderedValues []keyValue

type keyValue struct {
	Key   string
	Value float64
}

func (o orderedValues) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, kv := range o {
		if i > 0 {
			buf = append(buf, ',')
		}
		k, _ := json.Marshal(kv.Key)
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf = append(buf, k...)
		buf = append(buf, ':')
		buf = append(buf, v...)
	}
	return append(buf, '}'), nil
}

// ContractInput 是生成一份契约需要的全部输入。
type ContractInput struct {
	Obs        Observation
	Report     ValidationReport // 回放时 Passed=true、Checks 为空
	IsRevision bool
	Supersedes string // 被取代的 published_at；"" ⇒ null
	SourceURL  string // "" ⇒ 由 ArticleID 拼
	Replay     bool   // generated_by 带 /replay
}

const (
	contractSchemaVersion = "1.0"
	contractGenerator     = "contract@v1"
	pbocArticleBase       = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/"
)

// pbocArticleURL 由 article_id 拼回文章 URL（与 discover.go 的链接形状同源）。
// 需求原文叫 articleURL，与 ingest_test.go 既有的测试 helper 同名同签名会 redeclared，故改名。
func pbocArticleURL(articleID string) string {
	return pbocArticleBase + articleID + "/index.html"
}

// BuildContract 是纯函数：不读库、不做 I/O。
func BuildContract(in ContractInput, cfg Config) Contract {
	c := Contract{
		SchemaVersion:  contractSchemaVersion,
		Period:         in.Obs.Meta.Period,
		PeriodType:     in.Obs.Meta.PeriodType,
		PublishedAt:    in.Obs.Meta.PublishedAt,
		SourceURL:      in.SourceURL,
		ArticleID:      in.Obs.Meta.ArticleID,
		CaliberVersion: in.Obs.Meta.CaliberVersion,
		IsRevision:     in.IsRevision,
		ExtractedAt:    in.Obs.Meta.IngestedAt,
		Extractor:      in.Obs.Meta.Extractor,
		GeneratedBy:    contractGenerator,
		AbsentFields:   []string{},
		Units:          map[string]string{"balance": "万亿元", "flow": "亿元", "ratio": "百分数"},
		Validation:     ContractValidation{Passed: in.Report.Passed, Checks: []ContractCheck{}},
		Thresholds: ContractThresholds{
			ConfigVersion: cfg.ConfigVersion, TempScale: cfg.Signals.TempScale, Signals: cfg.Signals,
		},
		Data: orderedValues{},
	}
	if c.SourceURL == "" {
		c.SourceURL = pbocArticleURL(in.Obs.Meta.ArticleID)
	}
	if in.Supersedes != "" {
		s := in.Supersedes
		c.SupersedesPublishedAt = &s
	}
	if in.Replay {
		c.GeneratedBy += "/replay"
	}
	for _, f := range fieldOrder {
		if v, ok := in.Obs.Values[f]; ok {
			c.Data = append(c.Data, keyValue{f, v})
		} else {
			c.AbsentFields = append(c.AbsentFields, f)
		}
	}
	for _, chk := range in.Report.Checks {
		c.Validation.Checks = append(c.Validation.Checks, ContractCheck{
			ID: chk.ID, Status: string(chk.Status), Value: chk.Value, Reason: chk.Reason,
		})
	}
	return c
}

// JSON 是契约的字节形态：两空格缩进、固定键序、末尾换行。
func (c Contract) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// FileName 带 period_type：12 月的月报与年报 period 都是 YYYY-12，不带会撞名。
func (c Contract) FileName() string {
	return c.Period + "-" + c.PeriodType + ".json"
}
