package kicad

import "fmt"

// FitsLand reports whether a part offering partPads can sit on a land offering
// boardPads. Only the direction that ruins a board counts: KiCad footprints carry
// thermal vias, paste-only pads and paired mounting pads no vendor land pattern
// does, so a land with pads to spare is routine. Measured over 198 assigned parts
// on six boards, this raised two real faults; testing for equality raised three
// false alarms on one board alone.
func FitsLand(boardPads, partPads int) (bool, string) {
	if boardPads <= 0 || partPads <= 0 || partPads <= boardPads {
		return true, ""
	}
	return false, fmt.Sprintf("part has %d pads, the land has %d", partPads, boardPads)
}
