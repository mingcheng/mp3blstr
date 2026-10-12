package listview

import "testing"

func TestMoveWrapAndScroll(t *testing.T) {
	var s State
	s.SetRows(3, 10)
	s.Move(-1, 10, true)
	if s.Cursor != 9 || s.Offset != 7 {
		t.Fatalf("wrap up: %+v", s)
	}
	s.Move(1, 10, true)
	if s.Cursor != 0 || s.Offset != 0 {
		t.Fatalf("wrap down: %+v", s)
	}
	s.Move(-1, 10, false)
	if s.Cursor != 0 {
		t.Fatal("no wrap expected")
	}
}

func TestPaging(t *testing.T) {
	var s State
	s.SetRows(4, 10)
	s.Move(1, 10, false) // cursor 1
	s.PageDown(10)
	if s.Offset != 3 || s.Cursor != 4 {
		t.Fatalf("page down: %+v", s)
	}
	s.PageDown(10)
	s.PageDown(10)
	if s.Cursor != 9 {
		t.Fatalf("page down to end: %+v", s)
	}
	s.PageUp(10)
	s.PageUp(10)
	s.PageUp(10)
	s.PageUp(10)
	if s.Cursor != 0 || s.Offset != 0 {
		t.Fatalf("page up to start: %+v", s)
	}
}

func TestPanAndClamp(t *testing.T) {
	var s State
	s.PanBy(5, 12, 10)
	if s.Pan != 2 {
		t.Fatalf("pan limit: %d", s.Pan)
	}
	s.PanBy(-10, 12, 10)
	if s.Pan != 0 {
		t.Fatal("pan below 0")
	}
	s.SetRows(5, 3)
	s.Cursor = 7
	s.Clamp(3)
	if s.Cursor != 2 || s.Offset != 0 {
		t.Fatalf("clamp: %+v", s)
	}
}

func TestWidth(t *testing.T) {
	for in, want := range map[string]Width{"50%": {Percent: 50}, "80": {Cols: 80}, "0": {}} {
		got, err := ParseWidth(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, in := range []string{"10%", "101%", "30", "x"} {
		if _, err := ParseWidth(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
	w := Width{Percent: 50}
	if w.Resolve(200, 60, 188) != 100 || w.Resolve(100, 60, 88) != 60 || w.Resolve(70, 60, 58) != 58 {
		t.Error("percent resolve")
	}
	if (Width{Cols: 80}).Resolve(200, 60, 188) != 80 || (Width{}).Resolve(200, 60, 188) != 188 {
		t.Error("cols/zero resolve")
	}
}
