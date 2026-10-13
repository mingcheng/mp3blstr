//go:build !darwin

package filemanager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrashUnsupportedPreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.mp3")
	const content = "keep this file"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Trash(path); err == nil {
		t.Fatal("unsupported Trash operation succeeded")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != content {
		t.Fatalf("unsupported Trash operation changed the file: %q, %v", data, err)
	}
}
