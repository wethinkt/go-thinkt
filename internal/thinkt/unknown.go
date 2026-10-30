package thinkt

import (
	"maps"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// UnknownTally counts record keys a source's classifier has never seen. Each
// new key is logged once per reader so format drift shows up in the debug
// log without flooding it. Set Source so the log names the source.
type UnknownTally[K ~string] struct {
	Source Source
	counts map[K]int
}

// Note records one sighting; keyvals name the file and position for the log.
func (u *UnknownTally[K]) Note(key K, keyvals ...any) {
	if u.counts == nil {
		u.counts = make(map[K]int)
	}
	if u.counts[key] == 0 {
		tuilog.Log.Debug(string(u.Source)+": unknown record type", append([]any{"type", key}, keyvals...)...)
	}
	u.counts[key]++
}

// Snapshot returns a copy of the tally, or nil when nothing was seen.
func (u *UnknownTally[K]) Snapshot() map[K]int {
	if u.counts == nil {
		return nil
	}
	return maps.Clone(u.counts)
}
