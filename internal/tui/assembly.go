package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/part"
	"bomexpo/internal/webjson"
)

// bomexpo assigns from LCSC's catalogue, which is a superset of what JLCPCB will
// place. A part can be in stock at the shop and absent from the assembly library —
// C42387326 came back "No Part Selected" on a real order with 5,240 at LCSC. The two
// stocks differ in both directions: C23162 read 0 at the shop and 3.6M for assembly.

const asmLanes = 3

// asmRecord is what the assembly library says about a code.
type asmRecord struct {
	Found bool
	Stock int
	Lib   part.LibKind
	MPN   string
}

// asmResumeMsg says the assembly backoff is over.
type asmResumeMsg struct{}

type asmDoneMsg struct {
	code string
	rec  asmRecord
	err  error
}

// asmProvider is the source that quotes assembly, whichever source search is using.
func (m Model) asmProvider() part.Provider {
	for _, p := range m.srcs {
		if p.Caps().Assembly {
			return p
		}
	}
	return nil
}

// asmCmd asks the assembly library about one code.
func (m Model) asmCmd(code string) tea.Cmd {
	p := m.asmProvider()
	if p == nil || code == "" || m.asmFetching[code] || m.asmTried[code] >= maxFitAttempts {
		return nil
	}
	if _, have := m.asm[code]; have {
		return nil
	}
	if !m.asmWait.IsZero() && time.Now().Before(m.asmWait) {
		return nil
	}
	m.asmTried[code]++
	m.asmFetching[code] = true
	return func() tea.Msg {
		got, err := p.Detail(code)
		if err != nil {
			// Detail fails for a code the library has no record of, which is the
			// answer rather than a failure — unless the network said so.
			if webjson.RateLimited(err) {
				return asmDoneMsg{code: code, err: err}
			}
			return asmDoneMsg{code: code, rec: asmRecord{}}
		}
		return asmDoneMsg{code: code, rec: asmRecord{
			Found: true, Stock: got.Stock, Lib: got.Lib, MPN: got.MPN,
		}}
	}
}

// asmCmds fills the free lanes with assembly lookups.
func (m Model) asmCmds() []tea.Cmd {
	free := asmLanes - len(m.asmFetching)
	if free <= 0 {
		return nil
	}
	var cmds []tea.Cmd
	for i := range m.items {
		if len(cmds) >= free {
			break
		}
		if c := m.asmCmd(m.items[i].LCSC); c != nil {
			cmds = append(cmds, c)
		}
	}
	return cmds
}

// unplaceable reports whether the assembler has no record of this part, and says so.
func (m Model) unplaceable(i int) (bool, string) {
	if i < 0 || i >= len(m.items) {
		return false, ""
	}
	rec, have := m.asm[m.items[i].LCSC]
	if !have || rec.Found {
		return false, ""
	}
	return true, "the assembler has no record of this part — it will come back unmatched"
}

// asmStock is the stock the order is actually drawn from, and whether it is the
// assembler's. Falls back to the shop's when the library hasn't answered.
func (m Model) asmStock(i int) (int, bool) {
	if rec, have := m.asm[m.items[i].LCSC]; have && rec.Found {
		return rec.Stock, true
	}
	if p := m.assigned[i]; p != nil {
		return p.Stock, false
	}
	return 0, false
}

// asmTally counts what the assembly check found.
func (m Model) asmTally() (missing, checked, pending int) {
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		code := m.items[i].LCSC
		if code == "" {
			continue
		}
		rec, have := m.asm[code]
		switch {
		case have && rec.Found:
			checked++
		case have:
			missing++
		case m.asmTried[code] >= maxFitAttempts:
		default:
			pending++
		}
	}
	return
}

// asmCheck is the pre-flight line for it.
func (m Model) asmCheck(chk func(bool, string, string) string) []string {
	if m.asmProvider() == nil {
		return nil
	}
	missing, checked, pending := m.asmTally()
	switch {
	case missing > 0:
		return []string{chk(false, "", fmt.Sprintf("%s the assembler cannot place",
			plural(missing, "part", "parts")))}
	case pending > 0:
		return []string{dimStyle.Render(fmt.Sprintf("· checking %d parts against the assembly library…", pending))}
	case checked > 0:
		return []string{chk(true, fmt.Sprintf("all %d parts are in the assembly library", checked), "")}
	}
	return nil
}
