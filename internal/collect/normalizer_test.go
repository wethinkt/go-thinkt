package collect

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func TestNormalizePayload(t *testing.T) {
	p := tracewire.Payload{
		MachineID: " m ", Source: " Gemini ", Session: tracewire.Session{ID: "s", ProjectPath: " /p ", Title: " T "},
		Entries: []thinkt.Entry{{UUID: "u1"}, {UUID: ""}, {UUID: "  "}},
	}
	dropped, err := NormalizePayload(&p)
	if err != nil || dropped != 2 || len(p.Entries) != 1 {
		t.Fatalf("dropped %d err %v entries %d", dropped, err, len(p.Entries))
	}
	if p.MachineID != "m" || p.Source != "gemini" || p.Session.ProjectPath != "/p" || p.Session.Title != "T" {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []tracewire.Payload{
		{Source: "s", Session: tracewire.Session{ID: "x"}},
		{MachineID: "m", Session: tracewire.Session{ID: "x"}},
		{MachineID: "m", Source: "s"},
	} {
		if _, err := NormalizePayload(&bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
