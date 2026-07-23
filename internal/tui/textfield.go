package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type textfield struct {
	runes       []rune
	pos         int
	anchor      int
	width       int
	prompt      string
	placeholder string
	focused     bool
}

func newField(prompt, placeholder string, width int) textfield {
	return textfield{anchor: -1, width: width, prompt: prompt, placeholder: placeholder}
}

func (t textfield) Value() string { return string(t.runes) }

func (t *textfield) SetValue(s string) {
	t.runes = []rune(s)
	t.pos = len(t.runes)
	t.anchor = -1
}

func (t *textfield) Focus()       { t.focused = true }
func (t *textfield) Blur()        { t.focused = false }
func (t textfield) Focused() bool { return t.focused }

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func wordLeft(buf []rune, pos int) int {
	i := pos
	for i > 0 && !isWordRune(buf[i-1]) {
		i--
	}
	for i > 0 && isWordRune(buf[i-1]) {
		i--
	}
	return i
}

func wordRight(buf []rune, pos int) int {
	i := pos
	for i < len(buf) && !isWordRune(buf[i]) {
		i++
	}
	for i < len(buf) && isWordRune(buf[i]) {
		i++
	}
	return i
}

func (t *textfield) hasSelection() bool { return t.anchor >= 0 && t.anchor != t.pos }

func (t *textfield) selRange() (int, int) {
	lo, hi := t.anchor, t.pos
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (t *textfield) deleteSelection() bool {
	if !t.hasSelection() {
		t.anchor = -1
		return false
	}
	lo, hi := t.selRange()
	t.runes = append(t.runes[:lo], t.runes[hi:]...)
	t.pos = lo
	t.anchor = -1
	return true
}

func (t *textfield) move(to int, extend bool) {
	to = clampInt(to, 0, len(t.runes))
	if extend {
		if t.anchor < 0 {
			t.anchor = t.pos
		}
	} else {
		t.anchor = -1
	}
	t.pos = to
}

func (t *textfield) collapse(toEnd bool) bool {
	if !t.hasSelection() {
		t.anchor = -1
		return false
	}
	lo, hi := t.selRange()
	if toEnd {
		t.pos = hi
	} else {
		t.pos = lo
	}
	t.anchor = -1
	return true
}

func (t *textfield) Update(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "left":
		if !t.collapse(false) {
			t.move(t.pos-1, false)
		}
	case "right":
		if !t.collapse(true) {
			t.move(t.pos+1, false)
		}
	case "alt+left", "ctrl+left":
		t.move(wordLeft(t.runes, t.pos), false)
	case "alt+right", "ctrl+right":
		t.move(wordRight(t.runes, t.pos), false)
	case "home":
		t.move(0, false)
	case "end", "ctrl+e":
		t.move(len(t.runes), false)
	case "ctrl+a", "cmd+a", "super+a", "meta+a":
		t.anchor = 0
		t.pos = len(t.runes)
	case "shift+left":
		t.move(t.pos-1, true)
	case "shift+right":
		t.move(t.pos+1, true)
	case "alt+shift+left", "ctrl+shift+left":
		t.move(wordLeft(t.runes, t.pos), true)
	case "alt+shift+right", "ctrl+shift+right":
		t.move(wordRight(t.runes, t.pos), true)
	case "shift+home":
		t.move(0, true)
	case "shift+end":
		t.move(len(t.runes), true)
	case "backspace":
		if !t.deleteSelection() && t.pos > 0 {
			t.runes = append(t.runes[:t.pos-1], t.runes[t.pos:]...)
			t.pos--
		}
	case "delete", "ctrl+d":
		if !t.deleteSelection() && t.pos < len(t.runes) {
			t.runes = append(t.runes[:t.pos], t.runes[t.pos+1:]...)
		}
	case "alt+backspace", "ctrl+w":
		if !t.deleteSelection() {
			s := wordLeft(t.runes, t.pos)
			t.runes = append(t.runes[:s], t.runes[t.pos:]...)
			t.pos = s
		}
	case "alt+d":
		if !t.deleteSelection() {
			e := wordRight(t.runes, t.pos)
			t.runes = append(t.runes[:t.pos], t.runes[e:]...)
		}
	case "ctrl+u":
		if !t.deleteSelection() {
			t.runes = t.runes[t.pos:]
			t.pos = 0
		}
	case "ctrl+k":
		if !t.deleteSelection() {
			t.runes = t.runes[:t.pos]
		}
	default:
		t.insert(msg.Text)
	}
}

func (t *textfield) insert(s string) {
	if s == "" {
		return
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return
		}
	}
	t.deleteSelection()
	ins := []rune(s)
	t.runes = append(t.runes[:t.pos], append(ins, t.runes[t.pos:]...)...)
	t.pos += len(ins)
}

func (t textfield) View() string {
	if len(t.runes) == 0 {
		cursor := ""
		if t.focused {
			cursor = cursorStyle.Render(" ")
		}
		return t.prompt + cursor + dimStyle.Render(t.placeholder)
	}

	start := 0
	if t.width > 0 && t.pos >= t.width {
		start = t.pos - t.width + 1
	}

	selLo, selHi := -1, -1
	if t.hasSelection() {
		selLo, selHi = t.selRange()
	}

	var b strings.Builder
	b.WriteString(t.prompt)
	for i := start; i <= len(t.runes); i++ {
		ch := " "
		if i < len(t.runes) {
			ch = string(t.runes[i])
		}
		switch {
		case t.focused && i == t.pos && !t.hasSelection():
			b.WriteString(cursorStyle.Render(ch))
		case i >= selLo && i < selHi:
			b.WriteString(selectionStyle.Render(ch))
		case i < len(t.runes):
			b.WriteString(ch)
		}
	}
	return b.String()
}
