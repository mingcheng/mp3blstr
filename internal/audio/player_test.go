package audio

import (
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// sliceSource is an in-memory source of n samples whose value is the index.
type sliceSource struct{ pos, n int }

func (s *sliceSource) Stream(b [][2]float64) (int, bool) {
	if s.pos >= s.n {
		return 0, false
	}
	k := min(len(b), s.n-s.pos)
	for i := range k {
		v := float64(s.pos+i) / 8
		b[i] = [2]float64{v, -v}
	}
	s.pos += k
	return k, true
}
func (s *sliceSource) Err() error       { return nil }
func (s *sliceSource) Len() int         { return s.n }
func (s *sliceSource) Position() int    { return s.pos }
func (s *sliceSource) Seek(p int) error { s.pos = p; return nil }
func (s *sliceSource) Close() error     { return nil }
func sample(b []byte, i, ch int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[i*frameBytes+ch*4:]))
}

func TestPCMReader(t *testing.T) {
	src := &sliceSource{n: 3}
	r := &pcmReader{src: src, s: src}
	b := make([]byte, 2*frameBytes+3) // trailing partial frame is left alone
	n, err := r.Read(b)
	if n != 2*frameBytes || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if sample(b, 1, 0) != 0.125 || sample(b, 1, 1) != -0.125 {
		t.Fatalf("bad samples %v %v", sample(b, 1, 0), sample(b, 1, 1))
	}
	if n, err = r.Read(b); n != frameBytes || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if n, err = r.Read(b); n != 0 || err != io.EOF || !r.atEOF() {
		t.Fatalf("want EOF, got %d, %v", n, err)
	}
	if _, err := r.Seek(1, io.SeekStart); err != nil || r.atEOF() || r.position() != 1 {
		t.Fatalf("Seek: err=%v eof=%v pos=%d", err, r.atEOF(), r.position())
	}
	if n, err = r.Read(b); n != 2*frameBytes || err != nil || sample(b, 0, 0) != 0.125 {
		t.Fatalf("Read after seek = %d, %v", n, err)
	}
}
