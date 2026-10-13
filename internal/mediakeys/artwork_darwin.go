//go:build darwin

package mediakeys

import (
	"errors"
	"math"
	"structs"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/mingcheng/mp3blstr/internal/media"
)

type imageSize struct {
	_             structs.HostLayout
	Width, Height float64
}

type artworkRequest struct {
	path    string
	trackID int
}

// updateArtwork runs on the main thread. Only the current cover is retained;
// seeks/pauses reuse it, whereas a new track immediately clears stale artwork.
func (n *nowPlaying) updateArtwork(s Snapshot) {
	request := artworkRequest{}
	if s.Status != "Stopped" && !media.IsHTTPStream(s.Path) {
		request = artworkRequest{s.Path, s.TrackID}
	}
	if request == n.artworkTrack {
		return
	}
	n.artworkTrack = request
	n.clearArtwork()
	// One worker and one pending request bound work even during rapid skips.
	select {
	case <-n.artworkRequests:
	default:
	}
	if request.path != "" {
		n.artworkRequests <- request
	}
}

func (n *nowPlaying) loadArtwork() {
	for {
		select {
		case <-n.done:
			return
		case request := <-n.artworkRequests:
			art, err := n.readArtwork(request.path)
			select {
			case <-n.done:
				return
			default:
			}
			onMain(func() { n.applyArtwork(request, art, err) })
		}
	}
}

func (n *nowPlaying) applyArtwork(request artworkRequest, art *media.Artwork, err error) {
	if n.closed || request != n.artworkTrack {
		return
	}
	if err != nil {
		n.report(err)
		return
	}
	if art == nil {
		return
	}
	cover, err := newNativeArtwork(art)
	if err != nil {
		n.report(err)
		return
	}
	n.clearArtwork()
	n.artwork = cover
	// Loading an image must not rewind the system timeline to the position
	// captured when the track first started.
	n.publish(n.snapshotAt(time.Now()))
}

func (n *nowPlaying) snapshotAt(now time.Time) Snapshot {
	s := n.current
	if s.Status == "Playing" {
		s.Position += max(0, int(now.Sub(n.currentAt).Seconds()))
		if s.Duration > 0 {
			s.Position = min(s.Position, s.Duration)
		}
	}
	return s
}

func (n *nowPlaying) clearArtwork() {
	if n.artwork != 0 {
		n.artwork.Send(objc.RegisterName("release"))
		n.artwork = 0
	}
}

func newNativeArtwork(art *media.Artwork) (objc.ID, error) {
	if art == nil || len(art.PNG) == 0 || art.Width <= 0 || art.Height <= 0 {
		return 0, errors.New("invalid normalized artwork")
	}
	bounds := imageSize{Width: float64(art.Width), Height: float64(art.Height)}
	// MediaPlayer copies this block. Its captured Go bytes stay alive until
	// the system releases its last artwork reference, even after a track skip.
	block := objc.NewBlock(func(_ objc.Block, size imageSize) objc.ID {
		if size.Width <= 0 || size.Height <= 0 || math.IsNaN(size.Width) || math.IsNaN(size.Height) ||
			math.IsInf(size.Width, 0) || math.IsInf(size.Height, 0) {
			size = bounds
		}
		data := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), &art.PNG[0], uintptr(len(art.PNG)))
		image := objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).
			Send(objc.RegisterName("initWithData:"), data)
		image.Send(objc.RegisterName("setSize:"), size)
		return image.Send(objc.RegisterName("autorelease"))
	})
	defer block.Release()
	cover := objc.ID(objc.GetClass("MPMediaItemArtwork")).Send(objc.RegisterName("alloc")).
		Send(objc.RegisterName("initWithBoundsSize:requestHandler:"), bounds, block)
	if cover == 0 {
		return 0, errors.New("could not create native album artwork")
	}
	return cover, nil
}
