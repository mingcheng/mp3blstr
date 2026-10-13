package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mingcheng/mp3blasterr/internal/filemanager"
	"github.com/mingcheng/mp3blasterr/internal/keys"
	"github.com/mingcheng/mp3blasterr/internal/listview"
	"github.com/mingcheng/mp3blasterr/internal/media"
	"github.com/mingcheng/mp3blasterr/internal/playback"
	"github.com/mingcheng/mp3blasterr/internal/playlist"
)

// fmBegin enters file selection mode.
func (a *App) fmBegin() {
	a.Player.QuitAfterPlaylist = false
	a.fm = filemanager.New(a.fmDir, filemanager.Options{
		Matcher:    a.Matcher,
		Sort:       a.Opts.SortMode,
		HideOthers: a.Opts.HideOthers,
	})
	a.fmView = listview.State{}
	a.fmDisplay = a.Opts.FileDisplayMode
	if a.Opts.ID3Names && a.fmDisplay == filemanager.DisplaySize {
		a.fmDisplay = filemanager.DisplayID3
	}
	a.setMode(keys.ModeFileSelection)
}

// fmEnd leaves file mode and adds all selected files to the current group.
func (a *App) fmEnd() {
	if a.Mode != keys.ModeFileSelection {
		return
	}
	a.collectSelected()
	for _, f := range a.pending {
		a.Cur.AddSong(f)
	}
	a.pending = nil
	a.fmDir = a.fm.Dir()
	a.setMode(keys.ModeNormal)
}

// collectSelected moves the marked audio files of the current directory to
// the pending list.
func (a *App) collectSelected() {
	for _, p := range a.fm.SelectedPaths() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && a.Matcher.IsAudio(p) {
			a.pending = append(a.pending, p)
		}
	}
	a.fm.DeselectAll()
}

// fmChangeDir changes the file manager's directory, remembering the files
// marked in the old one.
func (a *App) fmChangeDir(p string, fromEntry bool) {
	if p != "." && p != "./" {
		a.collectSelected()
	}
	old := filepath.Base(strings.TrimRight(a.fm.Dir(), string(filepath.Separator)))
	if err := a.fm.ChangeDir(p); err != nil {
		if !fromEntry {
			a.Warning("Couldn't change directory.")
		}
		return
	}
	a.fmDir = a.fm.Dir()
	a.fmView = listview.State{Rows: a.fmView.Rows}
	if p == ".." || p == "../" {
		if i := a.fm.Find(old + "/"); i >= 0 {
			a.fmView.Set(i, a.fm.Len())
		}
	}
}

// execFileCommand runs the commands of file selection mode.
func (a *App) execFileCommand(cmd keys.Command) {
	if a.Mode != keys.ModeFileSelection {
		return
	}
	fm := a.fm
	v := &a.fmView
	switch cmd {
	case keys.CmdFileSelect:
		e := fm.Entry(v.Cursor)
		if e != nil && e.Name != "./" && e.Name != "../" {
			fm.Toggle(v.Cursor)
		}
		v.Move(1, fm.Len(), a.Opts.Wrap)
	case keys.CmdFileUpDir:
		a.fmChangeDir("..", false)
	case keys.CmdFileToggleSort:
		a.Opts.SortMode = fm.Sort().Next()
		fm.SetSort(a.Opts.SortMode)
		a.fmReload()
	case keys.CmdFileDelete:
		if fm.Entry(v.Cursor) == nil || fm.IsDir(v.Cursor) {
			break
		}
		path, name := fm.Path(v.Cursor), fm.Entry(v.Cursor).Name
		a.openConfirm([]string{"Delete this file permanently?", name}, func() {
			a.deleteFile(path)
		})
	case keys.CmdFileEnter:
		a.fileEnter()
	case keys.CmdFileAddFiles:
		a.fmEnd()
	case keys.CmdFileInvSelection:
		fm.Invert()
		for _, n := range []string{"./", "../"} {
			if i := fm.Find(n); i >= 0 && fm.Entry(i).Selected() {
				fm.Toggle(i)
			}
		}
	case keys.CmdFileRecursiveSelect:
		dir := fm.Dir()
		startTask(a, "Recursively selecting files", func(ctx context.Context, report func(TaskProgress)) ([]string, error) {
			return filemanager.CollectAudio(ctx, dir, a.Matcher, scanReporter(report))
		}, func(files []string) {
			if a.Mode != keys.ModeFileSelection {
				return
			}
			a.pending = append(a.pending, files...)
			a.fmEnd()
			a.SetMessage(fmt.Sprintf("Added %d files from this dir and all subdirs to current group.", len(files)))
		})
	case keys.CmdFileSetPath:
		a.openPopup([]string{"Enter path below"}, "", 1024, func(p string) {
			if p == "" {
				return
			}
			a.fmChangeDir(p, false)
		})
	case keys.CmdFileDirsAsGroups:
		dir := fm.Dir()
		startTask(a, "Adding dirs as groups", func(ctx context.Context, report func(TaskProgress)) ([]filemanager.DirGroup, error) {
			return filemanager.DirsAsGroups(ctx, dir, a.Matcher, scanReporter(report))
		}, func(groups []filemanager.DirGroup) {
			if a.Mode != keys.ModeFileSelection {
				return
			}
			for _, dg := range groups {
				g := playlist.NewGroup(dg.Name)
				for _, f := range dg.Files {
					g.AddSong(f)
				}
				a.Cur.AddGroup(g)
			}
			a.fmEnd()
			a.SetMessage(fmt.Sprintf("Added %d dirs as groups.", len(groups)))
		})
	case keys.CmdFileAddURL:
		a.openPopup([]string{"URL:"}, "", 2048, func(u string) {
			if u != "" {
				a.pending = append(a.pending, u)
			}
		})
	case keys.CmdFileRename:
		if fm.Entry(v.Cursor) == nil || fm.IsDir(v.Cursor) {
			break
		}
		a.openPopup([]string{"Filename:"}, fm.Entry(v.Cursor).Name, 2048, a.renameFile)
	}
}

// scanReporter turns directory scan progress into task progress.
func scanReporter(report func(TaskProgress)) func(filemanager.Progress) {
	return func(p filemanager.Progress) {
		report(TaskProgress{
			Detail:   p.Dir,
			Status:   fmt.Sprintf("%d audio files in %d folders", p.Files, p.Dirs),
			Fraction: p.Fraction,
		})
	}
}

// fmReload re-reads the directory after files were changed.
func (a *App) fmReload() {
	if err := a.fm.Reload(); err != nil {
		a.Warning("Couldn't read directory.")
	}
	a.fmView.Clamp(a.fm.Len())
}

// fileEnter enters the directory under the cursor, loads the playlist
// under it or plays the song under it.
func (a *App) fileEnter() {
	fm := a.fm
	i := a.fmView.Cursor
	e := fm.Entry(i)
	if e == nil {
		return
	}
	switch {
	case fm.IsDir(i):
		a.fmChangeDir(e.Name, true)
	case e.Kind == filemanager.KindPlaylist:
		file := fm.Path(i)
		a.fmEnd()
		a.LoadPlaylist(file)
	case e.Kind == filemanager.KindAudio:
		a.Player.PlayOne(fm.Path(i))
	}
}

// LoadPlaylist adds the songs and groups of a playlist file to the current
// group (used for --list/--autolist as well). It reports whether the file
// could be read.
func (a *App) LoadPlaylist(file string) bool {
	gs, err := playlist.Load(file, a.Cur)
	if err != nil {
		a.Warning("Couldn't load playlist (wrong path?)")
		return false
	}
	if gs.PlayMode != "" {
		if m, err := playback.ParsePlayMode(gs.PlayMode); err == nil {
			a.Player.Mode = m
			a.Opts.PlayMode = m
		}
	}
	if gs.Shuffle {
		a.Root.Shuffle = true
	}
	return true
}

// AddFiles appends songs to the current group (files from the command line).
func (a *App) AddFiles(files []string) {
	for _, f := range files {
		if !media.IsHTTPStream(f) {
			if abs, err := filepath.Abs(f); err == nil {
				f = abs
			}
		}
		a.Cur.AddSong(f)
	}
}

// writePlaylist saves the whole playlist as an M3U file name in the playlist
// directory, which is created when missing.
func (a *App) writePlaylist(name string) {
	if name == "" {
		return
	}
	if !playlist.IsM3U(name) {
		name += ".m3u"
	}
	name = media.ChopPath(name)
	if name == "" {
		return
	}
	dir := media.ExpandPath(a.Opts.PlaylistDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.Warning("Failed to create playlist dir.")
		return
	}
	if err := playlist.SaveM3U(filepath.Join(dir, name), a.Root); err != nil {
		a.Warning("Failed to write playlist.")
		return
	}
	a.SetMessage("Playlist written.")
}

// deleteFile deletes a file (after confirmation).
func (a *App) deleteFile(path string) {
	if a.Mode != keys.ModeFileSelection {
		return
	}
	if err := os.Remove(path); err != nil {
		a.Warning("Could not delete file.")
		return
	}
	media.ForgetID3(path)
	a.fmReload()
	a.SetMessage("File deleted.")
}

// renameFile renames the file under the cursor; existing files are never
// overwritten.
func (a *App) renameFile(newName string) {
	if a.Mode != keys.ModeFileSelection {
		return
	}
	i := a.fmView.Cursor
	if a.fm.Entry(i) == nil || newName == "" {
		a.Warning("Nothing to rename (to).")
		return
	}
	oldPath := a.fm.Path(i)
	newPath := filepath.Join(a.fm.Dir(), newName)
	if _, err := os.Stat(newPath); err == nil {
		a.Warning("Cowardly refusing to overwrite existing file.")
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		// Show the reason only; the paths don't fit in the message line.
		msg := err.Error()
		if le, ok := errors.AsType[*os.LinkError](err); ok {
			msg = le.Err.Error()
		}
		a.Warning("Rename failed: " + msg)
		return
	}
	media.ForgetID3(oldPath)
	a.fmReload()
	a.SetMessage("File renamed.")
}
