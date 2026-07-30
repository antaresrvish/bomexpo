package part

// Query is one search request. Fields a provider can't honour are ignored, so
// check Caps before offering one.
type Query struct {
	Keyword   string
	Page      int
	Size      int
	BasicOnly bool // restrict to the assembler's basic library
}

// Norm makes every provider paginate alike.
func (q Query) Norm() Query {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 {
		q.Size = 25
	}
	return q
}

// Result is one page of hits. Total is the full match count, often far larger
// than len(Items).
type Result struct {
	Items    []Part
	Total    int
	Page     int
	PageSize int
}

// Caps keeps the UI from offering a filter the source can't honour.
type Caps struct {
	BasicFilter bool // honours Query.BasicOnly
	Library     bool // populates Part.Lib
	Assembly    bool // populates Part.AsmMin and Part.Loss
}

// Provider must be safe for concurrent use: the TUI fans out one request per
// line item.
type Provider interface {
	ID() string    // stable identifier used in config and CLI flags
	Label() string // display name

	// Ready reports usability and, if not, what to do about it.
	Ready() (bool, string)

	Caps() Caps

	Search(Query) (Result, error)
	Detail(code string) (Part, error)  // may serve a cached copy
	Refresh(code string) (Part, error) // bypasses any cache
}
