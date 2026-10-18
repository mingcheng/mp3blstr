package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsHTTPStream(t *testing.T) {
	for in, want := range map[string]bool{
		"http://radio.example/stream": true,
		"HTTPS://Radio.Example/":      true,
		"ftp://example.com/a.mp3":     false,
		"/music/http.mp3":             false,
		"":                            false,
	} {
		if got := IsHTTPStream(in); got != want {
			t.Errorf("IsHTTPStream(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for in, want := range map[string]string{
		"":                  "",
		"/abs/path":         "/abs/path",
		"rel/~/path":        "rel/~/path",
		"~":                 filepath.Clean(home),
		"~/Music":           filepath.Clean(home + "/Music"),
		"~no-such-user-xyz": "~no-such-user-xyz", // unknown users stay as typed
	} {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadRecodeTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "table.tbl")
	table := strings.Join([]string{
		"# comment line",
		"0x41 0x42   # A -> B (hex)",
		"0143 0144   # c -> d (octal)",
		"101 0x46    # e -> F (decimal)",
		"0 0x41      # 0 is ignored",
		"junk",
	}, "\n")
	if err := os.WriteFile(path, []byte(table), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := LoadRecodeTable(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec("Ace 字"); got != "BdF 字" {
		t.Fatalf("recoded %q", got)
	}

	if err := os.WriteFile(path, []byte("256 65\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecodeTable(path); err == nil {
		t.Fatal("input characters above 255 must be rejected")
	}
	if _, err := LoadRecodeTable(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing table must be an error")
	}
}
