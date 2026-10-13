package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mingcheng/mp3blasterr/internal/media"
)

func touch(t *testing.T, p string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManager(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "b.mp3"), 10)
	touch(t, filepath.Join(dir, "A.mp3"), 20000)
	touch(t, filepath.Join(dir, "list.lst"), 1)
	touch(t, filepath.Join(dir, "notes.txt"), 1)
	touch(t, filepath.Join(dir, "Sub", "c.ogg"), 1)
	touch(t, filepath.Join(dir, "Sub", "Deep", "d.mp3"), 1)

	m, _ := media.NewMatcher(nil, nil)
	fm := New(dir, Options{Matcher: m})
	var got []string
	for _, e := range fm.Entries {
		got = append(got, e.Name)
	}
	want := []string{"../", "./", "Sub/", "A.mp3", "b.mp3", "list.lst", "notes.txt"}
	if len(got) != len(want) {
		t.Fatalf("entries %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries %v want %v", got, want)
		}
	}
	if d := fm.DisplayName(3, DisplaySize); d != "[  19K] A.mp3" {
		t.Fatalf("size display %q", d)
	}

	fm.SetSort(SortSizeSmall)
	fm.Reload()
	if fm.Entries[3].Name != "b.mp3" && fm.Entries[3].Name != "list.lst" {
		t.Fatalf("size sort: %s", fm.Entries[3].Name)
	}

	hidden := New(dir, Options{Matcher: m, HideOthers: true})
	if hidden.Find("notes.txt") != -1 || hidden.Find("list.lst") == -1 {
		t.Fatal("HideOtherFiles")
	}

	var reports []Progress
	all, err := CollectAudio(context.Background(), dir, m, func(p Progress) { reports = append(reports, p) })
	if err != nil || len(all) != 4 {
		t.Fatalf("recursive: %v %v", all, err)
	}
	last := reports[len(reports)-1]
	if last.Fraction != 1 || last.Files != 4 || last.Dirs < 3 {
		t.Fatalf("final progress %+v", last)
	}
	for i := 1; i < len(reports); i++ {
		if reports[i].Fraction < reports[i-1].Fraction {
			t.Fatalf("progress went backwards: %+v", reports)
		}
	}
	groups, err := DirsAsGroups(context.Background(), dir, m, nil)
	if err != nil || len(groups) != 2 || groups[0].Name != "[Deep]" || groups[1].Name != "[Sub]" {
		t.Fatalf("groups %+v", groups)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := 0
	if _, err := CollectAudio(ctx, dir, m, func(Progress) {
		stopped++
		cancel()
	}); !errors.Is(err, context.Canceled) || stopped != 1 {
		t.Fatalf("cancel: err=%v after %d reports", err, stopped)
	}
	if g, err := DirsAsGroups(ctx, dir, m, nil); err == nil || g != nil {
		t.Fatal("cancelled scan must not return results")
	}

	if err := fm.ChangeDir("Sub"); err != nil || filepath.Base(filepath.Clean(fm.Dir())) != "Sub" {
		t.Fatalf("chdir: %v %s", err, fm.Dir())
	}
}

func TestSortModeCycle(t *testing.T) {
	s := SortAlpha
	for range 7 {
		s = s.Next()
	}
	if s != SortAlpha {
		t.Fatal("cycle length")
	}
	if m, err := ParseSortMode("modify-new"); err != nil || m != SortModifyNew {
		t.Fatal("parse")
	}
}
