package config

import (
	"strings"
	"testing"

	"github.com/mingcheng/mp3blstr/internal/keys"
)

func TestMacOSSettings(t *testing.T) {
	o := Default()
	if !o.PauseOnSleep || !o.PauseOnOutputChange || o.UseTrash {
		t.Fatal("unexpected platform defaults")
	}
	if err := o.Parse(strings.NewReader("playback:\n  pause_on_sleep: false\n  pause_on_output_change: false\nfiles:\n  use_trash: true\nkeys:\n  add_paths: A\n")); err != nil {
		t.Fatal(err)
	}
	if o.PauseOnSleep || o.PauseOnOutputChange || !o.UseTrash || len(o.Keys) != 1 || o.Keys[0].Cmd != keys.CmdAddPaths {
		t.Fatal("platform settings or key binding were not applied")
	}
	for _, setting := range []string{"playback:\n  pause_on_sleep: invalid", "playback:\n  pause_on_output_change: invalid", "files:\n  use_trash: invalid"} {
		if err := Default().Parse(strings.NewReader(setting)); err == nil {
			t.Fatalf("invalid setting accepted: %s", setting)
		}
	}
}
