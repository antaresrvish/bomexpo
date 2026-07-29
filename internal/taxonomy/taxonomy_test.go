package taxonomy

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"bomexpo/internal/part"
)

// fakeSrc answers probe searches from a canned table, so the harvest can be tested
// without the network.
type fakeSrc struct {
	mu      sync.Mutex
	calls   []string
	byWord  map[string][]part.Part
	failAll bool
}

func (f *fakeSrc) ID() string            { return "fake" }
func (f *fakeSrc) Label() string         { return "Fake" }
func (f *fakeSrc) Ready() (bool, string) { return true, "" }
func (f *fakeSrc) Caps() part.Caps       { return part.Caps{} }
func (f *fakeSrc) Detail(string) (part.Part, error) {
	return part.Part{}, errors.New("not used")
}
func (f *fakeSrc) Refresh(string) (part.Part, error) {
	return part.Part{}, errors.New("not used")
}

func (f *fakeSrc) Search(q part.Query) (part.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, q.Keyword)
	f.mu.Unlock()
	if f.failAll {
		return part.Result{}, errors.New("network is down")
	}
	return part.Result{Items: f.byWord[q.Keyword]}, nil
}

func cat(parent, leaf string) part.Part {
	return part.Part{ParentCat: parent, Category: leaf}
}

func newFake() *fakeSrc {
	return &fakeSrc{byWord: map[string][]part.Part{
		"capacitor": {cat("Capacitors", "MLCC - SMD"), cat("Capacitors", "Tantalum"), cat("Capacitors", "MLCC - SMD")},
		"resistor":  {cat("Resistors", "Chip Resistor")},
		"connector": {cat("Connectors", "USB Connectors"), cat("Connectors", "Pin Headers")},
	}}
}

func TestHarvestCollectsEveryCategoryOnce(t *testing.T) {
	f := newFake()
	got, err := Harvest(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []Cat{
		{"Capacitors", "MLCC - SMD"}, {"Capacitors", "Tantalum"},
		{"Connectors", "Pin Headers"}, {"Connectors", "USB Connectors"},
		{"Resistors", "Chip Resistor"},
	}
	if len(got) != len(want) {
		t.Fatalf("harvested %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(f.calls) != len(probes) {
		t.Errorf("%d searches for %d probes", len(f.calls), len(probes))
	}
}

// A part with no category is skipped rather than becoming an empty box, and a
// missing parent gets a group so it can still be shown.
func TestHarvestSkipsUnlabelledParts(t *testing.T) {
	f := &fakeSrc{byWord: map[string][]part.Part{
		"capacitor": {cat("", ""), cat("", "Loose Leaf"), cat("Capacitors", "MLCC")},
	}}
	got, err := Harvest(f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Cat]bool{{"Capacitors", "MLCC"}: true, {"other", "Loose Leaf"}: true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected %+v", c)
		}
	}
}

func TestLoadCachesToDiskAndReadsItBack(t *testing.T) {
	dir := t.TempDir()
	SetCacheDir(dir)
	t.Cleanup(func() { cacheDir = defaultCacheDir })

	f := newFake()
	first, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("nothing harvested")
	}
	if _, err := os.Stat(filepath.Join(dir, "fake.json")); err != nil {
		t.Fatalf("no cache written: %v", err)
	}

	// a second Load must come off disk, not the source
	calls := len(f.calls)
	second, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != calls {
		t.Errorf("%d more searches ran — the cache was not used", len(f.calls)-calls)
	}
	if len(second) != len(first) {
		t.Errorf("cache returned %d categories, harvested %d", len(second), len(first))
	}
}

// A failed harvest with a cache on disk must serve the cache: yesterday's list
// beats no list.
func TestLoadFallsBackToAStaleCache(t *testing.T) {
	dir := t.TempDir()
	SetCacheDir(dir)
	t.Cleanup(func() { cacheDir = defaultCacheDir })

	good := newFake()
	want, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	// age the cache past its ttl by rewriting it with an old timestamp
	write("fake", want)
	b, _ := os.ReadFile(filepath.Join(dir, "fake.json"))
	old := string(b)
	os.WriteFile(filepath.Join(dir, "fake.json"),
		[]byte(replaceFetched(old, "2000-01-01T00:00:00Z")), 0o644)

	broken := &fakeSrc{failAll: true}
	got, err := Load(broken)
	if err != nil {
		t.Fatalf("a stale cache should be served, got %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("served %d categories, cached %d", len(got), len(want))
	}
}

// With nothing cached and the network down, Load reports the failure instead of
// pretending the catalogue is empty.
func TestLoadWithNoCacheReportsFailure(t *testing.T) {
	SetCacheDir(t.TempDir())
	t.Cleanup(func() { cacheDir = defaultCacheDir })
	if _, err := Load(&fakeSrc{failAll: true}); err == nil {
		t.Error("want an error when there is neither a cache nor a network")
	}
}

func TestMergeKeepsBothSidesWithoutDuplicates(t *testing.T) {
	a := []Cat{{"Capacitors", "MLCC"}, {"Resistors", "Chip"}}
	b := []Cat{{"Capacitors", "MLCC"}, {"Diodes", "Schottky"}}
	got := Merge(a, b)
	if len(got) != 3 {
		t.Fatalf("merged to %v, want 3 distinct", got)
	}
	seen := map[Cat]bool{}
	for _, c := range got {
		if seen[c] {
			t.Errorf("duplicate %+v", c)
		}
		seen[c] = true
	}
}

func TestFromPartsReadsResultLabels(t *testing.T) {
	got := FromParts([]part.Part{cat("Connectors", "USB"), cat("Connectors", "USB"), {}})
	if len(got) != 1 || got[0] != (Cat{"Connectors", "USB"}) {
		t.Errorf("got %v, want one Connectors/USB", got)
	}
}

// replaceFetched rewrites the cache's timestamp, which is how the test ages it.
func replaceFetched(json, when string) string {
	const key = `"fetched":"`
	i := indexOf(json, key)
	if i < 0 {
		return json
	}
	start := i + len(key)
	end := start
	for end < len(json) && json[end] != '"' {
		end++
	}
	return json[:start] + when + json[end:]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
