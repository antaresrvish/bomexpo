package part

// Query is one search request. Fields a provider can't honour are ignored —
// check Caps before offering the filter in the UI.
type Query struct {
	Keyword   string
	Page      int
	Size      int
	BasicOnly bool // restrict to the assembler's basic library
}

// Norm clamps page and size so every provider paginates alike.
func (q Query) Norm() Query {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 {
		q.Size = 25
	}
	return q
}

// Result is one page of search hits. Total is the full match count, which may
// be far larger than len(Items).
type Result struct {
	Items    []Part
	Total    int
	Page     int
	PageSize int
}

// Caps tells the UI what a provider actually supplies, so a source that can't
// filter by library doesn't advertise the toggle.
type Caps struct {
	BasicFilter bool // honours Query.BasicOnly
	Library     bool // populates Part.Lib
	AsmStock    bool // populates Part.AsmStock, AsmMin and Loss
}

// Provider is a searchable parts source. Implementations must be safe for
// concurrent use — the TUI fans out one request per line item.
type Provider interface {
	ID() string    // stable identifier used in config and CLI flags
	Label() string // display name

	// Ready reports whether the provider can be used, and if not, a short
	// hint telling the user what to do about it (e.g. set an API key).
	Ready() (bool, string)

	Caps() Caps

	Search(Query) (Result, error)
	// Detail fetches one part by code, allowing a cached copy.
	Detail(code string) (Part, error)
	// Refresh fetches one part by code, bypassing any cache.
	Refresh(code string) (Part, error)
}
