package media

import "testing"

func TestFormatDuration(t *testing.T) {
	for _, tc := range []struct {
		secs int
		want string
	}{
		{-5, "0:00"},
		{0, "0:00"},
		{9, "0:09"},
		{61, "1:01"},
		{3599, "59:59"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{36000 + 59, "10:00:59"},
	} {
		if got := FormatDuration(tc.secs); got != tc.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}
