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
	if rows < 1 {
		rows = 1
	}
	s.Rows = rows
	s.Clamp(n)
}

func (s *State) rows() int {
	if s.Rows < 1 {
		return 1
	}
	return s.Rows
}

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
	if max := n - s.rows(); s.Offset > max {
		s.Offset = max
	}
	if s.Offset < 0 {
		s.Offset = 0
	}
	s.ensureVisible()
}

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

func (s *State) pageStep() int {
	if st := s.rows() - 1; st > 0 {
		return st
	}
	return 1
}

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
	s.Offset += s.pageStep()
	if max := n - s.rows(); s.Offset > max {
		s.Offset = max
	}
	s.Cursor = s.Offset + rel
	if s.Cursor >= n {
		s.Cursor = n - 1
	}
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
	s.Offset -= s.pageStep()
	if s.Offset < 0 {
		s.Offset = 0
	}
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
	limit := maxWidth - width
	if limit < 0 {
		limit = 0
	}
	s.Pan += delta
	if s.Pan > limit {
		s.Pan = limit
	}
	if s.Pan < 0 {
		s.Pan = 0
	}
}

// Visible returns the half open range of visible indices.
func (s *State) Visible(n int) (from, to int) {
	from = s.Offset
	to = from + s.rows()
	if to > n {
		to = n
	}
	return from, to
}
