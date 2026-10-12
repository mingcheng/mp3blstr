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
	want := filepath.Join(home, ".config", "mp3blaster")
	if got := Dir(); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	if got := DefaultPath(); got != filepath.Join(want, "mp3blasterrc") {
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

func TestLoadFileDefaultAndLegacy(t *testing.T) {
	setHome(t)

	o := Default()
	if err := o.LoadFile(""); err != nil {
		t.Fatalf("missing default file should not fail: %v", err)
	}

	if err := os.WriteFile(LegacyPath(), []byte("Theme = modern\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o = Default()
	if err := o.LoadFile(""); err != nil || o.Theme != "modern" {
		t.Fatalf("legacy fallback: theme=%q err=%v", o.Theme, err)
	}

	if _, err := EnsureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DefaultPath(), []byte("Theme = classic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o = Default()
	o.Theme = "modern"
	if err := o.LoadFile(""); err != nil || o.Theme != "classic" {
		t.Fatalf("new path should win: theme=%q err=%v", o.Theme, err)
	}
}
