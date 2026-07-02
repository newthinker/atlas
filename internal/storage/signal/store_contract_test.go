package signal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-301)
// functional[0] "Save 赋 sig_<nano>_<counter> ID / GetByID 等值 / 未命中 ErrSymbolNotFound" → TestContract_SaveGetByID
// functional[1] "List/Count 全字段 filter；排序 generated_at ASC,id ASC"                    → TestContract_ListFiltersAndOrder
// functional[2] "metadata JSON 往返（nil/空/populated）"                                     → TestContract_MetadataRoundTrip
// boundary[0]   "空库 List 空集/Count 0；重开持久化"                                          → TestContract_Empty / TestSQLite_Persistence
// boundary[1]   "limit=0 不限制；offset 越界空；from/to 闭区间(match memory)"                → TestContract_Pagination / TestContract_TimeRangeEndpoints
// error_handling[0] "path 不可用 NewSQLiteStore 返回 error"                                  → TestSQLite_OpenError
// non_functional[1] "-race 并发 Save/List 无 data race"                                       → TestSQLite_ConcurrentSaveList

// storeFactory builds a fresh, empty Store for one subtest.
type storeFactory struct {
	name string
	make func(t *testing.T) Store
}

func contractStores() []storeFactory {
	return []storeFactory{
		{"memory", func(t *testing.T) Store { return NewMemoryStore(1000) }},
		{"sqlite", func(t *testing.T) Store {
			s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "signals.db"))
			if err != nil {
				t.Fatalf("NewSQLiteStore: %v", err)
			}
			t.Cleanup(func() { s.Close() })
			return s
		}},
	}
}

var idPattern = regexp.MustCompile(`^sig_\d+_\d+$`)

func at(sec int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, sec, 0, time.UTC)
}

// assertSignalEqual compares two signals with instant-equal time and DeepEqual
// metadata so memory (exact) and sqlite (JSON/UTC round-trip) share assertions.
func assertSignalEqual(t *testing.T, got, want core.Signal) {
	t.Helper()
	if got.ID != want.ID || got.Symbol != want.Symbol || got.Action != want.Action ||
		got.Confidence != want.Confidence || got.Price != want.Price ||
		got.Reason != want.Reason || got.Strategy != want.Strategy {
		t.Errorf("signal scalar mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if !got.GeneratedAt.Equal(want.GeneratedAt) {
		t.Errorf("GeneratedAt = %v, want %v", got.GeneratedAt, want.GeneratedAt)
	}
	if !reflect.DeepEqual(got.Metadata, want.Metadata) {
		t.Errorf("Metadata = %#v, want %#v", got.Metadata, want.Metadata)
	}
}

func TestContract_SaveGetByID(t *testing.T) {
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			s := sf.make(t)

			sig := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8, Price: 150, Strategy: "ma", GeneratedAt: at(1)}
			if err := s.Save(ctx, sig); err != nil {
				t.Fatalf("Save: %v", err)
			}

			all, err := s.List(ctx, ListFilter{})
			if err != nil || len(all) != 1 {
				t.Fatalf("List after save: n=%d err=%v", len(all), err)
			}
			id := all[0].ID
			if !idPattern.MatchString(id) {
				t.Errorf("ID %q does not match sig_<nano>_<counter>", id)
			}

			got, err := s.GetByID(ctx, id)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			want := sig
			want.ID = id
			assertSignalEqual(t, *got, want)

			_, err = s.GetByID(ctx, "sig_does_not_exist")
			if !errors.Is(err, core.ErrSymbolNotFound) {
				t.Errorf("missing GetByID err = %v, want ErrSymbolNotFound", err)
			}
		})
	}
}

func TestContract_ListFiltersAndOrder(t *testing.T) {
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			s := sf.make(t)
			// Insert out of time order to prove ORDER BY generated_at ASC.
			seed := []core.Signal{
				{Symbol: "AAPL", Action: core.ActionBuy, Strategy: "ma", GeneratedAt: at(30)},
				{Symbol: "AAPL", Action: core.ActionSell, Strategy: "pe", GeneratedAt: at(10)},
				{Symbol: "GOOG", Action: core.ActionBuy, Strategy: "ma", GeneratedAt: at(20)},
			}
			for _, sig := range seed {
				if err := s.Save(ctx, sig); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}

			cases := []struct {
				name      string
				filter    ListFilter
				wantSecs  []int // expected GeneratedAt seconds in order
				wantCount int
			}{
				{"all ordered", ListFilter{}, []int{10, 20, 30}, 3},
				{"by symbol", ListFilter{Symbol: "AAPL"}, []int{10, 30}, 2},
				{"by strategy", ListFilter{Strategy: "ma"}, []int{20, 30}, 2},
				{"by action", ListFilter{Action: core.ActionBuy}, []int{20, 30}, 2},
				{"from inclusive", ListFilter{From: at(20)}, []int{20, 30}, 2},
				{"to inclusive", ListFilter{To: at(20)}, []int{10, 20}, 2},
				{"from+to window", ListFilter{From: at(20), To: at(20)}, []int{20}, 1},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					got, err := s.List(ctx, c.filter)
					if err != nil {
						t.Fatalf("List: %v", err)
					}
					var secs []int
					for _, g := range got {
						secs = append(secs, g.GeneratedAt.Second())
					}
					if !reflect.DeepEqual(secs, c.wantSecs) {
						t.Errorf("List order = %v, want %v", secs, c.wantSecs)
					}
					n, err := s.Count(ctx, c.filter)
					if err != nil {
						t.Fatalf("Count: %v", err)
					}
					if n != c.wantCount {
						t.Errorf("Count = %d, want %d", n, c.wantCount)
					}
				})
			}
		})
	}
}

func TestContract_Pagination(t *testing.T) {
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			s := sf.make(t)
			for i := 0; i < 5; i++ {
				if err := s.Save(ctx, core.Signal{Symbol: "AAPL", GeneratedAt: at(i)}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}

			// limit=0 means unlimited (must NOT translate to LIMIT 0).
			all, _ := s.List(ctx, ListFilter{Limit: 0})
			if len(all) != 5 {
				t.Errorf("limit=0 returned %d, want all 5", len(all))
			}
			// limit caps.
			two, _ := s.List(ctx, ListFilter{Limit: 2})
			if len(two) != 2 || two[0].GeneratedAt.Second() != 0 || two[1].GeneratedAt.Second() != 1 {
				t.Errorf("limit=2 got %d rows, first secs wrong", len(two))
			}
			// offset skips.
			off, _ := s.List(ctx, ListFilter{Offset: 3})
			if len(off) != 2 || off[0].GeneratedAt.Second() != 3 {
				t.Errorf("offset=3 got %d rows starting %d", len(off), off[0].GeneratedAt.Second())
			}
			// offset beyond range → empty.
			none, _ := s.List(ctx, ListFilter{Offset: 99})
			if len(none) != 0 {
				t.Errorf("offset out of range returned %d, want 0", len(none))
			}
			// limit+offset together.
			page, _ := s.List(ctx, ListFilter{Limit: 2, Offset: 2})
			if len(page) != 2 || page[0].GeneratedAt.Second() != 2 {
				t.Errorf("limit=2 offset=2 got %d starting %d", len(page), page[0].GeneratedAt.Second())
			}
		})
	}
}

func TestContract_TimeRangeEndpoints(t *testing.T) {
	// Pins the shared From/To semantics: memory uses Before(From)/After(To), i.e.
	// endpoints are INCLUDED (a closed interval). sqlite must match memory.
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			s := sf.make(t)
			for _, sec := range []int{10, 20, 30} {
				s.Save(ctx, core.Signal{Symbol: "X", GeneratedAt: at(sec)})
			}
			got, _ := s.List(ctx, ListFilter{From: at(10), To: at(30)})
			if len(got) != 3 {
				t.Errorf("closed [10,30] returned %d, want 3 (endpoints included)", len(got))
			}
			only, _ := s.List(ctx, ListFilter{From: at(20), To: at(20)})
			if len(only) != 1 || only[0].GeneratedAt.Second() != 20 {
				t.Errorf("[20,20] returned %d, want exactly the 20 endpoint", len(only))
			}
		})
	}
}

func TestContract_MetadataRoundTrip(t *testing.T) {
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			cases := []struct {
				name string
				meta map[string]any
			}{
				{"nil", nil},
				{"empty", map[string]any{}},
				{"populated", map[string]any{"score": 0.85, "note": "hi"}},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					s := sf.make(t)
					s.Save(ctx, core.Signal{Symbol: "AAPL", Metadata: c.meta, GeneratedAt: at(1)})
					all, _ := s.List(ctx, ListFilter{})
					got, err := s.GetByID(ctx, all[0].ID)
					if err != nil {
						t.Fatalf("GetByID: %v", err)
					}
					if !reflect.DeepEqual(got.Metadata, c.meta) {
						t.Errorf("metadata round-trip = %#v, want %#v", got.Metadata, c.meta)
					}
				})
			}
		})
	}
}

func TestContract_Empty(t *testing.T) {
	for _, sf := range contractStores() {
		t.Run(sf.name, func(t *testing.T) {
			ctx := context.Background()
			s := sf.make(t)
			list, err := s.List(ctx, ListFilter{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("empty store List = %d, want 0", len(list))
			}
			n, err := s.Count(ctx, ListFilter{})
			if err != nil || n != 0 {
				t.Errorf("empty store Count = %d err=%v, want 0", n, err)
			}
		})
	}
}

func TestSQLite_Persistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist.db")

	s1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	s1.Save(ctx, core.Signal{Symbol: "AAPL", GeneratedAt: at(1)})
	s1.Close()

	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.List(ctx, ListFilter{})
	if err != nil || len(got) != 1 {
		t.Fatalf("after reopen List n=%d err=%v, want 1 (data persisted)", len(got), err)
	}
}

func TestSQLite_OpenError(t *testing.T) {
	// A path whose parent is an existing regular file cannot be MkdirAll'd.
	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewSQLiteStore(filepath.Join(file, "child", "signals.db"))
	if err == nil {
		t.Error("expected error when parent dir cannot be created")
	}
}

func TestSQLite_ConcurrentSaveList(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "conc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.Save(ctx, core.Signal{Symbol: "AAPL", GeneratedAt: at(j)})
				s.List(ctx, ListFilter{Symbol: "AAPL"})
			}
		}(i)
	}
	wg.Wait()

	n, err := s.Count(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 80 {
		t.Errorf("Count after concurrent saves = %d, want 80", n)
	}
}
