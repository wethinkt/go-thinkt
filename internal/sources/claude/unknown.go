package claude

import "github.com/wethinkt/go-thinkt/internal/tuilog"

// unknownTally counts record types the classifier has never seen. Each new
// type is logged once per reader so format drift shows up in the debug log
// without flooding it.
type unknownTally struct {
	counts map[EntryType]int
}

// note records one sighting; keyvals name the file and position for the log.
func (u *unknownTally) note(typ EntryType, keyvals ...any) {
	if u.counts == nil {
		u.counts = make(map[EntryType]int)
	}
	if u.counts[typ] == 0 {
		tuilog.Log.Debug("claude: unknown record type", append([]any{"type", typ}, keyvals...)...)
	}
	u.counts[typ]++
}

// snapshot returns a copy of the tally, or nil when nothing was seen.
func (u *unknownTally) snapshot() map[EntryType]int {
	if u.counts == nil {
		return nil
	}
	out := make(map[EntryType]int, len(u.counts))
	for k, v := range u.counts {
		out[k] = v
	}
	return out
}
