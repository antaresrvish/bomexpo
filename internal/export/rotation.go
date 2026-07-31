package export

import (
	"math"
	"regexp"

	"bomexpo/internal/kicad"
)

// JLCPCB's pick-and-place references 0° differently than the KiCad standard
// footprints for a handful of IC families; these offsets realign the exported
// angle. Passives, QFN/DFN and connectors match already or are part-specific,
// so anything not listed here keeps KiCad's rotation untouched.
var rotationRules = []struct {
	re     *regexp.Regexp
	offset float64
}{
	{regexp.MustCompile(`(?i)^SOT-223`), 180},
	{regexp.MustCompile(`(?i)^SOT-23`), 180},
	{regexp.MustCompile(`(?i)^SOT-89`), 180},
	{regexp.MustCompile(`(?i)^SOT-143`), 180},
	{regexp.MustCompile(`(?i)^SOT-3[56]3`), 180},
	{regexp.MustCompile(`(?i)^D_SOD-(123|323|523)`), 180},
	{regexp.MustCompile(`(?i)^SOIC-`), 270},
	{regexp.MustCompile(`(?i)^SSOP-`), 270},
	{regexp.MustCompile(`(?i)^TSSOP-`), 270},
	{regexp.MustCompile(`(?i)^HTSSOP-`), 270},
	{regexp.MustCompile(`(?i)^MSOP-`), 270},
	{regexp.MustCompile(`(?i)^VSSOP-`), 270},
	{regexp.MustCompile(`(?i)^TSOP-`), 270},
}

// FamilyOffset is the built-in rotation correction for a footprint family, or
// 0 when none applies. Exposed for the components table's rotation column.
func FamilyOffset(footprint string) float64 {
	off, _ := rotationOffset(footprint)
	return off
}

func rotationOffset(footprint string) (float64, bool) {
	for _, r := range rotationRules {
		if r.re.MatchString(footprint) {
			return r.offset, true
		}
	}
	return 0, false
}

// edaLibs are footprint libraries drawn to the vendor's own 0°, because that is
// where they came from. Applying a KiCad-family offset to one of these turns a
// correct angle into a wrong one: an easyeda2kicad SOT-23 went out 180° over on a
// real order, and the library prefix is the only thing that says so.
var edaLibs = regexp.MustCompile(`(?i)easyeda|jlc|lcsc`)

// correctRotation returns the JLCPCB-aligned angle. The family offset only applies
// to KiCad's own footprints. A part on the bottom is seen from the other side, so
// its angle mirrors.
func correctRotation(footprint, lib string, rot float64, bottom bool) float64 {
	if bottom {
		rot = 180 - rot
	}
	off, ok := rotationOffset(footprint)
	if !ok || edaLibs.MatchString(lib) {
		return normDeg(rot)
	}
	if bottom {
		off = -off
	}
	return normDeg(rot + off)
}

func normDeg(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	return d
}

type RotationFix struct {
	Designator string
	Footprint  string
	From, To   float64
	Manual     bool
}

// appliedRot is the angle written to the CPL: a manual per-designator override
// (applied literally) when present, otherwise the footprint-family correction.
func appliedRot(footprint, lib string, rot float64, bottom bool, override map[string]int, ref string) (float64, bool) {
	if off, ok := override[ref]; ok {
		return normDeg(rot + float64(off)), true
	}
	return correctRotation(footprint, lib, rot, bottom), false
}

// RotationFixes lists the placements whose angle changes in the CPL — via the
// family correction or a manual override — so the UI can show them instead of
// correcting silently.
func RotationFixes(placements []kicad.Placement, exclude map[string]bool, override map[string]int) []RotationFix {
	var out []RotationFix
	for _, p := range placements {
		if exclude[p.Designator] {
			continue
		}
		to, manual := appliedRot(p.Package, p.PackageLib, p.Rotation, p.Layer == "bottom", override, p.Designator)
		if manual || math.Abs(to-normDeg(p.Rotation)) > 1e-6 {
			out = append(out, RotationFix{p.Designator, p.Package, p.Rotation, to, manual})
		}
	}
	return out
}
