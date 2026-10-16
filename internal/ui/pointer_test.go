package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

func TestDetectPointer(t *testing.T) {
	for _, tc := range []struct {
		name, term, program, version, tmux, sty string
		want                                    pointerProtocol
	}{
		{name: "kitty", term: "xterm-kitty", want: pointerCSS},
		{name: "ghostty", program: "ghostty", want: pointerCSS},
		{name: "ghostty term", term: "xterm-ghostty", want: pointerCSS},
		{name: "foot", term: "foot", want: pointerCSS},
		{name: "foot extra", term: "foot-extra", want: pointerCSS},
		{name: "iterm", program: "iTerm.app", version: "3.5.0", want: pointerITerm},
		{name: "iterm later", program: "iTerm.app", version: "4.0.0", want: pointerITerm},
		{name: "iterm old", program: "iTerm.app", version: "3.4.9"},
		{name: "iterm missing version", program: "iTerm.app"},
		{name: "iterm malformed version", program: "iTerm.app", version: "3.x"},
		{name: "unknown terminal", term: "xterm-256color"},
		{name: "nested vscode", term: "xterm-kitty", program: "vscode"},
		{name: "tmux env", term: "xterm-kitty", tmux: "/tmp/tmux"},
		{name: "screen env", term: "foot", sty: "123.screen"},
		{name: "tmux term", term: "tmux-256color", program: "ghostty"},
		{name: "screen term", term: "screen-256color", program: "kitty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{
				"TERM": tc.term, "TERM_PROGRAM": tc.program,
				"TERM_PROGRAM_VERSION": tc.version, "TMUX": tc.tmux, "STY": tc.sty,
			}
			if got := detectPointer(func(key string) string { return env[key] }); got != tc.want {
				t.Fatalf("protocol = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPointerMouseOption(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TERM_PROGRAM", "kitty")
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	for _, mouse := range []bool{false, true} {
		u := New(Options{Theme: theme.Classic(), Mouse: mouse})
		if got := u.pointer != pointerUnsupported; got != mouse {
			t.Fatalf("Mouse=%t enabled pointer=%t", mouse, got)
		}
	}
}

type pointerTTY struct {
	tcell.Tty
	bytes.Buffer
	fail  error
	short bool
	calls int
}

func (t *pointerTTY) Write(p []byte) (int, error) {
	t.calls++
	if t.fail != nil {
		return 0, t.fail
	}
	if t.short {
		return len(p) - 1, nil
	}
	return t.Buffer.Write(p)
}

func (t *pointerTTY) WriteString(s string) (int, error) { return t.Write([]byte(s)) }
func (t *pointerTTY) Read(p []byte) (int, error)        { return t.Buffer.Read(p) }

type pointerTestScreen struct {
	tcell.Screen
	tty *pointerTTY
}

func (s *pointerTestScreen) Tty() (tcell.Tty, bool) { return s.tty, s.tty != nil }
func (s *pointerTestScreen) Fini() {
	s.tty.Buffer.WriteString("<fini>")
	s.Screen.Fini()
}
func (s *pointerTestScreen) Suspend() error {
	s.tty.Buffer.WriteString("<suspend>")
	return nil
}
func (s *pointerTestScreen) Resume() error {
	s.tty.Buffer.WriteString("<resume>")
	return nil
}

func newPointerTestScreen(t *testing.T, protocol pointerProtocol) (*pointerScreen, *pointerTTY) {
	t.Helper()
	tty := &pointerTTY{}
	sim := tcell.NewSimulationScreen("UTF-8")
	s := &pointerScreen{Screen: &pointerTestScreen{Screen: sim, tty: tty}, protocol: protocol}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Fini)
	return s, tty
}

func TestPointerSequencesAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		protocol   pointerProtocol
		hand, text string
	}{
		{pointerCSS, "pointer", "text"},
		{pointerITerm, "hand2", "xterm"},
	} {
		t.Run(tc.hand, func(t *testing.T) {
			s, tty := newPointerTestScreen(t, tc.protocol)
			for _, shape := range []pointerShape{pointerDefault, pointerHand, pointerHand, pointerText, pointerDefault} {
				if err := s.setPointer(shape); err != nil {
					t.Fatal(err)
				}
			}
			want := "\x1b]22;" + tc.hand + "\x1b\\\x1b]22;" + tc.text + "\x1b\\\x1b]22;\x1b\\"
			if tty.String() != want || tty.calls != 3 {
				t.Fatalf("pointer output = %q (%d writes), want %q", tty.String(), tty.calls, want)
			}
			tty.Reset()
			if err := s.setPointer(pointerHand); err != nil {
				t.Fatal(err)
			}
			if err := s.Suspend(); err != nil {
				t.Fatal(err)
			}
			if err := s.setPointer(pointerText); err != nil {
				t.Fatal(err)
			}
			if err := s.Resume(); err != nil {
				t.Fatal(err)
			}
			if err := s.setPointer(pointerHand); err != nil {
				t.Fatal(err)
			}
			s.Fini()
			if err := s.setPointer(pointerText); err != nil {
				t.Fatal(err)
			}
			hand := "\x1b]22;" + tc.hand + "\x1b\\"
			reset := "\x1b]22;\x1b\\"
			want = hand + reset + "<suspend><resume>" + hand + reset + "<fini>"
			if tty.String() != want {
				t.Fatalf("lifecycle output = %q, want %q", tty.String(), want)
			}
		})
	}
}

func TestPointerUnsupportedAndSimulation(t *testing.T) {
	s, tty := newPointerTestScreen(t, pointerUnsupported)
	if err := s.setPointer(pointerHand); err != nil {
		t.Fatal(err)
	}
	if tty.Len() != 0 {
		t.Fatalf("unsupported terminal received %q", tty.String())
	}
	s.Screen = tcell.NewSimulationScreen("UTF-8")
	s.protocol = pointerCSS
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	if err := s.setPointer(pointerHand); err != nil {
		t.Fatal(err)
	}
}

func TestPointerWriteErrors(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprintf("short=%t", short), func(t *testing.T) {
			s, tty := newPointerTestScreen(t, pointerCSS)
			want := io.ErrClosedPipe
			if short {
				tty.short = true
				want = io.ErrShortWrite
			} else {
				tty.fail = want
			}
			if err := s.setPointer(pointerHand); !errors.Is(err, want) {
				t.Fatalf("write error = %v, want %v", err, want)
			}
			if err := s.setPointer(pointerText); err != nil || tty.calls != 1 {
				t.Fatalf("failed pointer should be disabled: err=%v, writes=%d", err, tty.calls)
			}
			tty.fail, tty.short = nil, false
			s.Fini()
			if !errors.Is(s.pointerError(), want) {
				t.Fatalf("error not retained: %v", s.pointerError())
			}
			if got := tty.String(); got != "\x1b]22;\x1b\\<fini>" {
				t.Fatalf("failed write was not reset before finalization: %q", got)
			}
		})
	}
}

func TestPointerControlRegions(t *testing.T) {
	for _, width := range []int{80, 140} {
		for _, name := range []string{"classic", "modern"} {
			for _, nerd := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/nerd=%t", width, name, nerd), func(t *testing.T) {
					s, sim := helpBarScreen(t, width, name, nerd)
					s.width = listview.DefaultWidth
					s.app.AddFiles([]string{"/music/song.mp3"})
					s.Draw(sim)
					l := newLayout(width, 24, s.width)
					mx, mw, sx := l.cx(), 11, l.cx()
					buttonX := l.cx()
					if l.wide() {
						mx, mw, sx = l.px+1, l.pw-2, l.px+1
						buttonX = l.px + (l.pw-19)/2
					}
					for _, tc := range []struct {
						name string
						x, y int
						want pointerShape
					}{
						{"help entry below wheel handler", 1, 1, pointerHand},
						{"help gap", 26, 1, pointerDefault},
						{"help previous", width - 6, 0, pointerHand},
						{"help next", width - 6, 5, pointerHand},
						{"mode line", l.modeLine.x, l.modeLine.y, pointerHand},
						{"list row", l.list.x, l.list.y, pointerHand},
						{"empty list row", l.list.x, l.list.y + 1, pointerDefault},
						{"shuffle", sx, 11, pointerHand},
						{"previous", buttonX, l.buttonsTop(), pointerHand},
						{"mixer minus below wheel handler", mx, l.mixerY() + 2, pointerHand},
						{"mixer plus", mx + mw - 1, l.mixerY() + 2, pointerHand},
						{"border", 0, 0, pointerDefault},
						{"outside", -1, -1, pointerDefault},
					} {
						if got := s.pointerAt(tc.x, tc.y); got != tc.want {
							t.Errorf("%s: pointer at (%d,%d) = %v, want %v", tc.name, tc.x, tc.y, got, tc.want)
						}
					}
					if l.wide() && s.pointerAt(mx+4, l.mixerY()+2) != pointerHand {
						t.Error("volume bar is not interactive")
					}
					s.app.Input = app.InputSearch
					s.Draw(sim)
					if s.pointerAt(1, 1) != pointerDefault || s.pointerAt(l.list.x, l.list.y) != pointerDefault {
						t.Error("busy commands and list must not show a hand")
					}
					if s.pointerAt(mx, l.mixerY()+2) != pointerHand {
						t.Error("volume remains adjustable during search")
					}
					s.app.Input = app.InputDefault
					s.app.Exec(keys.CmdHelp)
					s.Draw(sim)
					if s.pointerAt(l.modeLine.x, l.modeLine.y) != pointerDefault {
						t.Error("inactive mode line must not show a hand")
					}
					if s.pointerAt(sx, 11) != pointerHand {
						t.Error("shuffle remains available in help mode")
					}
					s.clickCommand(rect{0, 0, 1, 1}, keys.CmdSetGroupTitle)
					if s.pointerAt(0, 0) != pointerDefault {
						t.Error("mode-gated command must not show a hand")
					}
				})
			}
		}
	}
}

func TestPointerProgressAndLayering(t *testing.T) {
	s, sim := helpBarScreen(t, 80, "classic", false)
	bar := rect{1, 1, 20, 1}
	c := canvas{scr: sim, w: 80, h: 24}
	s.drawProgress(c, bar, 0, 0)
	if s.pointerAt(1, 1) != pointerDefault {
		t.Fatal("unknown duration must not show a seek pointer")
	}
	s.drawProgress(c, bar, 10, 100)
	if s.pointerAt(1, 1) != pointerHand {
		t.Fatal("seek bar must show a hand")
	}
	s.onMouse(bar, func(tview.MouseAction, int, int) bool { return true }, pointerDefault)
	if s.pointerAt(1, 1) != pointerDefault {
		t.Fatal("blocking overlay must mask lower pointer regions")
	}
	s.onClick(rect{2, 1, 1, 1}, func() {})
	if s.pointerAt(2, 1) != pointerHand {
		t.Fatal("overlay button must override the blocker")
	}
}

func TestPointerRedrawAndDialogs(t *testing.T) {
	s, sim := helpBarScreen(t, 80, "classic", false)
	driver, tty := newPointerTestScreen(t, pointerCSS)
	driver.Screen = &pointerTestScreen{Screen: sim, tty: tty}
	s.Draw(driver)
	ev := tcell.NewEventMouse(1, 1, tcell.ButtonNone, tcell.ModNone)
	handled, _ := s.MouseHandler()(tview.MouseMove, ev, func(tview.Primitive) {})
	if handled || s.app.Mode != keys.ModeNormal || tty.String() != "\x1b]22;pointer\x1b\\" {
		t.Fatalf("hover triggered an action/redraw or no pointer: handled=%t, output=%q", handled, tty.String())
	}
	s.MouseHandler()(tview.MouseMove, ev, func(tview.Primitive) {})
	if tty.calls != 1 {
		t.Fatal("same shape must not be sent repeatedly")
	}
	tty.Reset()
	s.app.Exec(keys.CmdWritePlaylist)
	s.Draw(driver)
	if tty.String() != "\x1b]22;\x1b\\" || s.pointerAt(1, 1) != pointerDefault {
		t.Fatal("popup must reset the pointer without mouse movement")
	}
	l := newLayout(80, 24, s.width)
	if s.pointerAt(l.popup.x+1, l.popup.y+l.popup.h-2) != pointerText {
		t.Fatal("popup input must show a text pointer")
	}
	s.app.Popup = &app.Popup{Confirm: true}
	s.Draw(driver)
	if s.pointerAt(l.popup.x+1, l.popup.y+l.popup.h-2) != pointerDefault {
		t.Fatal("keyboard-only confirmation must not appear clickable")
	}
	s.app.Popup = nil
	s.app.Input = app.InputTask
	s.app.Task = &app.Task{Started: time.Now()}
	s.Draw(driver)
	if s.pointerAt(1, 1) != pointerDefault {
		t.Fatal("task dialog must mask underlying controls")
	}
	foundCancel := false
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			if s.pointerAt(x, y) == pointerHand {
				foundCancel = true
				if cellText(sim, x, y, len(cancelButton)) != cancelButton {
					t.Fatal("unexpected clickable task area")
				}
				x += len(cancelButton) - 1
			}
		}
	}
	if !foundCancel {
		t.Fatal("cancel button must show a hand")
	}
	s.app.Task, s.app.Input = nil, app.InputDefault
	s.Draw(driver)
	if s.pointerAt(1, 1) != pointerHand {
		t.Fatal("closing dialog must restore controls")
	}
	tty.Reset()
	sim.SetSize(40, 10)
	s.Draw(driver)
	if tty.String() != "\x1b]22;\x1b\\" || s.pointerAt(1, 1) != pointerDefault {
		t.Fatal("too-small layout must clear stale hit regions and pointer")
	}
}

func TestPointerErrorNotification(t *testing.T) {
	s, sim := helpBarScreen(t, 80, "classic", false)
	driver, tty := newPointerTestScreen(t, pointerCSS)
	driver.Screen = &pointerTestScreen{Screen: sim, tty: tty}
	s.Draw(driver)
	tty.fail = io.ErrClosedPipe
	ev := tcell.NewEventMouse(1, 1, tcell.ButtonNone, tcell.ModNone)
	handled, _ := s.MouseHandler()(tview.MouseMove, ev, func(tview.Primitive) {})
	if !handled || !s.app.Msg.Error || !strings.Contains(s.app.Msg.Text, "terminal mouse pointer") {
		t.Fatal("pointer failure must request a redraw with an error message")
	}
}
