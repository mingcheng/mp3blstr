package media

import (
	"bytes"
	"testing"
)

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
