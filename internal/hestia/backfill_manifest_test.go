package hestia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Context Checkpoint: TASK-003 done_criteria → test mapping
// functional[0]  三类型 json tag 与 §6 逐字一致 + 往返    → TestBackfillManifestRoundTrip
//                                                          TestBackfillManifestJSONKeysAreVerbatim
//                                                          TestBackfillManifestSaveWritesEmptyArraysNotNull
// functional[1]  逐篇追加立刻落盘（中途直接读文件）      → TestBackfillManifestAppendPersistsImmediately
//                                                          TestBackfillManifestAppendFailedPersistsImmediately
// functional[2]  sha256 = 内容的十六进制小写（具体值）    → TestBackfillArticleSHA256KnownContent
// functional[3]  「已抓」= id 在 articles 且 file 存在     → TestBackfillManifestHasArticleNeedsBothManifestAndFile
// boundary[0]    manifest 不存在 ⇒ 空 + nil error         → TestBackfillManifestLoadMissingFile
// boundary[1]    manifest 非法 JSON ⇒ 报错，不静默当空    → TestBackfillManifestLoadInvalidJSONErrors
// error_handling[0] 落盘失败 ⇒ 报错且既有 manifest 完好   → TestBackfillManifestSaveFailureKeepsPreviousFile

// fullManifest 是一份**每个字段都非零**的 manifest。
//
// 字段全部填非零值是往返测试的前提：任何一个字段留零值，「写出再读回相等」
// 对它就平凡为真（零值 == 零值），tag 拼错也照样绿。
func fullManifest() Manifest {
	return Manifest{
		From:               "2020-01",
		ScannedAt:          "2026-08-14T10:00:00Z",
		PagesScanned:       152,
		SearchPagesScanned: 82,
		Articles: []Article{{
			ID:        "2025092212551391606",
			Title:     "2021年2月金融统计数据报告",
			Published: "2021-03-10",
			URL:       "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2025092212551391606/index.html",
			File:      "articles/2025092212551391606.html",
			SHA256:    "c066d57274f9b0768ef2621a745713a902c6d05af108df171bf5f13874818d60",
			FetchedAt: "2026-08-14T10:03:22Z",
			Source:    "both",
		}},
		Failed:       []Failed{{ID: "2025092212550638999", URL: "https://www.pbc.gov.cn/x/index.html", Error: "HTTP 404"}},
		OnlyInIndex:  []string{"1111111"},
		OnlyInSearch: []string{"2222222"},
		// 非零值：TASK-006 新增字段，往返测试对留零值的字段平凡为真
		SearchSkippedReason: "search side failed, cross-check skipped: boom",
	}
}

// readManifestFile 绕开被测代码，直接从磁盘读原始 JSON。
//
// 断言「已经落盘」时必须走这条路：调用 loadManifest 会经过被测的读路径，
// 而「缓存在内存里」和「已经写到盘上」在那条路上看起来完全一样。
func readManifestFile(t *testing.T, dir string) Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("直接读 manifest 文件失败: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("磁盘上的 manifest 不是合法 JSON: %v\n内容: %s", err, raw)
	}
	return m
}

func TestBackfillManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	want := fullManifest()
	s.Manifest = want
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	back, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("重新 loadManifest: %v", err)
	}
	if !reflect.DeepEqual(back.Manifest, want) {
		t.Errorf("往返后字段不相等\n写出: %+v\n读回: %+v", want, back.Manifest)
	}
}

// TestBackfillManifestJSONKeysAreVerbatim 钉住 json tag 的**字面拼写**。
//
// 与往返测试互补，不是重复：往返走的是同一份 struct 定义，tag 拼成
// `pages_scaned` 也能自洽地写出再读回。tag 是与下游 M1c-2/-3 的契约，
// 只有把期望的 key 逐字写死才守得住。
func TestBackfillManifestJSONKeysAreVerbatim(t *testing.T) {
	raw, err := json.Marshal(fullManifest())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("Unmarshal 顶层: %v", err)
	}
	assertExactKeys(t, "Manifest", top, []string{
		"from", "scanned_at", "pages_scanned", "search_pages_scanned",
		"articles", "failed", "only_in_index", "only_in_search",
		"search_skipped_reason", // TASK-006 新增；omitempty，fullManifest 里非空故必然出现
	})

	var arts []map[string]json.RawMessage
	if err := json.Unmarshal(top["articles"], &arts); err != nil {
		t.Fatalf("Unmarshal articles: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("articles 应有 1 条，实得 %d", len(arts))
	}
	assertExactKeys(t, "Article", arts[0], []string{
		"id", "title", "published", "url", "file", "sha256", "fetched_at", "source",
	})

	var fails []map[string]json.RawMessage
	if err := json.Unmarshal(top["failed"], &fails); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(fails) != 1 {
		t.Fatalf("failed 应有 1 条，实得 %d", len(fails))
	}
	assertExactKeys(t, "Failed", fails[0], []string{"id", "url", "error"})
}

func assertExactKeys(t *testing.T, what string, got map[string]json.RawMessage, want []string) {
	t.Helper()
	var have []string
	for k := range got {
		have = append(have, k)
	}
	sort.Strings(have)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(have, sorted) {
		t.Errorf("%s 的 JSON key 与 design-spec §6 不一致\n实得: %v\n期望: %v", what, have, sorted)
	}
}

// TestBackfillManifestSaveWritesEmptyArraysNotNull：首跑（一篇都没抓到）时
// 数组字段落盘为 `[]` 而不是 `null`。下游用 jq / Go 读这份文件，null 会让
// `.articles[]` 这类表达式的行为与「抓到 0 篇」不一致。
func TestBackfillManifestSaveWritesEmptyArraysNotNull(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("读 manifest: %v", err)
	}
	for _, key := range []string{"articles", "failed", "only_in_index", "only_in_search"} {
		if !strings.Contains(string(raw), `"`+key+`": []`) {
			t.Errorf("%q 未落盘为空数组\n实际内容: %s", key, raw)
		}
	}
}

// TestBackfillManifestAppendPersistsImmediately 对应 functional[1]。
//
// 中途**直接读文件**（不调用任何 Flush/Close，也不经 loadManifest），
// 因为断点续抓的前提正是「进程被 Ctrl-C 掐死时盘上已经是全的」。
func TestBackfillManifestAppendPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}

	first := Article{ID: "1001", Title: "2020年1月金融统计数据报告", Published: "2020-02-10",
		URL: "https://example.invalid/1001/index.html", File: "articles/1001.html",
		SHA256: "aa", FetchedAt: "2026-08-14T10:00:01Z", Source: "index"}
	if err := s.AppendArticle(first); err != nil {
		t.Fatalf("AppendArticle 第一篇: %v", err)
	}
	got := readManifestFile(t, dir)
	if len(got.Articles) != 1 || got.Articles[0].ID != "1001" {
		t.Fatalf("追加第一篇后磁盘上应有且仅有 1001，实得 %+v", got.Articles)
	}
	if !reflect.DeepEqual(got.Articles[0], first) {
		t.Errorf("落盘的第一篇字段不全\n实得: %+v\n期望: %+v", got.Articles[0], first)
	}

	second := Article{ID: "1002", Title: "2020年2月金融统计数据报告", Published: "2020-03-10",
		URL: "https://example.invalid/1002/index.html", File: "articles/1002.html",
		SHA256: "bb", FetchedAt: "2026-08-14T10:00:02Z", Source: "search"}
	if err := s.AppendArticle(second); err != nil {
		t.Fatalf("AppendArticle 第二篇: %v", err)
	}
	got = readManifestFile(t, dir)
	if len(got.Articles) != 2 {
		t.Fatalf("追加第二篇后磁盘上应有 2 篇，实得 %d 篇: %+v", len(got.Articles), got.Articles)
	}
	if got.Articles[0].ID != "1001" || got.Articles[1].ID != "1002" {
		t.Errorf("落盘顺序应为追加顺序，实得 %s, %s", got.Articles[0].ID, got.Articles[1].ID)
	}
}

func TestBackfillManifestAppendFailedPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	f := Failed{ID: "1003", URL: "https://example.invalid/1003/index.html", Error: "HTTP 404"}
	if err := s.AppendFailed(f); err != nil {
		t.Fatalf("AppendFailed: %v", err)
	}
	got := readManifestFile(t, dir)
	if len(got.Failed) != 1 || !reflect.DeepEqual(got.Failed[0], f) {
		t.Fatalf("失败记录未立刻落盘，实得 %+v", got.Failed)
	}
}

// TestBackfillArticleSHA256KnownContent 断言**具体期望值**而非「长度是 64」。
// 期望值由 shasum -a 256 与 python3 hashlib 双工具交叉核对得出。
func TestBackfillArticleSHA256KnownContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "文章 HTML 片段",
			content: "<html><body>2021年2月金融统计数据报告</body></html>",
			want:    "c066d57274f9b0768ef2621a745713a902c6d05af108df171bf5f13874818d60",
		},
		{
			name:    "空内容",
			content: "",
			want:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := articleSHA256([]byte(tc.content)); got != tc.want {
				t.Errorf("articleSHA256 = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestBackfillManifestHasArticleNeedsBothManifestAndFile 对应 functional[3]：
// 「已抓」= id 在 articles **且** 对应 file 在磁盘存在，两个条件缺一不可。
//
// 四个子用例里有两个是专门用来杀掉「只查一边」的实现的：
//   - manifest 有记录但文件被删 ⇒ 只查 manifest 的实现会答 true，于是跳过重抓，
//     下游拿到一个不存在的路径；
//   - 文件在但 manifest 没记 ⇒ 只查文件的实现会答 true，于是 manifest 永远补不上
//     这一篇（反过来，若实现只查文件而 manifest 丢了，会重复记录同一篇）。
func TestBackfillManifestHasArticleNeedsBothManifestAndFile(t *testing.T) {
	newStore := func(t *testing.T, withRecord, withFile bool) *manifestStore {
		t.Helper()
		dir := t.TempDir()
		s, err := loadManifest(dir)
		if err != nil {
			t.Fatalf("loadManifest: %v", err)
		}
		if withFile {
			if err := os.MkdirAll(filepath.Join(dir, "articles"), 0o755); err != nil {
				t.Fatalf("建 articles 目录: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "articles", "1001.html"), []byte("<html/>"), 0o644); err != nil {
				t.Fatalf("写文章文件: %v", err)
			}
		}
		if withRecord {
			s.Manifest.Articles = append(s.Manifest.Articles, Article{ID: "1001", File: "articles/1001.html"})
		}
		return s
	}

	cases := []struct {
		name       string
		withRecord bool
		withFile   bool
		want       bool
	}{
		{name: "manifest 有记录且文件在 ⇒ 已抓", withRecord: true, withFile: true, want: true},
		{name: "manifest 有记录但文件被删 ⇒ 未抓（只查 manifest 的实现会在这里答 true）", withRecord: true, withFile: false, want: false},
		{name: "文件在但 manifest 没记 ⇒ 未抓（只查文件的实现会在这里答 true）", withRecord: false, withFile: true, want: false},
		{name: "两者都无 ⇒ 未抓", withRecord: false, withFile: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t, tc.withRecord, tc.withFile)
			if got := s.HasArticle("1001"); got != tc.want {
				t.Errorf("HasArticle(1001) = %v，期望 %v", got, tc.want)
			}
		})
	}
}

// TestBackfillManifestHasArticleRejectsEmptyFile：记录里 File 为空 ⇒ 未抓。
//
// 不是洁癖：File 为空时 filepath.Join(dir, "") 就是 dir 本身，而 dir 一定存在
// ——「文件是否存在」这一问会恒真，于是这一篇被永远当成已抓、永远不补。
func TestBackfillManifestHasArticleRejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	s.Manifest.Articles = append(s.Manifest.Articles, Article{ID: "1001"})
	if s.HasArticle("1001") {
		t.Error("File 为空的记录不应算已抓")
	}
}

// TestBackfillManifestHasArticleScansPastOtherIDs：manifest 里有多篇时，
// 查的是**这个 id 那一条**，不是碰上第一条就答。回填跑到后期 articles 有 200+ 条，
// 「只看第一条」在只放一篇的用例里完全看不出来。
func TestBackfillManifestHasArticleScansPastOtherIDs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "articles"), 0o755); err != nil {
		t.Fatalf("建 articles 目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "articles", "1003.html"), []byte("<html/>"), 0o644); err != nil {
		t.Fatalf("写文章文件: %v", err)
	}
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	s.Manifest.Articles = []Article{
		{ID: "1001", File: "articles/1001.html"}, // 文件不在
		{ID: "1002", File: "articles/1002.html"}, // 文件不在
		{ID: "1003", File: "articles/1003.html"}, // 文件在
	}
	if !s.HasArticle("1003") {
		t.Error("排在后面的已抓文章应被找到")
	}
	if s.HasArticle("1002") {
		t.Error("文件不在的中间那条不应算已抓")
	}
	if s.HasArticle("9999") {
		t.Error("不在 manifest 里的 id 不应算已抓")
	}
}

// TestBackfillArticleFilePath 钉住 Article.File 的口径：相对产物根目录、
// 分隔符固定是 /（这个值要写进 JSON 给下游读，不能随平台变）。
// HasArticle 判文件是否存在时按同一口径拼路径，抓取侧（TASK-006）落盘也照它。
func TestBackfillArticleFilePath(t *testing.T) {
	if got, want := articleFile("2025092212551391606"), "articles/2025092212551391606.html"; got != want {
		t.Errorf("articleFile = %q，期望 %q", got, want)
	}
}

// TestBackfillManifestLoadUnreadableErrors：manifest.json 读不出来（这里用「它其实是
// 个目录」构造）⇒ 报错。与 boundary[0] 的「不存在 ⇒ 空 + nil error」是**两件事**：
// 只有「不存在」是首跑的正常路径，其余读失败都得让人看见，不能一并当空 manifest。
func TestBackfillManifestLoadUnreadableErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "manifest.json"), 0o755); err != nil {
		t.Fatalf("构造不可读的 manifest: %v", err)
	}
	s, err := loadManifest(dir)
	if err == nil {
		t.Fatalf("manifest 读失败时必须报错，实得 nil error（store=%+v）", s)
	}
	if s != nil {
		t.Errorf("报错时不应同时返回 store，实得 %+v", s)
	}
}

// TestBackfillManifestLoadRejectsBrokenOutPath：`--out` 落在一个普通文件底下
// （`~/somefile/out`）⇒ **立刻**报错。
//
// 这条守的是「不存在 ⇒ 当首跑」那个豁免口的边界：路径压根不可能存在时，
// 若把它一并当成首跑，回填会一路跑到第一次落盘才炸——那时 400 次请求已经发完。
// 判据是 os.IsNotExist 而不是「读失败就算没有」，这两者只在这里区分得开。
func TestBackfillManifestLoadRejectsBrokenOutPath(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("构造阻挡文件: %v", err)
	}
	s, err := loadManifest(filepath.Join(blocker, "out"))
	if err == nil {
		t.Fatalf("--out 路径不可用时必须立刻报错，实得 nil error（store=%+v）", s)
	}
	if s != nil {
		t.Errorf("报错时不应同时返回 store，实得 %+v", s)
	}
}

// TestBackfillManifestLoadMissingFile：首跑的正常路径，不是错误。
func TestBackfillManifestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("manifest 不存在时应返回 nil error，实得 %v", err)
	}
	if s == nil {
		t.Fatal("manifest 不存在时应返回可用的 store，实得 nil")
	}
	if !reflect.DeepEqual(s.Manifest, Manifest{}) {
		t.Errorf("manifest 不存在时应返回空 Manifest，实得 %+v", s.Manifest)
	}
	if s.HasArticle("1001") {
		t.Error("空 manifest 不应认为任何文章已抓")
	}
}

// TestBackfillManifestLoadInvalidJSONErrors 对应 boundary[1]。
//
// 静默当空的后果不是「少一条记录」，而是断点续抓整个退化成**全量重抓 400 次
// 请求且不报错**——这正是本 sprint 反复在防的那类静默失效，所以这里同时断言
// 「报错」与「不返回一个可用的空 store」。
func TestBackfillManifestLoadInvalidJSONErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "根本不是 JSON", content: "{not json"},
		{name: "被截断的 JSON（进程写到一半被杀）", content: `{"from":"2020-01","articles":[{"id":"1001"`},
		{name: "合法 JSON 但不是对象", content: `["1001","1002"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(tc.content), 0o644); err != nil {
				t.Fatalf("写坏 manifest: %v", err)
			}
			s, err := loadManifest(dir)
			if err == nil {
				t.Fatalf("manifest 内容非法时必须报错，实得 nil error（store=%+v）", s)
			}
			if s != nil {
				t.Errorf("报错时不应同时返回 store（否则调用方一不留神就当空 manifest 用了），实得 %+v", s)
			}
			if !strings.Contains(err.Error(), "manifest.json") {
				t.Errorf("错误信息应指出出问题的文件，实得 %v", err)
			}
		})
	}
}

// TestBackfillManifestSaveFailureKeepsPreviousFile 对应 error_handling[0]：
// 写失败时磁盘上那份必须仍是**之前的完整内容**，不是截断的、也不是空的。
// 这正是「先写临时文件再 rename」与「直接 O_TRUNC 打开原文件」的区别。
func TestBackfillManifestSaveFailureKeepsPreviousFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("以 root 运行时 chmod 挡不住写入，构造不出落盘失败")
	}
	dir := t.TempDir()
	s, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	good := Article{ID: "1001", Title: "2020年1月金融统计数据报告", Published: "2020-02-10",
		URL: "https://example.invalid/1001/index.html", File: "articles/1001.html",
		SHA256: "aa", FetchedAt: "2026-08-14T10:00:01Z", Source: "index"}
	if err := s.AppendArticle(good); err != nil {
		t.Fatalf("第一次落盘应成功: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("读第一次落盘结果: %v", err)
	}

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err = s.AppendArticle(Article{ID: "1002", File: "articles/1002.html"})
	if err == nil {
		t.Fatal("目标目录不可写时必须返回错误")
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod 还原: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("写失败后 manifest 应仍可读: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("写失败破坏了既有 manifest\n之前: %s\n之后: %s", before, after)
	}
	got := readManifestFile(t, dir)
	if len(got.Articles) != 1 || !reflect.DeepEqual(got.Articles[0], good) {
		t.Errorf("既有 manifest 内容不完整，实得 %+v", got.Articles)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("列目录: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "manifest.json" {
			t.Errorf("写失败后残留了临时文件 %q", e.Name())
		}
	}
}
