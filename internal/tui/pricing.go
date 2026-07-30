package tui

import (
	"fmt"
	"sort"
)

// Board counts where the per-board price moves are decided by the parts' own quantity
// breaks and minimums, so they land at odd numbers rather than on 100/200/300.

// maxBoardsProbed bounds the search; past a few thousand the per-board price has
// flattened.
const maxBoardsProbed = 2000

type priceRow struct {
	Boards   int
	Total    float64
	PerBoard float64
	Complete bool // every line item had a price
	Why      string
}

// breakBoards are the counts where a part crosses a break or leaves its MOQ padding.
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
		// a tier repeating the price above it is not a break
		prev := -1.0
		for _, pr := range p.Prices {
			if pr.Ladder > 1 && prev >= 0 && pr.USD != prev {
				add(ceilDiv(pr.Ladder, per))
			}
			prev = pr.USD
		}
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

// pricingRows is the table: counts where the per-board cost moves, plus the target.
// A count matching the row above is dropped.
func (m Model) pricingRows(target, limit int) []priceRow {
	// $0.00 is an unassigned board, not a quote; let the caller say so
	if !m.anyPriced() {
		return nil
	}
	cands := m.breakBoards()
	if target > 0 {
		// the target may already be a break, so fold it in rather than appending
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

	if limit > 0 && len(out) > limit {
		out = thinRows(out, limit)
	}
	return out
}

// whyBreak names a part that changes here, so a row explains itself.
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
