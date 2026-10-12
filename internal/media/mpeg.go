package media

import (
	"bufio"
	"io"
)

// MPEGHeader holds the parameters of an MPEG audio frame header.
type MPEGHeader struct {
	Version    int // 0 = MPEG 1, 1 = MPEG 2, 2 = MPEG 2.5
	Layer      int // 1..3
	Bitrate    int // kbit/s
	SampleRate int // Hz
	Mode       string
}

var (
	mpegBitrates = [2][3][16]int{
		{ // MPEG 1
			{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
			{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
		},
		{ // MPEG 2 / 2.5
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
		},
	}
	mpegSampleRates = [3][3]int{
		{44100, 48000, 32000},
		{22050, 24000, 16000},
		{11025, 12000, 8000},
	}
	mpegModes = [4]string{"Stereo", "JointStereo", "DualChannel", "Mono"}
)

func parseFrameHeader(h uint32) (MPEGHeader, int, bool) {
	if h&0xffe00000 != 0xffe00000 {
		return MPEGHeader{}, 0, false
	}
	var version int
	switch (h >> 19) & 3 {
	case 3:
		version = 0
	case 2:
		version = 1
	case 0:
		version = 2
	default:
		return MPEGHeader{}, 0, false
	}
	layerBits := (h >> 17) & 3
	if layerBits == 0 {
		return MPEGHeader{}, 0, false
	}
	layer := 4 - int(layerBits)
	bri := (h >> 12) & 0xf
	sri := (h >> 10) & 3
	if bri == 0 || bri == 15 || sri == 3 {
		return MPEGHeader{}, 0, false
	}
	pad := int((h >> 9) & 1)
	tbl := 0
	if version > 0 {
		tbl = 1
	}
	br := mpegBitrates[tbl][layer-1][bri]
	sr := mpegSampleRates[version][sri]
	var length int
	switch {
	case layer == 1:
		length = (12*br*1000/sr + pad) * 4
	case layer == 3 && version > 0:
		length = 72*br*1000/sr + pad
	default:
		length = 144*br*1000/sr + pad
	}
	return MPEGHeader{
		Version:    version,
		Layer:      layer,
		Bitrate:    br,
		SampleRate: sr,
		Mode:       mpegModes[(h>>6)&3],
	}, length, true
}

// ReadMPEGHeader locates the first valid MPEG audio frame in r (skipping an
// ID3v2 tag) and returns its parameters.
func ReadMPEGHeader(r io.Reader) (MPEGHeader, bool) {
	br := bufio.NewReaderSize(r, 64*1024)
	if hdr, err := br.Peek(10); err == nil && string(hdr[:3]) == "ID3" {
		size := int(hdr[6]&0x7f)<<21 | int(hdr[7]&0x7f)<<14 | int(hdr[8]&0x7f)<<7 | int(hdr[9]&0x7f)
		size += 10
		if hdr[5]&0x10 != 0 {
			size += 10
		}
		if _, err := br.Discard(size); err != nil {
			return MPEGHeader{}, false
		}
	}
	const maxScan = 256 * 1024
	window := make([]byte, 0, 8192)
	for scanned := 0; scanned < maxScan; {
		chunk, err := br.Peek(4096)
		if len(chunk) < 4 {
			return MPEGHeader{}, false
		}
		window = append(window[:0], chunk...)
		for i := 0; i+4 <= len(window); i++ {
			h := uint32(window[i])<<24 | uint32(window[i+1])<<16 | uint32(window[i+2])<<8 | uint32(window[i+3])
			info, length, ok := parseFrameHeader(h)
			if !ok {
				continue
			}
			// Confirm with the next frame header when it is available.
			if j := i + length; length > 4 && j+4 <= len(window) {
				h2 := uint32(window[j])<<24 | uint32(window[j+1])<<16 | uint32(window[j+2])<<8 | uint32(window[j+3])
				if _, _, ok2 := parseFrameHeader(h2); !ok2 {
					continue
				}
			}
			return info, true
		}
		if err != nil {
			return MPEGHeader{}, false
		}
		n := len(window) - 3
		if _, err := br.Discard(n); err != nil {
			return MPEGHeader{}, false
		}
		scanned += n
	}
	return MPEGHeader{}, false
}
