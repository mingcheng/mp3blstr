//go:build darwin

package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func artworkImage(t *testing.T, width, height int, format string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		err = png.Encode(&buf, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func id3Picture(data []byte) []byte {
	picture := append([]byte("\x00image/png\x00\x03\x00"), data...)
	frame := make([]byte, 10)
	copy(frame, "APIC")
	binary.BigEndian.PutUint32(frame[4:], uint32(len(picture)))
	frame = append(frame, picture...)
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}
	n := len(frame)
	for i := 9; i >= 6; i-- {
		header[i], n = byte(n&127), n>>7
	}
	return append(header, frame...)
}

func flacPicture(data []byte) []byte {
	var picture bytes.Buffer
	write := func(n uint32) { binary.Write(&picture, binary.BigEndian, n) }
	write(3)
	write(9)
	picture.WriteString("image/png")
	write(0) // description
	write(8)
	write(4)
	write(32)
	write(0)
	write(uint32(len(data)))
	picture.Write(data)
	n := picture.Len()
	header := []byte{'f', 'L', 'a', 'C', 0x86, byte(n >> 16), byte(n >> 8), byte(n)}
	return append(header, picture.Bytes()...)
}

func TestReadEmbeddedArtwork(t *testing.T) {
	imageData := artworkImage(t, 8, 4, "png")
	for name, data := range map[string][]byte{
		"cover.mp3":  id3Picture(imageData),
		"cover.flac": flacPicture(imageData),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			art, err := ReadArtwork(path)
			if err != nil || art == nil {
				t.Fatalf("embedded artwork: %v, %v", art, err)
			}
			if art.Width != 8 || art.Height != 4 {
				t.Fatalf("artwork bounds = %dx%d", art.Width, art.Height)
			}
			decoded, err := png.Decode(bytes.NewReader(art.PNG))
			if err != nil {
				t.Fatal(err)
			}
			r, _, _, a := decoded.At(0, 0).RGBA()
			if r != 65535 || a != 65535 {
				t.Fatal("embedded picture pixels changed")
			}
		})
	}
}

func TestArtworkAbsenceAndFailure(t *testing.T) {
	for _, path := range []string{"", "https://invalid.example/stream.mp3"} {
		if art, err := ReadArtwork(path); art != nil || err != nil {
			t.Fatalf("nonlocal artwork = %v, %v", art, err)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "song.mp3")
	if _, err := ReadArtwork(path); err == nil {
		t.Fatal("missing file must report an error")
	}
	for _, data := range [][]byte{make([]byte, 256), append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), make([]byte, 128)...)} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if art, err := ReadArtwork(path); art != nil || err != nil {
			t.Fatalf("no picture should be normal: %v, %v", art, err)
		}
	}
	if err := os.WriteFile(path, id3Picture([]byte("broken PNG")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArtwork(path); err == nil {
		t.Fatal("corrupt artwork must report an error")
	}
	// A new open of the same path must not reuse artwork from an old file.
	if err := os.WriteFile(path, id3Picture(artworkImage(t, 2, 3, "png")), 0o600); err != nil {
		t.Fatal(err)
	}
	if art, err := ReadArtwork(path); err != nil || art == nil || art.Height != 3 {
		t.Fatalf("replacement artwork = %v, %v", art, err)
	}
}

func TestArtworkNormalizationBounds(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		art, err := normalizeArtwork(artworkImage(t, 2048, 1024, format))
		if err != nil {
			t.Fatal(err)
		}
		if art.Width != 1024 || art.Height != 512 {
			t.Fatalf("%s: scaled bounds = %dx%d", format, art.Width, art.Height)
		}
		if _, err := png.Decode(bytes.NewReader(art.PNG)); err != nil {
			t.Fatal(err)
		}
	}
	oversized := artworkImage(t, 1, 1, "png")
	binary.BigEndian.PutUint32(oversized[16:], 8192)
	binary.BigEndian.PutUint32(oversized[20:], 8192)
	binary.BigEndian.PutUint32(oversized[29:], crc32.ChecksumIEEE(oversized[12:29]))
	for _, invalid := range [][]byte{nil, []byte("not an image"), make([]byte, maxArtworkBytes+1), oversized} {
		if _, err := normalizeArtwork(invalid); err == nil {
			t.Fatal("invalid/oversized artwork was accepted")
		}
	}
}
