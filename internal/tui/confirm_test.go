package tui

import (
	"strings"
	"testing"
)

// exportModel is Export with a board that has issues and a path ready to write.
func exportModel(t *testing.T) Model {
	t.Helper()
	m := filterModel(t)
	m.w, m.h = 130, 32
	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)
	m.check.out.SetValue("/tmp/order.zip")
	if len(m.issues()) == 0 {
		t.Fatal("the fixture is supposed to have issues")
	}
	return m
}

// Exporting a board with issues asks first, and asking must not have already written
// anything.
func TestExportWithIssuesAsksFirst(t *testing.T) {
	m := exportModel(t)
	mm, cmd := m.updateCheck(key("x"))
	m = mm.(Model)
	if !m.check.confirm.open {
		t.Fatal("x with issues open should ask")
	}
	if cmd != nil {
		t.Error("it asked and started the export anyway")
	}
	if m.check.confirm.yes {
		t.Error("the default answer should be no — this writes an order")
	}
}

// A clean board exports without a question in the way.
func TestExportWithoutIssuesDoesNotAsk(t *testing.T) {
	m := exportModel(t)
	// resolve every issue
	for i := range m.items {
		m.excluded[i] = true
	}
	m = m.reindex()
	if len(m.issues()) != 0 {
		t.Fatalf("still %d issues", len(m.issues()))
	}
	mm, cmd := m.updateCheck(key("x"))
	if mm.(Model).check.confirm.open {
		t.Error("a clean board should not be asked about")
	}
	if cmd == nil {
		t.Error("a clean board should just export")
	}
}

// The question says what is wrong and names some of it, so the answer is informed.
func TestConfirmListsWhatIsWrong(t *testing.T) {
	m := exportModel(t)
	mm, _ := m.updateCheck(key("x"))
	m = mm.(Model)
	out := stripANSI(m.viewConfirm(m.contentW(), m.contentH()))

	for _, want := range []string{
		"Export with issues open?",
		"2 line items are not ready to order",
		"with no part assigned",
		"out of stock",
		"R1", "L1",
		"The zip will still be written, without them.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the question never says %q:\n%s", want, out)
		}
	}
	// the page behind it is still visible, so you can see what you are agreeing to
	if !strings.Contains(out, "Pre-fligh") {
		t.Error("the popup replaced the page instead of floating over it")
	}
}

// Both answers work, from the arrows and from the letters.
func TestConfirmAnswers(t *testing.T) {
	// no, via esc
	m := exportModel(t)
	mm, _ := m.updateCheck(key("x"))
	mm, cmd := mm.(Model).updateCheck(key("esc"))
	if mm.(Model).check.confirm.open || cmd != nil {
		t.Error("esc should close the question and write nothing")
	}

	// no, via the letter
	m = exportModel(t)
	mm, _ = m.updateCheck(key("x"))
	mm, cmd = mm.(Model).updateCheck(key("n"))
	if mm.(Model).check.confirm.open || cmd != nil {
		t.Error("n should close the question and write nothing")
	}

	// yes, via the letter
	m = exportModel(t)
	mm, _ = m.updateCheck(key("x"))
	mm, cmd = mm.(Model).updateCheck(key("y"))
	if mm.(Model).check.confirm.open {
		t.Error("y should close the question")
	}
	if cmd == nil {
		t.Error("y should start the export")
	}

	// yes, by moving the cursor and pressing enter
	m = exportModel(t)
	mm, _ = m.updateCheck(key("x"))
	m = mm.(Model)
	mm, _ = m.updateCheck(key("right"))
	m = mm.(Model)
	if !m.check.confirm.yes {
		t.Fatal("right should move the answer to yes")
	}
	if !strings.Contains(stripANSI(m.confirmButtons(90)), "▸ yes") {
		t.Error("the chosen answer should be marked, not just coloured")
	}
	mm, cmd = m.updateCheck(key("enter"))
	if mm.(Model).check.confirm.open || cmd == nil {
		t.Error("enter on yes should export")
	}
}

// While the question is up it owns the keyboard: a stray key must not pan the board
// or move the issue cursor behind it.
func TestConfirmOwnsTheKeyboard(t *testing.T) {
	m := exportModel(t)
	mm, _ := m.updateCheck(key("x"))
	m = mm.(Model)
	before := m
	for _, k := range []string{"j", "t", "q", "v", "0", "+"} {
		mm, cmd := m.updateCheck(key(k))
		got := mm.(Model)
		if cmd != nil {
			t.Errorf("%q started something while the question was up", k)
		}
		if got.mode != before.mode || got.check.cur != before.check.cur || got.boardv != before.boardv {
			t.Errorf("%q reached the page behind the question", k)
		}
		if got.check.qty.Focused() {
			t.Errorf("%q opened the board count behind the question", k)
		}
	}
	if !m.check.confirm.open {
		t.Error("the question closed on a key that means nothing to it")
	}
}

// Nothing to write to means nothing to confirm — say what is missing instead.
func TestExportWithNoPathSaysSoBeforeAsking(t *testing.T) {
	m := exportModel(t)
	m.check.out.SetValue("")
	mm, cmd := m.updateCheck(key("x"))
	if mm.(Model).check.confirm.open {
		t.Error("it asked about writing to nowhere")
	}
	if cmd != nil {
		t.Error("it tried to write to nowhere")
	}
	if !strings.Contains(mm.(Model).err, "output path is empty") {
		t.Errorf("err = %q, want it to name the missing path", mm.(Model).err)
	}
}
