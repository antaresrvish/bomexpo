package tui

import (
	"strings"

	"bomexpo/internal/part"
)

// Both the assign search and the Parts tab render through here, so the two tables
// look and measure identically.

const (
	scCode = 9
	scLib  = 9 // fits "Preferred", the longest library name
	scPkg  = 7
	// wide enough for a grouped 9-digit number: assembly stock hits the tens of
	// millions
	scStock = 11
	scPrice = 8
	scDs    = 9
	scMpn   = 18
)

// LIBRARY only exists for a source that reports it, so LCSC pays nothing for it.
type scols struct {
	code, lib, pkg, stock, price, ds, mpn, desc int
}

func (m Model) resultCols(w int) scols {
	c := scols{code: scCode, pkg: scPkg, stock: scStock, price: scPrice, ds: scDs, mpn: scMpn}
	seps := 6
	if src := m.src(); src != nil && src.Caps().Library {
		c.lib = scLib
		seps++
	}
	c.desc = w - (2 + c.code + c.lib + c.pkg + c.stock + c.price + c.ds + c.mpn) - 3*seps
	if c.desc < 8 {
		c.desc = 8
	}
	return c
}

// dsRange spans the DATASHEET column: content x=2, the 2-wide marker, then the
// columns before it joined by 3-wide separators.
func (c scols) dsRange() (int, int) {
	start := 4 + c.code + c.pkg + c.stock + c.price + 3*4
	if c.lib > 0 {
		start += c.lib + 3
	}
	return start, start + c.ds
}

// libCell colours by what it costs: basic is free, extended pays a setup fee.
func libCell(k part.LibKind, s string) string {
	switch k {
	case part.LibBasic:
		return okStyle.Render(s)
	case part.LibPreferred:
		return accentStyle.Render(s)
	case part.LibExtended:
		return warnStyle.Render(s)
	}
	return dimStyle.Render(s)
}

func libText(k part.LibKind) string {
	if !k.Known() {
		return "—"
	}
	return k.String()
}

func partHead(c scols, w int) string {
	heads := []string{pad("CODE", c.code)}
	if c.lib > 0 {
		heads = append(heads, pad("LIBRARY", c.lib))
	}
	heads = append(heads, pad("PKG", c.pkg), pad("STOCK", c.stock), pad("PRICE", c.price),
		pad("DATASHEET", c.ds), pad("MPN", c.mpn), pad("DESCRIPTION", c.desc))
	return colHeadStyle.Render(padRender(strings.Join(heads, " | "), w))
}

// marker is the 2-wide gutter. The cursor row stays unstyled per cell so the
// highlight reads as one block.
func partRow(p part.Part, c scols, w int, marker string, cursor bool) string {
	dsc := ""
	if p.Datasheet != "" {
		dsc = "datasheet"
	}
	plain := []string{pad(p.Code, c.code)}
	if c.lib > 0 {
		plain = append(plain, pad(libText(p.Lib), c.lib))
	}
	plain = append(plain,
		pad(p.Package, c.pkg), pad(groupThousands(p.Stock), c.stock),
		pad(p.PriceLabel(), c.price), pad(dsc, c.ds),
		pad(trunc(p.MPN, c.mpn), c.mpn), pad(p.Description(), c.desc),
	)
	if cursor {
		return selRowStyle.Render(padRender(marker+strings.Join(plain, "   "), w))
	}

	cells := []string{accentStyle.Render(plain[0])}
	if c.lib > 0 {
		cells = append(cells, libCell(p.Lib, plain[1]))
	}
	rest := plain[len(cells):]
	stock := okStyle.Render(rest[1])
	if !p.InStock() {
		stock = badStyle.Render(pad("out", c.stock))
	}
	cells = append(cells,
		subtleStyle.Render(rest[0]), stock, warnStyle.Render(rest[2]),
		dsCellStyle(dsc, rest[3]), rest[4], descStyle.Render(rest[5]),
	)
	return padRender(marker+strings.Join(cells, sepStyle.Render(" | ")), w)
}
