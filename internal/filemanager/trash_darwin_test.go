//go:build darwin

package filemanager

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTrashReportsNativeFailure(t *testing.T) {
	// A nonexistent path exercises the native call without touching the
	// user's Trash or any existing file.
	path := filepath.Join(t.TempDir(), "missing.mp3")
	if err := Trash(path); err == nil || !strings.Contains(err.Error(), "missing.mp3") {
		t.Fatalf("native Trash failure = %v", err)
	}
	for _, invalid := range []string{"", "bad\x00path"} {
		if err := Trash(invalid); err == nil {
			t.Fatalf("accepted invalid path %q", invalid)
		}
	}
}
