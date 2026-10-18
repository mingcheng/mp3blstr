package app

import "testing"

func TestLineEditorReplaceAndInsert(t *testing.T) {
	e := NewLineEditor("abc", 0)
	// Replace mode (the default) overwrites, then appends at the end.
	e.AddChar('x')
	e.End()
	e.AddChar('d')
	if e.Text() != "xbcd" || e.Cursor() != 4 {
		t.Fatalf("replace: %q at %d", e.Text(), e.Cursor())
	}
	if !e.ToggleInsert() || !e.InsertMode() {
		t.Fatal("ToggleInsert must switch to insert mode")
	}
	e.Home()
	e.AddChar('>')
	e.AddChar('\t') // control characters are ignored
	if e.Text() != ">xbcd" || e.Cursor() != 1 {
		t.Fatalf("insert: %q at %d", e.Text(), e.Cursor())
	}
	e.Backspace()
	e.Delete()
	if e.Text() != "bcd" || e.Cursor() != 0 {
		t.Fatalf("delete: %q at %d", e.Text(), e.Cursor())
	}
	e.Backspace() // at the start: nothing to do
	e.End()
	e.Delete() // at the end: nothing to do
	if e.Text() != "bcd" {
		t.Fatalf("no-op edits changed the text: %q", e.Text())
	}
}

func TestLineEditorLimit(t *testing.T) {
	e := NewLineEditor("abcdef", 4)
	if e.Text() != "abcd" {
		t.Fatalf("initial text not truncated: %q", e.Text())
	}
	e.End()
	e.AddChar('x')
	if e.Text() != "abcd" {
		t.Fatalf("typed past the limit: %q", e.Text())
	}

	e = NewLineEditor("ab", 4)
	// Replace mode at the start: 4 characters overwrite and extend to 4.
	if !e.Fits(4) || e.Fits(5) {
		t.Fatal("replace mode capacity")
	}
	e.ToggleInsert()
	if !e.Fits(2) || e.Fits(3) {
		t.Fatal("insert mode capacity")
	}
	if !NewLineEditor("", 0).Fits(1 << 20) {
		t.Fatal("no limit when maxLen is 0")
	}
}

func TestLineEditorCursorAndVisible(t *testing.T) {
	e := NewLineEditor("hello", 0)
	for _, tc := range []struct{ in, want int }{{-3, 0}, {2, 2}, {99, 5}} {
		if e.SetCursor(tc.in); e.Cursor() != tc.want {
			t.Errorf("SetCursor(%d) = %d, want %d", tc.in, e.Cursor(), tc.want)
		}
	}

	// Wide runes take two cells; the view scrolls to keep the cursor (which
	// needs a cell of its own after the text) visible.
	e = NewLineEditor("日本語テキスト", 0)
	e.End()
	text, col, first := e.Visible(6)
	if text != "スト" || col != 4 || first != 5 {
		t.Fatalf("end: %q col %d first %d", text, col, first)
	}
	e.Home()
	text, col, first = e.Visible(6)
	if text != "日本語" || col != 0 || first != 0 {
		t.Fatalf("home: %q col %d first %d", text, col, first)
	}
}
