// Package playlist implements mp3blstr's hierarchical playlist: a tree of
// groups, each containing songs and sub groups, with the bookkeeping needed
// to play groups in order or shuffled.
package playlist

import (
	"fmt"
	"github.com/mingcheng/mp3blstr/internal/media"
)

// Kind distinguishes the entries of a group.
type Kind int

// Item kinds.
const (
	KindSong   Kind = iota
	KindGroup       // a sub group
	KindParent      // the "[..]" link to the parent group
)

// ParentName is the display name of the link to the parent group.
const ParentName = "[..]"

// Item is a single entry in a group.
type Item struct {
	Kind  Kind
	Path  string // full path or URL (songs only)
	Group *Group // target group (groups and parent links)

	played   bool // played in the current run (songs only)
	selectID int  // order in which the item was marked; 0 = not marked
}

// Name returns the canonical name of the item: the full path for songs and
// the title for groups.
func (it *Item) Name() string {
	switch it.Kind {
	case KindGroup:
		return it.Group.Title
	case KindParent:
		return ParentName
	default:
		return it.Path
	}
}

// IsGroup reports whether the item is a group or a parent link.
func (it *Item) IsGroup() bool { return it.Kind != KindSong }

// Selected reports whether the item has been marked by the user.
func (it *Item) Selected() bool { return it.selectID > 0 }

// Group is a list of songs and sub groups.
type Group struct {
	Title   string
	Shuffle bool // play the songs of this group in random order
	Parent  *Group
	Items   []*Item // sub groups first; Items[0] is "[..]" in sub groups

	played   bool // the group has been selected for playing in this run
	playing  bool // the group is being played
	selectID int  // selectID of the next item to be marked
}

// NewGroup returns an empty group.
func NewGroup(title string) *Group { return &Group{Title: title, selectID: 1} }

// Len returns the number of entries.
func (g *Group) Len() int { return len(g.Items) }

// Item returns entry i or nil.
func (g *Group) Item(i int) *Item {
	if i < 0 || i >= len(g.Items) {
		return nil
	}
	return g.Items[i]
}

// IsGroup reports whether entry i is a group or parent link.
func (g *Group) IsGroup(i int) bool {
	it := g.Item(i)
	return it != nil && it.IsGroup()
}

// IsParentLink reports whether entry i is the "[..]" link.
func (g *Group) IsParentLink(i int) bool {
	it := g.Item(i)
	return it != nil && it.Kind == KindParent
}

// SubGroup returns the group entry i points at (also for "[..]").
func (g *Group) SubGroup(i int) *Group {
	if it := g.Item(i); it != nil && it.IsGroup() {
		return it.Group
	}
	return nil
}

// AddSong appends a song.
func (g *Group) AddSong(path string) *Item {
	it := &Item{Kind: KindSong, Path: path}
	g.Items = append(g.Items, it)
	return it
}

// AddGroup inserts sub after the existing sub groups (groups are always
// listed first) and links it back to g.
func (g *Group) AddGroup(sub *Group) {
	idx := 0
	for idx < len(g.Items) && g.Items[idx].IsGroup() {
		idx++
	}
	it := &Item{Kind: KindGroup, Group: sub}
	g.insert(idx, it)
	sub.Parent = g
	sub.Items = append([]*Item{{Kind: KindParent, Group: g}}, sub.Items...)
}

func (g *Group) insert(idx int, it *Item) {
	g.Items = append(g.Items, nil)
	copy(g.Items[idx+1:], g.Items[idx:])
	g.Items[idx] = it
}

// Delete removes entry i. The "[..]" link cannot be removed.
func (g *Group) Delete(i int) bool {
	it := g.Item(i)
	if it == nil || it.Kind == KindParent {
		return false
	}
	g.Items = append(g.Items[:i], g.Items[i+1:]...)
	return true
}

// Clear removes every entry except the parent link.
func (g *Group) Clear() {
	var keep []*Item
	for _, it := range g.Items {
		if it.Kind == KindParent {
			keep = append(keep, it)
		}
	}
	g.Items = keep
	g.selectID = 1
}

// RemoveDuplicates keeps the first occurrence of each song path across
// this group and its descendants, leaving groups and parent links intact.
func (g *Group) RemoveDuplicates() int {
	seen := make(map[string]bool)
	var remove func(*Group) int
	remove = func(group *Group) int {
		kept := group.Items[:0]
		removed := 0
		for _, it := range group.Items {
			switch it.Kind {
			case KindGroup:
				removed += remove(it.Group)
			case KindSong:
				if seen[it.Path] {
					removed++
					continue
				}
				seen[it.Path] = true
			}
			kept = append(kept, it)
		}
		clear(group.Items[len(kept):])
		group.Items = kept
		return removed
	}
	return remove(g)
}

// --- selection -----------------------------------------------------------

// ToggleSelect marks or unmarks entry i.
func (g *Group) ToggleSelect(i int) {
	it := g.Item(i)
	if it == nil {
		return
	}
	if it.selectID > 0 {
		it.selectID = 0
		return
	}
	g.markSelected(it)
}

// Select marks entry i (without toggling).
func (g *Group) Select(i int) {
	if it := g.Item(i); it != nil && it.selectID == 0 {
		g.markSelected(it)
	}
}

func (g *Group) markSelected(it *Item) {
	if g.selectID < 1 {
		g.selectID = 1
	}
	it.selectID = g.selectID
	g.selectID++
}

// DeselectAll unmarks every entry.
func (g *Group) DeselectAll() {
	for _, it := range g.Items {
		it.selectID = 0
	}
	g.selectID = 1
}

// MoveSelected moves the marked entries after (or before) entry to, in the
// order in which they were marked. The target itself must not be marked.
func (g *Group) MoveSelected(to int, before bool) bool {
	target := g.Item(to)
	if target == nil || target.Selected() {
		return false
	}
	for {
		found, sel := -1, 0
		for i, it := range g.Items {
			id := it.selectID
			if id == 0 {
				continue
			}
			// Before: take the earliest selection; after: the latest.
			if sel == 0 || (before && id < sel) || (!before && id > sel) {
				sel, found = id, i
			}
		}
		if found == -1 {
			return true
		}
		if found == to || !g.moveItem(found, to, before) {
			return false
		}
		if before && found > to {
			to++
		} else if !before && found < to {
			to--
		}
	}
}

// moveItem moves entry from to before/after entry to (indices before the
// move). The moved entry is deselected so MoveSelected terminates.
func (g *Group) moveItem(from, to int, before bool) bool {
	if from == to || from < 0 || from >= len(g.Items) || to < 0 || to >= len(g.Items) {
		return false
	}
	it := g.Items[from]
	it.selectID = 0
	g.Items = append(g.Items[:from], g.Items[from+1:]...)
	if from < to {
		to--
	}
	if !before {
		to++
	}
	g.insert(to, it)
	return true
}

// --- played state --------------------------------------------------------

// SetPlayed sets the played state of the group.
func (g *Group) SetPlayed(v bool) { g.played = v }

// SetPlaying sets the playing state of the group.
func (g *Group) SetPlaying(v bool) { g.playing = v }

// subGroups yields the real sub groups (never the parent link).
func (g *Group) subGroups(yield func(*Group) bool) {
	for _, it := range g.Items {
		if it.Kind == KindGroup && !yield(it.Group) {
			return
		}
	}
}

// IsPlaying reports whether this group or one of its sub groups is playing.
func (g *Group) IsPlaying() bool {
	if g.playing {
		return true
	}
	for sub := range g.subGroups {
		if sub.IsPlaying() {
			return true
		}
	}
	return false
}

// UnplayedSongs counts the songs not played yet.
func (g *Group) UnplayedSongs(recursive bool) int {
	n := 0
	for _, it := range g.Items {
		switch it.Kind {
		case KindGroup:
			if recursive {
				n += it.Group.UnplayedSongs(true)
			}
		case KindSong:
			if !it.played {
				n++
			}
		}
	}
	return n
}

// UnplayedGroups counts the groups that are not played yet and that still
// contain unplayed songs; g itself is included.
func (g *Group) UnplayedGroups(recursive bool) int {
	n := 0
	for sub := range g.subGroups {
		if recursive {
			n += sub.UnplayedGroups(true)
		} else if !sub.played && sub.UnplayedSongs(false) > 0 {
			n++
		}
	}
	if !g.played && g.UnplayedSongs(false) > 0 {
		n++
	}
	return n
}

// UnplayedGroup returns the idx-th unplayed group (depth first, sub groups
// before g itself) and optionally marks it played.
func (g *Group) UnplayedGroup(idx int, setPlayed, recursive bool) *Group {
	count := 0
	if recursive {
		for sub := range g.subGroups {
			n := sub.UnplayedGroups(true)
			if count+n > idx {
				return sub.UnplayedGroup(idx-count, setPlayed, true)
			}
			count += n
		}
	}
	if count == idx && !g.played && g.UnplayedSongs(true) > 0 {
		if setPlayed {
			g.played = true
		}
		return g
	}
	return nil
}

// UnplayedSong returns the idx-th unplayed song and optionally marks it as
// played.
func (g *Group) UnplayedSong(idx int, setPlayed, recursive bool) *Item {
	count := 0
	for _, it := range g.Items {
		switch it.Kind {
		case KindGroup:
			if !recursive {
				continue
			}
			n := it.Group.UnplayedSongs(true)
			if count+n > idx {
				return it.Group.UnplayedSong(idx-count, setPlayed, true)
			}
			count += n
		case KindSong:
			if it.played {
				continue
			}
			if count == idx {
				if setPlayed {
					it.played = true
				}
				return it
			}
			count++
		}
	}
	return nil
}

// ResetSongs sets the played state of all songs (optionally recursively).
func (g *Group) ResetSongs(recursive, played bool) {
	for _, it := range g.Items {
		switch it.Kind {
		case KindGroup:
			if recursive {
				it.Group.ResetSongs(true, played)
			}
		case KindSong:
			it.played = played
		}
	}
}

// ResetGroups marks g and all sub groups as not played.
func (g *Group) ResetGroups() {
	for sub := range g.subGroups {
		sub.ResetGroups()
	}
	g.played = false
}

// SetAllSongsPlayed marks all songs as played.
func (g *Group) SetAllSongsPlayed(recursive bool) { g.ResetSongs(recursive, true) }

// SetPlayed sets the played state of a song.
func (it *Item) SetPlayed(v bool) { it.played = v }

// FindSong returns the first song with the given path (in display order,
// searching sub groups recursively) and the group containing it.
func (g *Group) FindSong(path string) (*Group, *Item) {
	for _, it := range g.Items {
		switch it.Kind {
		case KindGroup:
			if sg, si := it.Group.FindSong(path); si != nil {
				return sg, si
			}
		case KindSong:
			if it.Path == path {
				return g, it
			}
		}
	}
	return nil, nil
}

// MarkPlayedThrough marks the songs of g up to and including it as played.
func (g *Group) MarkPlayedThrough(it *Item) {
	for _, s := range g.Items {
		if s.Kind != KindSong {
			continue
		}
		s.played = true
		if s == it {
			return
		}
	}
}

// GroupsBefore returns the groups played before target when g and its sub
// groups are played in order (depth first, sub groups before their parent,
// like UnplayedGroup). ok is false when target is not g or below it.
func (g *Group) GroupsBefore(target *Group) (before []*Group, ok bool) {
	var walk func(*Group) bool
	walk = func(cur *Group) bool {
		for sub := range cur.subGroups {
			if walk(sub) {
				return true
			}
		}
		if cur == target {
			return true
		}
		before = append(before, cur)
		return false
	}
	if !walk(g) {
		return nil, false
	}
	return before, true
}

// DisplayMode selects how songs are shown in the playlist window.
type DisplayMode int

// Display modes, in the order the "toggle display" command cycles through.
const (
	DisplayPath     DisplayMode = iota // full path or URL
	DisplayFileName                    // file name only
	DisplayID3                         // "[ID3] artist - title" when tagged
	numDisplayModes
)

// Next returns the following display mode (wrapping around).
func (m DisplayMode) Next() DisplayMode { return (m + 1) % numDisplayModes }

func (m DisplayMode) String() string {
	switch m {
	case DisplayPath:
		return "path"
	case DisplayID3:
		return "id3"
	default:
		return "filename"
	}
}

func ParseDisplayMode(s string) (DisplayMode, error) {
	switch s {
	case "path":
		return DisplayPath, nil
	case "filename":
		return DisplayFileName, nil
	case "id3":
		return DisplayID3, nil
	default:
		return 0, fmt.Errorf("unknown display mode %q", s)
	}
}

// DisplayName returns the text shown for entry it in display mode mode.
// Groups always show their title.
func DisplayName(it *Item, mode DisplayMode) string {
	if it.Kind != KindSong {
		return it.Name()
	}
	switch mode {
	case DisplayPath:
		return it.Path
	case DisplayID3:
		return media.ID3Name(it.Path, it.Path)
	default:
		return media.ChopPath(it.Path)
	}
}
