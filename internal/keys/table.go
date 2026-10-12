package keys

import "github.com/gdamore/tcell/v2"

// Group is the category of a binding. The help bar and the key reference
// on the help screen list the bindings category by category in this order.
type Group uint8

// Binding categories.
const (
	GroupPlaylist Group = iota
	GroupFiles
	GroupPlayback
	GroupSelection
	GroupDisplay
	GroupNavigation
	GroupGeneral
	numGroups
)

var groupNames = [numGroups]string{
	"Playlist", "File Manager", "Playback", "Search & Selection",
	"Display & Settings", "Navigation", "General",
}

func (g Group) String() string {
	if g < numGroups {
		return groupNames[g]
	}
	return ""
}

// Binding binds a key to a command in a set of program modes. Desc is the
// text shown in the help bar (19 characters max). Hidden bindings work but
// are not listed in the help bar: standard navigation keys, keys whose label
// is already shown elsewhere on screen and alternative keys of a command.
type Binding struct {
	Key    Key
	Cmd    Command
	Modes  Mode
	Desc   string
	Group  Group
	Hidden bool

	custom bool // set from the configuration file
}

// Table is an ordered list of bindings. When several bindings use the same
// key in the same mode, a configured one wins over the defaults, otherwise
// the first one wins.
type Table struct {
	bindings []Binding
}

// helpModes are the program modes a binding can be valid in.
var helpModes = []Mode{ModeNormal, ModeFileSelection, ModeHelp}

// Default returns the stock mp3blaster key bindings.
func Default() *Table {
	f := func(n int) Key { return Special(tcell.KeyF1 + tcell.Key(n-1)) }
	r := Rune
	s := Special
	const (
		N  = ModeNormal
		F  = ModeFileSelection
		NF = ModeNormal | ModeFileSelection
		A  = ModeAny
	)
	type entry struct {
		k    Key
		cmd  Command
		m    Mode
		desc string
	}
	t := &Table{}
	add := func(g Group, hidden bool, es ...entry) {
		for _, e := range es {
			t.bindings = append(t.bindings, Binding{Key: e.k, Cmd: e.cmd, Modes: e.m, Desc: e.desc, Group: g, Hidden: hidden})
		}
	}

	add(GroupPlaylist, false,
		entry{f(1), CmdSelectFiles, N, "Select Files"},
		entry{f(2), CmdAddGroup, N, "Add Group"},
		entry{f(5), CmdSetGroupTitle, N, "Set Group Title"},
		entry{f(3), CmdLoadPlaylist, N, "Load/Add Playlist"},
		entry{f(4), CmdWritePlaylist, N, "Write Playlist"},
		entry{r('C'), CmdClearPlaylist, N, "Clear Playlist"},
		entry{r('m'), CmdMoveAfter, N, "Move Files After"},
		entry{r('M'), CmdMoveBefore, N, "Move Files Before"},
		entry{r('d'), CmdDel, N, "Delete"},
		entry{r('D'), CmdDelMark, N, "Delete And Mark"},
	)
	add(GroupFiles, false,
		entry{f(1), CmdFileAddFiles, F, "Add Files To List"},
		entry{f(3), CmdFileRecursiveSelect, F, "Recurs. Select All"},
		entry{f(5), CmdFileDirsAsGroups, F, "Add Dirs As Groups"},
		entry{f(7), CmdFileAddURL, F, "Add URL(shoutcast)"},
		entry{f(4), CmdFileSetPath, F, "Enter New Path"},
		entry{s(tcell.KeyBackspace), CmdFileUpDir, F, "Go Up One Dir"},
		entry{r('s'), CmdFileToggleSort, F, "Toggle File Sort"},
		entry{r('r'), CmdFileRename, F, "Rename File"},
		entry{r('D'), CmdFileDelete, F, "Delete file"},
		entry{r('b'), CmdFileMarkBad, F, "Mark file(s) as bad"},
	)
	add(GroupPlayback, false,
		entry{r('R'), CmdToggleRepeat, A, "Toggle Repeat"},
		entry{r('Z'), CmdToggleShuffle, A, "Toggle GroupShuffle"},
		entry{f(8), CmdTogglePlaymode, N, "Toggle Play Mode"},
		entry{r('['), CmdPlayPrevGroup, A, "Previous Group"},
		entry{r(']'), CmdPlayNextGroup, A, "Next Group"},
		entry{r('E'), CmdPlaySkipEnd, A, "Skip to End Song"},
	)
	// The CD-player buttons and the mixer show these keys themselves; F6
	// and F7 are the original keys for repeat and shuffle.
	add(GroupPlayback, true,
		entry{f(6), CmdToggleRepeat, N, "Toggle Repeat"},
		entry{f(7), CmdToggleShuffle, N, "Toggle GroupShuffle"},
		entry{r('4'), CmdPlayPrevious, A, "Previous Song"},
		entry{r('5'), CmdPlayPlay, A, "Play Song"},
		entry{r('6'), CmdPlayNext, A, "Next Song"},
		entry{r('1'), CmdPlayRewind, A, "Rewind Song"},
		entry{r('2'), CmdPlayStop, A, "Stop Song"},
		entry{r('3'), CmdPlayForward, A, "Forward Song"},
		entry{r('t'), CmdToggleMixer, A, "Toggle Mixer Device"},
		entry{r('<'), CmdMixerVolDown, A, "MixerVolume Down"},
		entry{r('>'), CmdMixerVolUp, A, "MixerVolume Up"},
	)
	add(GroupSelection, false,
		entry{r('/'), CmdFileStartSearch, NF, "Start Search"},
		entry{r(' '), CmdSelect, N, "Select File"},
		entry{r(' '), CmdFileSelect, F, "Select File"},
		entry{f(2), CmdFileInvSelection, F, "Invert Selection"},
		entry{r('S'), CmdSelectItems, NF, "Select Some Items"},
		entry{r('U'), CmdDeselectItems, NF, "Unselect All"},
	)
	add(GroupDisplay, false,
		entry{r('f'), CmdToggleDisplay, NF, "Toggle File Display"},
		entry{r('w'), CmdToggleWrap, A, "Toggle List Wrap"},
		entry{r('T'), CmdToggleTheme, A, "Switch Theme"},
		entry{r('I'), CmdToggleNerdFont, A, "Toggle Nerd Font"},
		entry{f(10), CmdChangeThread, A, "Change #Threads"},
	)
	add(GroupNavigation, true,
		entry{s(tcell.KeyEnter), CmdEnter, N, "Enter"},
		entry{s(tcell.KeyEnter), CmdFileEnter, F, "Enter"},
		entry{s(tcell.KeyUp), CmdUp, A, "Move Scrollbar Up"},
		entry{s(tcell.KeyDown), CmdDown, A, "Move Scrolbar Down"},
		entry{s(tcell.KeyPgUp), CmdPrevPage, A, "Previous Page"},
		entry{s(tcell.KeyPgDn), CmdNextPage, A, "Next Page"},
		entry{s(tcell.KeyHome), CmdJumpTop, A, "Start Of List"},
		entry{s(tcell.KeyEnd), CmdJumpBottom, A, "End Of List"},
		entry{s(tcell.KeyLeft), CmdLeft, A, "Pan Window Left"},
		entry{s(tcell.KeyRight), CmdRight, A, "Pan Window Right"},
	)
	add(GroupGeneral, false,
		entry{r('?'), CmdHelp, A, "Show Help"},
		entry{r('q'), CmdQuitProgram, A, "Quit Mp3blaster"},
		entry{s(tcell.KeyCtrlL), CmdRefresh, A, "Refresh Screen"},
	)
	// Shown as "( - )" and "( + )" next to the help bar.
	add(GroupGeneral, true,
		entry{r('-'), CmdHelpPrev, A, "Scroll Up Helptext"},
		entry{r('+'), CmdHelpNext, A, "Scroll Down Helptxt"},
	)
	return t
}

// Lookup returns the command bound to key k in mode m. Configured bindings
// take precedence over the defaults; otherwise the first match wins.
func (t *Table) Lookup(k Key, m Mode) Command {
	found := CmdNone
	for _, b := range t.bindings {
		if b.Key != k || b.Modes&m == 0 {
			continue
		}
		if b.custom {
			return b.Cmd
		}
		if found == CmdNone {
			found = b.Cmd
		}
	}
	return found
}

// Bind sets the key of cmd (from the configuration file). The first binding
// of cmd gets the new key; the other default keys of cmd are removed, so a
// configured key replaces all default ones.
func (t *Table) Bind(cmd Command, k Key) {
	out := t.bindings[:0]
	seen := false
	for _, b := range t.bindings {
		if b.Cmd == cmd {
			if !seen {
				seen = true
				b.Key, b.custom = k, true
			} else if !b.custom {
				continue
			}
		}
		out = append(out, b)
	}
	t.bindings = out
}

// KeyFor returns the key bound to cmd.
func (t *Table) KeyFor(cmd Command) Key {
	for _, b := range t.bindings {
		if b.Cmd == cmd {
			return b.Key
		}
	}
	return Key{}
}

// LabelFor returns the 3 character label of the key bound to cmd.
func (t *Table) LabelFor(cmd Command) string {
	k := t.KeyFor(cmd)
	if k.IsZero() {
		return "   "
	}
	return k.Label()
}

// ForMode returns, in table order, the bindings that are valid in mode m.
func (t *Table) ForMode(m Mode) []Binding {
	var out []Binding
	for _, b := range t.bindings {
		if b.Modes&m != 0 {
			out = append(out, b)
		}
	}
	return out
}

// effective reports whether b actually triggers its command in one of the
// modes in m, i.e. it is not shadowed by another binding of the same key.
func (t *Table) effective(b Binding, m Mode) bool {
	for _, hm := range helpModes {
		if b.Modes&m&hm != 0 && t.Lookup(b.Key, hm) == b.Cmd {
			return true
		}
	}
	return false
}

// Grouped returns the working bindings valid in mode m, category by
// category, leaving out empty categories. With bar set, it returns the
// entries of the help bar: hidden bindings and second keys of a command are
// left out as well.
func (t *Table) Grouped(m Mode, bar bool) [][]Binding {
	var out [][]Binding
	for g := Group(0); g < numGroups; g++ {
		var list []Binding
		shown := map[Command]bool{}
		for _, b := range t.bindings {
			if b.Group != g || b.Modes&m == 0 || !t.effective(b, m) {
				continue
			}
			if bar && (b.Hidden || shown[b.Cmd]) {
				continue
			}
			shown[b.Cmd] = true
			list = append(list, b)
		}
		if len(list) > 0 {
			out = append(out, list)
		}
	}
	return out
}
