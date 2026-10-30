package codex

import "encoding/json"

// NewSearchLineFilter implements thinkt.SearchLineFilterer. A hit counts
// when the line has no top-level type or IsSearchable accepts it; lines
// without a hit are not decoded.
func (d *Discoverer) NewSearchLineFilter() func(line []byte, matched bool) bool {
	return func(line []byte, matched bool) bool {
		if !matched {
			return true
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) != nil || head.Type == "" {
			return true
		}
		return IsSearchable(line)
	}
}
