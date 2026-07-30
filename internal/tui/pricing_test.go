package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// priceModel is a board whose parts have known ladders, so the break maths can be
// checked against numbers worked out by hand.
func priceModel(t *testing.T) Model {
	t.Helper()
	m := New(Options{})
	m.w, m.h = 140, 40
	m.items = []kicad.Item{
		{Bases: []string{"C1"}, Designators: []string{"C1", "C2", "C3", "C4"}, Value: "100nF", Quantity: 4},
		{Bases: []string{"R1"}, Designators: []string{"R1"}, Value: "10k", Quantity: 1},
		{Bases: []string{"U1"}, Designators: []string{"U1"}, Value: "STM32", Quantity: 1},
	}
	m.excluded = make([]bool, len(m.items))
	m.assigned = []*part.Part{
		{Code: "C1525", Stock: 1e6, Prices: []part.Price{
			{Ladder: 1, USD: 0.01}, {Ladder: 100, USD: 0.004}, {Ladder: 1000, USD: 0.0023}}},
		{Code: "C25744", Stock: 1e6, MinBuy: 100, Prices: []part.Price{
			{Ladder: 1, USD: 0.008}, {Ladder: 500, USD: 0.0015}}},
		{Code: "C8734", Stock: 5000, Prices: []part.Price{
			{Ladder: 1, USD: 2.10}, {Ladder: 10, USD: 1.82}, {Ladder: 100, USD: 1.55}}},
	}
	return m.reindex()
}

func boardsOf(rows []priceRow) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.Boards
	}
	return out
}

// The board counts come from the parts, not from a made-up 100/200/300 ladder. A
// part used four times a board crosses its 100-piece break at 25 boards, not 100.
func TestBreakBoardsComeFromTheParts(t *testing.T) {
	m := priceModel(t)
	got := m.breakBoards()
	want := []int{1, 10, 25, 100, 250, 500}
	if len(got) != len(want) {
		t.Fatalf("breaks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("breaks = %v, want %v", got, want)
			break
		}
	}
}

// Each row says what changes there, so the number isn't a mystery.
func TestPricingRowsExplainThemselves(t *testing.T) {
	m := priceModel(t)
	rows := m.pricingRows(0, 10)
	want := map[int]string{
		1:   "",
		10:  "U1 hits 10",
		25:  "C1 hits 100",
		100: "R1 clears its minimum",
		250: "C1 hits 1000",
		500: "R1 hits 500",
	}
	for _, r := range rows {
		if got, ok := want[r.Boards]; !ok || got != r.Why {
			t.Errorf("%d boards: %q, want %q", r.Boards, r.Why, got)
		}
	}
	// and the price really goes down as the counts go up
	for i := 1; i < len(rows); i++ {
		if rows[i].PerBoard > rows[i-1].PerBoard {
			t.Errorf("per board went up from %d to %d boards: %.4f → %.4f",
				rows[i-1].Boards, rows[i].Boards, rows[i-1].PerBoard, rows[i].PerBoard)
		}
	}
}

// A tier listing the same price as the one below it is not a break. Counting it
// would produce a row claiming a part got cheaper where it didn't.
func TestPricingIgnoresTiersThatCostTheSame(t *testing.T) {
	m := priceModel(t)
	m.items = append(m.items, kicad.Item{
		Bases: []string{"L1"}, Designators: []string{"L1"}, Value: "4.7uH", Quantity: 1})
	m.excluded = append(m.excluded, false)
	m.assigned = append(m.assigned, &part.Part{Code: "C1", Stock: 1e6, Prices: []part.Price{
		{Ladder: 1, USD: 0.05}, {Ladder: 37, USD: 0.05}, {Ladder: 61, USD: 0.03}}})
	m = m.reindex()

	got := m.breakBoards()
	if hasBoard(got, 37) {
		t.Errorf("37 is not a break — L1 costs the same there: %v", got)
	}
	if !hasBoard(got, 61) {
		t.Errorf("61 is a real break and went missing: %v", got)
	}
	for _, r := range m.pricingRows(0, 20) {
		if r.Boards == 61 && r.Why != "L1 hits 61" {
			t.Errorf("61 boards is labelled %q, want L1 hits 61", r.Why)
		}
	}
}

func hasBoard(ns []int, want int) bool {
	for _, n := range ns {
		if n == want {
			return true
		}
	}
	return false
}

// The count you actually plan to order is marked, and appears exactly once even when
// it is already a break.
func TestPricingMarksYourOrderOnce(t *testing.T) {
	m := priceModel(t)
	for _, target := range []int{250, 300} {
		rows := m.pricingRows(target, 10)
		n := 0
		for _, r := range rows {
			if r.Boards == target {
				n++
				if r.Why != "your order" {
					t.Errorf("target %d is labelled %q", target, r.Why)
				}
			}
		}
		if n != 1 {
			t.Errorf("target %d appears %d times in %v", target, n, boardsOf(rows))
		}
	}
}

// Too many rows is as unhelpful as too few, and thinning must keep the ends and the
// target.
func TestPricingThinsButKeepsWhatMatters(t *testing.T) {
	m := priceModel(t)
	full := m.pricingRows(300, 0)
	if len(full) < 5 {
		t.Skipf("only %d rows, nothing to thin", len(full))
	}
	thin := m.pricingRows(300, 4)
	if len(thin) > 4 {
		t.Errorf("%d rows, asked for 4", len(thin))
	}
	if thin[0].Boards != full[0].Boards {
		t.Error("thinning dropped the first row")
	}
	if thin[len(thin)-1].Boards != full[len(full)-1].Boards {
		t.Error("thinning dropped the last row")
	}
	found := false
	for _, r := range thin {
		if r.Why == "your order" {
			found = true
		}
	}
	if !found {
		t.Error("thinning dropped the count you are ordering")
	}
}

// An unassigned board has nothing to price, and says so rather than showing zeroes.
func TestPricingWithNothingAssigned(t *testing.T) {
	m := priceModel(t)
	for i := range m.assigned {
		m.assigned[i] = nil
	}
	if got := m.breakBoards(); len(got) != 1 || got[0] != 1 {
		t.Errorf("breaks = %v, want just the baseline", got)
	}
	out := stripANSI(m.viewCheck(m.contentW(), m.contentH()))
	if !strings.Contains(out, "assign some parts") {
		t.Errorf("expected an explanation:\n%s", out)
	}
}

// q takes a board count, digits only, and sets what the table is priced for.
func TestCheckBoardCountInput(t *testing.T) {
	m := priceModel(t)
	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)

	mm, _ = m.updateCheck(key("q"))
	m = mm.(Model)
	if !m.check.qty.Focused() {
		t.Fatal("q should focus the board count")
	}
	for _, k := range []string{"2", "5", "x", "0"} { // the x must not land
		mm, _ = m.updateCheck(key(k))
		m = mm.(Model)
	}
	if got := m.check.qty.Value(); got != "250" {
		t.Errorf("typed value = %q, want 250 — letters are not quantities", got)
	}
	mm, _ = m.updateCheck(key("enter"))
	m = mm.(Model)
	if m.check.target != 250 {
		t.Errorf("target = %d, want 250", m.check.target)
	}
	if m.check.qty.Focused() {
		t.Error("enter should hand the keyboard back")
	}
	out := stripANSI(m.viewCheck(m.contentW(), m.contentH()))
	if !strings.Contains(out, "your order") {
		t.Errorf("the table does not mark the order:\n%s", out)
	}
}

// The highlighted per-board cell must not touch the label beside it — a background
// running into text reads as one word.
func TestPricingTargetCellKeepsItsGap(t *testing.T) {
	m := priceModel(t)
	m.check.target = 250
	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)

	var row string
	for _, ln := range strings.Split(m.viewCheck(m.contentW(), m.contentH()), "\n") {
		if strings.Contains(stripANSI(ln), "your order") {
			row = ln
			break
		}
	}
	if row == "" {
		t.Fatal("no target row on screen")
	}
	plain := stripANSI(row)
	i := strings.Index(plain, "your order")
	if i < 1 || plain[i-1] != ' ' {
		t.Errorf("no gap before the label: %q", plain)
	}
	// the highlight must close before the label starts
	upto := stripANSI(row[:strings.Index(row, "your order")])
	if !strings.HasSuffix(upto, " ") {
		t.Errorf("the styled cell runs into the label: %q", upto)
	}
}
