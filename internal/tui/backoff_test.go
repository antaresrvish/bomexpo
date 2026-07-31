package tui

import (
	"strings"
	"testing"
	"time"

	"bomexpo/internal/easyeda"
	"bomexpo/internal/webjson"
)

func rateLimited() error { return &webjson.HTTPError{Status: 403, Bytes: 919} }

// Being turned away is not the part's fault. Spending its attempts on a rate limit
// is what made the panel say it had given up on a part that was perfectly fetchable
// a moment earlier.
func TestARateLimitDoesNotSpendAPartsAttempts(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 46
	m.mode = modeTable
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	m.edaFetching = map[string]bool{}

	code := m.selCode()
	if m.askPadsCmd(code) == nil {
		t.Fatal("the first ask was refused")
	}
	mm, cmd := m.Update(footprintDoneMsg{code: code, err: rateLimited()})
	m = mm.(Model)

	if m.edaTried[code] != 0 {
		t.Errorf("attempts spent = %d, want the rate limit not to count", m.edaTried[code])
	}
	if !m.waitingOnVendor() {
		t.Error("carried on asking after being turned away")
	}
	if cmd == nil {
		t.Error("nothing scheduled to resume after the backoff")
	}
	// and nothing is asked while waiting
	if m.askPadsCmd(code) != nil {
		t.Error("asked again during the backoff")
	}

	out := stripANSI(strings.Join(m.partFootprintHeader(48), "\n"))
	if !strings.Contains(out, "turning us away") {
		t.Errorf("the panel blamed the part instead of the vendor: %q", out)
	}
	if strings.Contains(out, "could not reach") {
		t.Errorf("said it gave up when it is only waiting: %q", out)
	}

	// the pre-flight says why, rather than counting them as missing geometry
	pf := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(pf, "rate-limiting") {
		t.Errorf("pre-flight hid the reason:\n%s", pf)
	}
}

// When the backoff is over, asking resumes on its own.
func TestAskingResumesAfterTheBackoff(t *testing.T) {
	m := fitModel()
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	m.edaFetching = map[string]bool{}
	m.padWait = time.Now().Add(padBackoff)

	if m.askPadsCmd(m.selCode()) != nil {
		t.Fatal("asked during the backoff")
	}
	mm, cmd := m.Update(padResumeMsg{})
	m = mm.(Model)
	if m.waitingOnVendor() {
		t.Error("still waiting after the resume")
	}
	if cmd == nil {
		t.Error("the resume asked for nothing, so the panel stays blank until a keypress")
	}
}
