package playlist

import (
	"bytes"
	"strings"
	"testing"
)

func names(g *Group) []string {
	var out []string
	for _, it := range g.Items {
		out = append(out, it.Name())
	}
	return out
}

func TestAddGroupKeepsGroupsFirst(t *testing.T) {
	root := NewGroup("[Default]")
	root.AddSong("/a.mp3")
	root.AddGroup(NewGroup("[G1]"))
	root.AddGroup(NewGroup("[G2]"))
	got := strings.Join(names(root), ",")
	if got != "[G1],[G2],/a.mp3" {
		t.Fatalf("got %s", got)
	}
	if !root.SubGroup(0).IsParentLink(0) {
		t.Fatal("sub group must start with [..]")
	}
	if root.SubGroup(0).Delete(0) {
		t.Fatal("[..] must not be deletable")
	}
}

func TestMoveSelected(t *testing.T) {
	g := NewGroup("[x]")
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		g.AddSong(s)
	}
	g.ToggleSelect(4)              // e first
	g.ToggleSelect(0)              // then a
	if !g.MoveSelected(2, false) { // after c
		t.Fatal("move failed")
	}
	if got := strings.Join(names(g), ""); got != "bcead" {
		t.Fatalf("after: got %s", got)
	}

	g2 := NewGroup("[y]")
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		g2.AddSong(s)
	}
	g2.ToggleSelect(4)
	g2.ToggleSelect(0)
	g2.MoveSelected(2, true) // before c
	if got := strings.Join(names(g2), ""); got != "beacd" {
		t.Fatalf("before: got %s", got)
	}
}

func TestUnplayedAccounting(t *testing.T) {
	root := NewGroup("[r]")
	sub := NewGroup("[s]")
	root.AddGroup(sub)
	root.AddSong("r1")
	sub.AddSong("s1")
	sub.AddSong("s2")
	if n := root.UnplayedSongs(true); n != 3 {
		t.Fatalf("unplayed=%d", n)
	}
	if n := root.UnplayedGroups(true); n != 2 {
		t.Fatalf("groups=%d", n)
	}
	if it := root.UnplayedSong(0, true, true); it.Path != "s1" {
		t.Fatalf("first song %s", it.Path)
	}
	if n := root.UnplayedSongs(true); n != 2 {
		t.Fatalf("unplayed after=%d", n)
	}
	root.ResetSongs(true, false)
	if n := root.UnplayedSongs(true); n != 3 {
		t.Fatalf("reset=%d", n)
	}
}

func TestWriteParseRoundTrip(t *testing.T) {
	root := NewGroup("[Default]")
	root.Shuffle = true
	sub := NewGroup(`[My "Album"]`)
	sub.Shuffle = true
	sub.AddSong("/music/x.mp3")
	root.AddGroup(sub)
	root.AddSong("/music/y.mp3")

	var buf bytes.Buffer
	if err := Write(&buf, root, "allgroups"); err != nil {
		t.Fatal(err)
	}
	want := "<GLOBAL PLAYMODE=\"allgroups\" MODE=\"shuffle\">\n" +
		"<GROUP NAME=\"My \\\"Album\\\"\" MODE=\"shuffle\">\n\t/music/x.mp3\n</GROUP>\n/music/y.mp3\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s", buf.String())
	}

	loaded := NewGroup("[Default]")
	gs, err := Parse(strings.NewReader(buf.String()), loaded, "/base", false)
	if err != nil {
		t.Fatal(err)
	}
	if gs.PlayMode != "allgroups" || !gs.Shuffle {
		t.Fatalf("globals %+v", gs)
	}
	g := loaded.SubGroup(0)
	if g == nil || g.Title != `[My "Album"]` || !g.Shuffle || g.Item(1).Path != "/music/x.mp3" {
		t.Fatalf("group not restored: %+v", g)
	}
	if loaded.Item(1).Path != "/music/y.mp3" {
		t.Fatal("song after group lost")
	}
}

func TestParseM3URelative(t *testing.T) {
	g := NewGroup("[d]")
	_, err := Parse(strings.NewReader("#EXTM3U\n#EXTINF:1,x\nsong.mp3\nhttp://h/stream\n"), g, "/base", true)
	if err != nil {
		t.Fatal(err)
	}
	if g.Len() != 2 || !strings.HasSuffix(g.Item(0).Path, "song.mp3") || g.Item(1).Path != "http://h/stream" {
		t.Fatalf("got %v", names(g))
	}
}
