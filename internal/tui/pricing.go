package tui

import (
	"fmt"
	"sort"
)

// A fixed 1/100/200/300 ladder tells you almost nothing: none of those numbers are
// where anything actually happens. What matters is the board counts where the price
// per board changes, and those are decided by the parts — each one has its own
// quantity breaks and its own minimum order, and they land at odd numbers.
//
// So the table is computed: find the board counts where a part crosses a break or
// stops being over-bought, keep the ones where the per-board cost really moves, and
// show those.

// maxBoardsProbed is how far out to look for breaks. Past a few thousand boards the
// per-board price has flattened and the assembler's quote dominates anyway.
const maxBoardsProbed = 2000

// priceRow is one board count worth showing.
type priceRow struct {
	Boards   int
	Total    float64
	PerBoard float64
	Complete bool // every line item had a price
	// Why is what changes at this count, empty for the baseline rows.
	Why string
}

// breakBoards are the board counts where some part crosses a price break or leaves
// its minimum-order padding behind.
func (m Model) breakBoards() []int {
	seen := map[int]bool{1: true}
	add := func(n int) {
		if n > 1 && n <= maxBoardsProbed {
			seen[n] = true
		}
	}
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		per := it.Quantity
		if per <= 0 {
			continue
		}
		p := m.assigned[i]
		if p == nil {
			continue
		}
		// The board count at which the order first reaches each break — but only
		// where the tier is actually cheaper. A vendor listing the same price twice
		// is not a break, and a row saying "L1 hits 37" when L1 costs the same there
		// is worse than no row.
		prev := -1.0
		for _, pr := range p.Prices {
			if pr.Ladder > 1 && prev >= 0 && pr.USD != prev {
				add(ceilDiv(pr.Ladder, per))
			}
			prev = pr.USD
		}
		// and the count at which the minimum order stops padding the buy
		if p.MinBuy > per {
			add(ceilDiv(p.MinBuy, per))
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// anyPriced reports whether a single line item has a price to work from.
func (m Model) anyPriced() bool {
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		if p := m.assigned[i]; p != nil && len(p.Prices) > 0 {
			return true
		}
	}
	return false
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return a
	}
	return (a + b - 1) / b
}

// pricingRows is the table: the counts where the per-board cost actually moves, plus
// the target if one is set. Counts whose per-board price matches the row above are
// dropped — a row that says the same thing twice is noise.
func (m Model) pricingRows(target, limit int) []priceRow {
	// A $0.00 order is not an answer, it is an unassigned board. Say nothing and let
	// the caller explain, rather than printing a row that reads like a quote.
	if !m.anyPriced() {
		return nil
	}
	cands := m.breakBoards()
	if target > 0 {
		// The target may already be a break, so fold it in rather than appending —
		// otherwise it shows up as two identical rows.
		seen := map[int]bool{target: true}
		for _, n := range cands {
			seen[n] = true
		}
		cands = cands[:0]
		for n := range seen {
			cands = append(cands, n)
		}
		sort.Ints(cands)
	}

	var out []priceRow
	last := -1.0
	for _, n := range cands {
		tot, complete := m.costAt(n)
		per := tot / float64(n)
		isTarget := n == target
		// keep a row when the price moved by a hundredth of a cent or more, and
		// always keep the first row and the target
		if len(out) > 0 && !isTarget && last > 0 && abs(per-last) < 0.00005 {
			continue
		}
		row := priceRow{Boards: n, Total: tot, PerBoard: per, Complete: complete}
		switch {
		case isTarget:
			row.Why = "your order"
		case len(out) == 0:
			row.Why = ""
		default:
			row.Why = m.whyBreak(n)
		}
		out = append(out, row)
		last = per
	}

	// Too many rows is as unhelpful as too few. Thin from the middle, keeping the
	// first row, the target and the last.
	if limit > 0 && len(out) > limit {
		out = thinRows(out, limit)
	}
	return out
}

// whyBreak names a part that changes at this board count, so a row explains itself.
func (m Model) whyBreak(boards int) string {
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		p := m.assigned[i]
		if p == nil || it.Quantity <= 0 {
			continue
		}
		if p.MinBuy > it.Quantity && ceilDiv(p.MinBuy, it.Quantity) == boards {
			return it.ID() + " clears its minimum"
		}
		for _, pr := range p.Prices {
			if pr.Ladder > 1 && ceilDiv(pr.Ladder, it.Quantity) == boards {
				return fmt.Sprintf("%s hits %d", it.ID(), pr.Ladder)
			}
		}
	}
	return ""
}

// thinRows keeps the ends, the target and an even spread between them.
func thinRows(rows []priceRow, limit int) []priceRow {
	keep := map[int]bool{0: true, len(rows) - 1: true}
	for i, r := range rows {
		if r.Why == "your order" {
			keep[i] = true
		}
	}
	step := float64(len(rows)-1) / float64(limit-len(keep)+1)
	for x := step; x < float64(len(rows)-1) && len(keep) < limit; x += step {
		keep[int(x)] = true
	}
	out := make([]priceRow, 0, len(keep))
	for i, r := range rows {
		if keep[i] {
			out = append(out, r)
		}
	}
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
