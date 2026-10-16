package ui

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
)

func TestPanelNerdControls(t *testing.T) {
	for _, name := range []string{"classic", "modern"} {
		for _, width := range []int{60, 80, 140} {
			t.Run(fmt.Sprintf("%s/%d", name, width), func(t *testing.T) {
				s, sim := helpBarScreen(t, width, name, false)
				s.width = listview.DefaultWidth
				l := newLayout(width, 24, s.width)
				click := func(x, y int) bool {
					t.Helper()
					ev := tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)
					handled, _ := s.MouseHandler()(tview.MouseLeftClick, ev, func(tview.Primitive) {})
					return handled
				}
				for _, nerd := range []bool{true, false, true} {
					s.app.Opts.NerdFont = nerd
					s.Draw(sim)
					sx, sy, rx, ry := l.cx(), 11, l.cx(), 12
					mx, mw := l.cx(), 11
					if l.wide() {
						sx, rx, ry = l.px+1, l.px+15, 11
						mx, mw = l.px+1, l.pw-2
					}
					for _, control := range []struct {
						x, y  int
						label string
						state func() bool
					}{
						{sx, sy, "shuffle", s.app.Shuffle},
						{rx, ry, "repeat", s.app.Repeat},
					} {
						for _, checked := range []bool{false, true} {
							box := "[ ]"
							if checked {
								box = "[X]"
							}
							if nerd {
								box = " \U000F0131 "
								if checked {
									box = " \U000F0132 "
								}
							}
							want := box + " " + control.label
							if got := cellText(sim, control.x, control.y, 4+len(control.label)); got != want {
								t.Errorf("checkbox nerd=%t checked=%t = %q, want %q", nerd, checked, got, want)
							}
							// Both the icon and the last letter must remain clickable.
							x := control.x + 1
							if checked {
								x = control.x + 3 + len(control.label)
							}
							if !click(x, control.y) || control.state() == checked {
								t.Fatal("checkbox click did not toggle its state")
							}
							s.Draw(sim)
						}
					}
					if l.wide() && click(sx+11, sy) {
						t.Fatal("checkbox hit area extends into the gap")
					}

					my := l.mixerY() + 2
					for _, device := range []string{"Volume", "Balance"} {
						if got := s.app.Mixer.Current().Name; got != device {
							t.Fatalf("mixer device = %s, want %s", got, device)
						}
						s.app.MixerSet(50)
						s.Draw(sim)
						for _, button := range []struct {
							x     int
							plain string
							icon  string
							value int
						}{
							{mx, "[<]", " \U000F0141 ", 48},
							{mx + mw - 3, "[>]", " \U000F0142 ", 50},
						} {
							want := button.plain
							if nerd {
								want = button.icon
							}
							if got := cellText(sim, button.x, my, 3); got != want {
								t.Errorf("mixer button nerd=%t = %q, want %q", nerd, got, want)
							}
							if nerd {
								for col := 0; col < 3; col++ {
									_, _, style, _ := sim.GetContent(button.x+col, my)
									if style != s.th.Label.Bold(false) {
										t.Error("mixer icon must share playback button colours without bold")
									}
								}
							}
							if !click(button.x+1, my) || s.app.Mixer.Current().Value != button.value {
								t.Fatal("mixer icon click did not adjust the value")
							}
							s.Draw(sim)
						}
						ev := tcell.NewEventMouse(mx+1, my, tcell.WheelDown, tcell.ModNone)
						handled, _ := s.MouseHandler()(tview.MouseScrollDown, ev, func(tview.Primitive) {})
						if !handled || s.app.Mixer.Current().Value != 48 {
							t.Fatal("mixer scroll did not adjust the value")
						}
						if l.wide() {
							if !click(mx+mw-4, my) || s.app.Mixer.Current().Value != 100 {
								t.Fatal("volume bar endpoint must still set 100%")
							}
						}
						s.app.Exec(keys.CmdToggleMixer)
						s.Draw(sim)
					}
					for _, pager := range []struct {
						y    int
						key  string
						icon string
					}{{0, " - ", "\U000F0143"}, {5, " + ", "\U000F0140"}} {
						want := "(" + pager.key + ")"
						if nerd {
							want = pager.icon + pager.key + " "
						}
						if got := cellText(sim, width-6, pager.y, 5); got != want {
							t.Errorf("pager = %q, want %q", got, want)
						}
					}
					if !click(width-6, 5) || s.app.HelpBar.Offset == 0 {
						t.Fatal("next help icon did not page down")
					}
					s.Draw(sim)
					if !click(width-6, 0) || s.app.HelpBar.Offset != 0 {
						t.Fatal("previous help icon did not page up")
					}
					for y := 0; y < 24; y++ {
						if got := cellText(sim, width-1, y, 1); got == " " {
							t.Errorf("controls erased the right border at row %d", y)
						}
					}
				}
			})
		}
	}
}
