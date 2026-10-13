//go:build darwin

package mediakeys

import (
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"
)

var seekEventClass = sync.OnceValues(func() (objc.Class, error) {
	// purego does not support a Go callback returning a scalar double.
	// Objective-C forwarding supplies a real double return for the test event.
	return objc.RegisterClass("MP3BlstrTestPositionEvent", objc.GetClass("NSObject"), nil,
		[]objc.FieldDef{{Name: "position", Type: reflect.TypeFor[objc.ID](), Attribute: objc.ReadWrite}},
		[]objc.MethodDef{
			{Cmd: objc.RegisterName("respondsToSelector:"), Fn: func(self objc.ID, cmd objc.SEL, selector objc.SEL) bool {
				return selector == objc.RegisterName("positionTime") || objc.SendSuper[bool](self, cmd, selector)
			}},
			{Cmd: objc.RegisterName("methodSignatureForSelector:"), Fn: func(self objc.ID, cmd objc.SEL, selector objc.SEL) objc.ID {
				if selector == objc.RegisterName("positionTime") {
					return objc.ID(objc.GetClass("NSMethodSignature")).Send(objc.RegisterName("signatureWithObjCTypes:"), "d@:")
				}
				return self.SendSuper(cmd, selector)
			}},
			{Cmd: objc.RegisterName("forwardInvocation:"), Fn: func(self objc.ID, cmd objc.SEL, invocation objc.ID) {
				if objc.Send[objc.SEL](invocation, objc.RegisterName("selector")) != objc.RegisterName("positionTime") {
					self.SendSuper(cmd, invocation)
					return
				}
				number := self.Send(objc.RegisterName("position"))
				position := objc.Send[float64](number, objc.RegisterName("doubleValue"))
				invocation.Send(objc.RegisterName("setReturnValue:"), &position)
			}},
		})
})

func TestNativePositionCommand(t *testing.T) {
	n := nativeTestPlayer(t)
	class, err := seekEventClass()
	if err != nil {
		t.Fatal(err)
	}
	onMainSync(func() {
		command := objc.ID(objc.GetClass("MPChangePlaybackPositionCommand")).Send(objc.RegisterName("new"))
		if command == 0 {
			t.Error("could not create a private position command")
			return
		}
		defer command.Send(objc.RegisterName("release"))
		if err := n.registerCommand(command, Seek); err != nil {
			t.Error(err)
			return
		}
		// Keep it alive until Close removes the registered target.
		command.Send(objc.RegisterName("retain"))
		event := objc.ID(class).Send(objc.RegisterName("new"))
		defer event.Send(objc.RegisterName("release"))
		setPosition := func(position float64) {
			number := objc.ID(objc.GetClass("NSNumber")).Send(objc.RegisterName("numberWithDouble:"), position)
			event.Send(objc.RegisterName("setPosition:"), number)
		}
		setPosition(42.75)
		invoke := func() int64 {
			status, err := objc.InvokeBlock[int64](n.targets[0].block, event)
			if err != nil {
				t.Error(err)
			}
			return status
		}
		if invoke() == 0 || objc.Send[bool](command, objc.RegisterName("isEnabled")) {
			t.Error("idle player accepted position control")
		}
		n.current = Snapshot{Status: "Paused", TrackID: 8, Duration: 120, CanSeek: true}
		n.publish(n.current)
		if !objc.Send[bool](command, objc.RegisterName("isEnabled")) || invoke() != 0 {
			t.Error("seekable paused track did not enable position control")
			return
		}
		got := <-n.events
		if got.Action != Seek || got.Position != 42.75 || got.TrackID != 8 {
			t.Errorf("position event = %+v", got)
		}
		setPosition(math.NaN())
		if invoke() == 0 {
			t.Error("invalid native position was accepted")
		}
		setPosition(10.0)
		if invoke() != 0 || invoke() == 0 {
			t.Error("full command queue must return command-failed")
		}
		<-n.events
		n.current.CanSeek = false
		n.publish(n.current)
		if objc.Send[bool](command, objc.RegisterName("isEnabled")) || invoke() == 0 {
			t.Error("unseekable track enabled position control")
		}
	})
	n.Close()
	onMainSync(func() {
		if n.positionCommand != 0 {
			if objc.Send[bool](n.positionCommand, objc.RegisterName("isEnabled")) {
				t.Error("position control remained enabled after Close")
			}
			n.positionCommand.Send(objc.RegisterName("release"))
		}
	})
}
