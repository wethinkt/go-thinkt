package kimi

import "github.com/wethinkt/go-thinkt/internal/tuilog"

// unknownTally counts record keys the classifier has never seen. Each new
// key is logged once per reader so format drift shows up in the debug log
// without flooding it.
type unknownTally struct {
	counts map[string]int
}

// note records one sighting; keyvals name the file and position for the log.
func (u *unknownTally) note(key string, keyvals ...any) {
	if u.counts == nil {
		u.counts = make(map[string]int)
	}
	if u.counts[key] == 0 {
		tuilog.Log.Debug("kimi: unknown record type", append([]any{"type", key}, keyvals...)...)
	}
	u.counts[key]++
}

// snapshot returns a copy of the tally, or nil when nothing was seen.
func (u *unknownTally) snapshot() map[string]int {
	if u.counts == nil {
		return nil
	}
	out := make(map[string]int, len(u.counts))
	for k, v := range u.counts {
		out[k] = v
	}
	return out
}
