//go:build darwin

package mediakeys

import "testing"

func TestMediaAction(t *testing.T) {
	for _, test := range []struct {
		name   string
		data   uint32
		action Action
		down   bool
		ok     bool
	}{
		{"play down", 16<<16 | 0x0a<<8, PlayPause, true, true},
		{"play up", 16<<16 | 0x0b<<8, PlayPause, false, true},
		{"next down", 17<<16 | 0x0a<<8, Next, true, true},
		{"previous down", 18<<16 | 0x0a<<8, Previous, true, true},
		{"volume", 0x0a << 8, 0, false, false},
		{"other state", 16<<16 | 0x09<<8, 0, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			action, down, ok := mediaAction(test.data)
			if action != test.action || down != test.down || ok != test.ok {
				t.Fatalf("mediaAction(%#x) = (%v, %t, %t), want (%v, %t, %t)", test.data, action, down, ok, test.action, test.down, test.ok)
			}
		})
	}
}

func TestMediaKeyState(t *testing.T) {
	var keys mediaKeyState
	var actions []Action
	offer := func(action Action) bool {
		actions = append(actions, action)
		return true
	}
	nextDown := uint32(17<<16 | 0x0a<<8)
	nextUp := uint32(17<<16 | 0x0b<<8)
	if keys.handle(nextUp, offer) || !keys.handle(nextDown, offer) || !keys.handle(nextDown, offer) {
		t.Fatal("first key press must be handled, orphan release must not")
	}
	if len(actions) != 1 || actions[0] != Next {
		t.Fatalf("repeated keydown sent %v, want one Next", actions)
	}
	if !keys.handle(nextUp, offer) || !keys.handle(nextDown, offer) || len(actions) != 2 {
		t.Fatal("release must allow the next press")
	}
	keys.handle(nextUp, offer)
	if keys.handle(nextDown, func(Action) bool { return false }) || keys.handle(nextUp, offer) {
		t.Fatal("a dropped keydown must not consume its release")
	}
}
