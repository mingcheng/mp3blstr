package config

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blasterr/internal/filemanager"
	"github.com/mingcheng/mp3blasterr/internal/keys"
	"github.com/mingcheng/mp3blasterr/internal/playback"
)

func TestParseConfig(t *testing.T) {
	o := Default()
	err := o.Parse(strings.NewReader(`
# comment
ui:
  theme: modern
  nerd_font: yes
  list_width: 60%
  wrap_around: false
  colors:
    popup: {bg: red}
    popup_input:
      fg: "#ff8800"
playback:
  threads: 250
  Volume: 80
files:
  sort_mode: size-big
  audio_patterns:
    - \.mp3$
    - 'og,g$'
  playlist_patterns: \.lst$
state:
keys:
  play:
    play: p
  file.up_dir: bsp
  select_files: f2
  change_thread: "#"
  file:
    mp3_to_wav: x
`))
	if err != nil {
		t.Fatal(err)
	}
	if o.Threads != 250 || o.Wrap || o.SortMode != filemanager.SortSizeBig || o.Theme != "modern" || !o.NerdFont ||
		o.ListWidth.Percent != 60 || o.Volume != 80 {
		t.Fatalf("options %+v", o)
	}
	if len(o.AudioPatterns) != 2 || o.AudioPatterns[0] != `\.mp3$` || o.AudioPatterns[1] != `og,g$` {
		t.Fatalf("patterns %q", o.AudioPatterns)
	}
	if len(o.PlaylistPatterns) != 1 || o.PlaylistPatterns[0] != `\.lst$` {
		t.Fatalf("playlist patterns %q", o.PlaylistPatterns)
	}
	if o.Colours["popup.bg"] != "red" || o.Colours["popupinput.fg"] != "#ff8800" {
		t.Fatalf("colours %v", o.Colours)
	}
	want := []KeyBinding{
		{keys.CmdPlayPlay, keys.Rune('p')},
		{keys.CmdFileUpDir, keys.Special(tcell.KeyBackspace)},
		{keys.CmdSelectFiles, keys.Special(tcell.KeyF2)},
		{keys.CmdChangeThread, keys.Rune('#')},
	}
	if len(o.Keys) != len(want) {
		t.Fatalf("keys %+v", o.Keys)
	}
	for i := range want {
		if o.Keys[i] != want[i] {
			t.Fatalf("key %d: %+v want %+v", i, o.Keys[i], want[i])
		}
	}
}

func TestParseEmptyConfig(t *testing.T) {
	for _, in := range []string{"", "# only comments\n", "---\n", "~\n"} {
		o := Default()
		if err := o.Parse(strings.NewReader(in)); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestParseConfigErrors(t *testing.T) {
	for _, in := range []string{
		"playback: {threads: 33}",
		"bogus: 1",
		"ui: {bogus: 1}",
		"ui: {wrap_around: maybe}",
		"playback: {threads: [100, 150]}",
		"files: {audio_patterns: {a: b}}",
		"ui: {colors: {default: {fg: purple}}}",
		"ui:\n  colors:\n    default:\n      fg: #ff0000\n",
		"ui: dark",
		"keys: {play: {bogus: x}}",
		"keys: {play: {next: [n, m]}}",
		"keys: x",
		"ui:\n  theme: modern\n  Theme: classic",
		"Threads = 100",
		"- a\n- b",
		"ui: [",
	} {
		if err := Default().Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestParseArgs(t *testing.T) {
	c, err := ParseArgs([]string{"-R", "-p", "allrandom", "-t", "0", "a.mp3", "b.mp3"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Repeat || c.PlayMode != "allrandom" || c.Threads != 0 || !c.IsSet("threads") || len(c.Files) != 2 {
		t.Fatalf("%+v", c)
	}
	if _, err := ParseArgs([]string{"-l", "x.lst", "a.mp3"}); err == nil {
		t.Fatal("files with --list must fail")
	}
}

func TestApplyCLI(t *testing.T) {
	c, err := ParseArgs([]string{"-R", "-p", "onegroup", "-t", "50", "--theme", "Modern", "--list-width", "50%", "--no-mouse"})
	if err != nil {
		t.Fatal(err)
	}
	o := Default()
	if err := o.ApplyCLI(c); err != nil {
		t.Fatal(err)
	}
	if !o.Repeat || o.PlayMode != playback.PlayGroup || o.Threads != 50 || o.Theme != "modern" ||
		o.ListWidth.Percent != 50 || o.Mouse {
		t.Fatalf("%+v", o)
	}
	// Defaults of options that were not given are kept.
	if o.FPL != Default().FPL {
		t.Fatalf("runframes changed to %d", o.FPL)
	}

	for _, args := range [][]string{
		{"-p", "sideways"}, {"-r", "11"}, {"-t", "75"}, {"--theme", "neon"}, {"--list-width", "5"},
	} {
		c, err := ParseArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if err := Default().ApplyCLI(c); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}

func TestThemes(t *testing.T) {
	if th, err := ParseTheme(" Classic "); err != nil || th != "classic" {
		t.Fatalf("ParseTheme: %q, %v", th, err)
	}
	if NextTheme("classic") != "modern" || NextTheme("MODERN") != "classic" || NextTheme("bogus") != "classic" {
		t.Fatal("NextTheme should cycle through Themes")
	}
}
