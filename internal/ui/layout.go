package ui

import "github.com/mingcheng/mp3blstr/internal/listview"

// Minimum terminal size this interface needs.
const (
	minWidth  = 60
	minHeight = 20
)

// compactPanelWidth is the width of the right hand panel of the original
// (COLS - 12 .. COLS - 2); widePanelWidth is the width from which the panel
// switches to the extended layout.
const (
	compactPanelWidth = 11
	widePanelWidth    = 24
)

// layout holds the geometry of the screen, computed with the formulas of
// the original (main window: COLS-12 x LINES-13 at row 10, an 11 column
// panel on the right, a 4 line help bar at the top). The main window can be
// limited to a maximum width, in which case the panel gets the rest.
type layout struct {
	w, h  int
	mainW int // width of the main window including its borders
	px    int // first column of the right hand panel
	pw    int // width of the right hand panel (without the frame)

	help     rect // key overview
	main     rect // main window including its border
	list     rect // inside of the main window
	popup    rect
	modeLine rect
	nextSong rect
	status   rect // song status symbol
	title    rect // song title
	message  rect
}

// minMainWidth is the narrowest main window for a proportional width; it
// keeps the play mode and sort mode lines above the window readable.
const minMainWidth = 60

// newLayout computes the geometry. The main window is at most COLS-12 wide
// (the original rule); spec can make it narrower.
func newLayout(w, h int, spec listview.Width) layout {
	mainW := spec.Resolve(w, minMainWidth, w-12)
	l := layout{w: w, h: h, mainW: mainW, px: mainW, pw: w - 1 - mainW}
	l.help = rect{1, 1, w - 2, 4}
	l.main = rect{0, 10, mainW, h - 13}
	l.list = rect{1, 11, mainW - 2, h - 15}
	l.popup = l.list
	l.modeLine = rect{2, 6, mainW - 4, 1}
	l.nextSong = rect{19, 7, mainW - 21, 1}
	l.status = rect{1, h - 3, 2, 1}
	l.title = rect{4, h - 3, mainW - 6, 1}
	l.message = rect{1, h - 2, mainW - 2, 1}
	return l
}

func (l layout) tooSmall() bool { return l.w < minWidth || l.h < minHeight }

// wide reports whether the panel uses the extended layout.
func (l layout) wide() bool { return l.pw >= widePanelWidth }

// cx returns the first column of the compact panel content, centred when
// the panel is wider than in the original.
func (l layout) cx() int {
	if l.pw <= compactPanelWidth {
		return l.px
	}
	return l.px + (l.pw-compactPanelWidth)/2
}

// Right panel rows.
func (l layout) buttonsTop() int    { return l.h - 9 }
func (l layout) buttonsBottom() int { return l.h - 6 }
func (l layout) mixerY() int        { return l.h - 4 }
