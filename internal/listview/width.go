package listview

import (
	"fmt"
	"strconv"
	"strings"
)

// Width specifies how wide the main list window is: a fixed number of
// columns, a percentage of the terminal width, or unlimited (zero value).
type Width struct {
	Cols    int // fixed width in columns (0 = not fixed)
	Percent int // percentage of the terminal width (0 = not proportional)
}

// Width limits.
const (
	MinWidthCols    = 40
	MinWidthPercent = 20
)

// DefaultWidth is three quarters of the terminal.
var DefaultWidth = Width{Percent: 75}

// ParseWidth parses "50%", "80" or "0" (unlimited).
func ParseWidth(s string) (Width, error) {
	s = strings.TrimSpace(s)
	if p, ok := strings.CutSuffix(s, "%"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < MinWidthPercent || n > 100 {
			return Width{}, fmt.Errorf("invalid list width %q (%d%%..100%%)", s, MinWidthPercent)
		}
		return Width{Percent: n}, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || (n != 0 && n < MinWidthCols) {
		return Width{}, fmt.Errorf("invalid list width %q (0, >= %d columns or a percentage)", s, MinWidthCols)
	}
	return Width{Cols: n}, nil
}

// String formats the width the way ParseWidth accepts it.
func (w Width) String() string {
	if w.Percent > 0 {
		return strconv.Itoa(w.Percent) + "%"
	}
	return strconv.Itoa(w.Cols)
}

// Resolve returns the width for a terminal termW columns wide, between
// lo and hi (hi wins when lo > hi). The zero Width yields hi.
func (w Width) Resolve(termW, lo, hi int) int {
	n := hi
	switch {
	case w.Percent > 0:
		n = (termW*w.Percent + 50) / 100
		n = max(n, lo)
	case w.Cols > 0:
		n = w.Cols
	}
	return min(n, hi)
}
