package audio

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFormatOfContentType(t *testing.T) {
	for ct, want := range map[string]format{
		"audio/ogg":           formatVorbis,
		"audio/x-flac":        formatFLAC,
		"audio/wav":           formatWAV,
		"audio/mpeg":          formatMP3,
		"application/unknown": formatWAV, // the fallback
	} {
		if got := formatOfContentType(ct, formatWAV); got != want {
			t.Errorf("%s: got %v, want %v", ct, got, want)
		}
	}
}

// wavHeader returns the header of an endless 16 bit stereo 44.1 kHz WAV.
func wavHeader() []byte {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 0x7fffffff)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], 44100)
	binary.LittleEndian.PutUint32(h[28:], 44100*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], 0x7ffffff0) // int32 in beep
	return h
}

// TestStalledStreamCanBeCancelled checks that cancelling a stream unblocks
// a decoder waiting for data, so stopping a stalled stream never hangs.
func TestStalledStreamCanBeCancelled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(wavHeader())
		w.(http.Flusher).Flush()
		// Stall until the client gives up (or the test ends).
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release) // runs first, so Close doesn't wait for the handler

	d, err := decodeHTTP(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	if d.seekable || d.cancel == nil {
		t.Fatal("streams must be cancellable and not seekable")
	}
	returned := make(chan struct{})
	go func() {
		d.stream.Stream(make([][2]float64, 512)) // blocks: no data
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("Stream should block while the server stalls")
	case <-time.After(50 * time.Millisecond):
	}
	d.cancel()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not unblock the decoder")
	}
	d.stream.Close()
}
