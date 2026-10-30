package thinkt

// SearchLineFilterer is implemented by a StoreFactory whose session files
// contain lines that are not conversation content.
type SearchLineFilterer interface {
	// NewSearchLineFilter returns a predicate for one file. The scanner
	// calls it once per file and runs it on every line in order, matching
	// or not, so a filter can remember earlier lines. matched says whether
	// the line holds a hit; the predicate reports whether that hit counts.
	// A filter with no memory can return true at once when matched is false.
	NewSearchLineFilter() func(line []byte, matched bool) bool
}
