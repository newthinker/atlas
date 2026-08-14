package hestia

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// manifestFileName 与 articlesDirName 是 design-spec §8 定的产物布局：
//
//	<out>/manifest.json
//	<out>/articles/<article_id>.html
const (
	manifestFileName = "manifest.json"
	articlesDirName  = "articles"
)

// Manifest 是一次回填的全部产出清单，落盘为 <out>/manifest.json。
//
// 字段与 design-spec §6 的 JSON 形状**逐字一致**——这些 json tag 是与 M1c-2/-3
// 的契约（它们只读这份文件，不链接本包），改名等于改契约。
//
// 时间字段一律用字符串：ScannedAt / FetchedAt 是 RFC3339（`2026-08-14T10:00:00Z`），
// Published 是发布日期 `YYYY-MM-DD`。与本包既有的 Meta 同一约定——同一份结构里
// 混用 time.Time 与字符串日期，读的人得逐字段记住哪个是哪个。
type Manifest struct {
	From               string    `json:"from"`
	ScannedAt          string    `json:"scanned_at"`
	PagesScanned       int       `json:"pages_scanned"`
	SearchPagesScanned int       `json:"search_pages_scanned"`
	Articles           []Article `json:"articles"`
	Failed             []Failed  `json:"failed"`
	OnlyInIndex        []string  `json:"only_in_index"`
	OnlyInSearch       []string  `json:"only_in_search"`

	// SearchSkippedReason 非空 ⇒ 这一轮**根本没做**交叉校验（搜索侧失效，fail-open）。
	//
	// ⚠️ 它必须落进 manifest，不能只打一行 WARN（ADR-M1c1-02）：manifest 的读者是
	// M1c-2/-3，没有这个字段时「这次没做校验」与「校验通过」在他们看来完全一样
	// ——那正是本机制要防的失效模式本身。
	//
	// `omitempty`：正常完成时它不出现，于是「字段在不在」本身就是信号。
	SearchSkippedReason string `json:"search_skipped_reason,omitempty"`
}

// Article 是一篇已抓到并落盘的文章。
//
// File 是**相对 manifest 所在目录**的路径（`articles/<id>.html`），这样整个产物
// 目录可以整体挪走、整体 cp -r 备份而不失效。
//
// SHA256 是 File 内容的 sha256 十六进制小写，**只在首次抓取该篇时计算**
// （design-spec-addendum §A2）。它的用途是让下游验证本地文件未被篡改/截断，
// **不是**发现站点改版——本迭代「已抓就跳过、不重发请求」，压根不会重抓来比对。
//
// Source 取 `index` / `search` / `both`，记这一篇是被哪一侧扫描发现的。
type Article struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Published string `json:"published"`
	URL       string `json:"url"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	FetchedAt string `json:"fetched_at"`
	Source    string `json:"source"`
}

// Failed 是一篇抓取失败的记录。单篇失败不中止整趟回填，记下来继续。
type Failed struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

// manifestStore 是 manifest 的读写通道，一次回填一个实例。
//
// Manifest 字段可以直接改（如 PagesScanned），改完调 Save 落盘；追加文章/失败
// 记录走 AppendArticle / AppendFailed，它们**每次都立刻整份落盘**。
type manifestStore struct {
	// Manifest 是当前内存态。直接改字段后需自行调用 Save。
	Manifest Manifest

	dir string
}

// loadManifest 打开 dir 下的 manifest。
//
// 文件不存在 ⇒ 空 Manifest + nil error，这是首跑的正常路径。
// 文件存在但内容不是合法 JSON ⇒ **报错**，绝不静默当空：静默当空会让断点续抓
// 退化成全量重抓 400 次请求，而且退出码还是 0。
func loadManifest(dir string) (*manifestStore, error) {
	p := filepath.Join(dir, manifestFileName)
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &manifestStore{dir: dir}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读 %s: %w", p, err)
	}
	// 字面量 `null` 单独挡一道：它是**合法 JSON**，Unmarshal 会成功并留下一个零值
	// Manifest ⇒ 走到下面就是「0 篇已抓」，断点续抓静默退化成全量重抓 400+ 次请求
	// 且不报错。触发面窄（截断写产不出 null，jq 管道误操作可能），但代价与
	// 「manifest 丢了」等同，而它偏偏不走错误分支。
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("解析 %s: 内容是 JSON null —— 既不是「文件不存在」（首跑）也不是有效 manifest，拒绝当空处理", p)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", p, err)
	}
	return &manifestStore{Manifest: m, dir: dir}, nil
}

// Path 返回 manifest.json 的完整路径。
func (s *manifestStore) Path() string { return filepath.Join(s.dir, manifestFileName) }

// AppendArticle 追加一篇并**立刻**把整份 manifest 落盘。
//
// 不是攒到最后写：400+ 请求中途断掉（网络、Ctrl-C）不该让前面的白抓，
// 这是断点续抓的前提。
func (s *manifestStore) AppendArticle(a Article) error {
	s.Manifest.Articles = append(s.Manifest.Articles, a)
	return s.Save()
}

// AppendFailed 追加一条失败记录并立刻落盘，理由同 AppendArticle。
func (s *manifestStore) AppendFailed(f Failed) error {
	s.Manifest.Failed = append(s.Manifest.Failed, f)
	return s.Save()
}

// Save 原子地把整份 manifest 写到磁盘：先写同目录下的临时文件，再 rename。
//
// 写失败时磁盘上那份仍是之前的完整内容——直接 O_TRUNC 打开原文件的话，
// 磁盘满/进程被杀会留下一份截断的 JSON，而那份 JSON 恰好会让下次 loadManifest
// 报错，整趟回填的记录就此作废。
func (s *manifestStore) Save() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("建产物目录 %s: %w", s.dir, err)
	}
	raw, err := json.MarshalIndent(withEmptySlices(s.Manifest), "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 manifest: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(s.dir, manifestFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("建临时文件于 %s: %w", s.dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后这次 Remove 无害地失败

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("写 %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		return fmt.Errorf("落盘 %s: %w", s.Path(), err)
	}
	return nil
}

// HasArticle 回答「这篇已经抓过了吗」——**id 在 articles 里且对应文件在磁盘上存在**。
//
// 两个条件缺一不可：只查 manifest，文件被删后会跳过重抓、下游拿到一个不存在的
// 路径；只查文件，manifest 丢了会重复记录同一篇。
func (s *manifestStore) HasArticle(id string) bool {
	for _, a := range s.Manifest.Articles {
		if a.ID != id {
			continue
		}
		if a.File == "" {
			return false
		}
		if _, err := os.Stat(filepath.Join(s.dir, filepath.FromSlash(a.File))); err != nil {
			return false
		}
		return true
	}
	return false
}

// articleFile 返回一篇文章相对产物根目录的落盘路径，与 Article.File 同一口径。
// 用 path 而非 filepath：这个值要写进 JSON 给下游读，分隔符固定是 /。
func articleFile(id string) string { return path.Join(articlesDirName, id+".html") }

// articleSHA256 返回内容的 sha256 十六进制小写。
func articleSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// withEmptySlices 把 nil 切片换成空切片，好让首跑落盘的是 `[]` 而不是 `null`。
// 在副本上做——调用方看到的内存态不该因为「存了一次盘」而变。
func withEmptySlices(m Manifest) Manifest {
	if m.Articles == nil {
		m.Articles = []Article{}
	}
	if m.Failed == nil {
		m.Failed = []Failed{}
	}
	if m.OnlyInIndex == nil {
		m.OnlyInIndex = []string{}
	}
	if m.OnlyInSearch == nil {
		m.OnlyInSearch = []string{}
	}
	return m
}
