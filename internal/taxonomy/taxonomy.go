// Package taxonomy collects the categories a parts source uses.
//
// Neither LCSC nor JLCPCB publishes the taxonomy its search index labels results
// with: LCSC's category tree is the website's navigation, on a different id space
// than the results, and both ignore a category id in a query. Every result does
// carry a parent and leaf category, so the list is crawled from searches.
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
// Harvest runs a second round over the parents these turn up: ~94 leaves becomes
// ~258. Still incomplete, which is what Add is for.
var probes = []string{
	"capacitor", "resistor", "inductor", "connector", "microcontroller",
	"led", "diode", "transistor", "regulator", "crystal",
	"switch", "relay", "sensor", "fuse", "transformer",
	"amplifier", "memory", "mosfet", "ferrite", "antenna",
}

// A distributor's category list changes on the order of months.
const ttl = 7 * 24 * time.Hour

const workers = 6 // matches the vendor clients' own cap

// Cat is one leaf category and the group it belongs to.
type Cat struct {
	Parent string `json:"parent"`
	Leaf   string `json:"leaf"`
}

type cacheFile struct {
	Fetched time.Time `json:"fetched"`
	Cats    []Cat     `json:"cats"`
}

var cacheDir = defaultCacheDir

func defaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "bomexpo", "taxonomy")
}

// SetCacheDir points the cache somewhere else, for tests.
func SetCacheDir(dir string) { cacheDir = func() string { return dir } }

// Load returns the source's categories, crawling when the cache is cold or stale.
// A failed crawl serves the stale cache if there is one.
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
	// keep what the cache knew and this crawl missed
	write(p.ID(), Merge(cached, cats))
	return Merge(cached, cats), nil
}

// Harvest crawls the probe words, then the parents they turn up.
func Harvest(p part.Provider) ([]Cat, error) {
	seen, err := sweep(p, probes)
	if len(seen) == 0 {
		return nil, err
	}

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

// Add writes a result set's categories into the cache, which is how the crawl's
// blind spots close.
func Add(id string, ps []part.Part) []Cat {
	fresh := FromParts(ps)
	if len(fresh) == 0 {
		return nil
	}
	cached, _, err := read(id)
	if err != nil && len(cached) == 0 {
		return fresh // nothing cached yet
	}
	merged := Merge(cached, fresh)
	if len(merged) > len(cached) {
		write(id, merged)
	}
	return merged
}

// Merge folds extra categories in, dropping duplicates.
func Merge(base []Cat, extra ...[]Cat) []Cat {
	all := append([]Cat(nil), base...)
	for _, e := range extra {
		all = append(all, e...)
	}
	return dedupe(all)
}

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

// write ignores failures: an uncached taxonomy is just re-crawled next time.
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

// Keyword turns a category name into one a search box handles, since neither
// vendor takes a category id. Measured on LCSC: dropping the parenthesised note,
// the qualifier after a dash and the packaging words takes "Chip Resistor -
// Surface Mount" from 27 in-category hits a page to 89. Some categories still come
// back empty, so callers page and say so.
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
