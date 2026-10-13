package app

import (
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blasterr/internal/keys"
	"github.com/mingcheng/mp3blasterr/internal/playlist"
)

// searchTimeout ends an incremental search when no key is typed.
const searchTimeout = 2 * time.Second

// startSearch starts an incremental search: the cursor jumps to the first
// entry whose name starts with the characters typed so far.
func (a *App) startSearch() {
	a.search = ""
	a.Input = InputSearch
	a.SetMessage("Search:")
	a.armSearchTimeout()
}

// armSearchTimeout (re)starts the search timeout.
func (a *App) armSearchTimeout() {
	a.searchGen++
	gen := a.searchGen
	time.AfterFunc(searchTimeout, func() {
		a.view.Post(func() {
			if a.Input == InputSearch && a.searchGen == gen {
				a.endSearch()
			}
		})
	})
}

// endSearch leaves search input mode.
func (a *App) endSearch() {
	a.search = ""
	a.Input = InputDefault
	a.SetMessage("")
}

// searchChars are the punctuation characters that can be searched for.
const searchChars = " (){}[]<>,.?;:\"'=+-_!@#$%^&*"

// searchable reports whether r may be part of a search.
func searchable(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		r > 127 || strings.ContainsRune(searchChars, r)
}

// searchName is the name of entry i that searches match against: the file
// name of songs and files, the title of groups.
func (a *App) searchName(i int) string {
	if a.Mode == keys.ModeFileSelection {
		return a.fm.Entry(i).Name
	}
	it := a.Cur.Item(i)
	if it.Kind == playlist.KindSong {
		return path.Base(strings.ReplaceAll(it.Path, "\\", "/"))
	}
	return it.Name()
}

// searchKey handles keys during an incremental search.
func (a *App) searchKey(k keys.Key) {
	if k.Code == tcell.KeyEnter || k.Code == tcell.KeyEsc {
		a.endSearch()
		return
	}
	if k.Code != tcell.KeyRune {
		return
	}
	if !searchable(k.Rune) {
		return
	}
	needle := a.search + string(k.Rune)
	found := false
	n := a.MainLen()
	for i := range n {
		if strings.HasPrefix(a.searchName(i), needle) {
			a.MainView().Set(i, n)
			found = true
			break
		}
	}
	a.armSearchTimeout()
	if !found {
		a.view.Beep()
		return
	}
	a.search = needle
	a.SetMessage("Search: " + a.search)
}

// selectByPattern marks the entries whose displayed text matches pattern,
// a shell pattern or (SelectItems.SearchRegex) a regular expression.
func (a *App) selectByPattern(pattern string) {
	if pattern == "" {
		a.Warning("No pattern given.")
		return
	}
	match, err := a.patternMatcher(pattern)
	if err != nil {
		a.Warning("Invalid regular expression.")
		return
	}
	var deselectAll func()
	var selectEntry func(int)
	switch a.Mode {
	case keys.ModeNormal:
		deselectAll, selectEntry = a.Cur.DeselectAll, a.Cur.Select
	case keys.ModeFileSelection:
		deselectAll, selectEntry = a.fm.DeselectAll, a.fm.Select
	default:
		return
	}
	if a.Opts.SelectUnselectFirst {
		deselectAll()
	}
	for i := range a.MainLen() {
		if match(a.MainRow(i).Text) {
			selectEntry(i)
		}
	}
}

// patternMatcher compiles a selection pattern according to the
// SelectItems.* settings.
func (a *App) patternMatcher(pattern string) (func(string) bool, error) {
	fold := a.Opts.SelectCaseInsensitive
	if a.Opts.SelectRegex {
		if fold {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	if fold {
		pattern = strings.ToLower(pattern)
	}
	return func(s string) bool {
		if fold {
			s = strings.ToLower(s)
		}
		ok, _ := path.Match(pattern, s)
		return ok
	}, nil
}
