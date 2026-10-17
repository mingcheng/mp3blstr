package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

type nullView struct{}

func (nullView) Post(func()) {}
func (nullView) Beep()       {}
func (nullView) Sync()       {}
func (nullView) Quit()       {}

type nullBackend struct{}

func (nullBackend) Opener(string) func() (playback.Track, error) {
	return func() (playback.Track, error) { return nil, os.ErrNotExist }
}

func render(t *testing.T, w, h int, setup func(*app.App)) []string {
	return renderWidth(t, w, h, listview.Width{}, setup)
}

func renderWidth(t *testing.T, w, h int, listWidth listview.Width, setup func(*app.App)) []string {
	t.Helper()
	a := newRenderApp(t, nullBackend{}, nullView{})
	if setup != nil {
		setup(a)
	}
	return draw(t, a, w, h, listWidth)
}

func newRenderApp(t *testing.T, backend playback.Backend, view app.View) *app.App {
	t.Helper()
	m, _ := media.NewMatcher(nil, nil)
	return app.New(app.Deps{
		Options:  config.Default(),
		Keys:     keys.Default(),
		Matcher:  m,
		Mixer:    audio.NewMixer(),
		Backend:  backend,
		View:     view,
		StartDir: t.TempDir(),
	})
}

// draw renders a on a w x h screen and returns its lines.
func draw(t *testing.T, a *app.App, w, h int, listWidth listview.Width) []string {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	defer sim.Fini()
	sim.SetSize(w, h)
	s := newScreen(theme.Classic(), theme.Plain(false), false, listWidth)
	s.app = a
	s.Draw(sim)
	sim.Show()
	cells, cw, ch := sim.GetContents()
	lines := make([]string, ch)
	for y := 0; y < ch; y++ {
		var b strings.Builder
		for x := 0; x < cw; x++ {
			r := cells[y*cw+x].Runes
			if len(r) == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteString(string(r))
			}
		}
		lines[y] = b.String()
	}
	return lines
}

// TestClassicLayout checks the screen geometry against the original
// (80x24 terminal).
func TestClassicLayout(t *testing.T) {
	l := render(t, 80, 24, func(a *app.App) { a.AddFiles([]string{"/music/song.mp3"}) })
	checks := []struct {
		y    int
		want string
	}{
		{0, "┌─────────────────────────────────────────────────────────────────────────( - )┐"},
		{1, "│[F 1] Select Files        [F 2] Add Group           [F 5] Set Group Title     │"},
		{5, "├──────────────────────────────────────────────────────────────────┬──────( + )┤"},
		{6, "│ Global Playback: Play current group, including subgroups         │           │"},
		{7, "│ Next Song      :                                                 │           │"},
		{9, "│                                                                  │Threads:100│"},
		{10, "├───────────────────────────┤[Default]├────────────────────────────┼───────────┤"},
		{11, "│song.mp3                                                          │[ ] shuffle│"},
		{12, "│                                                                  │[ ] repeat │"},
		{15, "│                                                                  │ |<  |>  >|│"},
		{16, "│                                                                  │ 4   5   6 │"},
		{18, "│                                                                  │ <<  []  >>│"},
		{20, "├──────────────────────────────────────────────────────────────────┼[ t ]Mixer─┤"},
		{21, "│                                                                  │Volume     │"},
		{22, "│Press '?' to get help on available commands                       │[<]+100%[>]│"},
		{23, "└──────────────────────────────────────────────────────────────────┴───────────┘"},
	}
	for _, c := range checks {
		if l[c.y] != c.want {
			t.Errorf("line %d:\n got %q\nwant %q", c.y, l[c.y], c.want)
		}
	}
}

func TestPlaylistSizeAlignment(t *testing.T) {
	path := filepath.Join(t.TempDir(), strings.Repeat("a", 80)+".mp3")
	if err := os.WriteFile(path, make([]byte, 12345), 0o644); err != nil {
		t.Fatal(err)
	}
	lines := render(t, 80, 24, func(a *app.App) { a.AddFiles([]string{path}) })
	if got := strings.Index(lines[11], "[  12K]"); got != 62 {
		t.Fatalf("size should stay at the right edge, column %d: %q", got, lines[11])
	}
	if !strings.HasPrefix(lines[11], "│aaaa") || lines[11][61] != ' ' {
		t.Fatalf("long name must not overlap size: %q", lines[11])
	}
}

func TestPopupAndTooSmall(t *testing.T) {
	l := render(t, 80, 24, func(a *app.App) { a.HandleKey(keys.Special(tcell.KeyF5 - 1)) }) // F4
	if !strings.Contains(l[13], "Playlist will be saved in:") {
		t.Errorf("popup missing: %q", l[13])
	}
	small := render(t, 40, 10, nil)
	if !strings.Contains(strings.Join(small, "\n"), "Terminal too small") {
		t.Error("expected too-small notice")
	}
	confirm := render(t, 80, 24, func(a *app.App) {
		dir := t.TempDir()
		os.WriteFile(dir+"/song.mp3", []byte("x"), 0o644)
		a.Opts.PlaylistDir = dir
		a.HandleKey(keys.Special(tcell.KeyF1))
		a.Exec(keys.CmdLoadPlaylist) // change to dir
		a.MainView().Set(2, a.MainLen())
		a.HandleKey(keys.Rune('D'))
	})
	all := strings.Join(confirm, "\n")
	if !strings.Contains(all, "Delete this file permanently?") || !strings.Contains(all, "[y] Yes   [n] No") {
		t.Errorf("confirm dialog missing:\n%s", all)
	}
}

// TestLiveThemeSwitch checks that switching the theme and the Nerd Font
// icons at run time changes the rendering.
func TestLiveThemeSwitch(t *testing.T) {
	l := render(t, 80, 24, func(a *app.App) { a.HandleKey(keys.Rune('T')) })
	if !strings.HasPrefix(l[0], "╭") {
		t.Errorf("modern theme should use rounded borders: %q", l[0])
	}
	l = render(t, 80, 24, func(a *app.App) {
		a.AddFiles([]string{"/music/song.mp3"})
		a.HandleKey(keys.Rune('I'))
	})
	if !strings.Contains(l[11], "\U000F075A song.mp3") {
		t.Errorf("nerd font icon missing: %q", l[11])
	}
}

// TestTaskDialog checks the progress dialog of a background task.
func TestTaskDialog(t *testing.T) {
	l := render(t, 80, 24, func(a *app.App) {
		a.HandleKey(keys.Special(tcell.KeyF1))
		a.HandleKey(keys.Special(tcell.KeyF3)) // recursive select
	})
	all := strings.Join(l, "\n")
	for _, want := range []string{"Recursively selecting files ...", "  0%", "[ Cancel (Esc) ]", "0:00"} {
		if !strings.Contains(all, want) {
			t.Errorf("dialog lacks %q:\n%s", want, all)
		}
	}
	// It also fits the smallest supported terminal.
	small := strings.Join(render(t, 60, 20, func(a *app.App) {
		a.HandleKey(keys.Special(tcell.KeyF1))
		a.HandleKey(keys.Special(tcell.KeyF3))
	}), "\n")
	if !strings.Contains(small, "Recursively selecting") || !strings.Contains(small, "[ Cancel (Esc) ]") {
		t.Errorf("small dialog:\n%s", small)
	}
	if got := truncLeft("/very/long/path/to/music", 10); got != "…/to/music" {
		t.Errorf("truncLeft = %q", got)
	}
}

// TestListWidth checks the proportional and fixed main window widths.
func TestListWidth(t *testing.T) {
	add := func(a *app.App) { a.AddFiles([]string{"/music/song.mp3"}) }
	border := func(line string, x int) bool { return []rune(line)[x] == '│' }

	// Default 75%: 140 columns -> 105 column list, wide panel.
	wide := renderWidth(t, 140, 30, listview.DefaultWidth, add)
	if !border(wide[11], 104) || !border(wide[11], 139) {
		t.Errorf("list should be 105 columns wide: %q", wide[11])
	}
	if !strings.Contains(wide[11], "[ ] shuffle   [ ] repeat") {
		t.Errorf("wide panel expected: %q", wide[11])
	}
	if !strings.Contains(wide[26], "[ t ]Mixer") || !strings.Contains(wide[28], "[<]━") {
		t.Errorf("wide mixer expected:\n%s\n%s", wide[26], wide[28])
	}

	// 100 columns -> 75 column list.
	mid := renderWidth(t, 100, 24, listview.DefaultWidth, add)
	if !border(mid[11], 74) || !strings.Contains(mid[6], "including subgroups") {
		t.Errorf("list should be 75 columns wide: %q / %q", mid[11], mid[6])
	}

	// 70 columns: 75% would be 52, the minimum of 60 would leave too little
	// room, so the original COLS-12 limit applies.
	tiny := renderWidth(t, 70, 24, listview.DefaultWidth, add)
	if !border(tiny[11], 57) {
		t.Errorf("list should be 58 columns wide: %q", tiny[11])
	}

	// 80 columns: 60 column list, compact panel centred in 19 columns.
	small := renderWidth(t, 80, 24, listview.DefaultWidth, add)
	if !border(small[11], 59) || !strings.Contains(small[11], "│    [ ] shuffle    │") {
		t.Errorf("compact panel should be centred: %q", small[11])
	}

	// Fixed width in columns.
	fixed := renderWidth(t, 140, 30, listview.Width{Cols: 80}, add)
	if !border(fixed[11], 79) {
		t.Errorf("list should be 80 columns wide: %q", fixed[11])
	}

	// Zero value keeps the original COLS-12 rule.
	orig := renderWidth(t, 140, 30, listview.Width{}, add)
	if !border(orig[11], 127) || !strings.HasSuffix(orig[11], "│[ ] shuffle│") {
		t.Errorf("original layout expected: %q", orig[11])
	}
}
