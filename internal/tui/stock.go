package tui

import "fmt"

// thinHeadroom is where stock stops being comfortable. Stock moves between quoting a
// board and ordering it, so a part covering the run 1.3× is worth naming even though
// it covers.
const thinHeadroom = 2

// boardTarget is how many boards the checks measure against: what you said with q, or
// one board when you haven't.
func (m Model) boardTarget() int {
	if m.check.target > 0 {
		return m.check.target
	}
	return 1
}

// needFor is how many pieces of a line item the order takes.
func (m Model) needFor(i int) int {
	if i < 0 || i >= len(m.items) {
		return 0
	}
	return m.items[i].Quantity * m.boardTarget()
}

// stockShort reports whether the assigned part cannot fill the order, and says by how
// much. A part with any stock at all used to pass, which is how a run of 350 boards
// went out against 338 pieces.
func (m Model) stockShort(i int) (bool, string) {
	p := m.assigned[i]
	if p == nil {
		return false, ""
	}
	// The assembler's shelf is the one the order draws from, and it disagrees with the
	// shop's in both directions.
	stock, fromAsm := m.asmStock(i)
	if stock <= 0 {
		return false, ""
	}
	need := m.needFor(i)
	buy := p.BuyQty(need)
	if stock >= buy {
		return false, ""
	}
	whose := "shop stock"
	if fromAsm {
		whose = "assembly stock"
	}
	return true, fmt.Sprintf("%s %s, the order needs %s", whose, groupThousands(stock), groupThousands(buy))
}

// thinStock counts line items that cover the order but not comfortably, and names the
// tightest.
func (m Model) thinStock() (n int, tightest string) {
	low := 0.0
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		p := m.assigned[i]
		if p == nil {
			continue
		}
		stock, _ := m.asmStock(i)
		need := m.needFor(i)
		buy := p.BuyQty(need)
		if stock <= 0 || buy <= 0 || stock < buy {
			continue
		}
		h := float64(stock) / float64(buy)
		if h >= thinHeadroom {
			continue
		}
		n++
		if low == 0 || h < low {
			low, tightest = h, fmt.Sprintf("%s at %.1f×", m.items[i].ID(), h)
		}
	}
	return
}

// shortCount is how many line items cannot fill the order.
func (m Model) shortCount() int {
	n := 0
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		if bad, _ := m.stockShort(i); bad {
			n++
		}
	}
	return n
}
