//go:build darwin

package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"

	"github.com/dhowden/tag"
	"golang.org/x/image/draw"
)

const (
	maxArtworkBytes  = 16 << 20
	maxArtworkPixels = 32 << 20
	artworkSize      = 1024
)

// Artwork is a normalized, bounded PNG. Treat it as immutable after loading.
type Artwork struct {
	PNG           []byte
	Width, Height int
}

// ReadArtwork reads an embedded picture, without fetching URLs or changing
// the text metadata cache. Missing artwork is normal and returns nil, nil.
func ReadArtwork(path string) (*Artwork, error) {
	if path == "" || IsHTTPStream(path) {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if errors.Is(err, tag.ErrNoTagsFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading artwork tags in %q: %w", path, err)
	}
	picture := m.Picture()
	if picture == nil {
		return nil, nil
	}
	art, err := normalizeArtwork(picture.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding artwork in %q: %w", path, err)
	}
	return art, nil
}

func normalizeArtwork(data []byte) (*Artwork, error) {
	if len(data) == 0 || len(data) > maxArtworkBytes {
		return nil, errors.New("artwork is empty or exceeds 16 MiB")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxArtworkPixels/config.Height {
		return nil, errors.New("artwork exceeds the 32-megapixel limit")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	width, height := config.Width, config.Height
	if largest := max(width, height); largest > artworkSize {
		width, height = max(1, width*artworkSize/largest), max(1, height*artworkSize/largest)
		dst := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		src = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil, err
	}
	return &Artwork{PNG: buf.Bytes(), Width: width, Height: height}, nil
}
