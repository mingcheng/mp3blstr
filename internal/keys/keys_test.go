package keys

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestLabels(t *testing.T) {
	cases := map[Key]string{
		Rune('q'):                   " q ",
		Rune(' '):                   "spc",
		Special(tcell.KeyF1):        "F 1",
		Special(tcell.KeyF10):       "F10",
		Special(tcell.KeyEnter):     "ent",
		Special(tcell.KeyUp):        " up",
		Special(tcell.KeyCtrlL):     " ^L",
		Special(tcell.KeyBackspace): "bsp",
	}
	for k, want := range cases {
		if got := k.Label(); got != want {
			t.Errorf("%+v: got %q want %q", k, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := map[string]Key{
		"x":   Rune('x'),
		"spc": Rune(' '),
		"F12": Special(tcell.KeyF12),
		"pdn": Special(tcell.KeyPgDn),
		"kp5": Rune('5'),
		"s0d": Special(tcell.KeyEnter),
		"s41": Rune('A'),
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v", in, got, err)
		}
	}
	if _, err := Parse("nonsense"); err == nil {
		t.Error("expected error")
	}
}

func TestLookupByMode(t *testing.T) {
	tb := Default()
	f1 := Special(tcell.KeyF1)
	if tb.Lookup(f1, ModeNormal) != CmdSelectFiles || tb.Lookup(f1, ModeFileSelection) != CmdFileAddFiles {
		t.Fatal("F1 must depend on the mode")
	}
	if tb.Lookup(Rune('D'), ModeNormal) != CmdNone || tb.Lookup(Rune('D'), ModeFileSelection) != CmdFileDelete {
		t.Fatal("D must only delete files in file mode")
	}
	if tb.Lookup(Rune('q'), ModeHelp) != CmdQuitProgram {
		t.Fatal("q is valid everywhere")
	}
	tb.Bind(CmdPlayPlay, Rune('p'))
	if tb.Lookup(Rune('p'), ModeNormal) != CmdPlayPlay || tb.LabelFor(CmdPlayPlay) != " p " {
		t.Fatal("rebinding failed")
	}
	// The help bar layout of the original: 3 entries per line, normal mode
	// starts with Select Files, Add Group, Set Group Title.
	nm := tb.ForMode(ModeNormal)
	if nm[0].Desc != "Select Files" || nm[2].Desc != "Set Group Title" {
		t.Fatal("help order changed")
	}
}

func TestRepeatShuffleKeys(t *testing.T) {
	tb := Default()
	for _, m := range []Mode{ModeNormal, ModeFileSelection, ModeHelp} {
		if tb.Lookup(Rune('R'), m) != CmdToggleRepeat || tb.Lookup(Rune('Z'), m) != CmdToggleShuffle {
			t.Fatalf("R/Z must work in mode %d", m)
		}
	}
	f6, f7 := Special(tcell.KeyF6), Special(tcell.KeyF7)
	if tb.Lookup(f6, ModeNormal) != CmdToggleRepeat || tb.Lookup(f7, ModeNormal) != CmdToggleShuffle {
		t.Fatal("F6/F7 must keep working")
	}
	if tb.Lookup(f7, ModeFileSelection) != CmdFileAddURL {
		t.Fatal("F7 adds a URL in file mode")
	}
	if tb.LabelFor(CmdToggleRepeat) != " R " {
		t.Fatal("R is the primary repeat key")
	}

	// A configured key replaces all default keys of the command...
	tb.Bind(CmdToggleRepeat, Rune('6'))
	if tb.Lookup(Rune('6'), ModeNormal) != CmdToggleRepeat || tb.Lookup(Rune('6'), ModeFileSelection) != CmdToggleRepeat {
		t.Fatal("configured key should win over the default '6' (next song)")
	}
	if tb.Lookup(Rune('R'), ModeNormal) == CmdToggleRepeat || tb.Lookup(f6, ModeNormal) == CmdToggleRepeat {
		t.Fatal("default repeat keys should be gone")
	}
	// ...and wins over a default binding of another command.
	tb.Bind(CmdPlayStop, Rune('s'))
	if tb.Lookup(Rune('s'), ModeFileSelection) != CmdPlayStop {
		t.Fatal("configured key should win over the file sort default")
	}
}

func TestGroupedHelpBar(t *testing.T) {
	tb := Default()
	groups := tb.Grouped(ModeNormal, true)
	if groups[0][0].Desc != "Select Files" || groups[0][0].Group != GroupPlaylist {
		t.Fatal("the help bar starts with the playlist commands")
	}
	last := Group(0)
	seen := map[Command]bool{}
	for _, g := range groups {
		if g[0].Group < last {
			t.Fatal("categories out of order")
		}
		last = g[0].Group
		for _, b := range g {
			if b.Hidden || b.Group != g[0].Group || b.Modes&ModeNormal == 0 {
				t.Fatalf("unexpected entry %q", b.Desc)
			}
			if seen[b.Cmd] {
				t.Fatalf("duplicate entry %q", b.Desc)
			}
			seen[b.Cmd] = true
		}
	}
	for _, cmd := range []Command{CmdUp, CmdEnter, CmdPlayPlay, CmdToggleMixer, CmdHelpNext} {
		if seen[cmd] {
			t.Fatalf("command %d is shown elsewhere and should not be in the help bar", cmd)
		}
	}
	if !seen[CmdToggleRepeat] || !seen[CmdToggleShuffle] {
		t.Fatal("repeat and shuffle must be listed")
	}
	// A shadowed binding is not listed.
	tb.Bind(CmdPlayStop, Rune('C'))
	for _, g := range tb.Grouped(ModeNormal, true) {
		for _, b := range g {
			if b.Cmd == CmdClearPlaylist {
				t.Fatal("Clear Playlist lost its key and must not be listed")
			}
		}
	}
}
