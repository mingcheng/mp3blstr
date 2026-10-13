package media

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestAudioPaths(t *testing.T) {
	matcher, err := NewMatcher(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "\u97f3\u4e50 #1.mp3")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	for _, input := range []string{path, uri, strings.Replace(uri, "file:///", "file://localhost/", 1)} {
		paths, err := matcher.AudioPaths([]string{input, "https://example.com/live"})
		if err != nil || len(paths) != 2 || paths[0] != path {
			t.Fatalf("AudioPaths(%q) = %v, %v", input, paths, err)
		}
	}
	for _, inputs := range [][]string{
		nil, {"file://remote/music/song.mp3"}, {"file:relative.mp3"},
		{uri + "?query=1"}, {uri + "#fragment"}, {"file:///bad%ZZ.mp3"},
		{"https://"}, {dir}, {filepath.Join(dir, "unsupported.txt")},
		{path, filepath.Join(dir, "missing.mp3")},
	} {
		if paths, err := matcher.AudioPaths(inputs); err == nil || paths != nil {
			t.Fatalf("invalid batch = %v, %v", paths, err)
		}
	}
}

func TestGuessTagText(t *testing.T) {
	gb, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("周杰伦 - 晴天"))
	if err != nil {
		t.Fatal(err)
	}
	latin1 := func(raw []byte) string {
		var text []rune
		for _, b := range raw {
			text = append(text, rune(b))
		}
		return string(text)
	}
	for _, tc := range []struct{ input, want string }{
		{latin1(gb), "周杰伦 - 晴天"},
		{latin1([]byte("\xd5\xbe\xb3\xa4\xcb\xd8\xb2\xc4(sc.chinaz.com)")), "站长素材(sc.chinaz.com)"},
		{latin1([]byte("歌手")), "歌手"},
		{"歌手", "歌手"},
		{"plain title", "plain title"},
		{"caf\u00e9", "caf\u00e9"},
	} {
		if got := guessTagText(tc.input); got != tc.want {
			t.Errorf("guessTagText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestID3NameGuessesPerFileWhenEnabled(t *testing.T) {
	defer SetGuessLegacyTags(false)
	writeTag := func(name string, text []byte, encoding byte) string {
		t.Helper()
		frame := append([]byte{encoding}, text...)
		data := append([]byte("TIT2\x00\x00\x00"), byte(len(frame)), 0, 0)
		data = append(data, frame...)
		header := append([]byte("ID3\x03\x00\x00\x00\x00\x00"), byte(len(data)))
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, append(header, data...), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	gb, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("晴天"))
	if err != nil {
		t.Fatal(err)
	}
	gbPath := writeTag("gb.mp3", gb, 0)
	localTag := "\xd5\xbe\xb3\xa4\xcb\xd8\xb2\xc4(sc.chinaz.com)"
	localPath := writeTag("sample.mp3", []byte(localTag), 0)
	utf8Path := writeTag("utf8.mp3", []byte("歌手"), 3)
	SetGuessLegacyTags(false)
	if got := ID3Name(gbPath, "fallback"); got == "[ID3] Unknown Artist - 晴天" {
		t.Fatal("guessing should be disabled by default")
	}
	SetGuessLegacyTags(true)
	if got := ID3Name(gbPath, "fallback"); got != "[ID3] Unknown Artist - 晴天" {
		t.Fatalf("GBK name = %q", got)
	}
	if got := ID3Name(localPath, "fallback"); got != "[ID3] Unknown Artist - 站长素材(sc.chinaz.com)" {
		t.Fatalf("local tag bytes = %q", got)
	}
	if got := ID3Name(utf8Path, "fallback"); got != "[ID3] Unknown Artist - 歌手" {
		t.Fatalf("UTF-8 name = %q", got)
	}
}

func TestPersistentMetadataCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	defer ConfigureMetadataCache("", "")
	defer SetRecoder(nil)
	ConfigureMetadataCache(dir, "profile-a")
	path := filepath.Join(t.TempDir(), "song.mp3")
	writeTag := func(title string) {
		t.Helper()
		frame := append([]byte{3}, []byte(title)...)
		data := append([]byte("TIT2\x00\x00\x00"), byte(len(frame)), 0, 0)
		data = append(data, frame...)
		header := append([]byte("ID3\x03\x00\x00\x00\x00\x00"), byte(len(data)))
		if err := os.WriteFile(path, append(header, data...), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeTag("first")
	if got := ID3Name(path, "fallback"); got != "[ID3] Unknown Artist - first" {
		t.Fatalf("first tag = %q", got)
	}
	if tags, err := ReadTags(path); err != nil || tags.Title != "first" {
		t.Fatalf("first decoded tags = %+v, %v", tags, err)
	}
	reads := 0
	readDuration := func(string) (int, error) { reads++; return 123, nil }
	if got, err := CachedDuration(path, readDuration); err != nil || got != 123 {
		t.Fatalf("duration = %d, %v", got, err)
	}
	ConfigureMetadataCache(dir, "profile-a")
	if got := ID3Name(path, "fallback"); got != "[ID3] Unknown Artist - first" {
		t.Fatalf("reloaded tag = %q", got)
	}
	if tags, err := ReadTags(path); err != nil || tags.Title != "first" {
		t.Fatalf("reloaded decoded tags = %+v, %v", tags, err)
	}
	if _, err := CachedDuration(path, readDuration); err != nil || reads != 1 {
		t.Fatalf("reloaded duration: reads=%d, err=%v", reads, err)
	}
	ConfigureMetadataCache(dir, "profile-b")
	if _, err := CachedDuration(path, readDuration); err != nil || reads != 1 {
		t.Fatalf("duration should survive tag profile changes: reads=%d, err=%v", reads, err)
	}
	SetRecoder(func(text string) string {
		if text == "" {
			return text
		}
		return "new-" + text
	})
	if tags, err := ReadTags(path); err != nil || tags.Title != "new-first" {
		t.Fatalf("tag profile was not refreshed = %+v, %v", tags, err)
	}
	if got := ID3Name(path, "fallback"); got != "[ID3] Unknown Artist - new-first" {
		t.Fatalf("stale name after profile change = %q", got)
	}
	SetRecoder(nil)
	writeTag("later")
	modified := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	if got := ID3Name(path, "fallback"); got != "[ID3] Unknown Artist - later" {
		t.Fatalf("changed tag = %q", got)
	}
	if _, err := CachedDuration(path, readDuration); err != nil || reads != 2 {
		t.Fatalf("changed file duration: reads=%d, err=%v", reads, err)
	}
	ForgetID3(path)
	if _, err := os.Stat(cachePath(path)); !os.IsNotExist(err) {
		t.Fatalf("forgotten cache file should be removed: %v", err)
	}
}

func TestMetadataCacheDisabled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	defer ConfigureMetadataCache("", "")
	ConfigureMetadataCache("", "")
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	readDuration := func(string) (int, error) { reads++; return 90, nil }
	_, _ = CachedDuration(path, readDuration)
	_, _ = CachedDuration(path, readDuration)
	if reads != 1 {
		t.Fatalf("in-memory cache reads = %d", reads)
	}
	ConfigureMetadataCache("", "")
	_, _ = CachedDuration(path, readDuration)
	if reads != 2 {
		t.Fatalf("cache survived reset without disk: reads=%d", reads)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled cache created directory: %v", err)
	}
}

func TestMatcher(t *testing.T) {
	m, err := NewMatcher(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.MP3", "b.ogg", "c.wav", "d.flac", "http://x/stream"} {
		if !m.IsAudio(f) {
			t.Errorf("%s should be audio", f)
		}
	}
	if m.IsAudio("x.txt") || !m.IsPlaylist("x.LST") || !m.IsPlaylist("y.m3u") {
		t.Error("playlist/other detection")
	}
}

func TestChopPath(t *testing.T) {
	if ChopPath("/a/b/c.mp3") != "c.mp3" || ChopPath("http://h/a.mp3") != "http://h/a.mp3" {
		t.Fatal("ChopPath")
	}
}

func TestReadMPEGHeader(t *testing.T) {
	// MPEG 1 Layer III, 128 kbit/s, 44.1 kHz, joint stereo; frame length 417.
	hdr := []byte{0xff, 0xfb, 0x90, 0x44}
	frame := make([]byte, 417)
	copy(frame, hdr)
	data := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x02xx"), append(frame, hdr...)...)
	h, ok := ReadMPEGHeader(bytes.NewReader(data))
	if !ok {
		t.Fatal("header not found")
	}
	if h.Version != 0 || h.Layer != 3 || h.Bitrate != 128 || h.SampleRate != 44100 || h.Mode != "JointStereo" {
		t.Fatalf("%+v", h)
	}
}
