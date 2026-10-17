package playlist

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mingcheng/mp3blstr/internal/media"
)

// SortMode is an order the songs of a group can be sorted in, in the order
// the "sort playlist" command cycles through.
type SortMode int

// Sort modes.
const (
	SortName     SortMode = iota // file name
	SortArtist                   // artist, then album and title
	SortTitle                    // title, or the file name without tag
	SortDuration                 // shortest first
	SortSize                     // smallest first
	SortShuffle                  // random order
	numSortModes
)

// Next returns the following sort mode (wrapping around).
func (m SortMode) Next() SortMode { return (m + 1) % numSortModes }

func (m SortMode) String() string {
	switch m {
	case SortArtist:
		return "artist"
	case SortTitle:
		return "title"
	case SortDuration:
		return "duration"
	case SortSize:
		return "size"
	case SortShuffle:
		return "shuffle"
	default:
		return "name"
	}
}

// NeedsMetadata reports whether sorting in mode m reads the song files
// (tags, length or size), which may take a while.
func (m SortMode) NeedsMetadata() bool { return m != SortName && m != SortShuffle }

// SortInfo is what a song is sorted by besides its path. Unknown values
// (empty strings, negative numbers) sort after all known ones.
type SortInfo struct {
	Artist, Album, Title string
	Seconds              int   // length; -1 unknown
	Size                 int64 // file size in bytes; -1 unknown
}

// UnknownSortInfo is the SortInfo of a song nothing is known about.
var UnknownSortInfo = SortInfo{Seconds: -1, Size: -1}

// SortSongs orders the songs of g in mode m, which must not be
// SortShuffle. The "[..]" link and the sub groups stay at the top in their
// order; songs that compare equal keep their order. info returns the sort
// values of a song; it is only called for modes that need metadata.
func (g *Group) SortSongs(m SortMode, info func(path string) SortInfo) {
	byName := func(a, b *Item) int { return naturalCompare(fileName(a.Path), fileName(b.Path)) }
	var compare func(a, b *Item) int
	switch m {
	case SortName:
		compare = func(a, b *Item) int {
			return cmp.Or(byName(a, b), naturalCompare(a.Path, b.Path))
		}
	default:
		infos := map[*Item]SortInfo{}
		for _, it := range g.Items {
			if it.Kind == KindSong {
				infos[it] = info(it.Path)
			}
		}
		compare = func(a, b *Item) int {
			ia, ib := infos[a], infos[b]
			var c int
			switch m {
			case SortArtist:
				c = cmp.Or(
					compareText(ia.Artist, ib.Artist),
					compareText(ia.Album, ib.Album),
					compareText(ia.Title, ib.Title))
			case SortTitle:
				c = cmp.Or(
					naturalCompare(titleOf(a, ia), titleOf(b, ib)),
					compareText(ia.Artist, ib.Artist))
			case SortDuration:
				c = compareKnown(ia.Seconds, ib.Seconds)
			case SortSize:
				c = compareKnown(ia.Size, ib.Size)
			}
			return cmp.Or(c, byName(a, b))
		}
	}
	g.reorderSongs(func(songs []*Item) { slices.SortStableFunc(songs, compare) })
}

// ShuffleSongs puts the songs of g in random order using shuffle (the
// signature of rand.Shuffle). The "[..]" link and the sub groups stay at
// the top.
func (g *Group) ShuffleSongs(shuffle func(n int, swap func(i, j int))) {
	g.reorderSongs(func(songs []*Item) {
		shuffle(len(songs), func(i, j int) { songs[i], songs[j] = songs[j], songs[i] })
	})
}

// reorderSongs moves the "[..]" link and the sub groups to the top (keeping
// their order) and lets order arrange the songs after them.
func (g *Group) reorderSongs(order func(songs []*Item)) {
	var head, songs []*Item
	for _, it := range g.Items {
		if it.Kind == KindSong {
			songs = append(songs, it)
		} else {
			head = append(head, it)
		}
	}
	order(songs)
	g.Items = append(head, songs...)
}

// fileName is the name of a song file, as shown in file name display mode.
func fileName(path string) string { return media.ChopPath(path) }

// titleOf is the title tag of a song, or its file name without extension.
func titleOf(it *Item, info SortInfo) string {
	if info.Title != "" {
		return info.Title
	}
	name := fileName(it.Path)
	if !media.IsHTTPStream(it.Path) {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

// compareText compares two tag values naturally; empty values sort last.
func compareText(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	return naturalCompare(a, b)
}

// compareKnown compares two numbers; negative (unknown) values sort last.
func compareKnown[T int | int64](a, b T) int {
	switch {
	case a < 0 && b < 0:
		return 0
	case a < 0:
		return 1
	case b < 0:
		return -1
	}
	return cmp.Compare(a, b)
}

// naturalCompare compares strings case-insensitively, with runs of digits
// compared by their numeric value: "Track 2" < "track 10".
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if isDigit(ra) && isDigit(rb) {
			da, db := digitRun(a), digitRun(b)
			na, nb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if c := cmp.Or(cmp.Compare(len(na), len(nb)), cmp.Compare(na, nb)); c != 0 {
				return c
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if c := cmp.Compare(unicode.ToLower(ra), unicode.ToLower(rb)); c != 0 {
			return c
		}
		a, b = a[sa:], b[sb:]
	}
	return cmp.Compare(len(a), len(b))
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// digitRun returns the ASCII digits at the start of s.
func digitRun(s string) string {
	i := 0
	for i < len(s) && isDigit(rune(s[i])) {
		i++
	}
	return s[:i]
}
