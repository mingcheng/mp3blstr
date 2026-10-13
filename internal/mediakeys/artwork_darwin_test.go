//go:build darwin

package mediakeys

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/mingcheng/mp3blstr/internal/media"
)

func nativeTestPlayer(t *testing.T) *nowPlaying {
	t.Helper()
	keys, err := loadNowPlayingKeys()
	if err != nil {
		t.Fatal(err)
	}
	// No system info center: dictionaries and commands are private to the test.
	n := &nowPlaying{
		keys: keys, events: make(chan Command, 1), done: make(chan struct{}),
		artworkRequests: make(chan artworkRequest, 1),
		report:          func(err error) { t.Error(err) },
	}
	t.Cleanup(func() { n.Close() })
	return n
}

func testArtwork(t *testing.T) *media.Artwork {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &media.Artwork{PNG: buf.Bytes(), Width: 64, Height: 32}
}

func TestNativeArtworkRequestsAndLifetime(t *testing.T) {
	n := nativeTestPlayer(t)
	art := testArtwork(t)
	onMainSync(func() {
		cover, err := newNativeArtwork(art)
		if err != nil {
			t.Error(err)
			return
		}
		n.artwork = cover
		dict := n.metadata(Snapshot{Status: "Playing", Title: "Cover"})
		dict.Send(objc.RegisterName("retain"))
		defer dict.Send(objc.RegisterName("release"))
		value := dict.Send(objc.RegisterName("objectForKey:"), n.keys["MPMediaItemPropertyArtwork"])
		if value != cover {
			t.Error("native dictionary omitted artwork")
		}
		n.clearArtwork()
		art = nil
		runtime.GC()
		// The dictionary still owns the native artwork and its Go block.
		// imageWithSize invokes that block through the actual Objective-C ABI.
		for _, size := range []imageSize{{Width: 32, Height: 16}, {Width: 16, Height: 8}} {
			img := value.Send(objc.RegisterName("imageWithSize:"), size)
			if img == 0 || !objc.Send[bool](img, objc.RegisterName("isValid")) {
				t.Error("native artwork handler did not return a valid image")
				continue
			}
			if got := objc.Send[imageSize](img, objc.RegisterName("size")); got != size {
				t.Errorf("requested size %+v, got %+v", size, got)
			}
		}
		cleared := n.metadata(Snapshot{Status: "Stopped"})
		if cleared.Send(objc.RegisterName("objectForKey:"), n.keys["MPMediaItemPropertyArtwork"]) != 0 {
			t.Error("old artwork remained in the next dictionary")
		}
	})
}

func TestArtworkWorkerIgnoresStaleTracks(t *testing.T) {
	n := nativeTestPlayer(t)
	art := testArtwork(t)
	started := make(chan string, 3)
	release := make(chan struct{})
	n.readArtwork = func(path string) (*media.Artwork, error) {
		started <- path
		select {
		case <-release:
		case <-n.done:
		}
		return art, nil
	}
	go n.loadArtwork()
	n.Update(Snapshot{Status: "Playing", Path: "a.mp3", TrackID: 1, Duration: 60})
	select {
	case path := <-started:
		if path != "a.mp3" {
			t.Fatal(path)
		}
	case <-time.After(time.Second):
		t.Fatal("artwork worker did not start")
	}
	n.Update(Snapshot{Status: "Playing", Path: "b.mp3", TrackID: 2})
	n.Update(Snapshot{Status: "Paused", Path: "c.mp3", TrackID: 3, Position: 20})
	onMainSync(func() {
		if n.artwork != 0 || len(n.artworkRequests) != 1 {
			t.Error("rapid skips did not clear the cover or bound the queue")
		}
	})
	release <- struct{}{}
	select {
	case path := <-started:
		if path != "c.mp3" {
			t.Fatalf("queued stale artwork was read: %q", path)
		}
	case <-time.After(time.Second):
		t.Fatal("latest artwork was not read")
	}
	onMainSync(func() {
		if n.artwork != 0 {
			t.Error("stale artwork was published onto the new track")
		}
	})
	release <- struct{}{}
	deadline := time.Now().Add(time.Second)
	for {
		var ready bool
		onMainSync(func() { ready = n.artwork != 0 })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("current artwork was not published")
		}
		time.Sleep(time.Millisecond)
	}
	n.Update(Snapshot{Status: "Paused", Path: "c.mp3", TrackID: 3, Position: 30, Revision: 1})
	onMainSync(func() {
		if n.artwork == 0 || len(n.artworkRequests) != 0 {
			t.Error("seeking reread or cleared the cover")
		}
	})
	n.Update(Snapshot{Status: "Playing", Path: "https://example.invalid/live", TrackID: 4})
	onMainSync(func() {
		if n.artwork != 0 || len(n.artworkRequests) != 0 {
			t.Error("stream kept stale artwork or triggered a network lookup")
		}
	})
	n.Close()
	onMainSync(func() {
		n.applyArtwork(artworkRequest{"c.mp3", 3}, art, nil)
		if n.artwork != 0 {
			t.Error("closed controller accepted late artwork")
		}
	})
}
