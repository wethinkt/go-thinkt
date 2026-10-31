package gemini

import (
	"os"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// NewSearchFileFilter implements thinkt.SearchFileFilterer. For a JSONL
// session it replays the file first and keeps a hit only in a line whose
// content is in the rebuilt session: the final copy of a user or gemini
// message, a $patch that edited one, or a live $set.messages checkpoint.
// Rewound turns, earlier copies, notices and metadata lines do not count.
// An older .json session keeps every hit.
func (d *Discoverer) NewSearchFileFilter(path string) func(line []byte, matched bool) bool {
	if !strings.HasSuffix(path, ".jsonl") {
		return func([]byte, bool) bool { return true }
	}
	live := map[int]bool{}
	if f, err := os.Open(path); err == nil {
		if r, err := replay(f); err == nil && r != nil {
			live = r.Live
		}
		f.Close()
	}
	n := 0
	return func(line []byte, matched bool) bool {
		n++
		if !matched || !live[n] {
			return false
		}
		switch classifyLine(line) {
		case thinkt.ClassTurn, thinkt.ClassBookkeeping:
			return true
		}
		return false
	}
}
