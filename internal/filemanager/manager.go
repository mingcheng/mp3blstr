package filemanager

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mingcheng/mp3blasterr/internal/media"
)

// Kind classifies directory entries (and selects their colour).
type Kind int

// Entry kinds.
const (
	KindOther Kind = iota
	KindDir
	KindAudio
	KindPlaylist
)

// Entry is a single line in the file manager.
type Entry struct {
	Name    string // directories carry a trailing "/"
	Kind    Kind
	Size    int64
	ModTime time.Time

	selected bool
}

// Selected reports whether the entry has been marked.
func (e *Entry) Selected() bool { return e.selected }

// Options configure a Manager.
type Options struct {
	Matcher    *media.Matcher
	Sort       SortMode
	HideOthers bool // hide files that are neither audio files nor playlists
}

// Manager lists one directory at a time: "./" and "../" first, then the
// sub directories (alphabetically) and then the files (in the sort mode).
type Manager struct {
	opts    Options
	dir     string // cleaned absolute path
	Entries []*Entry
}

// New opens dir (falling back to "/" when it can't be read).
func New(dir string, opts Options) *Manager {
	m := &Manager{opts: opts}
	if err := m.ChangeDir(dir); err != nil {
		_ = m.ChangeDir(string(filepath.Separator))
	}
	return m
}

// SetSort changes the sort mode (call Reload afterwards).
func (m *Manager) SetSort(s SortMode) { m.opts.Sort = s }

// Sort returns the current sort mode.
func (m *Manager) Sort() SortMode { return m.opts.Sort }

// Dir returns the current directory, always ending in a separator.
func (m *Manager) Dir() string {
	if strings.HasSuffix(m.dir, string(filepath.Separator)) {
		return m.dir
	}
	return m.dir + string(filepath.Separator)
}

// Resolve returns the absolute path of p relative to the current directory.
func (m *Manager) Resolve(p string) string {
	p = media.ExpandPath(p)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(m.dir, p))
}

// ChangeDir enters directory p (relative to the current directory).
func (m *Manager) ChangeDir(p string) error {
	target := m.Resolve(p)
	st, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s: not a directory", target)
	}
	old := m.dir
	m.dir = target
	if err := m.Reload(); err != nil {
		m.dir = old
		return err
	}
	return nil
}

// Reload re-reads the current directory.
func (m *Manager) Reload() error {
	des, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	dirs := []*Entry{{Name: "./", Kind: KindDir}, {Name: "../", Kind: KindDir}}
	var files []*Entry
	for _, de := range des {
		full := filepath.Join(m.dir, de.Name())
		info, err := os.Stat(full) // follow symlinks
		if err != nil {
			continue
		}
		if info.IsDir() {
			dirs = append(dirs, &Entry{Name: de.Name() + "/", Kind: KindDir, ModTime: info.ModTime()})
			continue
		}
		e := &Entry{Name: de.Name(), Size: info.Size(), ModTime: info.ModTime()}
		switch {
		case m.opts.Matcher.IsAudio(de.Name()):
			e.Kind = KindAudio
		case m.opts.Matcher.IsPlaylist(de.Name()):
			e.Kind = KindPlaylist
		default:
			if m.opts.HideOthers {
				continue
			}
		}
		files = append(files, e)
	}
	slices.SortStableFunc(dirs, byNameFold)
	sortEntries(files, m.opts.Sort)
	m.Entries = append(dirs, files...)
	return nil
}

func byNameFold(a, b *Entry) int {
	return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
}

// sortEntries orders files in place; SortNone keeps the directory order.
func sortEntries(es []*Entry, mode SortMode) {
	var compare func(a, b *Entry) int
	switch mode {
	case SortAlpha:
		compare = byNameFold
	case SortAlphaCase:
		compare = func(a, b *Entry) int { return cmp.Compare(a.Name, b.Name) }
	case SortModifyNew:
		compare = func(a, b *Entry) int { return b.ModTime.Compare(a.ModTime) }
	case SortModifyOld:
		compare = func(a, b *Entry) int { return a.ModTime.Compare(b.ModTime) }
	case SortSizeSmall:
		compare = func(a, b *Entry) int { return cmp.Compare(a.Size, b.Size) }
	case SortSizeBig:
		compare = func(a, b *Entry) int { return cmp.Compare(b.Size, a.Size) }
	default:
		return
	}
	slices.SortStableFunc(es, compare)
}

// Len returns the number of entries.
func (m *Manager) Len() int { return len(m.Entries) }

// Entry returns entry i or nil.
func (m *Manager) Entry(i int) *Entry {
	if i < 0 || i >= len(m.Entries) {
		return nil
	}
	return m.Entries[i]
}

// Path returns the full path of entry i.
func (m *Manager) Path(i int) string {
	e := m.Entry(i)
	if e == nil {
		return ""
	}
	return filepath.Join(m.dir, strings.TrimSuffix(e.Name, "/"))
}

// IsDir reports whether entry i is a directory.
func (m *Manager) IsDir(i int) bool {
	e := m.Entry(i)
	return e != nil && e.Kind == KindDir
}

// Find returns the index of the entry called name, or -1.
func (m *Manager) Find(name string) int {
	for i, e := range m.Entries {
		if e.Name == name {
			return i
		}
	}
	return -1
}

// Toggle marks or unmarks entry i.
func (m *Manager) Toggle(i int) {
	if e := m.Entry(i); e != nil {
		e.selected = !e.selected
	}
}

// Select marks entry i.
func (m *Manager) Select(i int) {
	if e := m.Entry(i); e != nil {
		e.selected = true
	}
}

// Invert inverts the selection of all entries.
func (m *Manager) Invert() {
	for _, e := range m.Entries {
		e.selected = !e.selected
	}
}

// DeselectAll unmarks all entries.
func (m *Manager) DeselectAll() {
	for _, e := range m.Entries {
		e.selected = false
	}
}

// SelectedPaths returns the full paths of the marked entries.
func (m *Manager) SelectedPaths() []string {
	var out []string
	for i, e := range m.Entries {
		if e.selected {
			out = append(out, m.Path(i))
		}
	}
	return out
}

// SizeDesc formats a file size the way the original does: "[%4d%s]".
func SizeDesc(size int64) string {
	switch {
	case size < 10000:
		return fmt.Sprintf("[%4d%s]", size, "B")
	case size < 10240000:
		return fmt.Sprintf("[%4d%s]", size/1024, "K")
	case size < 2147483647:
		return fmt.Sprintf("[%4d%s]", size/(1024*1024), "M")
	default:
		return fmt.Sprintf("[%4d%s]", size/(1024*1024*1024), "G")
	}
}

// DisplayMode selects how files are shown.
type DisplayMode int

// Display modes, in the order the "toggle display" command cycles through.
const (
	DisplayPlain DisplayMode = iota // file name
	DisplaySize                     // size and file name
	DisplayID3                      // "[ID3] artist - title" for tagged audio files
	numDisplayModes
)

// Next returns the following display mode (wrapping around).
func (d DisplayMode) Next() DisplayMode { return (d + 1) % numDisplayModes }

// DisplayName returns the text for entry i in display mode mode.
// Directories always show their name.
func (m *Manager) DisplayName(i int, mode DisplayMode) string {
	e := m.Entry(i)
	if e == nil {
		return ""
	}
	if e.Kind == KindDir {
		return e.Name
	}
	switch {
	case mode == DisplaySize:
		return SizeDesc(e.Size) + " " + e.Name
	case mode == DisplayID3 && e.Kind == KindAudio:
		return media.ID3Name(m.Path(i), e.Name)
	default:
		return e.Name
	}
}

// skipDirs are never descended into by the recursive functions.
var skipDirs = map[string]bool{".AppleDouble": true, ".resources": true, ".finderinfo": true}

// listDir returns the sub directories and audio files of dir (sorted). It
// doesn't follow a symlinked dir, which could lead into a cycle, nor /dev.
func listDir(ctx context.Context, dir string, matcher *media.Matcher) (subdirs, audio []string) {
	if dir == "/dev" {
		return nil, nil
	}
	if st, err := os.Lstat(dir); err != nil || st.Mode()&fs.ModeSymlink != 0 {
		return nil, nil
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	for _, de := range des {
		if ctx.Err() != nil {
			return nil, nil
		}
		full := filepath.Join(dir, de.Name())
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		if st.IsDir() {
			if !skipDirs[de.Name()] {
				subdirs = append(subdirs, full)
			}
		} else if matcher.IsAudio(full) {
			audio = append(audio, full)
		}
	}
	slices.Sort(subdirs)
	slices.Sort(audio)
	return subdirs, audio
}

// Progress describes a running recursive scan.
type Progress struct {
	Dir      string  // directory being scanned
	Dirs     int     // directories scanned so far
	Files    int     // audio files found so far
	Fraction float64 // estimated part done, 0..1; never decreases
}

// scanner walks a directory tree, reporting progress and stopping when its
// context is cancelled. Every directory owns a slice of the progress range
// that is split evenly among its sub directories, so the estimate only
// grows even though the size of the tree is unknown in advance.
type scanner struct {
	ctx     context.Context
	matcher *media.Matcher
	report  func(Progress)
	p       Progress
}

func (s *scanner) notify() {
	if s.report != nil {
		s.report(s.p)
	}
}

// walk scans d and its sub directories (depth first) and calls visit for
// every directory after its sub directories. [lo, hi) is d's progress range.
func (s *scanner) walk(d string, top bool, lo, hi float64, visit func(dir string, audio []string, top bool)) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.p.Dir, s.p.Fraction = d, max(s.p.Fraction, lo)
	s.notify()
	subdirs, audio := listDir(s.ctx, d, s.matcher)
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.p.Dirs++
	s.p.Files += len(audio)
	step := (hi - lo) / float64(max(len(subdirs), 1))
	for i, sd := range subdirs {
		if err := s.walk(sd, false, lo+float64(i)*step, lo+float64(i+1)*step, visit); err != nil {
			return err
		}
	}
	visit(d, audio, top)
	s.p.Fraction = max(s.p.Fraction, hi)
	s.notify()
	return nil
}

// CollectAudio returns all audio files below dir (recursively, sorted).
// report (optional) is called with the progress; the scan stops with the
// context's error when ctx is cancelled.
func CollectAudio(ctx context.Context, dir string, matcher *media.Matcher, report func(Progress)) ([]string, error) {
	var out []string
	s := &scanner{ctx: ctx, matcher: matcher, report: report}
	err := s.walk(dir, true, 0, 1, func(_ string, audio []string, _ bool) {
		out = append(out, audio...)
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(out)
	return out, nil
}

// DirGroup is a directory turned into a playlist group.
type DirGroup struct {
	Name  string // "[dirname]"
	Files []string
}

// DirsAsGroups returns one group per sub directory of dir (recursively,
// deepest first) that contains audio files. Files directly in dir are not
// included, just like the original "Add Dirs As Groups". Progress and
// cancellation work as for CollectAudio.
func DirsAsGroups(ctx context.Context, dir string, matcher *media.Matcher, report func(Progress)) ([]DirGroup, error) {
	var out []DirGroup
	s := &scanner{ctx: ctx, matcher: matcher, report: report}
	err := s.walk(filepath.Clean(dir), true, 0, 1, func(d string, audio []string, top bool) {
		if !top && len(audio) > 0 {
			out = append(out, DirGroup{Name: "[" + filepath.Base(d) + "]", Files: audio})
		}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
