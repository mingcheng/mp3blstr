package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestDirAndDefaultPath(t *testing.T) {
	home := setHome(t)
	want := filepath.Join(home, ".config", "mp3blasterr")
	if got := Dir(); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	if got := DefaultPath(); got != filepath.Join(want, "config.yaml") {
		t.Fatalf("DefaultPath() = %q", got)
	}
	dir, err := EnsureDir()
	if err != nil || dir != want {
		t.Fatalf("EnsureDir() = %q, %v", dir, err)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
}

func TestLoadFileDefault(t *testing.T) {
	home := setHome(t)

	o := Default()
	if err := o.LoadFile(""); err != nil || o.Path != DefaultPath() {
		t.Fatalf("missing default file should not fail: path=%q err=%v", o.Path, err)
	}

	// Files of the original program are not read any more.
	if err := os.WriteFile(filepath.Join(home, ".mp3blasterrc"), []byte("Theme = modern\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o = Default()
	o.Theme = "classic"
	if err := o.LoadFile(""); err != nil || o.Theme != "classic" || o.Path != DefaultPath() {
		t.Fatalf("legacy file must be ignored: theme=%q path=%q err=%v", o.Theme, o.Path, err)
	}

	if _, err := EnsureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DefaultPath(), []byte("ui:\n  theme: modern\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o = Default()
	o.Theme = "classic"
	if err := o.LoadFile(""); err != nil || o.Theme != "modern" {
		t.Fatalf("default file: theme=%q err=%v", o.Theme, err)
	}
}
