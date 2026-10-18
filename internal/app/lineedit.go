package app

import "github.com/rivo/uniseg"

// LineEditor is the single line text input of popup windows. Like the
// original it starts in replace mode; the up/down keys toggle insert mode.
type LineEditor struct {
	buf    []rune
	pos    int
	insert bool
	maxLen int
	scroll int
}

// NewLineEditor creates an editor holding initial text.
func NewLineEditor(initial string, maxLen int) *LineEditor {
	e := &LineEditor{maxLen: maxLen}
	for _, r := range initial {
		if maxLen > 0 && len(e.buf) >= maxLen {
			break
		}
		e.buf = append(e.buf, r)
	}
	return e
}

// Text returns the current contents.
func (e *LineEditor) Text() string { return string(e.buf) }

// Cursor returns the cursor position (in runes).
func (e *LineEditor) Cursor() int { return e.pos }

// InsertMode reports whether characters are inserted (or replaced).
func (e *LineEditor) InsertMode() bool { return e.insert }

// ToggleInsert switches between insert and replace mode and returns the
// new mode (true for insert).
func (e *LineEditor) ToggleInsert() bool {
	e.insert = !e.insert
	return e.insert
}

// Fits reports whether n more characters typed at the cursor fit within
// the length limit: in insert mode they lengthen the text, in replace mode
// they overwrite it up to its end first.
func (e *LineEditor) Fits(n int) bool {
	if e.maxLen <= 0 {
		return true
	}
	newLen := len(e.buf) + n
	if !e.insert {
		newLen = max(len(e.buf), e.pos+n)
	}
	return newLen <= e.maxLen
}

// AddChar inserts or overwrites at the cursor. Control characters and
// characters beyond the length limit are ignored.
func (e *LineEditor) AddChar(r rune) {
	if r < ' ' {
		return
	}
	if e.insert || e.pos == len(e.buf) {
		if e.maxLen > 0 && len(e.buf) >= e.maxLen {
			return
		}
		e.buf = append(e.buf, 0)
		copy(e.buf[e.pos+1:], e.buf[e.pos:])
	}
	e.buf[e.pos] = r
	e.pos++
}

// Backspace deletes the character before the cursor.
func (e *LineEditor) Backspace() {
	if e.pos == 0 {
		return
	}
	e.buf = append(e.buf[:e.pos-1], e.buf[e.pos:]...)
	e.pos--
}

// Delete deletes the character under the cursor.
func (e *LineEditor) Delete() {
	if e.pos >= len(e.buf) {
		return
	}
	e.buf = append(e.buf[:e.pos], e.buf[e.pos+1:]...)
}

// Left moves the cursor left.
func (e *LineEditor) Left() {
	if e.pos > 0 {
		e.pos--
	}
}

// Right moves the cursor right.
func (e *LineEditor) Right() {
	if e.pos < len(e.buf) {
		e.pos++
	}
}

// Home moves to the start.
func (e *LineEditor) Home() { e.pos = 0 }

// End moves to the end.
func (e *LineEditor) End() { e.pos = len(e.buf) }

// SetCursor moves the cursor to rune index i, clamped to the text.
func (e *LineEditor) SetCursor(i int) { e.pos = min(max(i, 0), len(e.buf)) }

// Visible returns the part of the text visible in a field width cells wide,
// the cursor column within that field and the index of the first visible
// rune.
func (e *LineEditor) Visible(width int) (text string, cursorCol, first int) {
	if width < 1 {
		width = 1
	}
	if e.pos < e.scroll {
		e.scroll = e.pos
	}
	for e.scroll < e.pos && runesWidth(e.buf[e.scroll:e.pos]) >= width {
		e.scroll++
	}
	w := 0
	end := e.scroll
	for end < len(e.buf) {
		cw := uniseg.StringWidth(string(e.buf[end]))
		if w+cw > width {
			break
		}
		w += cw
		end++
	}
	return string(e.buf[e.scroll:end]), runesWidth(e.buf[e.scroll:e.pos]), e.scroll
}

// runesWidth returns the number of terminal cells r occupies.
func runesWidth(r []rune) int { return uniseg.StringWidth(string(r)) }
