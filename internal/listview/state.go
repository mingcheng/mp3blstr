// Package listview holds the cursor/scroll state of a scrollable list,
// replicating the behaviour of mp3blaster's scrollWin (selection bar, page
// up/down, wrap-around and horizontal panning).
package listview

// State is the view state of a list. Rows is the number of visible rows and
// must be kept up to date by the renderer (see SetRows).
type State struct {
	Cursor int
	Offset int
	Pan    int
	Rows   int
}

// SetRows updates the viewport height and keeps the cursor visible.
func (s *State) SetRows(rows, n int) {
	s.Rows = max(rows, 1)
	s.Clamp(n)
}

// rows is the viewport height, at least 1.
func (s *State) rows() int { return max(s.Rows, 1) }

// Clamp keeps cursor and offset within bounds for a list of n items.
func (s *State) Clamp(n int) {
	if n <= 0 {
		s.Cursor, s.Offset = 0, 0
		return
	}
	if s.Cursor >= n {
		s.Cursor = n - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	s.ensureVisible()
	s.Offset = max(0, min(s.Offset, n-s.rows()))
	s.ensureVisible()
}

// ensureVisible scrolls so that the cursor is within the viewport.
func (s *State) ensureVisible() {
	if s.Cursor < s.Offset {
		s.Offset = s.Cursor
	}
	if s.Cursor >= s.Offset+s.rows() {
		s.Offset = s.Cursor - s.rows() + 1
	}
}

// Move moves the cursor by delta, wrapping around when wrap is set (only
// single steps wrap, like the original).
func (s *State) Move(delta, n int, wrap bool) {
	if n == 0 || delta == 0 {
		return
	}
	c := s.Cursor + delta
	if c >= n {
		if !wrap {
			return
		}
		c = 0
	}
	if c < 0 {
		if !wrap {
			return
		}
		c = n - 1
	}
	s.Cursor = c
	s.ensureVisible()
}

// Set moves the cursor to index i.
func (s *State) Set(i, n int) {
	if i < 0 || i >= n {
		return
	}
	s.Cursor = i
	s.ensureVisible()
}

// pageStep is the number of lines a page scrolls: one line of the old page
// stays visible.
func (s *State) pageStep() int { return max(s.rows()-1, 1) }

// PageDown scrolls one page down, keeping the relative cursor position.
func (s *State) PageDown(n int) {
	if n == 0 {
		return
	}
	if s.Offset+s.rows() >= n {
		s.Cursor = n - 1
		s.Clamp(n)
		return
	}
	rel := s.Cursor - s.Offset
	s.Offset = min(s.Offset+s.pageStep(), n-s.rows())
	s.Cursor = min(s.Offset+rel, n-1)
	s.ensureVisible()
}

// PageUp scrolls one page up, keeping the relative cursor position.
func (s *State) PageUp(n int) {
	if n == 0 {
		return
	}
	if s.Offset == 0 {
		s.Cursor = 0
		return
	}
	rel := s.Cursor - s.Offset
	s.Offset = max(s.Offset-s.pageStep(), 0)
	s.Cursor = s.Offset + rel
	s.Clamp(n)
}

// Top jumps to the first item.
func (s *State) Top(n int) { s.Set(0, n) }

// Bottom jumps to the last item.
func (s *State) Bottom(n int) { s.Set(n-1, n) }

// PanBy pans the content horizontally; maxWidth is the length of the
// longest visible line and width the drawable width.
func (s *State) PanBy(delta, maxWidth, width int) {
	limit := max(maxWidth-width, 0)
	s.Pan = max(0, min(s.Pan+delta, limit))
}

// Visible returns the half open range of visible indices.
func (s *State) Visible(n int) (from, to int) {
	return s.Offset, min(s.Offset+s.rows(), n)
}
