// Package sankey builds the Prism earnings bridge (design doc
// docs/prism/atlas_prism_design.md §9 M3): segment templates, multi-period
// metrics and the ECharts sankey graph.
package sankey

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// DefaultSegmentAxis is the XBRL axis carrying business segment members;
// templates may override it for issuers using a custom axis.
const DefaultSegmentAxis = "StatementBusinessSegmentsAxis"

// Segment maps one reporting segment to its XBRL instance document member.
type Segment struct {
	// Key must be lower case (rejected at load time otherwise): viper
	// lower-cases the segment keys read by LoadManualSegments, so an
	// upper-case key here would never join.
	Key    string `mapstructure:"key"`
	NameZH string `mapstructure:"name_zh"`
	NameEN string `mapstructure:"name_en"`
	// Member: EDGAR 路径为 explicitMember local name;A 股(akshare)路径为主营构成名称。
	Member string `mapstructure:"xbrl_member"`
	// Aliases 是命中同一 key 的别名——A 股主营构成名称随年份漂移是常态
	// (实测茅台「系列酒」↔「其他系列酒」),EDGAR member 改名同理适用。
	Aliases []string `mapstructure:"aliases"`
}

// Template describes one issuer's segment layout. The trunk flow
// (revenue→cogs/gross→opex/operating→tax/net) is intentionally absent: those
// line items are identical across issuers and are built from fundamental_q
// by the periods engine (AD-5).
type Template struct {
	Company     string    `mapstructure:"company"` // symbol
	CIK         string    `mapstructure:"cik"`
	SegmentAxis string    `mapstructure:"segment_axis"`
	Segments    []Segment `mapstructure:"segments"`
	Version     int       `mapstructure:"version"`  // restatement breakpoint (design §10)
	SinceFY     int       `mapstructure:"since_fy"` // 0 = all fiscal years
}

// LoadTemplates reads every *.yaml under dir and returns upper-cased symbol →
// template.
//
// AD-16 error semantics — the three cases must stay distinguishable:
//
//	dir missing        → empty map, nil   (no templates configured is legal)
//	dir without yaml   → empty map, nil   (idem)
//	malformed yaml     → nil, error       (error text contains the file name)
//
// Callers must not discard the error: a template dir wiped by rsync --delete
// or a broken YAML would otherwise surface as sankey 404s with no log trace.
func LoadTemplates(dir string) (map[string]*Template, error) {
	out := make(map[string]*Template)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("reading sankey template dir %s: %w", dir, err)
	}
	srcOf := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		tmpl, err := loadTemplate(path)
		if err != nil {
			return nil, err
		}
		symbol := strings.ToUpper(tmpl.Company)
		// Without this check the later file silently wins, and which one that is
		// depends on the (platform-dependent) os.ReadDir order.
		if prev, ok := srcOf[symbol]; ok {
			return nil, fmt.Errorf("duplicate company %s declared in both %s and %s", symbol, prev, path)
		}
		srcOf[symbol] = path
		out[symbol] = tmpl
	}
	return out, nil
}

func loadTemplate(path string) (*Template, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading sankey template %s: %w", path, err)
	}
	var tmpl Template
	if err := v.Unmarshal(&tmpl); err != nil {
		return nil, fmt.Errorf("parsing sankey template %s: %w", path, err)
	}
	if tmpl.SegmentAxis == "" {
		tmpl.SegmentAxis = DefaultSegmentAxis
	}
	if err := tmpl.validate(); err != nil {
		return nil, fmt.Errorf("invalid sankey template %s: %w", path, err)
	}
	return &tmpl, nil
}

func (t *Template) validate() error {
	if t.Company == "" {
		return fmt.Errorf("company is required")
	}
	// CIK 仅 EDGAR 路径需要(编排层守卫);A 股(akshare)模板无 CIK,允许为空。
	if strings.Trim(t.CIK, "0123456789") != "" {
		return fmt.Errorf("cik %q must be digits only", t.CIK)
	}
	keys := make(map[string]bool, len(t.Segments))
	members := make(map[string]bool, len(t.Segments))
	for i, s := range t.Segments {
		switch {
		case s.Key == "":
			return fmt.Errorf("segments[%d].key is required", i)
		case s.Key != strings.ToLower(s.Key):
			return fmt.Errorf("segments[%d].key %q must be lower case, otherwise it never joins against the manual segments", i, s.Key)
		case s.Member == "":
			return fmt.Errorf("segments[%d].xbrl_member is required (key=%s)", i, s.Key)
		case keys[s.Key]:
			return fmt.Errorf("duplicate segment key %q", s.Key)
		}
		keys[s.Key] = true
		for _, name := range append([]string{s.Member}, s.Aliases...) {
			if name == "" {
				return fmt.Errorf("segments[%d] has empty alias (key=%s)", i, s.Key)
			}
			if members[name] {
				return fmt.Errorf("duplicate xbrl_member/alias %q", name)
			}
			members[name] = true
		}
	}
	return nil
}

// KeyByName returns member/构成名(含别名) → segment key,供 EDGAR 与 akshare
// 两条分部路径共用同一套映射逻辑。
func (t *Template) KeyByName() map[string]string {
	m := make(map[string]string, len(t.Segments))
	for _, s := range t.Segments {
		m[s.Member] = s.Key
		for _, a := range s.Aliases {
			m[a] = s.Key
		}
	}
	return m
}

var fiscalPeriodRe = regexp.MustCompile(`^([0-9]{4}Q[1-4]|FY[0-9]{4})$`)

// LoadManualSegments reads {dir}/{symbol}.yaml and returns
// fiscal_period → segment_key → amount for issuers whose segments cannot be
// parsed from XBRL. A missing file (or dir) yields an empty map and nil error.
//
// viper lower-cases every map key, so fiscal periods are normalised back to
// upper case before validation; a key that is neither 2006Q1 nor FY2006 is an
// error rather than a row that would never join against segment_revenue.
func LoadManualSegments(dir, symbol string) (map[string]map[string]float64, error) {
	out := make(map[string]map[string]float64)
	path := filepath.Join(dir, strings.ToLower(symbol)+".yaml")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("reading manual segments %s: %w", path, err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading manual segments %s: %w", path, err)
	}
	var raw map[string]map[string]float64
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("parsing manual segments %s: %w", path, err)
	}
	for period, values := range raw {
		norm := strings.ToUpper(period)
		if !fiscalPeriodRe.MatchString(norm) {
			return nil, fmt.Errorf("invalid fiscal_period %q in %s: want 2006Q1 or FY2006", period, path)
		}
		out[norm] = values
	}
	return out, nil
}
