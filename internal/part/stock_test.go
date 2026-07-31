package part

import "testing"

// Every case is a real line from the boards this was measured on.
func TestCoversAnOrder(t *testing.T) {
	cases := []struct {
		name         string
		stock, moq   int
		need         int
		want         bool
		wantHeadroom float64
	}{
		{"fpc1 on the 350-board order", 338, 1, 350, false, 338.0 / 350},
		{"d2, ten in stock", 10, 1, 350, false, 10.0 / 350},
		{"serv1, three per board", 920, 1, 1050, false, 920.0 / 1050},
		{"y2, thin but enough", 449, 1, 350, true, 449.0 / 350},
		{"plenty", 4198450, 1, 1400, true, 4198450.0 / 1400},
		{"the minimum order is what has to be in stock", 30, 50, 10, false, 30.0 / 50},
		{"no order to fill", 0, 1, 0, true, 0},
	}
	for _, c := range cases {
		p := Part{Stock: c.stock, MinBuy: c.moq}
		if got := p.Covers(c.need); got != c.want {
			t.Errorf("%s: Covers(%d) with stock %d = %v, want %v", c.name, c.need, c.stock, got, c.want)
		}
		if got := p.Headroom(c.need); c.need > 0 && abs(got-c.wantHeadroom) > 0.001 {
			t.Errorf("%s: Headroom = %.4f, want %.4f", c.name, got, c.wantHeadroom)
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
