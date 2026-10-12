package config

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blaster/internal/filemanager"
	"github.com/mingcheng/mp3blaster/internal/keys"
)

func TestParseConfig(t *testing.T) {
	o := Default()
	err := o.Parse(strings.NewReader(`
# comment
Threads = 250
WrapAround = no
AudiofileMatching = \\.mp3$, \.og\,g$
File.SortMode = size-big
Color.Popup.bg = red
Key.Play.Play = p
Key.File.UpDir = bsp
Key.SelectFiles = f2
Theme = modern
NerdFont = yes
ListWidth = 60%
`))
	if err != nil {
		t.Fatal(err)
	}
	if o.Threads != 250 || o.Wrap || o.SortMode != filemanager.SortSizeBig || o.Theme != "modern" || !o.NerdFont || o.ListWidth.Percent != 60 {
		t.Fatalf("options %+v", o)
	}
	if len(o.AudioPatterns) != 2 || o.AudioPatterns[0] != `\.mp3$` || o.AudioPatterns[1] != `og,g$` {
		t.Fatalf("patterns %q", o.AudioPatterns)
	}
	if o.Colours["popup.bg"] != "red" {
		t.Fatal("colour not set")
	}
	want := []KeyBinding{
		{keys.CmdPlayPlay, keys.Rune('p')},
		{keys.CmdFileUpDir, keys.Special(tcell.KeyBackspace)},
		{keys.CmdSelectFiles, keys.Special(tcell.KeyF2)},
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

func TestParseConfigErrors(t *testing.T) {
	for _, in := range []string{
		"Threads = 33",
		"Bogus = 1",
		"WrapAround = maybe",
		"Threads = 1, 2",
		"Color.Default.fg = purple",
		"no equals sign",
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
