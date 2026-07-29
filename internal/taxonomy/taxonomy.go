// Package taxonomy collects the categories a parts source uses, so a category
// can be picked before any search has run.
//
// Neither LCSC nor JLCPCB publishes the taxonomy its search index labels results
// with. LCSC's category tree is the website's navigation, on a different id space
// than the search results, and both vendors ignore a category id in a query. What
// they do return is a parent and leaf category on every row, so the taxonomy is
// harvested by running a handful of broad searches and keeping the labels. It is
// cached on disk because it barely changes.
package taxonomy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"bomexpo/internal/part"
)

// probes open the crawl. A broad word returns a page dominated by one category, so
// a second round searches the parent names round one found — that reaches the
// siblings, taking the harvest from ~94 leaves to ~258.
//
// It is still a crawl over a search box, so it will not find every category. What
// closes the gap is use: Add persists the categories of every search you run, so a
// category the crawl missed joins the list the first time you search into it.
var probes = []string{
	"capacitor", "resistor", "inductor", "connector", "microcontroller",
	"led", "diode", "transistor", "regulator", "crystal",
	"switch", "relay", "sensor", "fuse", "transformer",
	"amplifier", "memory", "mosfet", "ferrite", "antenna",
}

// ttl is how long a cached taxonomy is used before it's harvested again. A
// distributor's category list changes on the order of months.
const ttl = 7 * 24 * time.Hour

// workers is how many probes run at once, matching the vendor clients' own cap.
const workers = 6

// Cat is one leaf category and the group it belongs to.
type Cat struct {
	Parent string `json:"parent"`
	Leaf   string `json:"leaf"`
}

type cacheFile struct {
	Fetched time.Time `json:"fetched"`
	Cats    []Cat     `json:"cats"`
}

// cacheDir is overridable so tests don't touch the user's real cache.
var cacheDir = defaultCacheDir

func defaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "bomexpo", "taxonomy")
}

// SetCacheDir points the on-disk cache somewhere else, for tests.
func SetCacheDir(dir string) { cacheDir = func() string { return dir } }

// Load returns the source's categories, from cache when it's fresh and by
// harvesting otherwise. A harvest that fails entirely returns the stale cache if
// there is one, so a flaky network degrades to yesterday's list rather than none.
func Load(p part.Provider) ([]Cat, error) {
	if p == nil {
		return nil, nil
	}
	cached, age, err := read(p.ID())
	if err == nil && age < ttl && len(cached) > 0 {
		return cached, nil
	}

	cats, harvestErr := Harvest(p)
	if len(cats) == 0 {
		if len(cached) > 0 {
			return cached, nil // stale beats empty
		}
		return nil, harvestErr
	}
	// Keep anything the cache knew that this harvest happened to miss: the probes
	// reach most of a catalogue, not all of it, and which corners they miss varies.
	write(p.ID(), Merge(cached, cats))
	return Merge(cached, cats), nil
}

// Harvest crawls the source for its categories: the probe words, then the parent
// categories those turned up.
func Harvest(p part.Provider) ([]Cat, error) {
	seen, err := sweep(p, probes)
	if len(seen) == 0 {
		return nil, err
	}

	// Round two: a parent's name as the keyword returns a spread of its children,
	// which one broad word never does.
	var parents []string
	done := map[string]bool{}
	for _, c := range seen {
		if c.Parent != "" && c.Parent != "other" && !done[c.Parent] {
			done[c.Parent] = true
			parents = append(parents, c.Parent)
		}
	}
	more, _ := sweep(p, parents)
	return Merge(seen, more), nil
}

// sweep searches every keyword and returns the categories they mention.
func sweep(p part.Provider, words []string) ([]Cat, error) {
	var (
		mu   sync.Mutex
		seen []Cat
		errs []error
	)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, kw := range words {
		wg.Add(1)
		go func(kw string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := p.Search(part.Query{Keyword: kw, Size: 100})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, it := range res.Items {
				if c, ok := catOf(it); ok {
					seen = append(seen, c)
				}
			}
		}(kw)
	}
	wg.Wait()

	if len(seen) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	return dedupe(seen), nil
}

// Add folds a result set's categories into the cached list and keeps them. This is
// how the crawl's blind spots close: search into a category it missed and it is
// there from then on.
func Add(id string, ps []part.Part) []Cat {
	fresh := FromParts(ps)
	if len(fresh) == 0 {
		return nil
	}
	cached, _, err := read(id)
	if err != nil && len(cached) == 0 {
		return fresh // nothing cached yet; Load will write the crawl soon enough
	}
	merged := Merge(cached, fresh)
	if len(merged) > len(cached) {
		write(id, merged)
	}
	return merged
}

// Merge folds extra categories into a list, dropping duplicates. It's how a
// result set the user just searched teaches the cached taxonomy something new.
func Merge(base []Cat, extra ...[]Cat) []Cat {
	all := append([]Cat(nil), base...)
	for _, e := range extra {
		all = append(all, e...)
	}
	return dedupe(all)
}

// FromParts reads the categories off a result set.
func FromParts(ps []part.Part) []Cat {
	var out []Cat
	for _, p := range ps {
		if c, ok := catOf(p); ok {
			out = append(out, c)
		}
	}
	return dedupe(out)
}

func catOf(p part.Part) (Cat, bool) {
	leaf := strings.TrimSpace(p.Category)
	if leaf == "" {
		return Cat{}, false
	}
	parent := strings.TrimSpace(p.ParentCat)
	if parent == "" {
		parent = "other"
	}
	return Cat{Parent: parent, Leaf: leaf}, true
}

func dedupe(in []Cat) []Cat {
	seen := map[Cat]bool{}
	out := make([]Cat, 0, len(in))
	for _, c := range in {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Parent != out[j].Parent {
			return out[i].Parent < out[j].Parent
		}
		return out[i].Leaf < out[j].Leaf
	})
	return out
}

func path(id string) string {
	dir := cacheDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, id+".json")
}

func read(id string) ([]Cat, time.Duration, error) {
	p := path(id)
	if p == "" {
		return nil, 0, os.ErrNotExist
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, 0, err
	}
	var f cacheFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, 0, err
	}
	return f.Cats, time.Since(f.Fetched), nil
}

// write stores the list, ignoring failures: a taxonomy that can't be cached is
// re-harvested next time, which is slower but not wrong.
func write(id string, cats []Cat) {
	p := path(id)
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(cacheFile{Fetched: time.Now(), Cats: cats})
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o644)
}

var paren = regexp.MustCompile(`\s*\([^)]*\)`)

// Keyword turns a category name into something a keyword search can use, since
// neither vendor will take a category id. Measured against LCSC: the full name
// finds nothing for several categories, and dropping the parenthesised notes, the
// qualifier after a dash and the packaging words fixes most of them —
// "Chip Resistor - Surface Mount" goes from 27 in-category hits per page to 89,
// "Inductors (SMD)" from 3 to 13.
//
// It is a heuristic over a search box, not a category query: some categories still
// come back thin or empty, which is why the caller pages and says so when nothing
// matches.
func Keyword(category string) string {
	s := paren.ReplaceAllString(category, "")
	if i := strings.Index(s, " - "); i > 0 {
		s = s[:i]
	}
	for _, w := range []string{"SMD/SMT", "SMD", "SMT", "Surface Mount"} {
		s = strings.ReplaceAll(s, w, "")
	}
	if out := strings.Join(strings.Fields(s), " "); out != "" {
		return out
	}
	return strings.TrimSpace(category)
}
