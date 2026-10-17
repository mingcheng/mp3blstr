package app

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"

	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

// sortPlaylist sorts the songs of the group on screen in the next order of
// its cycle: name, artist, title, duration, size, shuffle. Orders that need
// tags, lengths or sizes read them in a background task.
func (a *App) sortPlaylist() {
	if a.Mode != keys.ModeNormal {
		return
	}
	g := a.Cur
	mode := a.groupView(g).sort
	var paths []string
	for _, it := range g.Items {
		if it.Kind == playlist.KindSong {
			paths = append(paths, it.Path)
		}
	}
	if !mode.NeedsMetadata() || len(paths) == 0 {
		a.applySort(g, mode, nil)
		return
	}
	startTask(a, "Sorting by "+mode.String(), func(ctx context.Context, report func(TaskProgress)) (map[string]playlist.SortInfo, error) {
		infos := make(map[string]playlist.SortInfo, len(paths))
		for i, path := range paths {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			report(TaskProgress{
				Detail:   media.ChopPath(path),
				Status:   fmt.Sprintf("%d of %d songs", i, len(paths)),
				Fraction: float64(i) / float64(len(paths)),
			})
			if _, ok := infos[path]; !ok {
				infos[path] = readSortInfo(path, mode)
			}
		}
		return infos, nil
	}, func(infos map[string]playlist.SortInfo) { a.applySort(g, mode, infos) })
}

// readSortInfo reads what sorting in mode needs to know about a song. It
// runs on a background goroutine.
func readSortInfo(path string, mode playlist.SortMode) playlist.SortInfo {
	info := playlist.UnknownSortInfo
	if media.IsHTTPStream(path) {
		return info
	}
	switch mode {
	case playlist.SortArtist, playlist.SortTitle:
		if t, err := media.ReadTags(path); err == nil {
			info.Artist, info.Album, info.Title = t.Artist, t.Album, t.Title
		}
	case playlist.SortDuration:
		if s, err := media.CachedDuration(path, audio.Duration); err == nil && s >= 0 {
			info.Seconds = s
		}
	case playlist.SortSize:
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			info.Size = st.Size()
		}
	}
	return info
}

// applySort reorders the songs of g. The selection bar stays on its entry
// and the next press of the sort key continues with the following order.
// Songs the player has not played yet are played in the new order.
func (a *App) applySort(g *playlist.Group, mode playlist.SortMode, infos map[string]playlist.SortInfo) {
	gv := a.groupView(g)
	cur := g.Item(gv.state.Cursor)
	if mode == playlist.SortShuffle {
		g.ShuffleSongs(rand.Shuffle)
	} else {
		g.SortSongs(mode, func(path string) playlist.SortInfo {
			if info, ok := infos[path]; ok {
				return info
			}
			return playlist.UnknownSortInfo
		})
	}
	if mode == playlist.SortDuration {
		for path, info := range infos {
			if info.Seconds >= 0 {
				a.durations[path] = info.Seconds
			}
		}
	}
	if i := slices.Index(g.Items, cur); i >= 0 {
		gv.state.Set(i, g.Len())
	}
	gv.sort = mode.Next()
	a.Player.SongsReordered()
	key := strings.TrimSpace(a.Keys.LabelFor(keys.CmdSortPlaylist))
	done := "sorted by " + mode.String()
	if mode == playlist.SortShuffle {
		done = "shuffled"
	}
	a.SetMessage(fmt.Sprintf("Playlist %s; press '%s' for %s", done, key, gv.sort))
}
