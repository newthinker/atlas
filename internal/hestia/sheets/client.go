package sheets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

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
	rangeFormat   = "'%s'!%s%d"               // 表名**无条件**单引号：不包的话中文表名的 A1 记法解析不了
	titleField    = "sheets.properties.title" // Tabs 只要标题，不拉整张表的格
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

// Tabs 列出全部工作表名。TASK-008 判缺表用。
func (c *Client) Tabs(ctx context.Context) ([]string, error) {
	ss, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Fields(titleField).Context(ctx).Do()
	if err != nil {
		return nil, wrapErr("列工作表", err)
	}
	out := make([]string, 0, len(ss.Sheets))
	for _, s := range ss.Sheets {
		if s.Properties != nil {
			out = append(out, s.Properties.Title)
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
	vr, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
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
