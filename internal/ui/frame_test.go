package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

func helpBarScreen(t *testing.T, width int, name string, nerd bool) (*Screen, tcell.SimulationScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Fini)
	sim.SetSize(width, 24)
	opts := config.Default()
	opts.Theme, opts.NerdFont = name, nerd
	opts.Colours = map[string]string{
		"button.fg": "#abcdef", "button.bg": "#123456",
		"label.fg": "#112233", "label.bg": "#fedcba",
	}
	m, err := media.NewMatcher(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := app.New(app.Deps{
		Options: opts, Keys: keys.Default(), Matcher: m, Mixer: audio.NewMixer(),
		Backend: nullBackend{}, View: nullView{}, StartDir: t.TempDir(),
	})
	th := theme.ByName(name)
	th.ApplyColours(opts.Colours)
	s := newScreen(th, theme.For(th, nerd), nerd, listview.Width{})
	s.app = a
	return s, sim
}

func cellText(sim tcell.Screen, x, y, width int) string {
	var text strings.Builder
	for col := 0; col < width; {
		r, combining, _, w := sim.GetContent(x+col, y)
		text.WriteRune(r)
		text.WriteString(string(combining))
		col += max(1, w)
	}
	return text.String()
}

func TestHelpBarKeycaps(t *testing.T) {
	for _, name := range []string{"classic", "modern"} {
		for _, nerd := range []bool{false, true} {
			for _, width := range []int{60, 80, 140} {
				t.Run(fmt.Sprintf("%s/nerd=%t/width=%d", name, nerd, width), func(t *testing.T) {
					s, sim := helpBarScreen(t, width, name, nerd)
					for _, tc := range []struct {
						key      keys.Key
						plain    string
						withIcon string
					}{
						{keys.Special(tcell.KeyF1), "[F 1]", " \U000F024B F1  "},
						{keys.Special(tcell.KeyF12), "[F12]", " \U000F024B F12 "},
						{keys.Rune('a'), "[ a ]", "  \U000F024B a  "},
						{keys.Rune(' '), "[spc]", " \U000F024B spc "},
						{keys.Special(tcell.KeyCtrlL), "[ ^L]", " \U000F024B ^L  "},
						{keys.Rune('界'), "[ 界]", " \U000F024B 界  "},
					} {
						s.app.Keys.Bind(keys.CmdSelectFiles, tc.key)
						s.Draw(sim)
						want := tc.plain
						keyWidth := 5
						if nerd {
							want = tc.withIcon
							keyWidth = helpKeyWidth
						}
						if got := cellText(sim, 1, 1, keyWidth); got != want {
							t.Errorf("%s keycap = %q, want %q", tc.key.Name(), got, want)
						}
						l := newLayout(width, 24, listview.Width{})
						_, _, playStyle, _ := sim.GetContent(l.cx()+4, l.buttonsTop())
						for x := 1; x <= keyWidth; x++ {
							_, _, style, _ := sim.GetContent(x, 1)
							wantStyle := s.th.Button
							if x == 1 || x == keyWidth {
								wantStyle = s.th.Default
							}
							if nerd {
								wantStyle = playStyle.Bold(false)
							}
							if style != wantStyle {
								t.Errorf("cell %d has incorrect keycap style: %v", x, style)
							}
						}
						_, _, gapStyle, _ := sim.GetContent(1+keyWidth, 1)
						if gapStyle != s.th.Default {
							t.Error("keycap style leaked into the gap")
						}
						start := 1 + keyWidth + 1
						if got := cellText(sim, start, 1, 12); got != "Select Files" {
							t.Errorf("description = %q", got)
						}
					}
					for y := 1; y <= 4; y++ {
						if got := cellText(sim, width-1, y, 1); got != s.gl.V {
							t.Errorf("help bar overwrote right border at row %d: %q", y, got)
						}
					}
				})
			}
		}
	}
}

func TestHelpBarIconsAndLiveSwitch(t *testing.T) {
	s, sim := helpBarScreen(t, 80, "classic", false)
	for _, mode := range []keys.Mode{keys.ModeNormal, keys.ModeFileSelection, keys.ModeHelp} {
		switch mode {
		case keys.ModeFileSelection:
			s.app.Exec(keys.CmdSelectFiles)
		case keys.ModeHelp:
			s.app.Exec(keys.CmdHelp)
		}
		lines := s.app.HelpBarLines()
		for _, nerd := range []bool{true, false} {
			s.app.Opts.NerdFont = nerd
			for offset := 0; offset < len(lines); offset += 4 {
				s.app.HelpBar.Offset = offset
				s.Draw(sim)
				for row := 0; row < 4 && s.app.HelpBar.Offset+row < len(lines); row++ {
					for col, entry := range lines[s.app.HelpBar.Offset+row] {
						x, y := 1+col*helpEntryWidth, 1+row
						keyWidth := 5
						keycap := "[" + s.app.Keys.KeyFor(entry.Cmd).Label() + "]"
						if nerd {
							keyWidth = helpKeyWidth
							icon := s.gl.Shortcuts[entry.Cmd]
							if icon == "" {
								icon = " "
							}
							if uniseg.StringWidth(icon) != 1 {
								t.Fatalf("command %v needs a one-cell icon, got %q", entry.Cmd, icon)
							}
							label := icon + " " + entry.Label
							pad := (keyWidth - uniseg.StringWidth(label)) / 2
							keycap = strings.Repeat(" ", pad) + label
							keycap += strings.Repeat(" ", keyWidth-uniseg.StringWidth(keycap))
						}
						at, remaining := x+keyWidth+1, helpEntryWidth-keyWidth-2
						if got := cellText(sim, x, y, keyWidth); got != keycap {
							t.Errorf("command %v keycap = %q, want %q", entry.Cmd, got, keycap)
						}
						want := crunch(entry.Desc, remaining)
						if got := cellText(sim, at, y, uniseg.StringWidth(want)); got != want {
							t.Errorf("command %v description = %q, want %q", entry.Cmd, got, want)
						}
						if got := cellText(sim, x+helpEntryWidth-1, y, 1); got != " " {
							t.Errorf("command %v overflows its column: %q", entry.Cmd, got)
						}
					}
				}
			}
		}
	}
	s.app.Exec(keys.CmdToggleTheme)
	s.Draw(sim)
	if s.th.Name != "modern" {
		t.Fatal("live theme switch did not apply")
	}
	_, _, style, _ := sim.GetContent(1, 1)
	if style != s.th.Default {
		t.Error("theme switch must preserve plain brackets without keycap background")
	}
	s.app.Exec(keys.CmdToggleNerdFont)
	s.Draw(sim)
	_, _, style, _ = sim.GetContent(1, 1)
	if style != s.th.Label.Bold(false) {
		t.Error("live theme switch lost keycap style")
	}
}

func TestHelpBarEmptyIconSlot(t *testing.T) {
	s, sim := helpBarScreen(t, 80, "modern", true)
	for _, nerd := range []bool{true, false, true} {
		s.app.Opts.NerdFont = nerd
		s.Draw(sim)
		want := "[F 5]"
		keyWidth := 5
		if nerd {
			want = "   F5  "
			keyWidth = helpKeyWidth
			if s.gl.Shortcuts[keys.CmdSetGroupTitle] != "" {
				t.Fatal("title shortcut should leave its icon slot empty")
			}
			// F1 and F5 have equal-length labels, so the reserved slot must
			// keep their key text at the same column within each keycap.
			for _, x := range []int{1, 1 + 2*helpEntryWidth} {
				if got := cellText(sim, x+3, 1, 1); got != "F" {
					t.Errorf("key text is misaligned at column %d: %q", x, got)
				}
			}
		}
		x := 1 + 2*helpEntryWidth
		if got := cellText(sim, x, 1, keyWidth); got != want {
			t.Errorf("empty icon keycap = %q, want %q", got, want)
		}
		for col := x; col < x+keyWidth; col++ {
			_, _, style, _ := sim.GetContent(col, 1)
			wantStyle := s.th.Button
			if col == x || col == x+keyWidth-1 {
				wantStyle = s.th.Default
			}
			if nerd {
				wantStyle = s.th.Label.Bold(false)
			}
			if style != wantStyle {
				t.Errorf("empty slot lost keycap background at column %d", col)
			}
		}
	}
}

func TestHelpBarMouse(t *testing.T) {
	for _, nerd := range []bool{false, true} {
		for _, x := range []int{1, 4, 7, 10, 25} {
			t.Run(fmt.Sprintf("nerd=%t/x=%d", nerd, x), func(t *testing.T) {
				s, sim := helpBarScreen(t, 80, "classic", nerd)
				s.app.Keys.Bind(keys.CmdSelectFiles, keys.Special(tcell.KeyF12))
				s.Draw(sim)
				click := tcell.NewEventMouse(x, 1, tcell.Button1, tcell.ModNone)
				consumed, _ := s.MouseHandler()(tview.MouseLeftClick, click, func(tview.Primitive) {})
				if !consumed || s.app.Mode != keys.ModeFileSelection {
					t.Fatal("click on keycap, icon or description did not run remapped command")
				}
				s.Draw(sim)
				wheel := tcell.NewEventMouse(7, 1, tcell.WheelDown, tcell.ModNone)
				consumed, _ = s.MouseHandler()(tview.MouseScrollDown, wheel, func(tview.Primitive) {})
				if !consumed || s.app.HelpBar.Offset == 0 {
					t.Fatal("help bar no longer scrolls over a shortcut")
				}
			})
		}
	}
}
