package playlist

import (
	"slices"
	"testing"
)

func TestNaturalCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"Track 2", "track 10", -1},
		{"track 10", "Track 2", 1},
		{"a", "B", -1},
		{"abc", "ABC", 0},
		{"file007", "file7", 0},
		{"file7", "file7b", -1},
		{"2.mp3", "10.mp3", -1},
		{"卡农", "卡农2", -1},
	} {
		if got := naturalCompare(c.a, c.b); got != c.want {
			t.Errorf("naturalCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// songs returns the song paths of g in order.
func songs(g *Group) []string {
	var out []string
	for _, it := range g.Items {
		if it.Kind == KindSong {
			out = append(out, it.Path)
		}
	}
	return out
}

func newSortGroup(t *testing.T) (*Group, *Group) {
	t.Helper()
	root := NewGroup("[Default]")
	g := NewGroup("[Sub]")
	root.AddGroup(g)
	for _, p := range []string{"/m/Track 10.mp3", "/m/b.mp3", "http://radio/live", "/m/Track 2.mp3", "/m/a.mp3"} {
		g.AddSong(p)
	}
	inner := NewGroup("[Inner]")
	g.AddGroup(inner)
	return g, inner
}

func TestSortSongs(t *testing.T) {
	info := map[string]SortInfo{
		"/m/Track 10.mp3": {Artist: "Beta", Album: "Two", Title: "zeta", Seconds: 200, Size: 300},
		"/m/b.mp3":        {Artist: "alpha", Album: "One", Title: "Mid", Seconds: 100, Size: 100},
		"/m/Track 2.mp3":  {Artist: "Beta", Album: "One", Title: "first", Seconds: 300, Size: -1},
		"/m/a.mp3":        {Seconds: -1, Size: 200},
	}
	lookup := func(p string) SortInfo {
		if in, ok := info[p]; ok {
			return in
		}
		return UnknownSortInfo
	}
	for _, c := range []struct {
		mode SortMode
		want []string
	}{
		{SortName, []string{"/m/a.mp3", "/m/b.mp3", "http://radio/live", "/m/Track 2.mp3", "/m/Track 10.mp3"}},
		// artist, then album, then title; songs without artist last (by name)
		{SortArtist, []string{"/m/b.mp3", "/m/Track 2.mp3", "/m/Track 10.mp3", "/m/a.mp3", "http://radio/live"}},
		// title tag, otherwise the file name without extension
		{SortTitle, []string{"/m/a.mp3", "/m/Track 2.mp3", "http://radio/live", "/m/b.mp3", "/m/Track 10.mp3"}},
		{SortDuration, []string{"/m/b.mp3", "/m/Track 10.mp3", "/m/Track 2.mp3", "/m/a.mp3", "http://radio/live"}},
		{SortSize, []string{"/m/b.mp3", "/m/a.mp3", "/m/Track 10.mp3", "http://radio/live", "/m/Track 2.mp3"}},
	} {
		g, inner := newSortGroup(t)
		g.SortSongs(c.mode, lookup)
		if got := songs(g); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.mode, got, c.want)
		}
		if g.Items[0].Kind != KindParent || g.Items[1].Group != inner {
			t.Errorf("%s: [..] and sub groups must stay at the top", c.mode)
		}
	}
}

func TestSortSongsIsStable(t *testing.T) {
	g := NewGroup("[Default]")
	for _, p := range []string{"/x/song.mp3", "/a/song.mp3", "/m/song.mp3"} {
		g.AddSong(p)
	}
	g.SortSongs(SortSize, func(string) SortInfo { return UnknownSortInfo })
	if got := songs(g); !slices.Equal(got, []string{"/x/song.mp3", "/a/song.mp3", "/m/song.mp3"}) {
		t.Fatalf("equal songs must keep their order: %q", got)
	}
	// SortName breaks ties on the file name by the full path.
	g.SortSongs(SortName, nil)
	if got := songs(g); !slices.Equal(got, []string{"/a/song.mp3", "/m/song.mp3", "/x/song.mp3"}) {
		t.Fatalf("name ties: %q", got)
	}
}

func TestShuffleSongs(t *testing.T) {
	g, inner := newSortGroup(t)
	before := songs(g)
	g.Items[3].selectID = 7
	marked := g.Items[3]
	reverse := func(n int, swap func(i, j int)) {
		for i := range n / 2 {
			swap(i, n-1-i)
		}
	}
	g.ShuffleSongs(reverse)
	got := songs(g)
	slices.Reverse(before)
	if !slices.Equal(got, before) {
		t.Fatalf("shuffle must use the given permutation: %q", got)
	}
	if g.Items[0].Kind != KindParent || g.Items[1].Group != inner || !slices.Contains(g.Items, marked) || !marked.Selected() {
		t.Fatal("shuffling must keep the head entries and the items themselves")
	}
}

func TestSortModeCycle(t *testing.T) {
	var names []string
	m := SortName
	for range numSortModes {
		names = append(names, m.String())
		m = m.Next()
	}
	if m != SortName || !slices.Equal(names, []string{"name", "artist", "title", "duration", "size", "shuffle"}) {
		t.Fatalf("cycle %q", names)
	}
	if SortName.NeedsMetadata() || SortShuffle.NeedsMetadata() || !SortArtist.NeedsMetadata() || !SortDuration.NeedsMetadata() {
		t.Fatal("NeedsMetadata")
	}
}
