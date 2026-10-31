package collect

import (
	"fmt"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

// NormalizePayload validates and cleans a relay payload in place. It
// returns an error when the payload cannot be stored (no machine, source
// or session id). Entries with no uuid cannot be keyed; they are removed
// and counted.
func NormalizePayload(p *tracewire.Payload) (dropped int, err error) {
	p.MachineID = strings.TrimSpace(p.MachineID)
	p.Source = strings.ToLower(strings.TrimSpace(p.Source))
	p.InstanceID = strings.TrimSpace(p.InstanceID)
	p.Session.ID = strings.TrimSpace(p.Session.ID)
	p.Session.ProjectPath = strings.TrimSpace(p.Session.ProjectPath)
	p.Session.Title = strings.TrimSpace(p.Session.Title)
	switch {
	case p.MachineID == "":
		return 0, fmt.Errorf("machine_id is required")
	case p.Source == "":
		return 0, fmt.Errorf("source is required")
	case p.Session.ID == "":
		return 0, fmt.Errorf("session.id is required")
	}
	valid := make([]thinkt.Entry, 0, len(p.Entries))
	for _, e := range p.Entries {
		if strings.TrimSpace(e.UUID) == "" {
			dropped++
			continue
		}
		valid = append(valid, e)
	}
	p.Entries = valid
	return dropped, nil
}
