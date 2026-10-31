package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

// maxPayloadBytes bounds one payload's encoded entries; a single larger
// entry travels alone.
const maxPayloadBytes = 8 << 20

// assignIDs gives entries with no UUID a stable composed one, so every
// shipped entry has a unique key: relay-noid:<first 16 hex of its hash>:<n>,
// where n counts earlier entries with the same hash in session order.
func assignIDs(entries []thinkt.Entry) {
	seen := map[string]int{}
	for i := range entries {
		if entries[i].UUID != "" {
			continue
		}
		h := tracewire.EntryHash(entries[i])[:16]
		n := seen[h]
		seen[h] = n + 1
		entries[i].UUID = fmt.Sprintf("relay-noid:%s:%d", h, n)
	}
}

// snapshotHash identifies a session snapshot apart from its generation.
func snapshotHash(s tracewire.Session) string {
	s.Generation = 0
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// diffSession compares a session's current entries with what the relay
// last shipped and returns the payloads to ship: changed or new entries,
// split by batchSize and maxPayloadBytes, the uuids that disappeared on
// the last payload, and the full session snapshot on every payload. All
// payloads carry generation gen. An unchanged session gives none. st is
// not modified; commit applies each payload once it is shipped or buffered.
func diffSession(meta thinkt.SessionMeta, entries []thinkt.Entry, st *sessionState, base tracewire.Payload, batchSize int, gen int64) []tracewire.Payload {
	if batchSize <= 0 {
		batchSize = 100
	}
	snap := tracewire.SessionFromMeta(meta)
	snap.Generation = gen

	present := make(map[string]bool, len(entries))
	var changed []thinkt.Entry
	for _, e := range entries {
		present[e.UUID] = true
		if st.Hashes[e.UUID] != stateHash(e) {
			changed = append(changed, e)
		}
	}
	var removed []string
	for uuid := range st.Hashes {
		if !present[uuid] {
			removed = append(removed, uuid)
		}
	}
	sort.Strings(removed)

	if len(changed) == 0 && len(removed) == 0 && snapshotHash(snap) == st.SessionHash {
		return nil
	}

	mk := func(es []thinkt.Entry) tracewire.Payload {
		p := base
		p.Session = snap
		p.Entries = es
		return p
	}
	var out []tracewire.Payload
	var cur []thinkt.Entry
	curBytes := 0
	for _, e := range changed {
		b, _ := json.Marshal(e)
		if len(cur) > 0 && (len(cur) >= batchSize || curBytes+len(b) > maxPayloadBytes) {
			out = append(out, mk(cur))
			cur, curBytes = nil, 0
		}
		cur = append(cur, e)
		curBytes += len(b)
	}
	if len(cur) > 0 || len(out) == 0 {
		out = append(out, mk(cur))
	}
	out[len(out)-1].Removed = removed
	return out
}

// nextGeneration is the generation of a new sync of a session: its start
// time in nanoseconds, never below the last one. It does not depend on the
// local state surviving, so a relay whose state file was lost still sends
// generations above what the collector holds.
func nextGeneration(st *sessionState, now time.Time) int64 {
	return max(now.UnixNano(), st.Generation+1)
}

// stateHash is the entry hash the state keeps: the first 16 bytes of
// tracewire.EntryHash, enough to notice a change at half the size.
func stateHash(e thinkt.Entry) string { return tracewire.EntryHash(e)[:32] }

// commit records a payload the collector accepted, or the buffer took.
func (st *sessionState) commit(p tracewire.Payload) {
	if st.Hashes == nil {
		st.Hashes = map[string]string{}
	}
	for _, e := range p.Entries {
		st.Hashes[e.UUID] = stateHash(e)
	}
	for _, uuid := range p.Removed {
		delete(st.Hashes, uuid)
	}
	st.Generation = p.Session.Generation
	st.SessionHash = snapshotHash(p.Session)
}
