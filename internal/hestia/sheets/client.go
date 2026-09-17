package sheets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
)

// 录入区的几何：表头第 3 行；数据行 4–15（1 月 = 第 4 行）；列 A–AI（C6：程序只碰这 35 列）。
const (
	headerRow     = 3
	entryFirstRow = 4
	entryLastRow  = 15
	entryLastCol  = 34 // AI
	valueInputRAW = "RAW"
	// valueRenderRaw 让 values.get 回**原始值**而不是格式化文本。
	//
	// 🔴 不显式要它就是 FORMATTED_VALUE（pinned 模块 sheets-gen.go:11695 的默认值）：
	// 「462.06」会变成字符串、「255800」会变成 "255,800"、同比会带 % 号、负数会变
	// "(933)"。而库里是 float64 ⇒ diff 每次都判不一致 ⇒ 幂等永远达不成、每次 --apply
	// 全量重写、dry-run 的「一致跳过」恒为 0。写入侧的 RAW 是另一回事，不要混。
	valueRenderRaw = "UNFORMATTED_VALUE"
	rangeFormat    = "'%s'!%s%d"                        // 表名**无条件**单引号：不包的话中文表名的 A1 记法解析不了
	propsField     = "sheets.properties(sheetId,title)" // Tabs / CreateYearTab 只要标题与 id，不拉整张表的格
)

// Client 是 Google Sheets 的薄壳：只会读表头、读录入区、列工作表、批量写格。
// 不认识数据库（C2）；调用方把纯值交进来。
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
}

// Option 只为测试开一个口子：把 endpoint 指向 httptest.Server。
type Option func(*[]option.ClientOption)

// WithEndpoint 把 API 根地址换成 url（测试用 httptest 起的真 HTTP 服务）。
func WithEndpoint(url string) Option {
	return func(opts *[]option.ClientOption) { *opts = append(*opts, option.WithEndpoint(url)) }
}

// NewClient 建 Sheets 客户端。credentialsFile 留空 ⇒ 返回 (nil, nil)，表示能力禁用（C9）；
// 文件不存在则是配置错误，报错。
//
// 出网用**默认 transport**（走 ProxyFromEnvironment）。Sheets 在境外，与
// hestia/fetch.go 对 PBOC 的直连策略相反——别照抄那边的空 Transport{}，
// 那会让本包直连境外 API 而失败（约束 C7 要求同进程内三种策略）。
func NewClient(ctx context.Context, credentialsFile, spreadsheetID string, opts ...Option) (*Client, error) {
	if credentialsFile == "" {
		return nil, nil // C9：能力禁用，不是错误
	}
	if _, err := os.Stat(credentialsFile); err != nil {
		return nil, fmt.Errorf("hestia sheets: 凭据文件 %s: %w", credentialsFile, err)
	}
	apiOpts := []option.ClientOption{option.WithCredentialsFile(credentialsFile)}
	for _, o := range opts {
		o(&apiOpts)
	}
	svc, err := sheets.NewService(ctx, apiOpts...)
	if err != nil {
		return nil, fmt.Errorf("hestia sheets: 建客户端: %w", err)
	}
	return &Client{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// Tabs 列出全部工作表名（按表序）。TASK-008 判缺表用。
func (c *Client) Tabs(ctx context.Context) ([]string, error) {
	props, err := c.sheetProps(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(props))
	for _, p := range props {
		out = append(out, p.Title)
	}
	return out, nil
}

// sheetProps 取全部工作表的 (sheetId, title)，按表序。
func (c *Client) sheetProps(ctx context.Context) ([]*sheets.SheetProperties, error) {
	ss, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Fields(propsField).Context(ctx).Do()
	if err != nil {
		return nil, wrapErr("列工作表", err)
	}
	out := make([]*sheets.SheetProperties, 0, len(ss.Sheets))
	for _, s := range ss.Sheets {
		if s.Properties != nil {
			out = append(out, s.Properties)
		}
	}
	return out, nil
}

// ReadHeader 读某张年度表的第 3 行（A3:AI3），原样返回单元格文本。
func (c *Client) ReadHeader(ctx context.Context, tab string) ([]string, error) {
	rows, err := c.read(ctx, tab, headerRow, headerRow)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]string, len(rows[0]))
	for i, v := range rows[0] {
		out[i] = fmt.Sprint(v)
	}
	return out, nil
}

// ReadEntryArea 读某张年度表的录入区（行 4–15，列 A–AI）。索引 0 = 1 月。
// Sheets API 会截掉尾部空行/空格，短行原样交出，由 Diff 按无值处理。
func (c *Client) ReadEntryArea(ctx context.Context, tab string) ([][]any, error) {
	return c.read(ctx, tab, entryFirstRow, entryLastRow)
}

func (c *Client) read(ctx context.Context, tab string, fromRow, toRow int) ([][]any, error) {
	rng := fmt.Sprintf("'%s'!A%d:%s%d", tab, fromRow, ColumnLetter(entryLastCol), toRow)
	vr, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).
		ValueRenderOption(valueRenderRaw).Context(ctx).Do()
	if err != nil {
		return nil, wrapErr("读 "+rng, err)
	}
	return vr.Values, nil
}

// WriteCells 用一次 values:batchUpdate 提交全部变更，valueInputOption=RAW（C5、C11）。
// 每个格一个 range；值原样发——数值列必须是 float64，发字符串会让那格变文本。
// 不按 Kind 过滤：只提交哪些格是 push 层的决定，这里给什么写什么。
func (c *Client) WriteCells(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	data := make([]*sheets.ValueRange, 0, len(changes))
	for _, ch := range changes {
		data = append(data, &sheets.ValueRange{
			Range:  fmt.Sprintf(rangeFormat, ch.Sheet, ColumnLetter(ch.Col), ch.Row),
			Values: [][]any{{ch.Want}},
		})
	}
	req := &sheets.BatchUpdateValuesRequest{ValueInputOption: valueInputRAW, Data: data}
	if _, err := c.svc.Spreadsheets.Values.BatchUpdate(c.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return wrapErr(fmt.Sprintf("批量写 %d 格", len(changes)), err)
	}
	return nil
}

// wrapErr 给 API 错误加上动作与 HTTP 状态：4xx/5xx 不能被吞成一句「失败」。
func wrapErr(action string, err error) error {
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		return fmt.Errorf("hestia sheets: %s: HTTP %d %s: %w", action, ge.Code, http.StatusText(ge.Code), err)
	}
	return fmt.Errorf("hestia sheets: %s: %w", action, err)
}

// titleYear 匹配年度表第 1 行标题开头的年份，如「2024 年 · 金融数据追踪」里的「2024 年」。
var titleYear = regexp.MustCompile(`^\s*\d{4}\s*年`)

// CreateYearTab 建一张新的年度表：复制模板 → 改名 → 改标题行 → 按年序排位。
//
// **复制模板而不是按表头程序生成**（M2b-1 的 D3）：模板表结构完整且录入区全空，
// 复制自带 54 列表头、单位行、AJ–BB 的 19 个公式与格式，且公式的相对引用会自动
// 落到新表自己的行上。按表头生成做不到这一点——那 19 个公式是人维护的分析逻辑，
// 重写代价高且易错。
//
// 四步缺一不可：
//
//	① DuplicateSheet —— API 默认给新表起名「<模板> 的副本」
//	② 改名 —— 不改的话选表逻辑按「2021年」找不到它
//	③ 改表内第 1 行标题 —— 复制来的还写着模板的年份
//	④ index 排位 —— 不给的话新表堆在末尾，2019 排在 2026 后面
//
// ②③ 两处都漏的话，表格看起来就像重复了五张 2024，而每张的数据都是对的——
// 最难察觉的那种错。
//
// 四步放在**一次** spreadsheets:batchUpdate 里，API 保证原子：任一步失败整批不生效，
// 不会留下一张「2024年 的副本」。新表的 sheetId 由本方指定（现有最大 id + 1），
// 这样 ②③④ 在同一批里就能引用它。
func (c *Client) CreateYearTab(ctx context.Context, templateTab, newTab string, year int, index int) error {
	props, err := c.sheetProps(ctx)
	if err != nil {
		return err
	}
	var tplID, maxID int64 = -1, -1
	for _, p := range props {
		if p.Title == templateTab {
			tplID = p.SheetId
		}
		maxID = max(maxID, p.SheetId)
	}
	if tplID < 0 {
		return fmt.Errorf("hestia sheets: 模板表 %q 不存在，无法建 %q", templateTab, newTab)
	}
	newID := maxID + 1

	title, err := c.readTitle(ctx, templateTab)
	if err != nil {
		return err
	}
	// 模板标题带年份（「2024 年 · 金融数据追踪」）⇒ 只换年份，保留后半句；
	// 不带年份（含空标题）⇒ 拼一个「<year> 年 · <原标题>」。
	var newTitle string
	if titleYear.MatchString(title) {
		newTitle = titleYear.ReplaceAllString(title, fmt.Sprintf("%d 年", year))
	} else {
		newTitle = fmt.Sprintf("%d 年 · %s", year, title)
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{
		// ① 复制模板
		{DuplicateSheet: &sheets.DuplicateSheetRequest{SourceSheetId: tplID, NewSheetId: newID}},
		// ② 改名
		{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: newID, Title: newTab},
			Fields:     "title",
		}},
		// ③ 改表内第 1 行标题
		{UpdateCells: &sheets.UpdateCellsRequest{
			Range:  &sheets.GridRange{SheetId: newID, StartRowIndex: 0, EndRowIndex: 1, StartColumnIndex: 0, EndColumnIndex: 1},
			Rows:   []*sheets.RowData{{Values: []*sheets.CellData{{UserEnteredValue: &sheets.ExtendedValue{StringValue: &newTitle}}}}},
			Fields: "userEnteredValue",
		}},
		// ④ 按年序排位。index 0 是「排最前」，omitempty 会把它吞掉，必须 ForceSendFields
		{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: newID, Index: int64(index), ForceSendFields: []string{"Index"}},
			Fields:     "index",
		}},
		// ⑤ 清空**新表**的录入区（行 4–15 × 列 A–AI）。
		//
		// 🔴 模板表与数据表是同一张：templateYearTab 是 "2024年"，而 tabName(2024) 也是
		// "2024年"。首次 --apply 会把「模板」填满，于是明年 1 月跨年建表时复制到的是一张
		// **带数据**的表——新年度表 2–12 月带着上一年的数字，而库里没有对应月份的行
		// **不会被 Diff 触碰**（Diff 只遍历 rows），所以那些假数字会一直留在表里。
		// ingest 固定 Apply+CreateSheets ⇒ 这件事会自动发生且没有任何告警。
		//
		// ⚠️ 清的是**刚 duplicate 出来的新表**（SheetId 是 newID，不是 tplID），
		// 而且只清录入区、不碰表头（行 1–3）与计算区 AJ–BB 的 19 个公式。
		// 新表此刻还没有任何人工数据可言——它这一秒才被复制出来。
		{UpdateCells: &sheets.UpdateCellsRequest{
			Range: &sheets.GridRange{
				SheetId:          newID,
				StartRowIndex:    int64(entryFirstRow - 1),
				EndRowIndex:      int64(entryLastRow),
				StartColumnIndex: 0,
				EndColumnIndex:   int64(entryLastCol + 1),
				ForceSendFields:  []string{"StartRowIndex", "StartColumnIndex"},
			},
			Fields: "userEnteredValue",
		}},
	}}
	if _, err := c.svc.Spreadsheets.BatchUpdate(c.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return wrapErr(fmt.Sprintf("建年度表 %s（复制 %s）", newTab, templateTab), err)
	}
	return nil
}

// readTitle 读某张表第 1 行第 1 格（表内标题）。空 ⇒ ""。
func (c *Client) readTitle(ctx context.Context, tab string) (string, error) {
	rng := fmt.Sprintf("'%s'!A1", tab)
	vr, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).
		ValueRenderOption(valueRenderRaw).Context(ctx).Do()
	if err != nil {
		return "", wrapErr("读 "+rng, err)
	}
	if len(vr.Values) == 0 || len(vr.Values[0]) == 0 {
		return "", nil
	}
	return fmt.Sprint(vr.Values[0][0]), nil
}
